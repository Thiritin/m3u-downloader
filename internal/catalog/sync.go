// Package catalog orchestrates a full catalog refresh from the Xtream API
// into the local SQLite store. Used by both the `m3u-dl sync` command and
// the TUI's first-run auto-sync.
package catalog

import (
	"context"
	"fmt"

	"github.com/Thiritin/m3u-downloader/internal/store"
	"github.com/Thiritin/m3u-downloader/internal/xtream"
)

// Progress is called at each major step with a human-readable status line.
type Progress func(stage string, count int)

// FullSync fetches all categories, all VODs, and all series listings from the
// provider and replaces the cached entries. Series seasons/episodes remain
// lazy — fetched only on drill-in — because per-series fetches don't bulk.
func FullSync(ctx context.Context, st *store.Store, xc *xtream.Client, progress Progress) error {
	if progress == nil {
		progress = func(string, int) {}
	}

	progress("fetching VOD categories", 0)
	vodCats, err := xc.GetVODCategories(ctx)
	if err != nil {
		return fmt.Errorf("get_vod_categories: %w", err)
	}
	progress("fetching series categories", len(vodCats))
	seriesCats, err := xc.GetSeriesCategories(ctx)
	if err != nil {
		return fmt.Errorf("get_series_categories: %w", err)
	}

	progress("storing categories", len(vodCats)+len(seriesCats))
	allCats := make([]store.CategoryRow, 0, len(vodCats)+len(seriesCats))
	for _, c := range vodCats {
		allCats = append(allCats, categoryRow(c, "vod"))
	}
	for _, c := range seriesCats {
		allCats = append(allCats, categoryRow(c, "series"))
	}
	if err := st.UpsertCategories(ctx, allCats); err != nil {
		return fmt.Errorf("upsert categories: %w", err)
	}

	progress("fetching all VODs", 0)
	vods, err := xc.GetVODStreams(ctx, "")
	if err != nil {
		return fmt.Errorf("get_vod_streams: %w", err)
	}
	progress("storing VODs", len(vods))
	vodRows := make([]store.VODRow, 0, len(vods))
	for _, v := range vods {
		catID := 0
		fmt.Sscanf(v.CategoryID, "%d", &catID)
		year := 0
		fmt.Sscanf(v.Year, "%d", &year)
		vodRows = append(vodRows, store.VODRow{
			StreamID: v.StreamID, CategoryID: catID, Name: v.Name,
			Year: year, Plot: v.Plot, StreamIcon: v.StreamIcon,
			ContainerExt: v.ContainerExtension,
		})
	}
	if err := st.UpsertVODs(ctx, vodRows); err != nil {
		return fmt.Errorf("upsert vods: %w", err)
	}

	progress("fetching all series", len(vods))
	series, err := xc.GetSeries(ctx, "")
	if err != nil {
		return fmt.Errorf("get_series: %w", err)
	}
	progress("storing series", len(series))
	seriesRows := make([]store.SeriesRow, 0, len(series))
	for _, s := range series {
		catID := 0
		fmt.Sscanf(s.CategoryID, "%d", &catID)
		backdrop := ""
		if len(s.Backdrop) > 0 {
			backdrop = s.Backdrop[0]
		}
		seriesRows = append(seriesRows, store.SeriesRow{
			SeriesID: s.SeriesID, CategoryID: catID, Name: s.Name,
			Plot: s.Plot, CoverURL: s.Cover, BackdropURL: backdrop,
		})
	}
	if err := st.UpsertSeries(ctx, seriesRows); err != nil {
		return fmt.Errorf("upsert series: %w", err)
	}

	// Mark every category as fetched so the TUI's lazy-load sees the cache as fresh.
	for _, c := range allCats {
		_ = st.MarkCategoryFetched(ctx, c.ID)
	}

	progress("done", len(vods)+len(series))
	return nil
}

func categoryRow(c xtream.Category, kind string) store.CategoryRow {
	id := 0
	fmt.Sscanf(c.CategoryID, "%d", &id)
	return store.CategoryRow{
		ID: id, Type: kind, Name: c.CategoryName, ParentID: c.ParentID,
	}
}
