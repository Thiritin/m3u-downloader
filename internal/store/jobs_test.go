package store

import (
	"context"
	"errors"
	"testing"
)

func TestEnqueueAndClaim(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	id, err := s.EnqueueJob(ctx, "vod", 100, "/m/Foo (2020)/Foo (2020).mkv")
	if err != nil { t.Fatal(err) }
	if id <= 0 { t.Errorf("id = %d", id) }

	got, err := s.ClaimNext(ctx)
	if err != nil { t.Fatal(err) }
	if got == nil { t.Fatal("ClaimNext returned nil on a non-empty queue") }
	if got.Status != "active" { t.Errorf("status = %q, want active", got.Status) }

	again, err := s.ClaimNext(ctx)
	if err != nil { t.Fatal(err) }
	if again != nil { t.Errorf("expected nil on empty pending: %+v", again) }
}

func TestEnqueueDuplicateBlocked(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.EnqueueJob(ctx, "vod", 100, "/m/x.mkv"); err != nil { t.Fatal(err) }
	_, err := s.EnqueueJob(ctx, "vod", 100, "/m/x.mkv")
	if err == nil {
		t.Fatal("expected duplicate enqueue to fail (unique active index)")
	}
	if !errors.Is(err, ErrAlreadyQueued) {
		t.Errorf("expected ErrAlreadyQueued, got %v", err)
	}
}

func TestCancelAndDeleteJob(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Cancel an active job
	id, _ := s.EnqueueJob(ctx, "vod", 100, "/p")
	if _, err := s.ClaimNext(ctx); err != nil { t.Fatal(err) }
	if err := s.CancelJob(ctx, id); err != nil { t.Fatal(err) }
	got, err := s.GetJobStatus(ctx, id)
	if err != nil { t.Fatal(err) }
	if got != "cancelled" { t.Errorf("status = %q, want cancelled", got) }

	// Delete the cancelled row
	if err := s.DeleteJob(ctx, id); err != nil { t.Fatal(err) }
	got, _ = s.GetJobStatus(ctx, id)
	if got != "" { t.Errorf("after delete, status = %q, want empty", got) }

	// After delete the same source can be re-enqueued
	if _, err := s.EnqueueJob(ctx, "vod", 100, "/p2"); err != nil {
		t.Errorf("re-enqueue after delete: %v", err)
	}
}

func TestJobStatusBySource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id1, _ := s.EnqueueJob(ctx, "vod", 100, "/p1")
	_, _ = s.EnqueueJob(ctx, "episode", 200, "/p2")
	if _, err := s.ClaimNext(ctx); err != nil { t.Fatal(err) }
	statuses, err := s.JobStatusBySource(ctx)
	if err != nil { t.Fatal(err) }
	if got := statuses[JobStatusKey("vod", 100)]; got != "active" {
		t.Errorf("vod:100 = %q, want active", got)
	}
	if got := statuses[JobStatusKey("episode", 200)]; got != "pending" {
		t.Errorf("episode:200 = %q, want pending", got)
	}
	// Completing a job should reflect in the map.
	if err := s.CompleteJob(ctx, id1); err != nil { t.Fatal(err) }
	statuses, _ = s.JobStatusBySource(ctx)
	if got := statuses[JobStatusKey("vod", 100)]; got != "completed" {
		t.Errorf("after complete, vod:100 = %q, want completed", got)
	}
}

func TestRequeueJobNoAttemptDoesNotIncrement(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.EnqueueJob(ctx, "vod", 100, "/p")
	if _, err := s.ClaimNext(ctx); err != nil { t.Fatal(err) }
	if err := s.RequeueJobNoAttempt(ctx, id, "connection limit"); err != nil { t.Fatal(err) }
	var attempts int
	if err := s.DB().QueryRow(`SELECT attempts FROM jobs WHERE id=?`, id).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Errorf("attempts = %d, want 0 (no-attempt requeue should not increment)", attempts)
	}
}

func TestRequeueClearsStartedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.EnqueueJob(ctx, "vod", 100, "/p")
	if _, err := s.ClaimNext(ctx); err != nil { t.Fatal(err) }
	if err := s.RequeueJob(ctx, id, "transient"); err != nil { t.Fatal(err) }
	var status string
	var startedAt *int64
	err := s.DB().QueryRow(`SELECT status, started_at FROM jobs WHERE id=?`, id).Scan(&status, &startedAt)
	if err != nil { t.Fatal(err) }
	if status != "pending" { t.Errorf("status = %q", status) }
	if startedAt != nil { t.Errorf("started_at not cleared: %v", startedAt) }
}

func TestFailJobAfterMaxAttempts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.EnqueueJob(ctx, "vod", 100, "/p")
	if _, err := s.ClaimNext(ctx); err != nil { t.Fatal(err) }
	if err := s.FailJob(ctx, id, "boom"); err != nil { t.Fatal(err) }
	var status string
	if err := s.DB().QueryRow(`SELECT status FROM jobs WHERE id=?`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" { t.Errorf("status = %q, want failed", status) }
	if _, err := s.EnqueueJob(ctx, "vod", 100, "/p"); err != nil {
		t.Errorf("re-enqueue after failure: %v", err)
	}
}
