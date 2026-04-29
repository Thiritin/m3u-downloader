package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/Thiritin/m3u-downloader/internal/store"
	"github.com/Thiritin/m3u-downloader/internal/xtream"
)

// RefreshSubscriptions walks every subscribed series, refetches its full
// get_series_info from the provider, persists any newly-discovered seasons
// or episodes, and enqueues download jobs for the new episodes.
//
// One provider failure does not abort the whole loop — the error is returned
// but processing continues for the remaining subscriptions.
func RefreshSubscriptions(ctx context.Context, st *store.Store, xc *xtream.Client, cfg EnqueueConfig, progress Progress) (newEpisodes int, err error) {
	if progress == nil {
		progress = func(string, int) {}
	}
	subs, err := st.ListSubscriptions(ctx)
	if err != nil {
		return 0, fmt.Errorf("list subscriptions: %w", err)
	}
	if len(subs) == 0 {
		return 0, nil
	}
	progress("refreshing subscriptions", len(subs))

	var errs []error
	for _, seriesID := range subs {
		n, err := refreshOne(ctx, st, xc, cfg, seriesID)
		if err != nil {
			errs = append(errs, fmt.Errorf("series %d: %w", seriesID, err))
			continue
		}
		newEpisodes += n
		_ = st.MarkSubscriptionChecked(ctx, seriesID)
	}
	if len(errs) > 0 {
		return newEpisodes, errors.Join(errs...)
	}
	return newEpisodes, nil
}

func refreshOne(ctx context.Context, st *store.Store, xc *xtream.Client, cfg EnqueueConfig, seriesID int) (int, error) {
	show, err := st.GetSeries(ctx, seriesID)
	if err != nil {
		return 0, fmt.Errorf("load series row: %w", err)
	}
	if show == nil {
		// Series was removed from the catalog — drop the dead subscription.
		_ = st.RemoveSubscription(ctx, seriesID)
		return 0, nil
	}

	known, err := st.EpisodeIDsForSeries(ctx, seriesID)
	if err != nil {
		return 0, err
	}

	info, err := xc.GetSeriesInfo(ctx, seriesID)
	if err != nil {
		return 0, fmt.Errorf("get_series_info: %w", err)
	}
	if err := persistSeriesInfo(ctx, st, seriesID, info); err != nil {
		return 0, fmt.Errorf("persist: %w", err)
	}

	// Re-list now that the cache is fresh and enqueue anything that wasn't
	// in the known set.
	allSeasons, _ := st.ListSeasons(ctx, seriesID)
	newCount := 0
	for _, season := range allSeasons {
		eps, _ := st.ListEpisodes(ctx, seriesID, season.SeasonNumber)
		for _, e := range eps {
			if _, seen := known[e.EpisodeID]; seen {
				continue
			}
			if err := EnqueueEpisode(ctx, st, cfg, *show, e); err == nil {
				newCount++
			}
		}
	}
	return newCount, nil
}

// SyncAndRefreshSubscriptions runs FullSync followed by RefreshSubscriptions.
// This is the entry point for both the `m3u-dl sync` CLI and the TUI's
// initial sync, so subscribed shows pick up new episodes on every refresh.
func SyncAndRefreshSubscriptions(ctx context.Context, st *store.Store, xc *xtream.Client, cfg EnqueueConfig, progress Progress) error {
	if err := FullSync(ctx, st, xc, progress); err != nil {
		return err
	}
	if _, err := RefreshSubscriptions(ctx, st, xc, cfg, progress); err != nil {
		// Subscription refresh failures are surfaced but don't fail the whole
		// sync — the catalog is already up to date and the user will see the
		// error in their next attempt.
		return fmt.Errorf("subscriptions: %w", err)
	}
	return nil
}
