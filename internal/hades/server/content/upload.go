package content

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/cirun"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	notificationdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/telemetry"
	"github.com/alipourhabibi/Hades/utils/connerr"
	"github.com/alipourhabibi/Hades/utils/paths"
	"github.com/alipourhabibi/Hades/utils/shake256"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type uploadWorkItem struct {
	module       *registryv1.Module
	files        []*gitstorage.File
	listFiles    []string
	dig          string
	digestStr    string
	digestBytes  []byte
	userId       string
	moduleId     string
	previousHead string
	// expectedHead is the branch head the tree was computed from, used as the
	// compare-and-swap value. It comes from git rather than from the database's
	// create_time ordering; see Upload.
	expectedHead string
	prevFiles    []*registryv1.File
	checks       ciResult
}

// ciResult records what the pre-push checks actually did, so the outcome can be
// persisted against the commit they cleared.
//
// Only a passing run is ever recorded. A lint or breaking violation rejects the
// push, and CIRun rows are keyed by (module, commit_hash), so a rejected push
// has no commit to key a record on.
type ciResult struct {
	// ran is false when linting is switched off for the deployment, in which
	// case no record is written: a row claiming "passed" for checks that never
	// executed would be worse than no row.
	ran bool
	// breakingRan is false when the module has breaking checks disabled or the
	// push has no predecessor to compare against. breaking_passed is still
	// recorded as true in that case, since nothing was found to break.
	breakingRan bool
}

const (
	// defaultMaxUploadFiles and defaultMaxUploadBytes bound a single push.
	// Upload merges the incoming set over every blob of the previous commit and
	// holds the result in memory, so without a cap one request can exhaust the
	// process. Overridable via sdk config.
	defaultMaxUploadFiles = 10_000
	defaultMaxUploadBytes = 256 << 20 // 256 MiB
)

// checkUploadLimits rejects a push that exceeds the configured file count or
// total byte size.
func (h *Handler) checkUploadLimits(files []*registryv1.File) error {
	maxFiles := h.sdkConfig.MaxUploadFiles
	if maxFiles <= 0 {
		maxFiles = defaultMaxUploadFiles
	}
	maxBytes := h.sdkConfig.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxUploadBytes
	}
	if len(files) > maxFiles {
		return connerr.ResourceExhausted(fmt.Sprintf("upload contains %d files, limit is %d", len(files), maxFiles))
	}
	var total int64
	for _, f := range files {
		total += int64(len(f.Content))
		if total > maxBytes {
			return connerr.ResourceExhausted(fmt.Sprintf("upload exceeds the %d byte limit", maxBytes))
		}
	}
	return nil
}

func (h *Handler) Upload(ctx context.Context, contents []*registryv1.UploadRequestContent) ([]*registryv1.Commit, error) {
	start := time.Now()

	tracer := telemetry.Tracer("hades/upload")
	ctx, span := tracer.Start(ctx, "upload")
	defer func() {
		telemetry.UploadLatency.Record(ctx, float64(time.Since(start).Milliseconds()))
		span.End()
	}()

	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		err := connerr.Unauthenticated("not authenticated")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no user in context")
		telemetry.UploadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, err
	}

	// Validate every path before any work happens. The buf.build protocol
	// adapter carries buf's own wire types, which protovalidate cannot
	// constrain, so this is the only check covering both upload routes.
	for _, content := range contents {
		if err := paths.Validate(content.Files); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid file path")
			telemetry.UploadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, connerr.InvalidArgument(err.Error())
		}
		if err := h.checkUploadLimits(content.Files); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "upload too large")
			telemetry.UploadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
	}

	policies := make([]*constants.Policy, 0, len(contents))
	for _, content := range contents {
		moduleFullName := content.ModuleRef.Owner + "/" + content.ModuleRef.Module
		policies = append(policies, &constants.Policy{
			Subject:      user.Username,
			ResourceType: string(constants.ResourceModule),
			Action:       string(constants.PUSH),
			Domain:       moduleFullName,
		})
	}
	if len(policies) > 0 {
		resp, err := h.authz.BatchCan(ctx, policies)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "batch auth check")
			telemetry.UploadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
		if !resp.Allowed {
			err := connerr.PermissionDenied("permission denied pushing to module " + resp.Policy.Domain)
			span.RecordError(err)
			span.SetStatus(codes.Error, "permission denied")
			telemetry.UploadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
	}

	var dedupCommits []*registryv1.Commit
	var workItems []uploadWorkItem
	var totalProtoBytes int64
	var totalFileCount int64

	for _, content := range contents {
		module, err := h.moduleDB.GetModulesByRefs(ctx, content.ModuleRef)
		if err != nil {
			h.logger.Error("failed to resolve module", "error", err, "procedure", "Upload",
				"owner", content.ModuleRef.GetOwner(), "module", content.ModuleRef.GetModule())
			return nil, connerr.FromDB(err)
		}
		if len(module) == 0 {
			return nil, connerr.NotFound("module not found")
		}

		moduleCommit, err := h.commitDB.GetCommitByOwnerModule(ctx, []*registryv1.ModuleRef{content.ModuleRef})
		if err != nil {
			h.logger.Error("failed to read the module's latest commit", "error", err, "procedure", "Upload",
				"module", module[0].Name)
			return nil, connerr.FromDB(err)
		}
		emptyCommit := len(moduleCommit) == 0
		var previousHead string
		if !emptyCommit {
			previousHead = moduleCommit[0].CommitHash
		}

		// The compare-and-swap value comes from git, not from the database.
		//
		// The database's "latest commit" is ordered by create_time, whose
		// resolution is one second on SQLite, so two commits written in the
		// same second order arbitrarily. The branch head is the thing the ref
		// update is actually racing against, so it is the thing to compare
		// against.
		var expectedHead string
		if head, err := h.gitStorage.ListCommits(ctx, module[0].Name, module[0].DefaultBranch, 1); err == nil && len(head) > 0 {
			expectedHead = head[0].SHA
		} else if err != nil && !errors.Is(err, gitstorage.ErrNotFound) {
			h.logger.Error("failed to read the module branch head", "error", err, "procedure", "Upload",
				"module", module[0].Name)
			return nil, connerr.InternalCause("failed to read the module's current head", err)
		}
		// Anything else leaves expectedHead empty, which means "the branch must
		// not exist yet": either there is no branch, or reading it returned
		// not-found.

		var files []*registryv1.File
		var listFiles []string
		var prevFiles []*registryv1.File

		if !emptyCommit {
			gitBlobs, err := h.gitStorage.ListBlobs(ctx, module[0].Name, moduleCommit[0].CommitHash)
			if err != nil {
				// Typically a repository the git backend cannot open, which is
				// what a misconfigured storage root looks like from here.
				h.logger.Error("failed to read the previous commit's files", "error", err, "procedure", "Upload",
					"module", module[0].Name, "commit_hash", moduleCommit[0].CommitHash)
				return nil, connerr.Internal("failed to read the module's current contents")
			}
			// The uploaded content is the module, not a patch on top of it.
			//
			// This used to seed the map from gitBlobs and overlay the incoming
			// files, which made every push a union with everything ever pushed
			// before. A module's contents could then only grow: a proto file
			// deleted or renamed away stayed published forever, and stayed
			// readable by direct path, with no way to withdraw a schema short
			// of deleting the module. It also defeated the deletion support in
			// both git backends, because ExistingPaths only removes a path that
			// is absent from the incoming set and the merge put every path in
			// that set.
			//
			// buf push sends the complete module content, so treating it as
			// authoritative is both what the protocol means and what makes the
			// digest describe what is actually stored. The previous contents
			// are still read: prevFiles feeds the breaking-change check, and
			// listFiles becomes ExistingPaths so the backends know what to
			// remove.
			uploadFiles := map[string]*registryv1.File{}
			for _, f := range content.Files {
				if f.Path == "buf.yaml" {
					continue // ignore user-supplied buf.yaml; registry settings control it
				}
				uploadFiles[f.Path] = &registryv1.File{Path: f.Path, Content: f.Content}
			}
			files = make([]*registryv1.File, 0, len(uploadFiles))
			for _, f := range uploadFiles {
				files = append(files, f)
			}
			listFiles = make([]string, 0, len(gitBlobs))
			for _, f := range gitBlobs {
				if f.Path == "buf.yaml" {
					continue
				}
				listFiles = append(listFiles, f.Path)
			}
			prevFiles = make([]*registryv1.File, 0, len(gitBlobs))
			for _, f := range gitBlobs {
				if f.Path == "buf.yaml" {
					continue
				}
				prevFiles = append(prevFiles, &registryv1.File{Path: f.Path, Content: f.Content})
			}
		} else {
			listFiles = []string{}
			// Strip any user-supplied buf.yaml from the initial push.
			files = make([]*registryv1.File, 0, len(content.Files))
			for _, f := range content.Files {
				if f.Path != "buf.yaml" {
					files = append(files, f)
				}
			}
		}

		// The path filter runs BEFORE the digest, not after.
		//
		// The digest is the dedup key, is stored on the commit, and is served to
		// clients to verify what they downloaded. Computing it over the
		// pre-filter set meant it described files that were then dropped and
		// never stored: two uploads differing only in a filtered file got
		// different digests and became different commits, and a client
		// verifying the digest against what it downloaded got a mismatch.
		files = paths.GetPath(files)

		// The limit is checked against the merged set, which is what is
		// actually held in memory and written, not only against the incoming
		// files. Checking per UploadRequestContent let a request with many
		// contents multiply the effective limit, and never checked the merge at
		// all.
		if err := h.checkUploadLimits(files); err != nil {
			return nil, err
		}

		digest, err := shake256.DigestFiles(files)
		if err != nil {
			h.logger.Error("failed to digest uploaded files", "error", err, "procedure", "Upload",
				"module", module[0].Name)
			return nil, connerr.Internal("failed to digest the uploaded files")
		}

		dig, _ := strings.CutPrefix(digest.String(), "shake256:")
		commit, err := h.commitDB.GetCommitByDigest(ctx, module[0].Id, dig)
		if err != nil {
			h.logger.Error("failed to look up the commit digest", "error", err, "procedure", "Upload",
				"module", module[0].Name)
			return nil, connerr.FromDB(err)
		}
		if commit != nil {
			dedupCommits = append(dedupCommits, commit)
			continue
		}

		totalFileCount += int64(len(files))
		for _, f := range files {
			totalProtoBytes += int64(len(f.Content))
		}
		gitFiles := make([]*gitstorage.File, len(files))
		for i, f := range files {
			gitFiles[i] = &gitstorage.File{Path: f.Path, Content: f.Content}
		}

		// Lint runs on every push including the first. Only the breaking check
		// needs a predecessor, and runProtoChecks already guards that on
		// len(prevFiles) > 0.
		var checks ciResult
		if h.protoLinter != nil && h.sdkConfig.LintEnabled {
			_, checksSpan := tracer.Start(ctx, "upload.proto_checks")
			var err error
			checks, err = h.runProtoChecks(ctx, checksSpan, files, prevFiles, lintPresetToRule(module[0].LintPreset), module[0].BreakingEnabled)
			if err != nil {
				checksSpan.End()
				return nil, err
			}
			checksSpan.End()
		}

		workItems = append(workItems, uploadWorkItem{
			module:       module[0],
			files:        gitFiles,
			listFiles:    listFiles,
			dig:          dig,
			digestStr:    dig,
			digestBytes:  digest.Value(),
			userId:       user.Id,
			moduleId:     module[0].Id,
			previousHead: previousHead,
			expectedHead: expectedHead,
			prevFiles:    prevFiles,
			checks:       checks,
		})
	}

	// Multi-module uploads are not atomic across modules.
	//
	// Each module gets its own git write and its own database transaction, so a
	// failure on the third module leaves the first two durable in both stores.
	// That cannot be fixed by widening the transaction: the git writes are not
	// transactional and cannot be rolled back as a set.
	//
	// What is fixed is the silence. A failure part-way through no longer
	// returns a bare error that reads as "nothing happened"; it names the
	// commits that did land, so the client knows what it is looking at. See
	// partialUploadError.
	var newCommits []*registryv1.Commit

	for _, w := range workItems {
		var logID uuid.UUID
		if h.gitalyOpLog != nil {
			logID, _ = h.gitalyOpLog.CreatePending(ctx, gitalyoplog.OpCommitFiles, w.module.Name, w.userId)
		}

		_, gitalySpan := tracer.Start(ctx, "upload.gitaly_write")

		wCopy := w

		commitId, err := h.gitStorage.PutFiles(ctx, gitstorage.PutFilesRequest{
			RepoPath: wCopy.module.Name,
			Branch:   wCopy.module.DefaultBranch,
			Files:    wCopy.files,
			// Paths present in the previous commit and absent from Files are
			// deleted by this commit. They used to be impossible to express.
			ExistingPaths: wCopy.listFiles,
			AuthorName:    user.Username,
			AuthorEmail:   user.Email,
			Message:       "upload:" + wCopy.dig,
			Digest:        wCopy.dig,
			// Compare-and-swap: the tree was computed from previousHead, so the
			// write is refused if the branch has moved since. Without it two
			// overlapping pushes each computed a tree from the same parent and
			// the second discarded the first, with both returning success.
			ExpectedHead: wCopy.expectedHead,
		})
		if err != nil {
			if errors.Is(err, gitstorage.ErrRefMoved) {
				gitalySpan.RecordError(err)
				gitalySpan.SetStatus(codes.Error, "ref moved")
				gitalySpan.End()
				if h.gitalyOpLog != nil && logID != uuid.Nil {
					_ = h.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusFailed, "", err.Error())
				}
				h.logger.Warn("push rejected: the module changed while the push was being prepared",
					"procedure", "Upload", "module", wCopy.module.Name)
				return nil, connerr.Aborted("the module changed while this push was being prepared; fetch and push again")
			}
			gitalySpan.RecordError(err)
			gitalySpan.SetStatus(codes.Error, "gitaly write")
			gitalySpan.End()
			if h.gitalyOpLog != nil && logID != uuid.Nil {
				_ = h.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusFailed, "", err.Error())
			}
			h.logger.Error("failed to write files to git", "error", err, "procedure", "Upload",
				"module", wCopy.module.Name, "branch", wCopy.module.DefaultBranch)
			return nil, h.partialUploadError(connerr.InternalCause("failed to write the commit", err),
				wCopy.module.Name, newCommits)
		}
		if len(commitId) < 32 {
			h.rollback(ctx, wCopy, commitId)
			gitalySpan.End()
			h.logger.Error("git returned an unusable commit id", "procedure", "Upload",
				"module", wCopy.module.Name, "commit_id", commitId)
			return nil, connerr.Internal("commit ID is less than 32 characters")
		}

		// A fresh UUID, not the first 32 characters of the git hash
		// reinterpreted as one. That produced a value with no version or
		// variant bits, which is not a valid UUID, while the same table's other
		// writer used uuid.New: one column, two provenances. The git hash is
		// recoverable from commit_hash, which is what that column is for.
		id := uuid.New()

		gitalySpan.End()

		result, err := h.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
			if err := h.commitDB.Create(
				txCtx,
				id, commitId, wCopy.userId, wCopy.moduleId,
				registryv1.DigestType_DIGEST_TYPE_B5,
				wCopy.digestStr, wCopy.userId, "",
			); err != nil {
				h.rollback(ctx, wCopy, commitId)
				h.logger.Error("failed to record the commit", "error", err, "procedure", "Upload",
					"module", wCopy.module.Name, "commit_hash", commitId)
				return nil, connerr.FromDB(err)
			}

			// The CI record goes in with the commit rather than after it. It is
			// the answer to "did this commit pass the checks", and a commit
			// without one is exactly the gap that made GetCIRun always 404.
			if h.ciRunDB != nil && wCopy.checks.ran {
				if _, err := h.ciRunDB.Create(txCtx, cirun.CreateParams{
					ModuleID:   wCopy.moduleId,
					CommitHash: commitId,
					LintPassed: true,
					// breaking_passed is true only when a comparison actually
					// happened. Every row used to be (lint=1, breaking=1,
					// ran=0): the registry asserted a check it had not run,
					// which is worse than asserting nothing, and anything built
					// on top, a UI badge, a CI gate, a policy check, inherited
					// a false green. A reader that looks at breaking_passed
					// alone is now safe; breaking_ran still separates "not run"
					// from "ran and was clean".
					BreakingPassed: wCopy.checks.breakingRan,
					BreakingRan:    wCopy.checks.breakingRan,
				}); err != nil {
					h.rollback(ctx, wCopy, commitId)
					h.logger.Error("failed to record the CI run", "error", err, "procedure", "Upload",
						"module", wCopy.module.Name, "commit_hash", commitId)
					return nil, connerr.FromDB(err)
				}
			}

			if h.sdkConfig.Enabled && len(h.sdkConfig.Generators) > 0 {
				if err := h.sdkJobDB.CreateBatch(txCtx, id.String(), wCopy.moduleId, h.sdkConfig.Generators); err != nil {
					h.rollback(ctx, wCopy, commitId)
					h.logger.Error("failed to enqueue SDK jobs", "error", err, "procedure", "Upload",
						"module", wCopy.module.Name, "commit_hash", commitId)
					return nil, connerr.FromDB(err)
				}
				// No module attribute. The metric SDK retains one time series
				// per unique attribute set for the process lifetime, so a
				// module id here means the counter's memory grows with the
				// number of modules ever pushed to and never shrinks. Module
				// identity belongs on the span and in the log line, both of
				// which carry it a few lines above, because those are sampled
				// and expire; a metric dimension is neither.
				telemetry.SDKJobsEnqueued.Add(ctx, int64(len(h.sdkConfig.Generators)))
			}

			return &registryv1.Commit{
				Id:         id.String(),
				CommitHash: commitId,
				OwnerId:    wCopy.userId,
				ModuleId:   wCopy.moduleId,
				// digestBytes, not []byte(digestStr). digestStr is the hex
				// rendering, so converting it gave 128 ASCII characters where a
				// b5 digest is 64 raw bytes, and buf rejected the module with
				// "invalid shake256 digest value: expected 64 bytes, got 128".
				// The column stays hex text, which is what a human reads; the
				// wire carries bytes, which is what the field is declared as.
				Digest: &registryv1.Digest{
					Value: wCopy.digestBytes,
					Type:  registryv1.DigestType_DIGEST_TYPE_B5,
				},
			}, nil
		}, 30*time.Second)

		if err != nil {
			if h.gitalyOpLog != nil && logID != uuid.Nil {
				_ = h.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusFailed, "", err.Error())
			}
			return nil, h.partialUploadError(err, wCopy.module.Name, newCommits)
		}

		newCommit, ok := result.(*registryv1.Commit)
		if !ok {
			h.logger.Error("unexpected result type from the commit transaction",
				"procedure", "Upload", "module", wCopy.module.Name)
			return nil, connerr.Internal("failed to record the commit")
		}
		if h.gitalyOpLog != nil && logID != uuid.Nil {
			_ = h.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusCompleted, newCommit.CommitHash, "")
		}
		h.notifyCommitPushed(ctx, wCopy.module, newCommit, user)
		newCommits = append(newCommits, newCommit)
	}

	commits := make([]*registryv1.Commit, 0, len(dedupCommits)+len(newCommits))
	commits = append(commits, dedupCommits...)
	commits = append(commits, newCommits...)

	// Process-wide allocation and GC counters are deliberately not sampled here.
	// runtime.ReadMemStats stops the world, and under any concurrency its
	// deltas attribute every goroutine's allocations to whichever request
	// happened to bracket them. Runtime memory is exported once per interval by
	// the telemetry package instead.
	telemetry.UploadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "ok")))
	telemetry.UploadProtoBytes.Record(ctx, totalProtoBytes)
	telemetry.UploadFileCount.Record(ctx, totalFileCount)

	return commits, nil
}

// partialUploadError reports a failure that happened after earlier modules in
// the same request were already committed.
//
// When nothing landed it returns cause unchanged. When something did, it says
// so and names what: the client saw one failed upload while the registry had
// two new commits, and had no way to tell.
func (h *Handler) partialUploadError(cause error, failedModule string, landed []*registryv1.Commit) error {
	if len(landed) == 0 {
		return cause
	}
	names := make([]string, 0, len(landed))
	for _, c := range landed {
		names = append(names, c.CommitHash)
	}
	h.logger.Error("upload partially applied", "procedure", "Upload",
		"failed_module", failedModule, "committed", names, "error", cause)
	return connerr.Aborted(fmt.Sprintf(
		"upload failed on module %s, but %d earlier module(s) in this request were already committed (%s); "+
			"re-push only the modules that failed",
		failedModule, len(landed), strings.Join(names, ", ")))
}

// rollback undoes a git write whose database transaction failed.
//
// The error is logged rather than discarded. A failed rollback leaves the
// branch ahead of the database, and saying nothing meant the discrepancy
// surfaced later as a confusing failure far from its cause. It is also recorded
// in the operation log so the reconciliation pass can pick it up.
func (h *Handler) rollback(ctx context.Context, w uploadWorkItem, commitID string) {
	if err := h.gitStorage.RollbackCommit(ctx, w.module.Name, w.module.DefaultBranch, commitID, w.previousHead); err != nil {
		h.logger.Error("failed to roll back the git commit after a failed transaction",
			"error", err, "procedure", "Upload", "module", w.module.Name, "commit_hash", commitID)
	}
}

// runProtoChecks lints the incoming file set and, when the module asks for it
// and a predecessor exists, checks it for breaking changes against that
// predecessor. A violation is returned as an error and rejects the push.
//
// The returned ciResult describes what ran, so a cleared push can record it.
func (h *Handler) runProtoChecks(ctx context.Context, checksSpan trace.Span, files, prevFiles []*registryv1.File, lintPreset string, breakingEnabled bool) (ciResult, error) {
	var result ciResult

	tmpDir, err := os.MkdirTemp("", "hades-proto-*")
	if err != nil {
		checksSpan.RecordError(err)
		checksSpan.SetStatus(codes.Error, "mktemp")
		return result, connerr.Internal("failed to create temp directory")
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	for _, f := range files {
		if err := writeProtoFile(tmpDir, f.Path, f.Content); err != nil {
			checksSpan.RecordError(err)
			checksSpan.SetStatus(codes.Error, "write proto file")
			return result, connerr.Internal("failed to write proto file")
		}
	}

	// Write buf.yaml once, used by both lint and breaking.
	// If buf.yaml already came from git (via writeProtoFile above), this is a no-op.
	if err := ensureBufYAML(tmpDir, lintPreset, breakingEnabled); err != nil {
		checksSpan.RecordError(err)
		checksSpan.SetStatus(codes.Error, "write buf.yaml")
		return result, connerr.Internal("failed to write buf.yaml")
	}

	if err := h.protoLinter.Lint(ctx, tmpDir); err != nil {
		checksSpan.RecordError(err)
		checksSpan.SetStatus(codes.Error, "lint")
		return result, connerr.InvalidArgument(err.Error())
	}
	result.ran = true

	if h.breakingChecker != nil && h.sdkConfig.BreakingEnabled && breakingEnabled && len(prevFiles) > 0 {
		prevTmpDir, err := os.MkdirTemp("", "hades-prev-*")
		if err != nil {
			checksSpan.RecordError(err)
			checksSpan.SetStatus(codes.Error, "mktemp prev")
			return result, connerr.Internal("failed to create temp directory for previous files")
		}
		defer func() { _ = os.RemoveAll(prevTmpDir) }()

		for _, f := range prevFiles {
			if err := writeProtoFile(prevTmpDir, f.Path, f.Content); err != nil {
				checksSpan.RecordError(err)
				checksSpan.SetStatus(codes.Error, "write prev proto file")
				return result, connerr.Internal("failed to write previous proto file")
			}
		}
		if err := h.breakingChecker.Check(ctx, tmpDir, prevTmpDir); err != nil {
			checksSpan.RecordError(err)
			checksSpan.SetStatus(codes.Error, "breaking check")
			return result, connerr.InvalidArgument(err.Error())
		}
		result.breakingRan = true
	}
	return result, nil
}

// notifyCommitPushed raises a notification on everyone with a stake in the
// module except the person who pushed.
//
// Best effort by design: a notification that cannot be written must not fail a
// push that has already been committed to git and to the database.
func (h *Handler) notifyCommitPushed(ctx context.Context, module *registryv1.Module, commit *registryv1.Commit, pusher *identityv1.User) {
	if h.notificationDB == nil || module == nil || commit == nil {
		return
	}

	// An organisation owns modules but nobody signs in as one, so the members
	// are the real recipients. ListMembers is empty for a personal namespace,
	// where the owner is a person who can be notified directly.
	recipients := []string{}
	if h.orgDB != nil {
		if members, err := h.orgDB.ListMembers(ctx, module.OwnerId); err == nil {
			for _, m := range members {
				if m.User != nil {
					recipients = append(recipients, m.User.Id)
				}
			}
		} else {
			h.logger.Error("failed to list org members for push notification", "error", err, "module", module.Name)
		}
	}
	if len(recipients) == 0 {
		recipients = append(recipients, module.OwnerId)
	}

	title := fmt.Sprintf("New commit on %s", module.Name)
	body := fmt.Sprintf("%s pushed %s to %s.", pusher.Username, shortHash(commit.CommitHash), module.Name)

	// One insert for the whole set rather than one per member. A large
	// organisation meant one round trip per member on the push path.
	targets := make([]string, 0, len(recipients))
	seen := make(map[string]struct{}, len(recipients))
	for _, userID := range recipients {
		if userID == "" || userID == pusher.Id {
			continue
		}
		if _, dup := seen[userID]; dup {
			continue
		}
		seen[userID] = struct{}{}
		targets = append(targets, userID)
	}
	if len(targets) == 0 {
		return
	}
	if err := h.notificationDB.CreateBatch(ctx, targets, notificationdb.TypeCommitPushed, title, body, commit.Id); err != nil {
		h.logger.Error("failed to create push notifications", "error", err,
			"recipients", len(targets), "module", module.Name)
	}
}

// shortHash abbreviates a git commit hash for display.
func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// generateBufYAML builds buf.yaml content from the module's current DB settings.
func generateBufYAML(m *registryv1.Module, registryHost string) []byte {
	bsrName := m.Name
	if registryHost != "" {
		bsrName = registryHost + "/" + m.Name
	}
	out := fmt.Sprintf("version: v2\nmodules:\n  - path: .\n    name: %s\nlint:\n  use:\n    - %s\n",
		bsrName, lintPresetToRule(m.LintPreset))
	if m.BreakingEnabled {
		out += "breaking:\n  use:\n    - FILE\n"
	}
	return []byte(out)
}

func lintPresetToRule(p registryv1.LintPreset) string {
	switch p {
	case registryv1.LintPreset_LINT_PRESET_BASIC:
		return "BASIC"
	case registryv1.LintPreset_LINT_PRESET_MINIMAL:
		return "MINIMAL"
	case registryv1.LintPreset_LINT_PRESET_COMMENTS:
		return "COMMENTS"
	default:
		return "DEFAULT"
	}
}

// ensureBufYAML writes buf.yaml from DB settings into dir for lint/breaking checks.
// Always overwrites so proto checks use current registry settings, not a user-supplied file.
func ensureBufYAML(dir, lintPreset string, breakingEnabled bool) error {
	content := fmt.Sprintf("version: v2\nlint:\n  use:\n    - %s\n", lintPreset)
	if breakingEnabled {
		content += "breaking:\n  use:\n    - FILE\n"
	}
	return os.WriteFile(filepath.Join(dir, "buf.yaml"), []byte(content), 0o600)
}

// writeProtoFile writes one upload file into dir.
//
// The path is validated and the resolved destination is confirmed to still be
// under dir. Upload rejects unsafe paths before reaching this point; the check
// is repeated here because this function turns caller-supplied strings into
// filesystem writes and must not depend on a caller doing the right thing.
func writeProtoFile(dir, relPath string, content []byte) error {
	if err := paths.ValidatePath(relPath); err != nil {
		return err
	}
	dest := filepath.Join(dir, relPath)
	cleanDir := filepath.Clean(dir) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(dest), cleanDir) {
		// InvalidArgument, not a bare error: a raw error reaches the client as
		// a 500, so a caller who sent a bad path was told the server broke.
		return connerr.InvalidArgument(fmt.Sprintf("refusing to write outside the working directory: %q", relPath))
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return err
	}
	return os.WriteFile(dest, content, 0o600)
}
