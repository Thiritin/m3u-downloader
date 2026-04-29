package tui

import (
	"errors"

	"github.com/Thiritin/m3u-downloader/internal/store"
)

// friendlyEnqueueMsg turns store errors into user-readable text. The most
// common case is ErrAlreadyQueued (caused by the unique partial index that
// blocks re-queueing the same title while a job is pending or active).
func friendlyEnqueueMsg(success string, err error) string {
	if err == nil {
		return success
	}
	if errors.Is(err, store.ErrAlreadyQueued) {
		return "already queued or in progress"
	}
	return "ERR: " + err.Error()
}
