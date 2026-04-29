package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Thiritin/m3u-downloader/internal/store"
	"github.com/Thiritin/m3u-downloader/internal/xtream"
)

type browseLevel int

const (
	levelCategories browseLevel = iota
	levelItems
	levelSeasons
	levelEpisodes
)

type browseModel struct {
	store    *store.Store
	xc       *xtream.Client
	level    browseLevel
	cats     list.Model
	items    list.Model
	seasons  list.Model
	episodes list.Model

	moviesDir string
	seriesDir string

	currentCategory store.CategoryRow
	currentSeries   store.SeriesRow
	currentSeason   store.SeasonRow

	// previewedCatID is the category whose items the right pane currently shows.
	// Used to detect when the user moves the cursor on the categories list and
	// auto-load the highlighted category into the items pane.
	previewedCatID int

	statusMsg string
}

type catItem struct{ row store.CategoryRow }

func (i catItem) Title() string       { return i.row.Name }
func (i catItem) Description() string { return strings.ToUpper(i.row.Type) }
func (i catItem) FilterValue() string { return i.row.Name }

// statusBadge returns a short prefix shown ahead of titles that already have
// a job in the queue. Empty if no job exists.
func statusBadge(status string) string {
	switch status {
	case "active":
		return "[↓] "
	case "pending":
		return "[Q] "
	case "completed":
		return "[✓] "
	case "failed":
		return "[✗] "
	}
	return ""
}

type vodItem struct {
	row   store.VODRow
	badge string
}

func (i vodItem) Title() string {
	base := i.row.Name
	if i.row.Year > 0 {
		base = fmt.Sprintf("%s (%d)", i.row.Name, i.row.Year)
	}
	return i.badge + base
}
func (i vodItem) Description() string { return "" }
func (i vodItem) FilterValue() string { return i.row.Name }

type seriesItem struct {
	row   store.SeriesRow
	badge string
}

func (i seriesItem) Title() string       { return i.badge + i.row.Name }
func (i seriesItem) Description() string { return "" }
func (i seriesItem) FilterValue() string { return i.row.Name }

type seasonItem struct{ row store.SeasonRow }

func (i seasonItem) Title() string       { return fmt.Sprintf("Season %02d", i.row.SeasonNumber) }
func (i seasonItem) Description() string { return i.row.Name }
func (i seasonItem) FilterValue() string { return i.Title() }

type episodeItem struct {
	row   store.EpisodeRow
	badge string
}

func (i episodeItem) Title() string {
	return i.badge + fmt.Sprintf("S%02dE%02d  %s", i.row.SeasonNumber, i.row.EpisodeNum, i.row.Title)
}
func (i episodeItem) Description() string { return "" }
func (i episodeItem) FilterValue() string { return i.row.Title }

func newBrowseModel(st *store.Store, xc *xtream.Client, moviesDir, seriesDir string) browseModel {
	mk := func(title string) list.Model {
		l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
		l.Title = title
		l.SetShowStatusBar(false)
		l.SetFilteringEnabled(true)
		return l
	}
	return browseModel{
		store:     st,
		xc:        xc,
		moviesDir: moviesDir,
		seriesDir: seriesDir,
		cats:      mk("Categories"),
		items:     mk("Items"),
		seasons:   mk("Seasons"),
		episodes:  mk("Episodes"),
	}
}

func (m browseModel) Init() tea.Cmd {
	return loadCategoriesCmd(m.store, m.xc)
}

// --- messages ---

type categoriesLoadedMsg struct{ rows []store.CategoryRow }
type itemsLoadedMsg struct {
	vods   []store.VODRow
	series []store.SeriesRow
}
type seriesInfoLoadedMsg struct {
	seasons  []store.SeasonRow
	episodes []store.EpisodeRow
}
type errMsg struct{ err error }

// --- commands ---

func loadCategoriesCmd(st *store.Store, xc *xtream.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		fillIfEmpty := func(kind string, fetch func() ([]xtream.Category, error)) error {
			rows, err := st.ListCategories(ctx, kind)
			if err != nil {
				return err
			}
			if len(rows) > 0 {
				return nil
			}
			cats, err := fetch()
			if err != nil {
				return err
			}
			toRows := make([]store.CategoryRow, 0, len(cats))
			for _, c := range cats {
				id := 0
				fmt.Sscanf(c.CategoryID, "%d", &id)
				toRows = append(toRows, store.CategoryRow{
					ID: id, Type: kind, Name: c.CategoryName, ParentID: c.ParentID,
				})
			}
			return st.UpsertCategories(ctx, toRows)
		}
		if err := fillIfEmpty("vod", func() ([]xtream.Category, error) {
			return xc.GetVODCategories(ctx)
		}); err != nil {
			return errMsg{err}
		}
		if err := fillIfEmpty("series", func() ([]xtream.Category, error) {
			return xc.GetSeriesCategories(ctx)
		}); err != nil {
			return errMsg{err}
		}

		vodCats, _ := st.ListCategories(ctx, "vod")
		seriesCats, _ := st.ListCategories(ctx, "series")
		return categoriesLoadedMsg{rows: append(vodCats, seriesCats...)}
	}
}

func loadItemsCmd(st *store.Store, xc *xtream.Client, cat store.CategoryRow) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		switch cat.Type {
		case "vod":
			vods, _ := st.ListVODs(ctx, cat.ID)
			if len(vods) == 0 || cat.FetchedAt == 0 {
				api, err := xc.GetVODStreams(ctx, fmt.Sprint(cat.ID))
				if err != nil {
					return errMsg{err}
				}
				rows := make([]store.VODRow, 0, len(api))
				for _, v := range api {
					y := 0
					fmt.Sscanf(v.Year, "%d", &y)
					rows = append(rows, store.VODRow{
						StreamID: v.StreamID, CategoryID: cat.ID, Name: v.Name,
						Year: y, Plot: v.Plot, StreamIcon: v.StreamIcon,
						ContainerExt: v.ContainerExtension,
					})
				}
				_ = st.UpsertVODs(ctx, rows)
				_ = st.MarkCategoryFetched(ctx, cat.ID)
				vods, _ = st.ListVODs(ctx, cat.ID)
			}
			return itemsLoadedMsg{vods: vods}
		case "series":
			ser, _ := st.ListSeries(ctx, cat.ID)
			if len(ser) == 0 || cat.FetchedAt == 0 {
				api, err := xc.GetSeries(ctx, fmt.Sprint(cat.ID))
				if err != nil {
					return errMsg{err}
				}
				rows := make([]store.SeriesRow, 0, len(api))
				for _, s := range api {
					backdrop := ""
					if len(s.Backdrop) > 0 {
						backdrop = s.Backdrop[0]
					}
					rows = append(rows, store.SeriesRow{
						SeriesID: s.SeriesID, CategoryID: cat.ID, Name: s.Name,
						Plot: s.Plot, CoverURL: s.Cover, BackdropURL: backdrop,
					})
				}
				_ = st.UpsertSeries(ctx, rows)
				_ = st.MarkCategoryFetched(ctx, cat.ID)
				ser, _ = st.ListSeries(ctx, cat.ID)
			}
			return itemsLoadedMsg{series: ser}
		}
		return errMsg{fmt.Errorf("unknown category type: %s", cat.Type)}
	}
}

func loadSeriesInfoCmd(st *store.Store, xc *xtream.Client, sr store.SeriesRow) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		seasons, _ := st.ListSeasons(ctx, sr.SeriesID)
		if len(seasons) == 0 {
			info, err := xc.GetSeriesInfo(ctx, sr.SeriesID)
			if err != nil {
				return errMsg{err}
			}
			seasonRows := make([]store.SeasonRow, 0, len(info.Seasons))
			for _, s := range info.Seasons {
				seasonRows = append(seasonRows, store.SeasonRow{
					SeriesID: sr.SeriesID, SeasonNumber: s.SeasonNumber,
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
						EpisodeID: id, SeriesID: sr.SeriesID, SeasonNumber: sn,
						EpisodeNum: e.EpisodeNum, Title: e.Title, Plot: e.Info.Plot,
						ContainerExt: e.ContainerExtension, DurationSecs: dur,
					})
				}
			}
			_ = st.ReplaceSeasonsAndEpisodes(ctx, sr.SeriesID, seasonRows, epRows)
			seasons, _ = st.ListSeasons(ctx, sr.SeriesID)
		}
		return seriesInfoLoadedMsg{seasons: seasons}
	}
}

func loadEpisodesCmd(st *store.Store, seriesID, season int) tea.Cmd {
	return func() tea.Msg {
		eps, _ := st.ListEpisodes(context.Background(), seriesID, season)
		return seriesInfoLoadedMsg{episodes: eps}
	}
}

// --- update / view ---

func (m browseModel) Update(msg tea.Msg) (browseModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		leftWidth := msg.Width / 3
		rightWidth := msg.Width - leftWidth - 4
		listH := msg.Height - 5
		m.cats.SetSize(leftWidth-2, listH)
		m.items.SetSize(rightWidth-2, listH)
		m.seasons.SetSize(rightWidth-2, listH)
		m.episodes.SetSize(rightWidth-2, listH)
		return m, nil
	case categoriesLoadedMsg:
		items := make([]list.Item, 0, len(msg.rows))
		for _, r := range msg.rows {
			items = append(items, catItem{r})
		}
		m.cats.SetItems(items)
		// Kick off an initial preview of the first category so the right
		// pane isn't empty on entry.
		if ci, ok := m.cats.SelectedItem().(catItem); ok && m.previewedCatID == 0 {
			m.previewedCatID = ci.row.ID
			m.currentCategory = ci.row
			return m, loadItemsCmd(m.store, m.xc, ci.row)
		}
		return m, nil
	case itemsLoadedMsg:
		// Pull current job statuses so already-queued/downloaded items show
		// a badge when the user scrolls into them.
		statuses, _ := m.store.JobStatusBySource(context.Background())
		out := make([]list.Item, 0, len(msg.vods)+len(msg.series))
		for _, v := range msg.vods {
			out = append(out, vodItem{
				row:   v,
				badge: statusBadge(statuses[store.JobStatusKey("vod", v.StreamID)]),
			})
		}
		for _, s := range msg.series {
			out = append(out, seriesItem{row: s})
		}
		m.items.SetItems(out)
		// Don't change level here — that happens in drillIn. itemsLoadedMsg
		// can fire from either preview (cursor move on categories) or drillIn.
		return m, nil
	case seriesInfoLoadedMsg:
		if msg.seasons != nil {
			items := make([]list.Item, 0, len(msg.seasons))
			for _, s := range msg.seasons {
				items = append(items, seasonItem{s})
			}
			m.seasons.SetItems(items)
			m.level = levelSeasons
		}
		if msg.episodes != nil {
			statuses, _ := m.store.JobStatusBySource(context.Background())
			items := make([]list.Item, 0, len(msg.episodes))
			for _, e := range msg.episodes {
				items = append(items, episodeItem{
					row:   e,
					badge: statusBadge(statuses[store.JobStatusKey("episode", e.EpisodeID)]),
				})
			}
			m.episodes.SetItems(items)
			m.level = levelEpisodes
		}
		return m, nil
	case errMsg:
		m.statusMsg = "ERR: " + msg.err.Error()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			return m.drillIn()
		case "esc", "backspace":
			return m.drillOut()
		case " ":
			return m.queueSelected()
		case "r":
			if m.level == levelCategories {
				return m, loadCategoriesCmd(m.store, m.xc)
			}
			if m.currentCategory.ID != 0 {
				cat := m.currentCategory
				cat.FetchedAt = 0
				return m, loadItemsCmd(m.store, m.xc, cat)
			}
		}
	}
	var cmd tea.Cmd
	switch m.level {
	case levelCategories:
		m.cats, cmd = m.cats.Update(msg)
		// After any cursor movement on the categories list, auto-preview
		// that category's items in the right pane. Reading from local
		// SQLite is fast, so this is fine on every keystroke.
		if ci, ok := m.cats.SelectedItem().(catItem); ok && ci.row.ID != m.previewedCatID {
			m.previewedCatID = ci.row.ID
			m.currentCategory = ci.row
			cmd = tea.Batch(cmd, loadItemsCmd(m.store, m.xc, ci.row))
		}
	case levelItems:
		m.items, cmd = m.items.Update(msg)
	case levelSeasons:
		m.seasons, cmd = m.seasons.Update(msg)
	case levelEpisodes:
		m.episodes, cmd = m.episodes.Update(msg)
	}
	return m, cmd
}

func (m browseModel) isFiltering() bool {
	switch m.level {
	case levelCategories:
		return m.cats.FilterState() == list.Filtering
	case levelItems:
		return m.items.FilterState() == list.Filtering
	case levelSeasons:
		return m.seasons.FilterState() == list.Filtering
	case levelEpisodes:
		return m.episodes.FilterState() == list.Filtering
	}
	return false
}

func (m browseModel) drillIn() (browseModel, tea.Cmd) {
	switch m.level {
	case levelCategories:
		ci, ok := m.cats.SelectedItem().(catItem)
		if !ok {
			return m, nil
		}
		m.currentCategory = ci.row
		// Items are already preview-loaded as the cursor moved; just shift focus.
		// If for some reason the preview hasn't fired yet (initial state), still
		// kick off the load.
		var cmd tea.Cmd
		if m.previewedCatID != ci.row.ID {
			m.previewedCatID = ci.row.ID
			cmd = loadItemsCmd(m.store, m.xc, ci.row)
		}
		m.level = levelItems
		return m, cmd
	case levelItems:
		if si, ok := m.items.SelectedItem().(seriesItem); ok {
			m.currentSeries = si.row
			return m, loadSeriesInfoCmd(m.store, m.xc, si.row)
		}
	case levelSeasons:
		if si, ok := m.seasons.SelectedItem().(seasonItem); ok {
			m.currentSeason = si.row
			return m, loadEpisodesCmd(m.store, m.currentSeries.SeriesID, si.row.SeasonNumber)
		}
	}
	return m, nil
}

func (m browseModel) drillOut() (browseModel, tea.Cmd) {
	if m.level > levelCategories {
		m.level--
	}
	return m, nil
}

func (m browseModel) queueSelected() (browseModel, tea.Cmd) {
	cfg := EnqueueConfig{MoviesDir: m.moviesDir, SeriesDir: m.seriesDir}
	ctx := context.Background()
	var refreshCmd tea.Cmd
	switch m.level {
	case levelItems:
		switch it := m.items.SelectedItem().(type) {
		case vodItem:
			err := enqueueVOD(ctx, m.store, cfg, it.row)
			m.statusMsg = friendlyEnqueueMsg("queued movie", err)
			if err == nil && m.currentCategory.ID != 0 {
				refreshCmd = loadItemsCmd(m.store, m.xc, m.currentCategory)
			}
		case seriesItem:
			n, err := enqueueSeries(ctx, m.store, cfg, it.row)
			m.statusMsg = countOrError("queued show", n, err)
		}
	case levelSeasons:
		if it, ok := m.seasons.SelectedItem().(seasonItem); ok {
			n, err := enqueueSeason(ctx, m.store, cfg, m.currentSeries, it.row.SeasonNumber)
			m.statusMsg = countOrError("queued season", n, err)
		}
	case levelEpisodes:
		if it, ok := m.episodes.SelectedItem().(episodeItem); ok {
			err := enqueueEpisode(ctx, m.store, cfg, m.currentSeries, it.row)
			m.statusMsg = friendlyEnqueueMsg("queued episode", err)
			if err == nil {
				refreshCmd = loadEpisodesCmd(m.store, m.currentSeries.SeriesID, m.currentSeason.SeasonNumber)
			}
		}
	}
	return m, refreshCmd
}

func countOrError(msg string, n int, err error) string {
	if err != nil {
		return "ERR: " + err.Error()
	}
	return fmt.Sprintf("%s (%d episodes)", msg, n)
}

func (m browseModel) View(w, h int) string {
	leftWidth := w / 3
	rightWidth := w - leftWidth - 4
	leftPane := pane.Width(leftWidth).Height(h - 3).Render(m.cats.View())
	var right string
	switch m.level {
	case levelCategories, levelItems:
		right = m.items.View()
	case levelSeasons:
		right = m.seasons.View()
	case levelEpisodes:
		right = m.episodes.View()
	}
	rightPane := pane.Width(rightWidth).Height(h - 3).Render(right)
	main := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
	footer := statusBar.Render(fmt.Sprintf("level: %d   %s   q queue • enter drill • esc back • space queue • ctrl+c quit",
		m.level, m.statusMsg))
	return lipgloss.JoinVertical(lipgloss.Left, main, footer)
}
