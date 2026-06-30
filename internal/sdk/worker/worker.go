// Package worker polls the sdk_jobs table for pending code-generation jobs
// and processes them concurrently. Each job streams proto files from Gitaly,
// runs protoc, and uploads the output to S3. Stale running jobs (from
// crashed workers) are periodically reset to pending so they can be retried.
package worker

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/sdkjob"
	gitstorage "github.com/alipourhabibi/Hades/internal/hades/storage/git"
	"github.com/alipourhabibi/Hades/internal/sdk/generate"
	"github.com/alipourhabibi/Hades/internal/sdk/storage"
	"github.com/alipourhabibi/Hades/internal/telemetry"
	"github.com/alipourhabibi/Hades/utils/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	// stalenessTimeout is how long a job may remain in 'running' state before
	// it is assumed abandoned (worker crashed) and reset to 'pending'.
	stalenessTimeout = 5 * time.Minute
	// recoveryInterval is how often the worker checks for stale jobs.
	recoveryInterval = 1 * time.Minute
)

// Worker polls for pending sdk_jobs and processes them concurrently.
type Worker struct {
	jobStorage   sdkjob.Storage
	commitDB     commit.Storage
	gitStorage   gitstorage.Storage
	generators   map[string]*generate.Generator // keyed by plugin name
	backend      storage.Backend
	logger       *log.LoggerWrapper
	pollInterval time.Duration
	concurrency  int
}

// New creates a Worker.
func New(
	jobStorage sdkjob.Storage,
	commitDB commit.Storage,
	gitStorage gitstorage.Storage,
	generators map[string]*generate.Generator,
	backend storage.Backend,
	logger *log.LoggerWrapper,
	pollInterval time.Duration,
	concurrency int,
) *Worker {
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}
	if concurrency == 0 {
		concurrency = 4
	}
	return &Worker{
		jobStorage:   jobStorage,
		commitDB:     commitDB,
		gitStorage:   gitStorage,
		generators:   generators,
		backend:      backend,
		logger:       logger,
		pollInterval: pollInterval,
		concurrency:  concurrency,
	}
}

// Run starts the polling loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	sem := make(chan struct{}, w.concurrency)
	pollTicker := time.NewTicker(w.pollInterval)
	recoveryTicker := time.NewTicker(recoveryInterval)
	defer pollTicker.Stop()
	defer recoveryTicker.Stop()

	// Recover any jobs left stuck in 'running' from a previous crash.
	w.recoverStale(ctx)

	w.logger.Info("SDK worker started", "pollInterval", w.pollInterval, "concurrency", w.concurrency)
	for {
		select {
		case <-ctx.Done():
			return
		case <-recoveryTicker.C:
			w.recoverStale(ctx)
		case <-pollTicker.C:
			jobs, err := w.jobStorage.ClaimPending(ctx, w.concurrency)
			if err != nil {
				w.logger.Error("SDK worker: failed to claim jobs", "error", err)
				continue
			}
			for _, job := range jobs {
				sem <- struct{}{}
				go func(j *sdkjob.SDKJob) {
					defer func() { <-sem }()
					w.process(ctx, j)
				}(job)
			}
		}
	}
}

func (w *Worker) recoverStale(ctx context.Context) {
	n, err := w.jobStorage.RecoverStaleJobs(ctx, stalenessTimeout)
	if err != nil {
		w.logger.Error("SDK worker: stale job recovery failed", "error", err)
		return
	}
	if n > 0 {
		w.logger.Info("SDK worker: recovered stale jobs", "count", n)
	}
}

// dirBytes returns the total size in bytes of all regular files under dir.
func dirBytes(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (w *Worker) process(ctx context.Context, job *sdkjob.SDKJob) {
	start := time.Now()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	tracer := telemetry.Tracer("hades/sdk.worker")
	ctx, span := tracer.Start(ctx, "sdk.process_job",
		trace.WithAttributes(
			attribute.String("sdk.job_id", job.ID),
			attribute.String("sdk.language", job.Language),
			attribute.String("sdk.plugin", job.Plugin),
			attribute.Int("sdk.attempt", job.Attempts),
		),
	)

	lg := w.logger.With("jobID", job.ID, "plugin", job.Plugin, "language", job.Language, "attempt", job.Attempts)
	langAttr := metric.WithAttributes(attribute.String("language", job.Language))

	// markFailed logs, records the span error, and persists failure.
	// 'dead' status is set automatically by MarkFailed when Attempts >= MaxAttempts.
	markFailed := func(msg string) {
		lg.Error("SDK worker: " + msg)
		span.RecordError(fmt.Errorf("%s", msg))
		span.SetStatus(codes.Error, msg)
		span.End()
		_ = w.jobStorage.MarkFailed(ctx, job.ID, msg, job.Attempts)
		telemetry.SDKJobsCompleted.Add(ctx, 1, metric.WithAttributes(
			attribute.String("status", "failed"),
			attribute.String("language", job.Language),
		))
		telemetry.SDKJobDuration.Record(ctx, float64(time.Since(start).Milliseconds()), langAttr)
	}

	// 1. Fetch commit metadata (CommitHash + Module.Name needed for BlobService).
	commitRecord, err := w.commitDB.GetCommitById(ctx, job.CommitID)
	if err != nil {
		markFailed(fmt.Sprintf("get commit: %v", err))
		return
	}
	if commitRecord.Module == nil {
		commitRecord.Module = &registryv1.Module{}
	}
	if commitRecord.Module.Name == "" {
		commitRecord.Module.Name = commitRecord.ModuleId
	}

	// 2. Create temp dir for proto sources.
	protoDir, err := os.MkdirTemp("", "hades-gen-*")
	if err != nil {
		markFailed(fmt.Sprintf("mktemp proto: %v", err))
		return
	}
	defer func() { _ = os.RemoveAll(protoDir) }()

	// 3. Stream blobs directly from Gitaly to disk - no in-memory file buffer.
	//    Peak memory ≈ one gRPC frame (~64 KB) rather than the full repo.
	streamStart := time.Now()
	var streamMemBefore runtime.MemStats
	runtime.ReadMemStats(&streamMemBefore)

	_, blobSpan := tracer.Start(ctx, "sdk.stream_blobs")
	if err := w.gitStorage.StreamBlobsToDir(ctx, commitRecord.Module.Name, commitRecord.CommitHash, protoDir); err != nil {
		blobSpan.RecordError(err)
		blobSpan.SetStatus(codes.Error, "stream blobs")
		blobSpan.End()
		markFailed(fmt.Sprintf("stream blobs: %v", err))
		return
	}
	blobSpan.End()

	streamDurationMs := float64(time.Since(streamStart).Milliseconds())
	var streamMemAfter runtime.MemStats
	runtime.ReadMemStats(&streamMemAfter)
	streamAllocDelta := int64(streamMemAfter.TotalAlloc - streamMemBefore.TotalAlloc)

	// Sanity check: at least one .proto file must be present.
	hasProto := false
	_ = filepath.WalkDir(protoDir, func(_ string, d os.DirEntry, _ error) error {
		if !d.IsDir() && filepath.Ext(d.Name()) == ".proto" {
			hasProto = true
			return fmt.Errorf("stop") // sentinel to short-circuit the walk
		}
		return nil
	})
	if !hasProto {
		markFailed("no .proto files in commit")
		return
	}

	// 4. Locate the generator for this job's plugin.
	gen, ok := w.generators[job.Plugin]
	if !ok {
		markFailed(fmt.Sprintf("no generator registered for plugin %q", job.Plugin))
		return
	}

	// 5. Run protoc; outDir is owned here - Generate removes it on error.
	genStart := time.Now()
	var genMemBefore runtime.MemStats
	runtime.ReadMemStats(&genMemBefore)

	_, genSpan := tracer.Start(ctx, "sdk.generate")
	outDir, err := gen.Generate(ctx, protoDir)
	if err != nil {
		genSpan.RecordError(err)
		genSpan.SetStatus(codes.Error, "generate")
		genSpan.End()
		markFailed(fmt.Sprintf("generate: %v", err))
		return
	}
	genSpan.End()
	defer func() { _ = os.RemoveAll(outDir) }()

	genDurationMs := float64(time.Since(genStart).Milliseconds())
	var genMemAfter runtime.MemStats
	runtime.ReadMemStats(&genMemAfter)
	genAllocDelta := int64(genMemAfter.TotalAlloc - genMemBefore.TotalAlloc)

	// 6. Upload from disk to S3, skipping already-present objects (idempotent).
	//    On retry after a partial failure, only missing files are uploaded.
	s3Start := time.Now()
	var s3MemBefore runtime.MemStats
	runtime.ReadMemStats(&s3MemBefore)

	key := fmt.Sprintf("%s/%s/%s", commitRecord.Module.Name, commitRecord.CommitHash, job.Language)
	_, uploadSpan := tracer.Start(ctx, "sdk.upload_s3")
	loc, err := w.backend.Upload(ctx, key, outDir)
	if err != nil {
		uploadSpan.RecordError(err)
		uploadSpan.SetStatus(codes.Error, "upload s3")
		uploadSpan.End()
		markFailed(fmt.Sprintf("upload: %v", err))
		return
	}
	uploadSpan.End()

	s3DurationMs := float64(time.Since(s3Start).Milliseconds())
	var s3MemAfter runtime.MemStats
	runtime.ReadMemStats(&s3MemAfter)
	s3AllocDelta := int64(s3MemAfter.TotalAlloc - s3MemBefore.TotalAlloc)

	_ = w.jobStorage.MarkSucceeded(ctx, job.ID, loc)
	lg.Info("SDK worker: job succeeded", "location", loc)

	span.End()

	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)
	allocDelta := int64(memAfter.TotalAlloc - memBefore.TotalAlloc)
	gcRuns := int64(memAfter.NumGC - memBefore.NumGC)
	gcPauseMs := float64(memAfter.PauseTotalNs-memBefore.PauseTotalNs) / 1e6

	// Record per-job totals.
	telemetry.SDKJobsCompleted.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", "succeeded"),
		attribute.String("language", job.Language),
	))
	telemetry.SDKJobDuration.Record(ctx, float64(time.Since(start).Milliseconds()), langAttr)
	telemetry.SDKProtoBytes.Record(ctx, dirBytes(protoDir), langAttr)
	telemetry.SDKOutputBytes.Record(ctx, dirBytes(outDir), langAttr)
	telemetry.SDKAllocBytes.Record(ctx, allocDelta, langAttr)
	telemetry.SDKGCRuns.Record(ctx, gcRuns, langAttr)
	telemetry.SDKGCPauseMs.Record(ctx, gcPauseMs, langAttr)

	// Record per-phase breakdowns.
	telemetry.SDKStreamDurationMs.Record(ctx, streamDurationMs, langAttr)
	telemetry.SDKStreamAllocBytes.Record(ctx, streamAllocDelta, langAttr)
	telemetry.SDKGenDurationMs.Record(ctx, genDurationMs, langAttr)
	telemetry.SDKGenAllocBytes.Record(ctx, genAllocDelta, langAttr)
	telemetry.SDKS3DurationMs.Record(ctx, s3DurationMs, langAttr)
	telemetry.SDKS3AllocBytes.Record(ctx, s3AllocDelta, langAttr)
}
