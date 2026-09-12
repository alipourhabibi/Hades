package gogit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// repoLocks serialises writes to one repository within this process.
//
// It sits above the compare-and-swap, not instead of it. The CAS is the
// authority and is what makes concurrent writers correct; the lock only means
// that the common case of two pushes to the same module waits rather than one
// of them failing and having to be retried. See docs2/adr/007.
var repoLocks sync.Map // repoPath -> *sync.Mutex

func lockRepo(repoPath string) func() {
	value, _ := repoLocks.LoadOrStore(repoPath, &sync.Mutex{})
	mu, ok := value.(*sync.Mutex)
	if !ok {
		// Only this function ever stores into repoLocks, so this cannot
		// happen; a bare assertion would turn a future mistake into a panic on
		// the push path.
		mu = &sync.Mutex{}
	}
	mu.Lock()
	return mu.Unlock
}

// PutFiles writes one commit and moves the branch to it.
//
// Three things it does that the previous implementation did not:
//
//   - It honours req.ExistingPaths. A path that is in ExistingPaths and not in
//     Files is removed from the new tree. Before, the argument was discarded
//     and the tree was seeded from the parent and only ever added to, so a file
//     could never be deleted from a module.
//   - The ref update is a compare-and-swap against req.ExpectedHead, so two
//     overlapping pushes cannot silently discard each other's work.
//   - The tree is sorted the way git sorts trees, so `git fsck` is clean and
//     the tree hash matches what canonical git produces for the same content.
func (g *GoGitStorage) PutFiles(_ context.Context, req git.PutFilesRequest) (string, error) {
	unlock := lockRepo(req.RepoPath)
	defer unlock()

	repo, err := g.openRepo(req.RepoPath)
	if err != nil {
		return "", err
	}

	objStore := repo.Storer

	var parentCommit *object.Commit
	var parentHash plumbing.Hash
	branchRef := plumbing.NewBranchReferenceName(req.Branch)
	ref, refErr := repo.Reference(branchRef, true)
	switch {
	case refErr == nil:
		parentHash = ref.Hash()
		parentCommit, err = repo.CommitObject(parentHash)
		if err != nil {
			return "", fmt.Errorf("gogit: read branch head %s: %w", parentHash, err)
		}
	case errors.Is(refErr, plumbing.ErrReferenceNotFound):
		// First commit on this branch.
	default:
		return "", fmt.Errorf("gogit: read branch %s: %w", req.Branch, refErr)
	}

	// Compare-and-swap check. It is repeated after the objects are written,
	// against the same value, because the lock above is per-process and a
	// second process could have moved the branch in between.
	if err := checkExpectedHead(req.ExpectedHead, parentHash, refErr == nil); err != nil {
		return "", err
	}

	allFiles := map[string]plumbing.Hash{}
	if parentCommit != nil {
		parentTree, err := parentCommit.Tree()
		if err != nil {
			return "", fmt.Errorf("gogit: read parent tree: %w", err)
		}
		// The iteration error is propagated. Discarding it produced a silently
		// truncated file set with err == nil, so a partially read module
		// looked like a small module and the next commit dropped the rest.
		if err := parentTree.Files().ForEach(func(f *object.File) error {
			allFiles[f.Name] = f.Hash
			return nil
		}); err != nil {
			return "", fmt.Errorf("gogit: walk parent tree: %w", err)
		}
	}

	// Deletions first, so a path that appears in both lists is written rather
	// than removed.
	incoming := make(map[string]struct{}, len(req.Files))
	for _, f := range req.Files {
		incoming[f.Path] = struct{}{}
	}
	for _, p := range req.ExistingPaths {
		if _, kept := incoming[p]; !kept {
			delete(allFiles, p)
		}
	}

	for _, f := range req.Files {
		blob := &plumbing.MemoryObject{}
		blob.SetType(plumbing.BlobObject)
		w, err := blob.Writer()
		if err != nil {
			return "", fmt.Errorf("gogit: open blob writer for %s: %w", f.Path, err)
		}
		if _, err := w.Write(f.Content); err != nil {
			_ = w.Close()
			return "", fmt.Errorf("gogit: write blob %s: %w", f.Path, err)
		}
		if err := w.Close(); err != nil {
			return "", fmt.Errorf("gogit: close blob %s: %w", f.Path, err)
		}
		blobHash, err := objStore.SetEncodedObject(blob)
		if err != nil {
			return "", fmt.Errorf("gogit: store blob %s: %w", f.Path, err)
		}
		allFiles[f.Path] = blobHash
	}

	treeHash, err := buildNestedTree(objStore, allFiles)
	if err != nil {
		return "", fmt.Errorf("gogit: build tree: %w", err)
	}

	now := time.Now()
	sig := object.Signature{Name: req.AuthorName, Email: req.AuthorEmail, When: now}
	commit := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      commitMessage(req),
		TreeHash:     treeHash,
		ParentHashes: []plumbing.Hash{},
	}
	if parentCommit != nil {
		commit.ParentHashes = []plumbing.Hash{parentHash}
	}

	commitEnc := &plumbing.MemoryObject{}
	commitEnc.SetType(plumbing.CommitObject)
	if err := commit.Encode(commitEnc); err != nil {
		return "", fmt.Errorf("gogit: encode commit: %w", err)
	}
	commitHash, err := objStore.SetEncodedObject(commitEnc)
	if err != nil {
		return "", fmt.Errorf("gogit: store commit: %w", err)
	}

	// Re-check the branch immediately before moving it. go-git's reference
	// storer has no atomic compare-and-swap, so this narrows the window rather
	// than closing it; the in-process lock closes it for the single-node
	// deployment gogit is the backend for. A deployment with more than one
	// writer process should use the Gitaly backend, whose ref update is a real
	// CAS server-side.
	current, currentErr := repo.Reference(branchRef, true)
	switch {
	case currentErr == nil:
		if err := checkExpectedHead(req.ExpectedHead, current.Hash(), true); err != nil {
			return "", err
		}
	case errors.Is(currentErr, plumbing.ErrReferenceNotFound):
		if err := checkExpectedHead(req.ExpectedHead, plumbing.ZeroHash, false); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("gogit: re-read branch %s: %w", req.Branch, currentErr)
	}

	newRef := plumbing.NewHashReference(branchRef, commitHash)
	if err := objStore.SetReference(newRef); err != nil {
		return "", fmt.Errorf("gogit: update ref %s: %w", req.Branch, err)
	}

	return commitHash.String(), nil
}

// commitMessage returns the message to record, appending the digest when one
// was supplied.
func commitMessage(req git.PutFilesRequest) string {
	if req.Digest == "" {
		return req.Message
	}
	return req.Message + "\n\ndigest:" + req.Digest
}

// checkExpectedHead compares the caller's expectation against the branch.
func checkExpectedHead(expected string, actual plumbing.Hash, exists bool) error {
	switch {
	case expected == "" && exists:
		return fmt.Errorf("%w: expected no branch, found %s", git.ErrRefMoved, actual)
	case expected == "":
		return nil
	case !exists:
		return fmt.Errorf("%w: expected %s, branch does not exist", git.ErrRefMoved, expected)
	case !strings.EqualFold(actual.String(), expected):
		return fmt.Errorf("%w: expected %s, found %s", git.ErrRefMoved, expected, actual)
	}
	return nil
}

// buildNestedTree recursively converts a flat path→blobHash map into a proper
// git tree object with nested sub-trees for directory paths.
func buildNestedTree(store storer.EncodedObjectStorer, files map[string]plumbing.Hash) (plumbing.Hash, error) {
	topBlobs := map[string]plumbing.Hash{}
	subdirs := map[string]map[string]plumbing.Hash{}

	for path, hash := range files {
		if idx := strings.IndexByte(path, '/'); idx == -1 {
			topBlobs[path] = hash
		} else {
			dir, rest := path[:idx], path[idx+1:]
			if subdirs[dir] == nil {
				subdirs[dir] = map[string]plumbing.Hash{}
			}
			subdirs[dir][rest] = hash
		}
	}

	var treeEntries []object.TreeEntry

	for name, hash := range topBlobs {
		treeEntries = append(treeEntries, object.TreeEntry{
			Name: name,
			Mode: filemode.Regular,
			Hash: hash,
		})
	}

	for dir, subfiles := range subdirs {
		subtreeHash, err := buildNestedTree(store, subfiles)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		treeEntries = append(treeEntries, object.TreeEntry{
			Name: dir,
			Mode: filemode.Dir,
			Hash: subtreeHash,
		})
	}

	sort.Slice(treeEntries, func(i, j int) bool {
		return treeSortKey(treeEntries[i]) < treeSortKey(treeEntries[j])
	})

	tree := &object.Tree{Entries: treeEntries}
	enc := &plumbing.MemoryObject{}
	enc.SetType(plumbing.TreeObject)
	if err := tree.Encode(enc); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("gogit: encode tree: %w", err)
	}
	return store.SetEncodedObject(enc)
}

// treeSortKey returns the name git sorts a tree entry by.
//
// Git's canonical ordering compares a directory entry as though its name had a
// trailing slash, so "foo.proto" sorts before "foo/" even though "foo" sorts
// before "foo.proto". A plain name comparison, which is what this did, produces
// trees that `git fsck` reports as "not properly sorted" and whose hash differs
// from what canonical git computes for identical content, so nothing can be
// cross-checked against another git implementation.
func treeSortKey(e object.TreeEntry) string {
	if e.Mode == filemode.Dir {
		return e.Name + "/"
	}
	return e.Name
}

func (g *GoGitStorage) RollbackCommit(_ context.Context, repoPath, branch, currentHead, previousHead string) error {
	if previousHead == "" {
		return nil
	}
	unlock := lockRepo(repoPath)
	defer unlock()

	repo, err := g.openRepo(repoPath)
	if err != nil {
		return err
	}

	// The branch is only reset when it still points where the caller left it.
	// Resetting unconditionally meant a failed database transaction on one push
	// could revert a different push that had succeeded in between.
	if currentHead != "" {
		ref, err := repo.Reference(plumbing.NewBranchReferenceName(branch), true)
		if err != nil {
			if errors.Is(err, plumbing.ErrReferenceNotFound) {
				return nil
			}
			return fmt.Errorf("gogit: read branch %s: %w", branch, err)
		}
		if !strings.EqualFold(ref.Hash().String(), currentHead) {
			return fmt.Errorf("%w: not rolling back %s, it has moved to %s",
				git.ErrRefMoved, branch, ref.Hash())
		}
	}

	prev := plumbing.NewHash(previousHead)
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), prev)
	return repo.Storer.SetReference(ref)
}
