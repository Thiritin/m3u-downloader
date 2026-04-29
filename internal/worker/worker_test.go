package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Thiritin/m3u-downloader/internal/store"
)

func TestProcessOne_VOD_HappyPath(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	tsBytes, err := os.ReadFile("../../testdata/tiny.ts")
	if err != nil { t.Fatal(err) }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(len(tsBytes)))
			return
		}
		w.Write(tsBytes)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "s.db")
	st, err := store.Open(dbPath)
	if err != nil { t.Fatal(err) }
	defer st.Close()

	moviesDir := filepath.Join(tmp, "Movies")
	if err := os.MkdirAll(moviesDir, 0o755); err != nil { t.Fatal(err) }
	dest := filepath.Join(moviesDir, "Test (2024)", "Test (2024).mkv")
	id, err := st.EnqueueJob(context.Background(), "vod", 1, dest)
	if err != nil { t.Fatal(err) }

	w := &Worker{
		Store:          st,
		UserAgent:      "LimePlayer",
		MoviesDir:      moviesDir,
		Remux:          true,
		MaxRetries:     3,
		BackoffSeconds: []int{1, 1, 1},
		ResolveURL: func(j store.JobRow) (string, error) {
			return srv.URL + "/movie", nil
		},
	}
	if err := w.processOne(context.Background()); err != nil {
		t.Fatalf("processOne: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("expected %s to exist: %v", dest, err)
	}
	jobs, _ := st.ListJobs(context.Background(), "completed")
	if len(jobs) != 1 || jobs[0].ID != id {
		t.Errorf("unexpected completed jobs: %+v", jobs)
	}
}

func TestProcessOne_DestUnmounted_FailsFast(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "s.db")
	st, err := store.Open(dbPath)
	if err != nil { t.Fatal(err) }
	defer st.Close()

	// MoviesDir points at a directory that does NOT exist — simulates unmounted volume
	moviesDir := filepath.Join(tmp, "definitely-not-mounted", "Movies")
	dest := filepath.Join(moviesDir, "Foo (2024)", "Foo (2024).mkv")
	if _, err := st.EnqueueJob(context.Background(), "vod", 1, dest); err != nil { t.Fatal(err) }

	w := &Worker{
		Store:          st,
		UserAgent:      "LimePlayer",
		MoviesDir:      moviesDir,
		MaxRetries:     3,
		BackoffSeconds: []int{1, 1, 1},
		ResolveURL:     func(j store.JobRow) (string, error) { return "http://unused", nil },
	}
	if err := w.processOne(context.Background()); err != nil {
		t.Fatalf("processOne: %v", err)
	}
	jobs, _ := st.ListJobs(context.Background(), "failed")
	if len(jobs) != 1 {
		t.Fatalf("expected 1 failed job, got %d", len(jobs))
	}
	if jobs[0].Attempts != 1 {
		t.Errorf("attempts = %d, want 1 (fail-fast, no retries)", jobs[0].Attempts)
	}
}
