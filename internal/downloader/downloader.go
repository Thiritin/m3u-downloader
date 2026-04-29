// Package downloader streams an HTTP body to disk with optional Range-based resume.
package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// ErrConnectionLimit is returned when the provider rejects the request because
// the per-account simultaneous-stream cap has been reached. Worker treats
// this specially: wait, retry, do not count toward attempts.
var ErrConnectionLimit = errors.New("provider connection limit reached")

// ProgressFunc is called periodically (~1Hz) with bytes-downloaded-so-far and total (0 if unknown).
type ProgressFunc func(downloaded, total int64)

type Downloader struct {
	UserAgent string
	Client    *http.Client
}

func (d *Downloader) client() *http.Client {
	if d.Client != nil { return d.Client }
	return &http.Client{Timeout: 0}
}

// Download fetches url to {dest}.part, optionally resuming, then renames to dest.
func (d *Downloader) Download(ctx context.Context, url, dest string, progress ProgressFunc) error {
	part := dest + ".part"

	rangesOK := false
	total := int64(0)
	if headReq, headErr := http.NewRequestWithContext(ctx, "HEAD", url, nil); headErr == nil {
		headReq.Header.Set("User-Agent", d.UserAgent)
		if headResp, err := d.client().Do(headReq); err == nil {
			if headResp.StatusCode/100 == 2 {
				rangesOK = headResp.Header.Get("Accept-Ranges") == "bytes"
				if cl := headResp.Header.Get("Content-Length"); cl != "" {
					total, _ = strconv.ParseInt(cl, 10, 64)
				}
			}
			headResp.Body.Close()
		}
	}

	var offset int64
	if fi, err := os.Stat(part); err == nil && rangesOK {
		offset = fi.Size()
		if total > 0 && offset >= total {
			return os.Rename(part, dest)
		}
	} else {
		os.Remove(part)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil { return err }
	req.Header.Set("User-Agent", d.UserAgent)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	resp, err := d.client().Do(req)
	if err != nil { return fmt.Errorf("GET: %w", err) }
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		// Server ignored Range — start over with truncated file.
		offset = 0
		if err := os.Truncate(part, 0); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if total == 0 {
			if cl := resp.Header.Get("Content-Length"); cl != "" {
				total, _ = strconv.ParseInt(cl, 10, 64)
			}
		}
	case 206:
	case 403:
		return ErrConnectionLimit
	default:
		return fmt.Errorf("download: unexpected status %d", resp.StatusCode)
	}

	flag := os.O_CREATE | os.O_WRONLY
	if offset > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(part, flag, 0o644)
	if err != nil { return err }
	defer f.Close()

	written := offset
	last := time.Now()
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil { return err }
			written += int64(n)
			if progress != nil && time.Since(last) > time.Second {
				progress(written, total)
				last = time.Now()
			}
		}
		if rerr == io.EOF { break }
		if rerr != nil { return rerr }
	}
	if progress != nil { progress(written, total) }
	if err := f.Close(); err != nil { return err }
	return os.Rename(part, dest)
}
