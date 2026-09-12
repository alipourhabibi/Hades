package content

import (
	"context"
	"time"

	identityv1 "github.com/alipourhabibi/Hades/api/gen/api/identity/v1"
	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/constants"
	"github.com/alipourhabibi/Hades/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

// injectBufYAML replaces or appends a buf.yaml entry in files with content
// generated from the module's current DB settings, so downloads always reflect
// the latest registry configuration rather than whatever was stored in git.
func injectBufYAML(files []*registryv1.File, m *registryv1.Module, registryHost string) []*registryv1.File {
	generated := generateBufYAML(m, registryHost)
	out := make([]*registryv1.File, 0, len(files)+1)
	found := false
	for _, f := range files {
		if f.Path == "buf.yaml" {
			out = append(out, &registryv1.File{Path: "buf.yaml", Content: generated})
			found = true
		} else {
			out = append(out, f)
		}
	}
	if !found {
		out = append(out, &registryv1.File{Path: "buf.yaml", Content: generated})
	}
	return out
}

func (h *Handler) Download(ctx context.Context, commitIDs []string, moduleRefs []*registryv1.ModuleRef) ([]*registryv1.DownloadResponseContent, error) {
	start := time.Now()

	tracer := telemetry.Tracer("hades/download")
	ctx, span := tracer.Start(ctx, "download")
	defer func() {
		telemetry.DownloadLatency.Record(ctx, float64(time.Since(start).Milliseconds()))
		span.End()
	}()

	user, _ := ctx.Value(constants.ContextKeyUser).(*identityv1.User)

	var contents []*registryv1.DownloadResponseContent

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
		if len(modules) > 0 {
			pbFiles = injectBufYAML(pbFiles, modules[0], h.registryHost)
		}
		contents = append(contents, &registryv1.DownloadResponseContent{
			Commit: commit,
			Files:  pbFiles,
		})
	}

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
		// Build a lookup so we can find the module for each commit.
		moduleByID := make(map[string]*registryv1.Module, len(modules))
		for _, m := range modules {
			moduleByID[m.Id] = m
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
			if m, ok := moduleByID[commit.ModuleId]; ok {
				pbFiles = injectBufYAML(pbFiles, m, h.registryHost)
			}
			contents = append(contents, &registryv1.DownloadResponseContent{
				Commit: commit,
				Files:  pbFiles,
			})
		}
	}

	var totalProtoBytes int64
	for _, c := range contents {
		for _, f := range c.Files {
			totalProtoBytes += int64(len(f.Content))
		}
	}

	// Process-wide allocation and GC counters are not sampled per request:
	// runtime.ReadMemStats stops the world, and its deltas are process-global,
	// so under concurrency they attribute unrelated work to this download.
	telemetry.DownloadRequests.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "ok")))
	telemetry.DownloadProtoBytes.Record(ctx, totalProtoBytes)

	return contents, nil
}
