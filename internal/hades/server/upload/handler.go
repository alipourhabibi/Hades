// Package upload implements the schema upload pipeline. It validates proto
// files, computes content-addressable digests, deduplicates against existing
// commits, and writes new commits to both Gitaly and PostgreSQL inside a
// unit-of-work transaction with saga-style compensation.
package upload

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"connectrpc.com/connect"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/config"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	"github.com/alipourhabibi/Hades/internal/hades/server/authorization"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db"
	commitdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	moduledb "github.com/alipourhabibi/Hades/internal/hades/storage/db/module"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/proto/breaking"
	"github.com/alipourhabibi/Hades/internal/proto/lint"
	"github.com/alipourhabibi/Hades/internal/telemetry"
	connErr "github.com/alipourhabibi/Hades/utils/errors"
	"github.com/alipourhabibi/Hades/utils/paths"
	"github.com/alipourhabibi/Hades/utils/shake256"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// uploadWorkItem holds all data needed to perform one upload write (git + DB).
// It is populated during the read-only Phase 1 loop and consumed during
// the per-content UoW in Phase 2.
type uploadWorkItem struct {
	module       *registryv1.Module
	files        []*gitstorage.File
	listFiles    []string
	dig          string // hex digest without "shake256:" prefix
	digestStr    string // same, for DB column
	digestBytes  []byte
	userId       string
	moduleId     string
	previousHead string // CommitHash of the last commit, "" for an empty repo
	prevFiles    []*registryv1.File
}

// Handler processes uploads using own proto types.
// The buf adapter (Server in upalod.go) wraps this to expose the buf.build wire protocol.
// The own CLI will call Handler.Upload directly once the own upload service is registered.
type Handler struct {
	moduleDB        moduledb.Storage
	commitDB        commitdb.Storage
	gitStorage      gitstorage.Storage
	sdkJobDB        sdkjob.Storage
	sdkConfig       config.SDKConfig
	protoLinter     *lint.Linter
	breakingChecker *breaking.Checker
	uow             db.UnitOfWork
	authz           *authorization.Server
	gitalyOpLog     *gitalyoplog.GitalyOpLogStorage
}

func NewHandler(deps *server.Dependencies) *Handler {
	return &Handler{
		moduleDB:        deps.ModuleDB,
		commitDB:        deps.CommitDB,
		gitStorage:      deps.GitStorage,
		sdkJobDB:        deps.SDKJobDB,
		sdkConfig:       deps.SDKConfig,
		protoLinter:     deps.ProtoLinter,
		breakingChecker: deps.BreakingChk,
		uow:             deps.UoW,
		authz:           deps.Authorization,
		gitalyOpLog:     deps.GitalyOpLog,
	}
}

// Upload processes a batch of upload content items and returns the resulting commits.
// All auth, tracing, and telemetry are handled here - the buf adapter is a pure model converter.
func (h *Handler) Upload(ctx context.Context, contents []*registryv1.UploadRequestContent) ([]*registryv1.Commit, error) {
	start := time.Now()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	tracer := telemetry.Tracer("hades/upload")
	ctx, span := tracer.Start(ctx, "upload")
	defer func() {
		telemetry.UploadLatency.Record(ctx, float64(time.Since(start).Milliseconds()))
		span.End()
	}()

	user, ok := ctx.Value(constants.ContextKeyUser).(*registryv1.User)
	if !ok {
		err := connErr.Internal("missing user in context")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no user in context")
		telemetry.UploadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, err
	}

	// Collect all push policies and check them in a single BatchEnforce call
	// rather than one OPA roundtrip per content item.
	policies := make([]*constants.Policy, 0, len(contents))
	for _, content := range contents {
		moduleFullName := content.ModuleRef.Owner + "/" + content.ModuleRef.Module
		policies = append(policies, &constants.Policy{
			Subject: user.Username,
			Object:  string(constants.REPOSITORY),
			Action:  string(constants.PUSH),
			Domain:  moduleFullName,
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
			err := connErr.PermissionDenied("permission denied pushing to module " + resp.Policy.Domain)
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

	// Phase 1: read-only - DB reads, digest computation, proto checks.
	// No Gitaly writes happen here.
	for _, content := range contents {
		module, err := h.moduleDB.GetModulesByRefs(ctx, content.ModuleRef)
		if err != nil {
			return nil, err
		}
		if len(module) == 0 {
			return nil, connErr.NotFound("module not found")
		}

		var emptyCommit bool
		var previousHead string
		moduleCommit, err := h.commitDB.GetCommitByOwnerModule(ctx, []*registryv1.ModuleRef{content.ModuleRef})
		if err != nil {
			// Not found means the module has no commits yet - treat as empty.
			var ce *connect.Error
			if errors.As(connErr.FromPgx(err), &ce) && ce.Code() == connect.CodeNotFound {
				emptyCommit = true
			} else {
				return nil, connErr.FromPgx(err)
			}
		} else if len(moduleCommit) > 0 {
			previousHead = moduleCommit[0].CommitHash
		}

		var files []*registryv1.File
		var listFiles []string
		var prevFiles []*registryv1.File

		if !emptyCommit {
			gitBlobs, err := h.gitStorage.ListBlobs(ctx, module[0].Name, moduleCommit[0].CommitHash)
			if err != nil {
				return nil, err
			}
			uploadFiles := map[string]*registryv1.File{}
			for _, f := range gitBlobs {
				uploadFiles[f.Path] = &registryv1.File{Path: f.Path, Content: f.Content}
			}
			for _, f := range content.Files {
				uploadFiles[f.Path] = &registryv1.File{Path: f.Path, Content: f.Content}
			}
			files = make([]*registryv1.File, 0, len(uploadFiles))
			for _, f := range uploadFiles {
				files = append(files, f)
			}
			listFiles = make([]string, 0, len(gitBlobs))
			for _, f := range gitBlobs {
				listFiles = append(listFiles, f.Path)
			}
			prevFiles = make([]*registryv1.File, len(gitBlobs))
			for i, f := range gitBlobs {
				prevFiles[i] = &registryv1.File{Path: f.Path, Content: f.Content}
			}
		} else {
			listFiles = []string{}
			files = content.Files
		}

		digest, err := shake256.DigestFiles(files)
		if err != nil {
			return nil, err
		}

		dig, _ := strings.CutPrefix(digest.String(), "shake256:")
		commit, err := h.commitDB.GetCommitByQuery(ctx, map[string]any{"digest_value": dig})
		if err != nil {
			// Not found is expected - not a dedup hit.
			var ce *connect.Error
			if errors.As(connErr.FromPgx(err), &ce) && ce.Code() != connect.CodeNotFound {
				return nil, connErr.FromPgx(err)
			}
		}
		if err == nil && commit != nil {
			dedupCommits = append(dedupCommits, commit)
			continue
		}

		files = paths.GetPath(files)
		totalFileCount += int64(len(files))
		for _, f := range files {
			totalProtoBytes += int64(len(f.Content))
		}
		// Convert []*registryv1.File to []*gitstorage.File for the work item.
		gitFiles := make([]*gitstorage.File, len(files))
		for i, f := range files {
			gitFiles[i] = &gitstorage.File{Path: f.Path, Content: f.Content}
		}

		// Proto health checks. Storing per-file digests in the DB would let us
		// skip the full blob fetch on re-upload, but that requires a schema change.
		if h.protoLinter != nil && h.sdkConfig.LintEnabled {
			_, checksSpan := tracer.Start(ctx, "upload.proto_checks")
			if err := h.runProtoChecks(ctx, checksSpan, files, prevFiles, emptyCommit); err != nil {
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
			prevFiles:    prevFiles,
		})
	}

	// Phase 2: per-content UoW - Gitaly write + DB insert.
	// Each content is committed atomically: the UoW callback opens the DB
	// transaction, then fires the Gitaly write. If the DB insert fails after
	// the Gitaly write, RollbackCommit is called as compensation.
	// Auth is already enforced above; direct storage access is intentional.
	var newCommits []*registryv1.Commit
	var totalGitalyAllocBytes int64

	for _, w := range workItems {
		// Write 'pending' log before the Gitaly call (auto-committed).
		var logID uuid.UUID
		if h.gitalyOpLog != nil {
			logID, _ = h.gitalyOpLog.CreatePending(ctx, gitalyoplog.OpCommitFiles, w.module.Name, w.userId)
		}

		var gitalyMemBefore runtime.MemStats
		runtime.ReadMemStats(&gitalyMemBefore)
		_, gitalySpan := tracer.Start(ctx, "upload.gitaly_write")

		wCopy := w // capture for closure
		result, err := h.uow.Do(ctx, func(ctx context.Context) (interface{}, error) {
			// Git write inside the UoW so the DB transaction is already open.
			commitId, err := h.gitStorage.PutFiles(ctx, wCopy.module.Name, wCopy.module.DefaultBranch, wCopy.files, user.Username, user.Email, "upload:"+wCopy.dig, wCopy.listFiles)
			if err != nil {
				return nil, err
			}
			if len(commitId) < 32 {
				_ = h.gitStorage.RollbackCommit(ctx, wCopy.module.Name, wCopy.module.DefaultBranch, commitId, wCopy.previousHead)
				return nil, connErr.Internal("commit ID is less than 32 characters")
			}
			id, err := uuid.Parse(commitId[:32])
			if err != nil {
				_ = h.gitStorage.RollbackCommit(ctx, wCopy.module.Name, wCopy.module.DefaultBranch, commitId, wCopy.previousHead)
				return nil, connErr.Internal("cannot parse commit UUID")
			}

			if err := h.commitDB.Create(
				ctx,
				id, commitId, wCopy.userId, wCopy.moduleId,
				registryv1.DigestType_DIGEST_TYPE_B5,
				wCopy.digestStr, wCopy.userId, "",
			); err != nil {
				_ = h.gitStorage.RollbackCommit(ctx, wCopy.module.Name, wCopy.module.DefaultBranch, commitId, wCopy.previousHead)
				return nil, connErr.FromPgx(err)
			}

			if h.sdkConfig.Enabled && len(h.sdkConfig.Generators) > 0 {
				if err := h.sdkJobDB.CreateBatch(ctx, id.String(), wCopy.moduleId, h.sdkConfig.Generators); err != nil {
					_ = h.gitStorage.RollbackCommit(ctx, wCopy.module.Name, wCopy.module.DefaultBranch, commitId, wCopy.previousHead)
					return nil, connErr.FromPgx(err)
				}
				telemetry.SDKJobsEnqueued.Add(ctx, int64(len(h.sdkConfig.Generators)),
					metric.WithAttributes(attribute.String("module", wCopy.moduleId)),
				)
			}

			return &registryv1.Commit{
				Id:         id.String(),
				CommitHash: commitId,
				OwnerId:    wCopy.userId,
				ModuleId:   wCopy.moduleId,
				Digest: &registryv1.Digest{
					Value: []byte(wCopy.digestStr),
					Type:  registryv1.DigestType_DIGEST_TYPE_B5,
				},
			}, nil
		}, 30*time.Second)

		var gitalyMemAfter runtime.MemStats
		runtime.ReadMemStats(&gitalyMemAfter)
		totalGitalyAllocBytes += int64(gitalyMemAfter.TotalAlloc - gitalyMemBefore.TotalAlloc)

		if err != nil {
			gitalySpan.RecordError(err)
			gitalySpan.SetStatus(codes.Error, "gitaly write or db commit")
			gitalySpan.End()
			if h.gitalyOpLog != nil && logID != uuid.Nil {
				_ = h.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusFailed, "", err.Error())
			}
			return nil, err
		}
		gitalySpan.End()

		newCommit := result.(*registryv1.Commit)
		if h.gitalyOpLog != nil && logID != uuid.Nil {
			_ = h.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusCompleted, newCommit.CommitHash, "")
		}
		newCommits = append(newCommits, newCommit)
	}

	// Merge dedup hits with newly inserted commits for the response.
	commits := make([]*registryv1.Commit, 0, len(dedupCommits)+len(newCommits))
	commits = append(commits, dedupCommits...)
	commits = append(commits, newCommits...)

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	allocDelta := int64(memAfter.TotalAlloc - memBefore.TotalAlloc)
	gcRuns := int64(memAfter.NumGC - memBefore.NumGC)
	gcPauseMs := float64(memAfter.PauseTotalNs-memBefore.PauseTotalNs) / 1e6

	telemetry.UploadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "ok")))
	telemetry.UploadProtoBytes.Record(ctx, totalProtoBytes)
	telemetry.UploadFileCount.Record(ctx, totalFileCount)
	telemetry.UploadAllocBytes.Record(ctx, allocDelta)
	telemetry.UploadGitalyAllocBytes.Record(ctx, totalGitalyAllocBytes)
	telemetry.UploadGCRuns.Record(ctx, gcRuns)
	telemetry.UploadGCPauseMs.Record(ctx, gcPauseMs)

	return commits, nil
}

func (h *Handler) runProtoChecks(ctx context.Context, checksSpan trace.Span, files, prevFiles []*registryv1.File, emptyCommit bool) error {
	tmpDir, err := os.MkdirTemp("", "hades-proto-*")
	if err != nil {
		checksSpan.RecordError(err)
		checksSpan.SetStatus(codes.Error, "mktemp")
		return connErr.Internal("failed to create temp directory")
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	for _, f := range files {
		if err := writeProtoFile(tmpDir, f.Path, f.Content); err != nil {
			checksSpan.RecordError(err)
			checksSpan.SetStatus(codes.Error, "write proto file")
			return connErr.Internal("failed to write proto file")
		}
	}
	if err := h.protoLinter.Lint(ctx, tmpDir); err != nil {
		checksSpan.RecordError(err)
		checksSpan.SetStatus(codes.Error, "lint")
		return connErr.InvalidArgument(err.Error())
	}

	if !emptyCommit && h.breakingChecker != nil && h.sdkConfig.BreakingEnabled && len(prevFiles) > 0 {
		prevTmpDir, err := os.MkdirTemp("", "hades-prev-*")
		if err != nil {
			checksSpan.RecordError(err)
			checksSpan.SetStatus(codes.Error, "mktemp prev")
			return connErr.Internal("failed to create temp directory for previous files")
		}
		defer func() { _ = os.RemoveAll(prevTmpDir) }()

		for _, f := range prevFiles {
			if err := writeProtoFile(prevTmpDir, f.Path, f.Content); err != nil {
				checksSpan.RecordError(err)
				checksSpan.SetStatus(codes.Error, "write prev proto file")
				return connErr.Internal("failed to write previous proto file")
			}
		}
		if err := h.breakingChecker.Check(ctx, tmpDir, prevTmpDir); err != nil {
			checksSpan.RecordError(err)
			checksSpan.SetStatus(codes.Error, "breaking check")
			return connErr.InvalidArgument(err.Error())
		}
	}
	return nil
}

func writeProtoFile(dir, path string, content []byte) error {
	dest := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, content, 0o644)
}
