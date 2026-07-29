package content

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/gitalyoplog"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
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
	prevFiles    []*registryv1.File
}

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

	user, ok := ctx.Value(constants.ContextKeyUser).(*identityv1.User)
	if !ok {
		err := connErr.Unauthenticated("not authenticated")
		span.RecordError(err)
		span.SetStatus(codes.Error, "no user in context")
		telemetry.UploadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, err
	}

	policies := make([]*constants.Policy, 0, len(contents))
	for _, content := range contents {
		moduleFullName := content.ModuleRef.Owner + "/" + content.ModuleRef.Module
		policies = append(policies, &constants.Policy{
			Subject: user.Username,
			Object:  string(constants.ResourceModule),
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

	for _, content := range contents {
		module, err := h.moduleDB.GetModulesByRefs(ctx, content.ModuleRef)
		if err != nil {
			return nil, err
		}
		if len(module) == 0 {
			return nil, connErr.NotFound("module not found")
		}

		moduleCommit, err := h.commitDB.GetCommitByOwnerModule(ctx, []*registryv1.ModuleRef{content.ModuleRef})
		if err != nil {
			return nil, connErr.FromPgx(err)
		}
		emptyCommit := len(moduleCommit) == 0
		var previousHead string
		if !emptyCommit {
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
		commit, err := h.commitDB.GetCommitByDigest(ctx, module[0].Id, dig)
		if err != nil {
			return nil, connErr.FromPgx(err)
		}
		if commit != nil {
			dedupCommits = append(dedupCommits, commit)
			continue
		}

		files = paths.GetPath(files)
		totalFileCount += int64(len(files))
		for _, f := range files {
			totalProtoBytes += int64(len(f.Content))
		}
		gitFiles := make([]*gitstorage.File, len(files))
		for i, f := range files {
			gitFiles[i] = &gitstorage.File{Path: f.Path, Content: f.Content}
		}

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

	var newCommits []*registryv1.Commit
	var totalGitalyAllocBytes int64

	for _, w := range workItems {
		var logID uuid.UUID
		if h.gitalyOpLog != nil {
			logID, _ = h.gitalyOpLog.CreatePending(ctx, gitalyoplog.OpCommitFiles, w.module.Name, w.userId)
		}

		var gitalyMemBefore runtime.MemStats
		runtime.ReadMemStats(&gitalyMemBefore)
		_, gitalySpan := tracer.Start(ctx, "upload.gitaly_write")

		wCopy := w

		commitId, err := h.gitStorage.PutFiles(ctx, wCopy.module.Name, wCopy.module.DefaultBranch, wCopy.files, user.Username, user.Email, "upload:"+wCopy.dig, wCopy.listFiles)
		if err != nil {
			gitalySpan.RecordError(err)
			gitalySpan.SetStatus(codes.Error, "gitaly write")
			gitalySpan.End()
			if h.gitalyOpLog != nil && logID != uuid.Nil {
				_ = h.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusFailed, "", err.Error())
			}
			return nil, err
		}
		if len(commitId) < 32 {
			_ = h.gitStorage.RollbackCommit(ctx, wCopy.module.Name, wCopy.module.DefaultBranch, commitId, wCopy.previousHead)
			gitalySpan.End()
			return nil, connErr.Internal("commit ID is less than 32 characters")
		}
		id, err := uuid.Parse(commitId[:32])
		if err != nil {
			_ = h.gitStorage.RollbackCommit(ctx, wCopy.module.Name, wCopy.module.DefaultBranch, commitId, wCopy.previousHead)
			gitalySpan.End()
			return nil, connErr.Internal("cannot parse commit UUID")
		}

		var gitalyMemAfter runtime.MemStats
		runtime.ReadMemStats(&gitalyMemAfter)
		totalGitalyAllocBytes += int64(gitalyMemAfter.TotalAlloc - gitalyMemBefore.TotalAlloc)
		gitalySpan.End()

		result, err := h.uow.Do(ctx, func(txCtx context.Context) (interface{}, error) {
			if err := h.commitDB.Create(
				txCtx,
				id, commitId, wCopy.userId, wCopy.moduleId,
				registryv1.DigestType_DIGEST_TYPE_B5,
				wCopy.digestStr, wCopy.userId, "",
			); err != nil {
				_ = h.gitStorage.RollbackCommit(ctx, wCopy.module.Name, wCopy.module.DefaultBranch, commitId, wCopy.previousHead)
				return nil, connErr.FromPgx(err)
			}

			if h.sdkConfig.Enabled && len(h.sdkConfig.Generators) > 0 {
				if err := h.sdkJobDB.CreateBatch(txCtx, id.String(), wCopy.moduleId, h.sdkConfig.Generators); err != nil {
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

		if err != nil {
			if h.gitalyOpLog != nil && logID != uuid.Nil {
				_ = h.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusFailed, "", err.Error())
			}
			return nil, err
		}

		newCommit := result.(*registryv1.Commit)
		if h.gitalyOpLog != nil && logID != uuid.Nil {
			_ = h.gitalyOpLog.UpdateStatus(ctx, logID, gitalyoplog.StatusCompleted, newCommit.CommitHash, "")
		}
		newCommits = append(newCommits, newCommit)
	}

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
