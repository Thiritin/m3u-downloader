package remux

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnsureFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	if err := EnsureFFmpeg(); err != nil {
		t.Errorf("EnsureFFmpeg: %v", err)
	}
}

func TestRemuxToMKV(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	src := "../../testdata/tiny.ts"
	dst := filepath.Join(t.TempDir(), "out.mkv")
	if err := ToMKV(context.Background(), src, dst); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() == 0 {
		t.Error("output is empty")
	}
}
