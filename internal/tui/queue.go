package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Thiritin/m3u-downloader/internal/store"
)

type queueModel struct {
	store *store.Store
	jobs  list.Model
}

type jobItem struct{ row store.JobRow }

func (i jobItem) Title() string {
	return fmt.Sprintf("[%s] %s #%d  %s", i.row.Status, i.row.Kind, i.row.SourceID, i.row.DestPath)
}
func (i jobItem) Description() string {
	if i.row.TotalBytes > 0 {
		pct := float64(i.row.ProgressBytes) / float64(i.row.TotalBytes) * 100
		return fmt.Sprintf("%.1f%%   %dMB / %dMB   attempts=%d   %s",
			pct, i.row.ProgressBytes>>20, i.row.TotalBytes>>20, i.row.Attempts, i.row.LastError)
	}
	return fmt.Sprintf("attempts=%d   %s", i.row.Attempts, i.row.LastError)
}
func (i jobItem) FilterValue() string { return i.row.DestPath }

func newQueueModel(st *store.Store) queueModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Queue"
	l.SetShowStatusBar(false)
	return queueModel{store: st, jobs: l}
}

type queueRefreshMsg struct{ rows []store.JobRow }
type tickMsg time.Time

func refreshQueueCmd(st *store.Store) tea.Cmd {
	return func() tea.Msg {
		rows, _ := st.ListJobs(context.Background())
		return queueRefreshMsg{rows: rows}
	}
}

func tickQueue() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m queueModel) Init() tea.Cmd {
	return tea.Batch(refreshQueueCmd(m.store), tickQueue())
}

func (m queueModel) Update(msg tea.Msg) (queueModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.jobs.SetSize(msg.Width-2, msg.Height-5)
		return m, nil
	case queueRefreshMsg:
		items := make([]list.Item, 0, len(msg.rows))
		for _, r := range msg.rows {
			items = append(items, jobItem{r})
		}
		m.jobs.SetItems(items)
		return m, nil
	case tickMsg:
		return m, tea.Batch(refreshQueueCmd(m.store), tickQueue())
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			if it, ok := m.jobs.SelectedItem().(jobItem); ok && it.row.Status == "failed" {
				_ = m.store.RetryFailedJob(context.Background(), it.row.ID)
				return m, refreshQueueCmd(m.store)
			}
		case "R":
			_, _ = m.store.RetryAllFailedJobs(context.Background())
			return m, refreshQueueCmd(m.store)
		case "d", "x":
			// Cancel an active job (worker will react within ~1s) or remove
			// any other row from the list.
			if it, ok := m.jobs.SelectedItem().(jobItem); ok {
				ctx := context.Background()
				if it.row.Status == "active" {
					_ = m.store.CancelJob(ctx, it.row.ID)
				} else {
					_ = m.store.DeleteJob(ctx, it.row.ID)
				}
				return m, refreshQueueCmd(m.store)
			}
		}
	}
	var cmd tea.Cmd
	m.jobs, cmd = m.jobs.Update(msg)
	return m, cmd
}

func (m queueModel) View(w, h int) string {
	body := pane.Width(w-2).Height(h-3).Render(m.jobs.View())
	footer := statusBar.Render("r retry failed   R retry all failed   d cancel/remove   b browse   ctrl+c quit")
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}
