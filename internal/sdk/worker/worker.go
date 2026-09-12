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
	"sync"
	"time"

	registryv1 "github.com/alipourhabibi/Hades/api/gen/api/registry/v1"
	"github.com/alipourhabibi/Hades/internal/hades/storage/db/commit"
	notificationdb "github.com/alipourhabibi/Hades/internal/hades/storage/db/notification"
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
	notifyDB     notificationdb.Storage
	logger       *log.LoggerWrapper
	pollInterval time.Duration
	concurrency  int
}

// WithNotifications attaches the notification store so job outcomes reach the
// person who pushed the commit. Optional: without it the worker runs exactly as
// before and simply raises nothing.
func (w *Worker) WithNotifications(n notificationdb.Storage) *Worker {
	w.notifyDB = n
	return w
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

// Run starts the polling loop. It blocks until ctx is cancelled and every
// in-flight job has finished.
func (w *Worker) Run(ctx context.Context) {
	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup
	pollTicker := time.NewTicker(w.pollInterval)
	recoveryTicker := time.NewTicker(recoveryInterval)
	defer pollTicker.Stop()
	defer recoveryTicker.Stop()

	// Wait for in-flight generation to finish rather than returning the moment
	// the context is cancelled. Returning immediately killed jobs mid-flight
	// and left their rows in 'running' until stale recovery reclaimed them
	// minutes later.
	defer wg.Wait()

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
			// Capacity is reserved BEFORE anything is claimed, and exactly as
			// many jobs are claimed as there are free slots. Claiming first and
			// then blocking on the semaphore marked jobs 'running' that this
			// worker might never start: if the context was cancelled while
			// blocked, they stayed 'running' until stale recovery reclaimed
			// them.
			slots := acquireAvailable(sem, w.concurrency)
			if slots == 0 {
				continue
			}

			jobs, err := w.jobStorage.ClaimPending(ctx, slots)
			if err != nil {
				w.logger.Error("SDK worker: failed to claim jobs", "error", err)
				for i := 0; i < slots; i++ {
					<-sem
				}
				continue
			}
			// Give back the slots no job was claimed for.
			for i := len(jobs); i < slots; i++ {
				<-sem
			}

			for _, job := range jobs {
				wg.Add(1)
				go func(j *sdkjob.SDKJob) {
					defer wg.Done()
					defer func() { <-sem }()
					w.process(ctx, j)
				}(job)
			}
		}
	}
}

// acquireAvailable takes up to max slots from sem without blocking and returns
// how many it got.
func acquireAvailable(sem chan struct{}, max int) int {
	n := 0
	for n < max {
		select {
		case sem <- struct{}{}:
			n++
		default:
			return n
		}
	}
	return n
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
	// A walk error is ignored on purpose: this is a metric, and failing a
	// finished job because a size could not be measured would be worse than an
	// under-reported number.
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil //nolint:nilerr // see above: measurement is best effort
		}
		if info, infoErr := d.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (w *Worker) process(ctx context.Context, job *sdkjob.SDKJob) {
	start := time.Now()

	// Terminal status writes use a context detached from the worker's, bounded
	// separately. When the worker is shutting down, ctx is already cancelled and
	// MarkFailed/MarkSucceeded would fail with it, leaving the row stuck in
	// 'running' until stale recovery reclaims it minutes later.
	statusCtx, cancelStatus := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancelStatus()

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

	// commitRecord is read by markFailed to address the notification. It is set
	// once step 1 succeeds; a failure before that has nobody to notify, since
	// the job row alone does not say who pushed.
	var commitRecord *registryv1.Commit

	// markFailed logs, records the span error, and persists failure.
	// 'dead' status is set automatically by MarkFailed when Attempts >= MaxAttempts.
	markFailed := func(msg string) {
		lg.Error("SDK worker: " + msg)
		span.RecordError(fmt.Errorf("%s", msg))
		span.SetStatus(codes.Error, msg)
		span.End()
		_ = w.jobStorage.MarkFailed(statusCtx, job.ID, msg, job.Attempts)
		w.notifyJobOutcome(statusCtx, job, commitRecord, msg)
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

	_, genSpan := tracer.Start(ctx, "sdk.generate")
	// job.PluginOptions overrides the statically configured options. The column
	// was written, selected and scanned but never read, so a job that recorded
	// its own options silently used the server-wide ones instead.
	outDir, err := gen.Generate(ctx, protoDir, job.PluginOptions)
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

	// 6. Upload from disk to S3, skipping already-present objects (idempotent).
	//    On retry after a partial failure, only missing files are uploaded.
	s3Start := time.Now()

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

	_ = w.jobStorage.MarkSucceeded(statusCtx, job.ID, loc)
	w.notifyJobOutcome(statusCtx, job, commitRecord, "")
	lg.Info("SDK worker: job succeeded", "location", loc)

	span.End()

	// Record per-job totals.
	telemetry.SDKJobsCompleted.Add(ctx, 1, metric.WithAttributes(
		attribute.String("status", "succeeded"),
		attribute.String("language", job.Language),
	))
	telemetry.SDKJobDuration.Record(ctx, float64(time.Since(start).Milliseconds()), langAttr)
	telemetry.SDKProtoBytes.Record(ctx, dirBytes(protoDir), langAttr)
	telemetry.SDKOutputBytes.Record(ctx, dirBytes(outDir), langAttr)
	// Process-wide allocation and GC counters are not sampled per job:
	// runtime.ReadMemStats stops the world, and with several jobs running
	// concurrently its deltas attribute every goroutine's allocations to
	// whichever job happened to bracket them.

	// Record per-phase breakdowns.
	telemetry.SDKStreamDurationMs.Record(ctx, streamDurationMs, langAttr)
	telemetry.SDKGenDurationMs.Record(ctx, genDurationMs, langAttr)
	telemetry.SDKS3DurationMs.Record(ctx, s3DurationMs, langAttr)
}

// notifyJobOutcome tells the author of the commit how their SDK job ended.
//
// failure is the error message, or empty for success. Best effort: a
// notification write must not change the job's recorded outcome, which has
// already been persisted by the time this runs.
//
// A retryable failure notifies on every attempt. That is deliberate: each
// attempt is a real event, and suppressing all but the last would mean saying
// nothing at all for a job that never reaches its retry limit.
func (w *Worker) notifyJobOutcome(ctx context.Context, job *sdkjob.SDKJob, commitRecord *registryv1.Commit, failure string) {
	if w.notifyDB == nil || commitRecord == nil || commitRecord.CreatedByUserId == "" {
		return
	}

	moduleName := commitRecord.ModuleId
	if commitRecord.Module != nil && commitRecord.Module.Name != "" {
		moduleName = commitRecord.Module.Name
	}

	notificationType := notificationdb.TypeSDKSucceeded
	title := fmt.Sprintf("%s SDK ready for %s", job.Language, moduleName)
	body := fmt.Sprintf("The %s SDK for commit %s is available.", job.Language, shortHash(commitRecord.CommitHash))
	if failure != "" {
		notificationType = notificationdb.TypeSDKFailed
		title = fmt.Sprintf("%s SDK generation failed for %s", job.Language, moduleName)
		body = fmt.Sprintf("Generating the %s SDK for commit %s failed: %s", job.Language, shortHash(commitRecord.CommitHash), failure)
	}

	if err := w.notifyDB.Create(ctx, commitRecord.CreatedByUserId, notificationType, title, body, job.ID); err != nil {
		w.logger.Error("SDK worker: failed to create notification", "error", err, "jobID", job.ID)
	}
}

// shortHash abbreviates a git commit hash for display.
func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
