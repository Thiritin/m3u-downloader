package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Thiritin/m3u-downloader/internal/catalog"
	"github.com/Thiritin/m3u-downloader/internal/store"
	"github.com/Thiritin/m3u-downloader/internal/xtream"
)

type searchModel struct {
	store     *store.Store
	xc        *xtream.Client
	results   list.Model
	moviesDir string
	seriesDir string
	statusMsg string
	loaded    bool

	// Cached snapshots so we can re-render badges when job state changes
	// without re-fetching the whole catalog from SQLite.
	allVODs   []store.VODRow
	allSeries []store.SeriesRow
}

func newSearchModel(st *store.Store, xc *xtream.Client, moviesDir, seriesDir string) searchModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Search (press / to filter)"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	return searchModel{
		store:     st,
		xc:        xc,
		results:   l,
		moviesDir: moviesDir,
		seriesDir: seriesDir,
	}
}

type searchIndexLoadedMsg struct {
	vods     []store.VODRow
	series   []store.SeriesRow
	statuses map[string]string
}

func loadSearchIndexCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		vods, _ := st.ListAllVODs(ctx)
		series, _ := st.ListAllSeries(ctx)
		statuses, _ := st.JobStatusBySource(ctx)
		return searchIndexLoadedMsg{vods: vods, series: series, statuses: statuses}
	}
}

// refreshBadgesCmd just re-reads the jobs table and re-applies badges to the
// already-loaded items. Cheap because the catalog snapshot is in memory.
type badgesRefreshedMsg struct {
	statuses map[string]string
}

func refreshBadgesCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		statuses, _ := st.JobStatusBySource(context.Background())
		return badgesRefreshedMsg{statuses: statuses}
	}
}

func (m searchModel) Init() tea.Cmd {
	return loadSearchIndexCmd(m.store)
}

func (m searchModel) Update(msg tea.Msg) (searchModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.results.SetSize(msg.Width-2, msg.Height-5)
		return m, nil
	case searchIndexLoadedMsg:
		m.allVODs = msg.vods
		m.allSeries = msg.series
		m.results.SetItems(buildSearchItems(m.allVODs, m.allSeries, msg.statuses))
		m.loaded = true
		m.statusMsg = fmt.Sprintf("indexed %d titles", len(m.allVODs)+len(m.allSeries))
		return m, nil
	case badgesRefreshedMsg:
		if m.loaded {
			m.results.SetItems(buildSearchItems(m.allVODs, m.allSeries, msg.statuses))
		}
		return m, nil
	case tea.KeyMsg:
		// While the list's filter prompt is active, let the list consume keys.
		if m.results.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case " ":
			return m.queueSelected()
		}
	}
	var cmd tea.Cmd
	m.results, cmd = m.results.Update(msg)
	return m, cmd
}

func buildSearchItems(vods []store.VODRow, series []store.SeriesRow, statuses map[string]string) []list.Item {
	items := make([]list.Item, 0, len(vods)+len(series))
	for _, v := range vods {
		items = append(items, vodItem{
			row:   v,
			badge: statusBadge(statuses[store.JobStatusKey("vod", v.StreamID)]),
		})
	}
	for _, s := range series {
		// Series have no per-show job, so they never carry a badge in the
		// search view. Drill into the show via Browse to see episode-level
		// badges, or open the Queue view to monitor downloads.
		items = append(items, seriesItem{row: s})
	}
	return items
}

func (m searchModel) queueSelected() (searchModel, tea.Cmd) {
	cfg := catalog.EnqueueConfig{MoviesDir: m.moviesDir, SeriesDir: m.seriesDir}
	ctx := context.Background()
	switch it := m.results.SelectedItem().(type) {
	case vodItem:
		err := catalog.EnqueueVOD(ctx, m.store, cfg, it.row)
		m.statusMsg = friendlyEnqueueMsg("queued movie", err)
	case seriesItem:
		n, err := catalog.EnqueueSeries(ctx, m.store, m.xc, cfg, it.row)
		if err != nil {
			m.statusMsg = "ERR: " + err.Error()
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("queued show: %s (%d episodes, subscribed)", it.row.Name, n)
	}
	// After every enqueue, re-render badges so the just-queued item shows [Q].
	return m, refreshBadgesCmd(m.store)
}

func (m searchModel) View(w, h int) string {
	body := pane.Width(w - 2).Height(h - 3).Render(m.results.View())
	footer := statusBar.Render(fmt.Sprintf(
		"%s   /  filter   space  queue   b  browse   q  queue view   ctrl+c  quit",
		m.statusMsg))
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}
