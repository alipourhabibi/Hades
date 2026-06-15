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
	"github.com/alipourhabibi/Hades/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

// moduleQuerier is the subset of ModuleStorage used by the Handler.
type moduleQuerier interface {
	GetModulesByRefs(ctx context.Context, refs ...*registryv1.ModuleRef) ([]*registryv1.Module, error)
}

// commitQuerier is the subset of CommitStorage used by the Handler.
type commitQuerier interface {
	GetCommitByOwnerModule(ctx context.Context, refs []*registryv1.ModuleRef) ([]*registryv1.Commit, error)
}

// blobLister is the subset of BlobService used by the Handler.
type blobLister interface {
	ListBlobs(ctx context.Context, commit *registryv1.Commit) ([]*registryv1.DownloadResponseContent, error)
}

// readAccessChecker is the subset of the authorization Server used by the Handler.
type readAccessChecker interface {
	CheckReadAccess(ctx context.Context, user *registryv1.User, modules []*registryv1.Module) error
}

// Handler provides download queries using own proto types.
// The buf adapter (bufdownload) wraps this to expose the buf.build wire protocol.
type Handler struct {
	moduleDB moduleQuerier
	commitDB commitQuerier
	blobSvc  blobLister
	authz    readAccessChecker
}

func New(deps *server.Dependencies) *Handler {
	return &Handler{
		moduleDB: deps.ModuleDB,
		commitDB: deps.CommitDB,
		blobSvc:  deps.GitalyBlobStorage,
		authz:    deps.Authorization,
	}
}

// Download resolves module refs to file trees, enforcing read access on private modules.
func (h *Handler) Download(ctx context.Context, refs []*registryv1.ModuleRef) ([]*registryv1.DownloadResponseContent, error) {
	start := time.Now()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	tracer := telemetry.Tracer("hades/download")
	ctx, span := tracer.Start(ctx, "download")
	defer func() {
		telemetry.DownloadLatency.Record(ctx, float64(time.Since(start).Milliseconds()))
		span.End()
	}()

	// user may be nil for anonymous access; CheckReadAccess handles the nil case.
	user, _ := ctx.Value(constants.ContextKeyUser).(*registryv1.User)

	modules, err := h.moduleDB.GetModulesByRefs(ctx, refs...)
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

	var gitalyMemBefore runtime.MemStats
	runtime.ReadMemStats(&gitalyMemBefore)
	_, blobSpan := tracer.Start(ctx, "download.list_blobs")

	commits, err := h.commitDB.GetCommitByOwnerModule(ctx, refs)
	if err != nil {
		blobSpan.RecordError(err)
		blobSpan.SetStatus(codes.Error, "fetch commits")
		blobSpan.End()
		telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
		return nil, err
	}

	var contents []*registryv1.DownloadResponseContent
	for _, commit := range commits {
		blobs, err := h.blobSvc.ListBlobs(ctx, commit)
		if err != nil {
			blobSpan.RecordError(err)
			blobSpan.SetStatus(codes.Error, "list blobs")
			blobSpan.End()
			telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "error")))
			return nil, err
		}
		contents = append(contents, blobs...)
	}
	blobSpan.End()

	var gitalyMemAfter runtime.MemStats
	runtime.ReadMemStats(&gitalyMemAfter)
	gitalyAllocDelta := int64(gitalyMemAfter.TotalAlloc - gitalyMemBefore.TotalAlloc)

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
	telemetry.DownloadGitalyAllocBytes.Record(ctx, gitalyAllocDelta)
	telemetry.DownloadGCRuns.Record(ctx, gcRuns)
	telemetry.DownloadGCPauseMs.Record(ctx, gcPauseMs)

	return contents, nil
}
