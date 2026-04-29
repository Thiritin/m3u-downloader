package tui

import (
	"context"
	"errors"
	"fmt"

	"github.com/Thiritin/m3u-downloader/internal/plex"
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

// EnqueueConfig carries the output roots needed to compute Plex paths.
type EnqueueConfig struct {
	MoviesDir string
	SeriesDir string
}

func enqueueVOD(ctx context.Context, st *store.Store, cfg EnqueueConfig, v store.VODRow) error {
	dest := plex.MoviePath(cfg.MoviesDir, v.Name, v.Year)
	_, err := st.EnqueueJob(ctx, "vod", v.StreamID, dest)
	return err
}

func enqueueEpisode(ctx context.Context, st *store.Store, cfg EnqueueConfig, show store.SeriesRow, e store.EpisodeRow) error {
	dest := plex.EpisodePath(cfg.SeriesDir, show.Name, e.SeasonNumber, e.EpisodeNum)
	_, err := st.EnqueueJob(ctx, "episode", e.EpisodeID, dest)
	return err
}

// enqueueSeason queues every episode in the given series + season.
func enqueueSeason(ctx context.Context, st *store.Store, cfg EnqueueConfig, show store.SeriesRow, season int) (int, error) {
	eps, err := st.ListEpisodes(ctx, show.SeriesID, season)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range eps {
		if err := enqueueEpisode(ctx, st, cfg, show, e); err == nil {
			count++
		}
	}
	return count, nil
}

// enqueueSeries queues every episode of every season of show. Requires that
// season info has already been fetched (open the show in TUI first).
func enqueueSeries(ctx context.Context, st *store.Store, cfg EnqueueConfig, show store.SeriesRow) (int, error) {
	seasons, err := st.ListSeasons(ctx, show.SeriesID)
	if err != nil {
		return 0, err
	}
	if len(seasons) == 0 {
		return 0, fmt.Errorf("series %q has no cached seasons; open it first to fetch", show.Name)
	}
	total := 0
	for _, s := range seasons {
		n, _ := enqueueSeason(ctx, st, cfg, show, s.SeasonNumber)
		total += n
	}
	return total, nil
}
