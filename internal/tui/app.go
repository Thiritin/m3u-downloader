// Package tui contains the Bubble Tea TUI for browsing and queuing.
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

type view int

const (
	viewBrowse view = iota
	viewSearch
	viewQueue
)

type App struct {
	store    *store.Store
	xtream   *xtream.Client
	keys     keyMap
	view     view
	browse   browseModel
	search   searchModel
	queue    queueModel
	width    int
	height   int

	// Sync state for first-run indexing.
	syncing  bool
	syncMsg  string
}

func New(st *store.Store, xc *xtream.Client, moviesDir, seriesDir string) *App {
	a := &App{store: st, xtream: xc, keys: defaultKeys()}
	a.browse = newBrowseModel(st, xc, moviesDir, seriesDir)
	a.search = newSearchModel(st, xc, moviesDir, seriesDir)
	a.queue = newQueueModel(st)
	return a
}

func (a *App) Run() error {
	p := tea.NewProgram(a, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// initialSyncCmd checks whether the catalog is empty; if so, runs a full sync
// and emits progress messages along the way.
func initialSyncCmd(st *store.Store, xc *xtream.Client) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		n, _ := st.CountAll(ctx)
		if n > 0 {
			return syncDoneMsg{skipped: true}
		}
		err := catalog.FullSync(ctx, st, xc, nil)
		if err != nil {
			return syncDoneMsg{err: err}
		}
		return syncDoneMsg{}
	}
}

type syncDoneMsg struct {
	skipped bool
	err     error
}

func (a *App) Init() tea.Cmd {
	a.syncing = true
	a.syncMsg = "checking catalog…"
	return tea.Batch(
		initialSyncCmd(a.store, a.xtream),
		a.queue.Init(),
	)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Window-size messages must reach every child so their internal lists
	// know how big to render. Bubble Tea sends WindowSizeMsg once on startup
	// and on every resize — if we drop it during sync the lists stay 0x0
	// and render "No items." even when populated.
	if m, ok := msg.(tea.WindowSizeMsg); ok {
		a.width, a.height = m.Width, m.Height
		var bcmd, scmd, qcmd tea.Cmd
		a.browse, bcmd = a.browse.Update(msg)
		a.search, scmd = a.search.Update(msg)
		a.queue, qcmd = a.queue.Update(msg)
		return a, tea.Batch(bcmd, scmd, qcmd)
	}

	if m, ok := msg.(syncDoneMsg); ok {
		a.syncing = false
		if m.err != nil {
			a.syncMsg = "sync error: " + m.err.Error()
			return a, nil
		}
		if m.skipped {
			a.syncMsg = "catalog already populated"
		} else {
			a.syncMsg = "catalog synced"
		}
		// Now that the catalog has data, kick off the browse and search loaders.
		// Also re-emit the current size so child lists get sized in case
		// the original WindowSizeMsg arrived before they were ready.
		size := tea.WindowSizeMsg{Width: a.width, Height: a.height}
		var bcmd, scmd tea.Cmd
		a.browse, bcmd = a.browse.Update(size)
		a.search, scmd = a.search.Update(size)
		return a, tea.Batch(a.browse.Init(), a.search.Init(), bcmd, scmd)
	}

	if m, ok := msg.(tea.KeyMsg); ok {
		// Don't intercept global hotkeys while syncing or while a child list
		// has its filter prompt open.
		if !a.syncing && !a.childIsFiltering() {
			switch m.String() {
			case "ctrl+c":
				return a, tea.Quit
			case "q":
				if a.view != viewQueue {
					a.view = viewQueue
					return a, nil
				}
			case "b":
				if a.view != viewBrowse {
					a.view = viewBrowse
					return a, nil
				}
			case "s":
				if a.view != viewSearch {
					a.view = viewSearch
					return a, nil
				}
			}
		}
	}

	if a.syncing {
		return a, nil
	}

	// Async data messages must reach the right child regardless of which
	// view is currently visible — e.g. searchIndexLoadedMsg arrives even
	// when the user is in browse view.
	var bcmd, scmd, qcmd tea.Cmd
	switch msg.(type) {
	case categoriesLoadedMsg, itemsLoadedMsg, seriesInfoLoadedMsg, errMsg:
		a.browse, bcmd = a.browse.Update(msg)
		return a, bcmd
	case searchIndexLoadedMsg:
		a.search, scmd = a.search.Update(msg)
		return a, scmd
	case queueRefreshMsg, tickMsg:
		a.queue, qcmd = a.queue.Update(msg)
		return a, qcmd
	}

	// Otherwise route to the active view (key messages, etc.).
	var cmd tea.Cmd
	switch a.view {
	case viewBrowse:
		a.browse, cmd = a.browse.Update(msg)
	case viewSearch:
		a.search, cmd = a.search.Update(msg)
	case viewQueue:
		a.queue, cmd = a.queue.Update(msg)
	}
	return a, cmd
}

func (a *App) childIsFiltering() bool {
	// While any child list is in filtering mode, suppress global hotkeys
	// so typing into the filter doesn't accidentally switch views.
	switch a.view {
	case viewBrowse:
		return a.browse.isFiltering()
	case viewSearch:
		return a.search.results.FilterState() == list.Filtering
	}
	return false
}

func (a *App) View() string {
	if a.syncing {
		body := pane.
			Width(a.width-2).Height(a.height-3).
			Align(lipgloss.Center, lipgloss.Center).
			Render("Indexing catalog from provider…\n\n" + a.syncMsg + "\n\n(this only happens on first run)")
		footer := statusBar.Render("ctrl+c quit")
		return lipgloss.JoinVertical(lipgloss.Left, body, footer)
	}
	header := statusBar.Render(fmt.Sprintf("[b] browse   [s] search   [q] queue   |   %s", a.syncMsg))
	var body string
	switch a.view {
	case viewQueue:
		body = a.queue.View(a.width, a.height-1)
	case viewSearch:
		body = a.search.View(a.width, a.height-1)
	default:
		body = a.browse.View(a.width, a.height-1)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}
