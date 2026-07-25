// Package download provides module file download logic using internal proto
// types. The buf adapter (bufdownload) wraps this to expose the buf.build
// wire protocol; the internal registry API uses it directly.
package download

import (
	"context"
	"runtime"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/hades/server"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

type moduleQuerier interface {
	GetModulesByRefs(ctx context.Context, refs ...*registryv1.ModuleRef) ([]*registryv1.Module, error)
}

type commitQuerier interface {
	GetCommitById(ctx context.Context, id string) (*registryv1.Commit, error)
	GetCommitByOwnerModule(ctx context.Context, refs []*registryv1.ModuleRef) ([]*registryv1.Commit, error)
}

type readAccessChecker interface {
	CheckReadAccess(ctx context.Context, user *registryv1.User, modules []*registryv1.Module) error
}

type Handler struct {
	moduleDB   moduleQuerier
	commitDB   commitQuerier
	gitStorage gitstorage.Storage
	authz      readAccessChecker
}

func New(deps *server.Dependencies) *Handler {
	return &Handler{
		moduleDB:   deps.ModuleDB,
		commitDB:   deps.CommitDB,
		gitStorage: deps.GitStorage,
		authz:      deps.Authorization,
	}
}

// Download resolves refs to file trees and enforces read access.
//
// commitIDs are resource IDs that the buf CLI resolved as commit IDs from a
// prior GetGraph call. moduleRefs are owner/module name-based refs.
// The two paths are kept separate so each uses the correct storage lookup:
// commit IDs go directly to GetCommitById; name refs use GetModulesByRefs
// then GetCommitByOwnerModule for the latest commit.
func (h *Handler) Download(ctx context.Context, commitIDs []string, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.DownloadResponseContent, error) {
	start := time.Now()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	tracer := telemetry.Tracer("hades/download")
	ctx, span := tracer.Start(ctx, "download")
	defer func() {
		telemetry.DownloadLatency.Record(ctx, float64(time.Since(start).Milliseconds()))
		span.End()
	}()

	user, _ := ctx.Value(constants.ContextKeyUser).(*registryv1.User)

	var contents []*registryv1.DownloadResponseContent

	// --- Commit ID path ---
	// ResourceRef.id from buf CLI after GetGraph is a commit UUID.
	// Resolve the commit directly, then fetch the full module (for visibility
	// check) using the module_id stored on the commit.
	for _, commitID := range commitIDs {
		commit, err := h.commitDB.GetCommitById(ctx, commitID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "get commit by id")
			telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
		modules, err := h.moduleDB.GetModulesByRefs(ctx, &registryv1.ModuleRef{Id: commit.ModuleId})
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "get module for commit")
			telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
		if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
			telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
		gitFiles, err := h.gitStorage.ListBlobs(ctx, commit.Module.Name, commit.CommitHash)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "list blobs")
			telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
		pbFiles := make([]*registryv1.File, len(gitFiles))
		for i, f := range gitFiles {
			pbFiles[i] = &registryv1.File{Path: f.Path, Content: f.Content}
		}
		contents = append(contents, &registryv1.DownloadResponseContent{
			Commit: commit,
			Files:  pbFiles,
		})
	}

	// --- Module name path ---
	// owner/module name refs: look up the module, check access, then fetch
	// the latest commit and its files.
	if len(moduleRefs) > 0 {
		modules, err := h.moduleDB.GetModulesByRefs(ctx, moduleRefs...)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "get modules")
			telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
		if err := h.authz.CheckReadAccess(ctx, user, modules); err != nil {
			telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
		commits, err := h.commitDB.GetCommitByOwnerModule(ctx, moduleRefs)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "fetch commits")
			telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
		for _, commit := range commits {
			gitFiles, err := h.gitStorage.ListBlobs(ctx, commit.Module.Name, commit.CommitHash)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "list blobs")
				telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
				return nil, err
			}
			pbFiles := make([]*registryv1.File, len(gitFiles))
			for i, f := range gitFiles {
				pbFiles[i] = &registryv1.File{Path: f.Path, Content: f.Content}
			}
			contents = append(contents, &registryv1.DownloadResponseContent{
				Commit: commit,
				Files:  pbFiles,
			})
		}
	}

	var gitalyMemAfter runtime.MemStats
	runtime.ReadMemStats(&gitalyMemAfter)

	var totalProtoBytes int64
	for _, c := range contents {
		for _, f := range c.Files {
			totalProtoBytes += int64(len(f.Content))
		}
	}

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	allocDelta := int64(memAfter.TotalAlloc - memBefore.TotalAlloc)
	gcRuns := int64(memAfter.NumGC - memBefore.NumGC)
	gcPauseMs := float64(memAfter.PauseTotalNs-memBefore.PauseTotalNs) / 1e6

	telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "ok")))
	telemetry.DownloadProtoBytes.Record(ctx, totalProtoBytes)
	telemetry.DownloadAllocBytes.Record(ctx, allocDelta)
	telemetry.DownloadGCRuns.Record(ctx, gcRuns)
	telemetry.DownloadGCPauseMs.Record(ctx, gcPauseMs)

	return contents, nil
}
