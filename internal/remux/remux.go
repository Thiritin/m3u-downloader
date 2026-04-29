// Package remux wraps ffmpeg to remux input streams into MKV without re-encoding.
package remux

import (
	"context"
	"fmt"
	"os/exec"
)

// EnsureFFmpeg returns an error if ffmpeg is not available on PATH.
func EnsureFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found on PATH; install with `brew install ffmpeg`")
	}
	return nil
}

// ToMKV runs `ffmpeg -i src -c copy -map 0 dst` and returns ffmpeg's stderr if it fails.
func ToMKV(ctx context.Context, src, dst string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-i", src,
		"-c", "copy",
		"-map", "0",
		"-f", "matroska",
		dst,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg remux: %w\n%s", err, string(out))
	}
	return nil
}
