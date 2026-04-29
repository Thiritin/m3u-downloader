package catalog

import (
	"context"
	"fmt"

	"github.com/Thiritin/m3u-downloader/internal/plex"
	"github.com/Thiritin/m3u-downloader/internal/store"
	"github.com/Thiritin/m3u-downloader/internal/xtream"
)

// EnqueueConfig carries the output roots needed to compute Plex destination
// paths for queued movies and episodes.
type EnqueueConfig struct {
	MoviesDir string
	SeriesDir string
}

// EnsureSeasonsCached returns the cached seasons for a series, fetching them
// from the Xtream API and persisting if the local cache is empty. This is the
// shared fetch path used both when the user drills into a show in the TUI and
// when they queue a series whose seasons haven't been loaded yet.
func EnsureSeasonsCached(ctx context.Context, st *store.Store, xc *xtream.Client, seriesID int) ([]store.SeasonRow, error) {
	seasons, err := st.ListSeasons(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	if len(seasons) > 0 {
		return seasons, nil
	}
	info, err := xc.GetSeriesInfo(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	if err := persistSeriesInfo(ctx, st, seriesID, info); err != nil {
		return nil, err
	}
	return st.ListSeasons(ctx, seriesID)
}

func persistSeriesInfo(ctx context.Context, st *store.Store, seriesID int, info *xtream.SeriesInfo) error {
	seasonRows := make([]store.SeasonRow, 0, len(info.Seasons))
	for _, s := range info.Seasons {
		seasonRows = append(seasonRows, store.SeasonRow{
			SeriesID: seriesID, SeasonNumber: s.SeasonNumber,
			Name: s.Name, Overview: s.Overview, CoverURL: s.Cover,
		})
	}
	var epRows []store.EpisodeRow
	for seasonStr, eps := range info.Episodes {
		sn := 0
		fmt.Sscanf(seasonStr, "%d", &sn)
		for _, e := range eps {
			id := 0
			fmt.Sscanf(e.ID, "%d", &id)
			dur := 0
			fmt.Sscanf(e.Info.Duration, "%d", &dur)
			epRows = append(epRows, store.EpisodeRow{
				EpisodeID: id, SeriesID: seriesID, SeasonNumber: sn,
				EpisodeNum: e.EpisodeNum, Title: e.Title, Plot: e.Info.Plot,
				ContainerExt: e.ContainerExtension, DurationSecs: dur,
			})
		}
	}
	return st.ReplaceSeasonsAndEpisodes(ctx, seriesID, seasonRows, epRows)
}

func EnqueueVOD(ctx context.Context, st *store.Store, cfg EnqueueConfig, v store.VODRow) error {
	dest := plex.MoviePath(cfg.MoviesDir, v.Name, v.Year)
	_, err := st.EnqueueJob(ctx, "vod", v.StreamID, dest)
	return err
}

func EnqueueEpisode(ctx context.Context, st *store.Store, cfg EnqueueConfig, show store.SeriesRow, e store.EpisodeRow) error {
	dest := plex.EpisodePath(cfg.SeriesDir, show.Name, e.SeasonNumber, e.EpisodeNum)
	_, err := st.EnqueueJob(ctx, "episode", e.EpisodeID, dest)
	return err
}

// EnqueueSeason queues every episode in the given series + season.
func EnqueueSeason(ctx context.Context, st *store.Store, cfg EnqueueConfig, show store.SeriesRow, season int) (int, error) {
	eps, err := st.ListEpisodes(ctx, show.SeriesID, season)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, e := range eps {
		if err := EnqueueEpisode(ctx, st, cfg, show, e); err == nil {
			count++
		}
	}
	return count, nil
}

// EnqueueSeries queues every episode of every season of show, fetching season
// info from the provider on demand if not already cached. It also adds the
// show to series_subscriptions so future catalog refreshes auto-enqueue any
// newly-released episodes.
func EnqueueSeries(ctx context.Context, st *store.Store, xc *xtream.Client, cfg EnqueueConfig, show store.SeriesRow) (int, error) {
	seasons, err := EnsureSeasonsCached(ctx, st, xc, show.SeriesID)
	if err != nil {
		return 0, err
	}
	if len(seasons) == 0 {
		return 0, fmt.Errorf("series %q has no seasons", show.Name)
	}
	total := 0
	for _, s := range seasons {
		n, _ := EnqueueSeason(ctx, st, cfg, show, s.SeasonNumber)
		total += n
	}
	if err := st.AddSubscription(ctx, show.SeriesID); err != nil {
		return total, fmt.Errorf("subscribe %q: %w", show.Name, err)
	}
	return total, nil
}
