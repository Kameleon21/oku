package tui

import (
	"fmt"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── List item types ────────────────────────────────────────────────────────

type userBookItem struct {
	book    model.UserBook
	density Density
}

func (i userBookItem) Title() string {
	return i.book.Book.Title
}

func (i userBookItem) Description() string {
	author := i.book.Book.AuthorString()
	if author == "" {
		author = "Unknown author"
	}
	progress := i.book.Progress()
	if i.book.Book.Pages > 0 {
		page := i.book.CurrentPage
		if len(i.book.UserBookReads) > 0 {
			page = i.book.UserBookReads[0].ProgressPages
		}
		progress += " " + miniProgressBar(page, i.book.Book.Pages, 8)
	}

	switch i.density {
	case DensityCompact:
		return progress
	case DensityVerbose:
		if meta := format.BookMeta(i.book.Book); meta != "" {
			return fmt.Sprintf("%s · %s · %s", author, progress, meta)
		}
		return fmt.Sprintf("%s · %s", author, progress)
	default:
		return fmt.Sprintf("%s · %s", author, progress)
	}
}

func (i userBookItem) FilterValue() string {
	return i.book.Book.Title + " " + i.book.Book.AuthorString()
}

func (m Model) handleLibraryKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.activeKeys()
	switch {
	case key.Matches(msg, k.Quit, k.ForceQuit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		m.openHelp()
		return m, nil
	case key.Matches(msg, k.NextSection):
		m.nextSection()
		return m, nil
	case key.Matches(msg, k.PrevSection):
		m.prevSection()
		return m, nil
	case key.Matches(msg, k.Up, k.Down):
		// The focused list moves its own cursor.
		var cmd tea.Cmd
		if m.section == sectionReading {
			m.readingList, cmd = m.readingList.Update(msg)
		} else {
			m.okuList, cmd = m.okuList.Update(msg)
		}
		return m, cmd
	case key.Matches(msg, k.Search):
		m.focusSearchInput()
		return m, nil
	case key.Matches(msg, k.Refresh):
		return m.startOp(loadLibraryCmd(m.ctx, m.app, true))
	case key.Matches(msg, k.Sync):
		return m.startOp(syncAllAndReloadCmd(m.ctx, m.app))
	case key.Matches(msg, k.Density):
		cmd := m.cycleDensity()
		return m, cmd
	case key.Matches(msg, k.Details):
		// Enter used to move the book to another shelf, so a stray keypress
		// silently rewrote the library. It now only brings the selection into
		// the detail pane; g/w/f/d still change the status.
		b := m.selectedLibraryBook()
		if b == nil {
			cmd := m.showToast(toastError, "no book selected")
			return m, cmd
		}
		cmd := m.showToast(toastInfo, b.Book.Title)
		return m, cmd
	case key.Matches(msg, k.ProgressUp):
		return m.quickProgress(+10)
	case key.Matches(msg, k.ProgressDown):
		return m.quickProgress(-10)
	case key.Matches(msg, k.Update):
		if b := m.selectedLibraryBook(); b != nil {
			m.openPageModal(*b)
			return m, nil
		}
	case key.Matches(msg, k.Rate):
		if b := m.selectedLibraryBook(); b != nil {
			m.openReviewRatingModal(*b)
			return m, nil
		}
	case key.Matches(msg, k.SetReading):
		return m.changeSelectedLibraryStatus(model.StatusCurrentlyReading)
	case key.Matches(msg, k.SetWant):
		return m.changeSelectedLibraryStatus(model.StatusWantToRead)
	case key.Matches(msg, k.SetFinished):
		return m.changeSelectedLibraryStatus(model.StatusRead)
	case key.Matches(msg, k.SetDNF):
		return m.confirmStatusChange(model.StatusDidNotFinish)
	case key.Matches(msg, k.SetIgnored):
		return m.confirmStatusChange(model.StatusIgnored)
	case key.Matches(msg, k.Timer):
		return m.toggleTimerForSelection()
	}
	return m, nil
}

// toggleTimerForSelection starts a reading timer for the highlighted book, or
// stops the one that is running. Only the Reading list holds books a timer can
// track, so elsewhere it says where to press it.
func (m Model) toggleTimerForSelection() (tea.Model, tea.Cmd) {
	if m.isLoading() {
		// timerState only catches up when the load lands, so two quick presses
		// would otherwise start two sessions.
		cmd := m.showToast(toastWarn, inFlightNotice)
		return m, cmd
	}
	if m.timerState != nil {
		return m.startOp(stopTimerCmd(m.app))
	}
	if m.section != sectionReading {
		cmd := m.showToast(toastWarn, "Timers track a book you are reading — press t in the Reading list")
		return m, cmd
	}
	b := m.selectedLibraryBook()
	if b == nil {
		cmd := m.showToast(toastError, "no book selected")
		return m, cmd
	}
	return m.startOp(startTimerForBookCmd(m.app, b.Book.ID))
}

// inFlightNotice is the answer to a mutation pressed while one is running.
const inFlightNotice = "Please wait — an update is still in flight"

// confirmStatusChange asks first. Ignoring a book takes it out of the library
// and a DNF closes the read; U can put the shelf back while the toast is up,
// but neither should happen because a finger slipped one key.
func (m Model) confirmStatusChange(status model.Status) (tea.Model, tea.Cmd) {
	b := m.selectedLibraryBook()
	if b == nil {
		cmd := m.showToast(toastError, "no book selected")
		return m, cmd
	}
	m.confirm = newConfirmState(fmt.Sprintf("Mark '%s' as %s?", b.Book.Title, status.Label()))
	m.confirmCmd = changeStatusCmd(m.ctx, m.app, b.Book.ID, b.Book.Title, b.StatusID, status)
	return m, nil
}

// quickProgress applies a relative page update. UpdateProgress is
// read-modify-write, so firing a second one while the first is in flight
// would silently lose an update.
func (m Model) quickProgress(delta int) (tea.Model, tea.Cmd) {
	b := m.selectedLibraryBook()
	if b == nil {
		return m, nil
	}
	if m.isLoading() {
		cmd := m.showToast(toastWarn, inFlightNotice)
		return m, cmd
	}
	return m.startOp(quickProgressCmd(m.ctx, m.app, b.Book.ID, b.Book.Title, currentPage(*b), delta))
}

// currentPage is where a book stands: the open read's progress when there
// is one, the book's own page otherwise.
func currentPage(b model.UserBook) int {
	if len(b.UserBookReads) > 0 {
		return b.UserBookReads[0].ProgressPages
	}
	return b.CurrentPage
}

// ── List helpers ───────────────────────────────────────────────────────────

// refreshListItems rebuilds both library lists. The returned command must be
// run: with filtering enabled, SetItems reapplies an active filter.
func (m *Model) refreshListItems() tea.Cmd {
	toItems := func(books []model.UserBook) []list.Item {
		items := make([]list.Item, 0, len(books))
		for _, b := range books {
			items = append(items, userBookItem{
				book:    b,
				density: m.density,
			})
		}
		return items
	}
	readingCmd := m.readingList.SetItems(toItems(m.readingBooks))
	okuCmd := m.okuList.SetItems(toItems(m.okuBooks))
	return tea.Batch(readingCmd, okuCmd)
}

// newListDelegate is the item renderer every list shares: title over
// description, the selection marked by a bar in the accent colour.
func newListDelegate(spacing int) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(spacing)

	delegate.Styles.NormalTitle = lipgloss.NewStyle().
		Foreground(th.Text).
		Padding(0, 0, 0, 2)
	delegate.Styles.NormalDesc = lipgloss.NewStyle().
		Foreground(th.TextMuted).
		Padding(0, 0, 0, 2)
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(th.Accent).
		Foreground(th.Accent).
		Bold(true).
		Padding(0, 0, 0, 1)
	delegate.Styles.SelectedDesc = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(th.Accent).
		Foreground(th.TextMuted).
		Padding(0, 0, 0, 1)
	delegate.Styles.DimmedTitle = lipgloss.NewStyle().
		Foreground(th.TextDim).
		Padding(0, 0, 0, 2)
	delegate.Styles.DimmedDesc = lipgloss.NewStyle().
		Foreground(th.Border).
		Padding(0, 0, 0, 2)
	return delegate
}

func (m Model) selectedLibraryBook() *model.UserBook {
	var item list.Item
	if m.section == sectionOku {
		item = m.okuList.SelectedItem()
	} else {
		item = m.readingList.SelectedItem()
	}
	if item == nil {
		return nil
	}
	ub, ok := item.(userBookItem)
	if !ok {
		return nil
	}
	book := ub.book
	return &book
}

func (m Model) changeSelectedLibraryStatus(status model.Status) (tea.Model, tea.Cmd) {
	b := m.selectedLibraryBook()
	if b == nil {
		cmd := m.showToast(toastError, "no book selected")
		return m, cmd
	}
	return m.startOp(changeStatusCmd(m.ctx, m.app, b.Book.ID, b.Book.Title, b.StatusID, status))
}
