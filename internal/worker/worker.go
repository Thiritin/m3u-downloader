// Package worker drives a single download job from claim through finalize.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Thiritin/m3u-downloader/internal/downloader"
	"github.com/Thiritin/m3u-downloader/internal/remux"
	"github.com/Thiritin/m3u-downloader/internal/store"
)

// SidecarFn writes poster/fanart files for the given job into its destination folder.
type SidecarFn func(ctx context.Context, j store.JobRow) error

// ResolveFn turns a job into a fully-qualified provider URL.
type ResolveFn func(j store.JobRow) (string, error)

type Worker struct {
	Store          *store.Store
	UserAgent      string
	MoviesDir      string // configured root for VOD output; must exist (mount check)
	SeriesDir      string // configured root for series output; must exist (mount check)
	ResolveURL     ResolveFn
	Sidecars       SidecarFn
	BackoffSeconds []int
	MaxRetries     int
	Remux          bool
	Logger         *slog.Logger
}

func (w *Worker) log() *slog.Logger {
	if w.Logger != nil { return w.Logger }
	return slog.Default()
}

// errPermanent wraps errors that must NOT be retried (disk full, dest unmounted).
type errPermanent struct{ err error }

func (e errPermanent) Error() string { return e.err.Error() }
func (e errPermanent) Unwrap() error { return e.err }

func isPermanent(err error) bool {
	var p errPermanent
	return errors.As(err, &p)
}

// Run polls the queue and processes one job at a time. Returns when ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	if w.MaxRetries == 0 {
		w.MaxRetries = 3
	}
	if len(w.BackoffSeconds) == 0 {
		w.BackoffSeconds = []int{5, 30, 120}
	}
	if w.Remux {
		if err := remux.EnsureFFmpeg(); err != nil {
			return err
		}
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			if err := w.processOne(ctx); err != nil {
				w.log().Error("processOne", "err", err)
			}
		}
	}
}

func (w *Worker) processOne(ctx context.Context) error {
	job, err := w.Store.ClaimNext(ctx)
	if err != nil {
		return fmt.Errorf("claim: %w", err)
	}
	if job == nil {
		return nil
	}
	w.log().Info("processing", "id", job.ID, "kind", job.Kind, "src", job.SourceID)

	runErr := w.runJob(ctx, *job)

	// If the user cancelled or deleted the job mid-flight, the runJob ctx
	// was cancelled by the per-job poller. Don't transition state — the
	// row is already at status='cancelled' (or gone). Just clean up.
	if status, _ := w.Store.GetJobStatus(ctx, job.ID); status != "active" {
		w.log().Info("job cancelled or removed by user", "id", job.ID, "status", status)
		_ = os.Remove(job.DestPath + ".raw")
		_ = os.Remove(job.DestPath + ".raw.part")
		return nil
	}

	if runErr == nil {
		return w.Store.CompleteJob(ctx, job.ID)
	}
	w.log().Warn("job failed", "id", job.ID, "err", runErr)

	// Provider connection-limit hit: wait and requeue WITHOUT consuming an attempt.
	if errors.Is(runErr, downloader.ErrConnectionLimit) {
		w.log().Info("connection limit hit; sleeping 30s before requeue", "id", job.ID)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(30 * time.Second):
		}
		return w.Store.RequeueJobNoAttempt(ctx, job.ID, runErr.Error())
	}

	// Permanent failures (dest unmounted, disk full): fail immediately, no retry.
	if isPermanent(runErr) {
		return w.Store.FailJob(ctx, job.ID, runErr.Error())
	}

	// Transient: retry with backoff if attempts remain.
	if job.Attempts < w.MaxRetries {
		backoffIdx := job.Attempts
		if backoffIdx >= len(w.BackoffSeconds) {
			backoffIdx = len(w.BackoffSeconds) - 1
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(w.BackoffSeconds[backoffIdx]) * time.Second):
		}
		return w.Store.RequeueJob(ctx, job.ID, runErr.Error())
	}
	return w.Store.FailJob(ctx, job.ID, runErr.Error())
}

func (w *Worker) runJob(parent context.Context, job store.JobRow) error {
	if job.DestPath == "" {
		return errPermanent{errors.New("job has no dest_path")}
	}

	// Per-job context so the user can cancel an in-flight download from the
	// queue view. A goroutine polls the DB once per second; if the row's
	// status changes away from 'active' (cancelled or deleted), we cancel.
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, err := w.Store.GetJobStatus(parent, job.ID)
				if err == nil && status != "active" {
					cancel()
					return
				}
			}
		}
	}()
	// Verify the configured library root exists. When a network/external volume
	// is unmounted, the mount-point dir often remains as an empty placeholder,
	// so we must check the EXACT configured root, not a parent of dest_path.
	root := w.MoviesDir
	if job.Kind == "episode" {
		root = w.SeriesDir
	}
	if root != "" {
		if _, err := os.Stat(root); err != nil {
			return errPermanent{fmt.Errorf("destination root missing (disk unmounted?): %s: %w", root, err)}
		}
	}
	if err := os.MkdirAll(filepath.Dir(job.DestPath), 0o755); err != nil {
		return errPermanent{fmt.Errorf("mkdir dest: %w", err)}
	}

	url, err := w.ResolveURL(job)
	if err != nil {
		return fmt.Errorf("resolve url: %w", err)
	}

	tmp := job.DestPath + ".raw"
	d := &downloader.Downloader{UserAgent: w.UserAgent}
	progress := func(got, total int64) {
		_ = w.Store.UpdateProgress(ctx, job.ID, got, total)
	}
	if err := d.Download(ctx, url, tmp, progress); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	// Some providers list titles they don't actually have and respond
	// 200 OK with an empty body. Catch that before passing garbage to ffmpeg.
	if fi, err := os.Stat(tmp); err == nil && fi.Size() == 0 {
		_ = os.Remove(tmp)
		return errPermanent{fmt.Errorf("provider returned empty file (title not actually available)")}
	}

	if w.Remux {
		if err := remux.ToMKV(ctx, tmp, job.DestPath); err != nil {
			return err
		}
		_ = os.Remove(tmp)
	} else {
		if err := os.Rename(tmp, job.DestPath); err != nil {
			return err
		}
	}

	if w.Sidecars != nil {
		if err := w.Sidecars(ctx, job); err != nil {
			w.log().Warn("sidecars failed (non-fatal)", "id", job.ID, "err", err)
		}
	}
	return nil
}
