package gogit

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

func (g *GoGitStorage) PutFiles(_ context.Context, repoPath, branch string, files []*git.File, authorName, authorEmail, commitMsg string, _ []string) (string, error) {
	repo, err := g.openRepo(repoPath)
	if err != nil {
		return "", err
	}

	objStore := repo.Storer

	var parentCommit *object.Commit
	var parentHash plumbing.Hash
	branchRef := plumbing.NewBranchReferenceName(branch)
	ref, refErr := repo.Reference(branchRef, true)
	if refErr == nil {
		parentHash = ref.Hash()
		parentCommit, _ = repo.CommitObject(parentHash)
	}

	allFiles := map[string]plumbing.Hash{}
	if parentCommit != nil {
		if parentTree, err := parentCommit.Tree(); err == nil {
			_ = parentTree.Files().ForEach(func(f *object.File) error {
				allFiles[f.Name] = f.Blob.Hash
				return nil
			})
		}
	}

	for _, f := range files {
		blob := &plumbing.MemoryObject{}
		blob.SetType(plumbing.BlobObject)
		w, _ := blob.Writer()
		_, _ = w.Write(f.Content)
		_ = w.Close()
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
	sig := object.Signature{Name: authorName, Email: authorEmail, When: now}
	commit := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      commitMsg,
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

	newRef := plumbing.NewHashReference(branchRef, commitHash)
	if err := objStore.SetReference(newRef); err != nil {
		return "", fmt.Errorf("gogit: update ref %s: %w", branch, err)
	}

	return commitHash.String(), nil
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
		return treeEntries[i].Name < treeEntries[j].Name
	})

	tree := &object.Tree{Entries: treeEntries}
	enc := &plumbing.MemoryObject{}
	enc.SetType(plumbing.TreeObject)
	if err := tree.Encode(enc); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("gogit: encode tree: %w", err)
	}
	return store.SetEncodedObject(enc)
}

func (g *GoGitStorage) RollbackCommit(_ context.Context, repoPath, branch, _, previousHead string) error {
	if previousHead == "" {
		return nil
	}
	repo, err := g.openRepo(repoPath)
	if err != nil {
		return err
	}
	prev := plumbing.NewHash(previousHead)
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branch), prev)
	return repo.Storer.SetReference(ref)
}

