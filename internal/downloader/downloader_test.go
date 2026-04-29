package downloader

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDownload_403MapsToConnectionLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" { return }
		w.WriteHeader(403)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "out.bin")
	d := &Downloader{UserAgent: "LimePlayer"}
	err := d.Download(context.Background(), srv.URL, dest, nil)
	if !errors.Is(err, ErrConnectionLimit) {
		t.Errorf("got %v, want ErrConnectionLimit", err)
	}
}

func TestDownload_FullFile(t *testing.T) {
	body := strings.Repeat("A", 10000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "LimePlayer" {
			t.Errorf("UA = %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	d := &Downloader{UserAgent: "LimePlayer"}
	if err := d.Download(context.Background(), srv.URL, dest, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != body { t.Errorf("body mismatch, got %d bytes", len(got)) }
}

func TestDownload_ResumesFromPart(t *testing.T) {
	body := strings.Repeat("B", 1000)
	var rangeHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader = r.Header.Get("Range")
		if r.Method == "HEAD" {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		if rangeHeader != "" {
			w.Header().Set("Content-Range", "bytes 400-999/1000")
			w.WriteHeader(206)
			w.Write([]byte(body[400:]))
			return
		}
		w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := os.WriteFile(dest+".part", []byte(body[:400]), 0o644); err != nil { t.Fatal(err) }
	d := &Downloader{UserAgent: "LimePlayer"}
	if err := d.Download(context.Background(), srv.URL, dest, nil); err != nil { t.Fatal(err) }
	got, _ := os.ReadFile(dest)
	if string(got) != body { t.Errorf("body mismatch, got %d bytes (expected %d)", len(got), len(body)) }
	if !strings.HasPrefix(rangeHeader, "bytes=400-") {
		t.Errorf("range header = %q", rangeHeader)
	}
}

func TestDownload_FallbackWhenServerIgnoresRange(t *testing.T) {
	body := strings.Repeat("C", 500)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.bin")
	os.WriteFile(dest+".part", []byte("xxx"), 0o644)
	d := &Downloader{UserAgent: "LimePlayer"}
	if err := d.Download(context.Background(), srv.URL, dest, nil); err != nil { t.Fatal(err) }
	got, _ := os.ReadFile(dest)
	if string(got) != body {
		t.Errorf("expected fresh full body; got %q", string(got))
	}
}
