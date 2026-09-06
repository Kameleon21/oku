package tui

import (
	"fmt"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// ── List item types ────────────────────────────────────────────────────────

// miniBarRoom is the columns a row spends on everything but the bar: the
// list's own indent, the author, the page counts and the separators. What is
// left of the pane is the bar's.
const miniBarRoom = 34

// miniBarMin and miniBarMax keep the bar readable in a narrow pane and stop
// it from swallowing a wide one.
const miniBarMin, miniBarMax = 6, 16

type userBookItem struct {
	book    model.UserBook
	density Density
	// barW is the mini progress bar's width, which follows the pane: the
	// delegate renders the description as it is given, so the row is sized
	// when it is built.
	barW int
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
		progress += " " + miniProgressBar(currentPage(i.book), i.book.Book.Pages, i.barW)
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

// inFlightNotice is the answer to a mutation pressed while one is running.
const inFlightNotice = "Please wait — an update is still in flight"

// ── Library section ────────────────────────────────────────────────────────

// librarySection is one shelf of the library: the Reading list or the Oku
// (want to read) list. One type, two instances.
type librarySection struct {
	sh   *shared
	st   styles
	tab  tab
	list list.Model
	// w is the pane's inner width, which the rows are drawn to.
	w int
}

func newLibrarySection(sh *shared, st styles, t tab) *librarySection {
	return &librarySection{sh: sh, st: st, tab: t, list: newList(st)}
}

// newList is a list with only its rows: the pane's border already carries
// the name and the count, so a list spends none of its rows on its own title
// bar or on pagination dots. Filtering stays enabled (SetItems reapplies an
// active filter) but its title-bar row is not drawn.
func newList(st styles) list.Model {
	l := list.New(nil, newListDelegate(0, st), 40, 12)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	return l
}

// books is the shelf this instance shows.
func (s *librarySection) books() []model.UserBook {
	if s.tab == tabOku {
		return s.sh.oku
	}
	return s.sh.reading
}

func (s *librarySection) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return s.handleKey(msg)
	case dataChangedMsg:
		if msg.kind == dataLibrary || msg.kind == dataDensity {
			return s.rebuild()
		}
		return nil
	case list.FilterMatchesMsg:
		// Carries no list id: only the list that is filtering asked for it.
		if s.list.FilterState() == list.Unfiltered {
			return nil
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return cmd
}

func (s *librarySection) handleKey(msg tea.KeyMsg) tea.Cmd {
	k := keysFor(s)
	switch {
	case key.Matches(msg, k.Up, k.Down):
		// The list moves its own cursor.
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		return cmd
	case key.Matches(msg, k.Refresh):
		return request(reqRefresh{})
	case key.Matches(msg, k.ProgressUp):
		return s.progress(+10)
	case key.Matches(msg, k.ProgressDown):
		return s.progress(-10)
	case key.Matches(msg, k.Update):
		if b := s.selected(); b != nil {
			return request(reqOpenModal{newPageModal(s.sh, s.st, *b)})
		}
	case key.Matches(msg, k.Rate):
		if b := s.selected(); b != nil {
			return request(reqOpenModal{newReviewModal(s.sh, s.st, *b)})
		}
	case key.Matches(msg, k.SetReading):
		return s.changeStatus(model.StatusCurrentlyReading, false)
	case key.Matches(msg, k.SetWant):
		return s.changeStatus(model.StatusWantToRead, false)
	case key.Matches(msg, k.SetFinished):
		return s.changeStatus(model.StatusRead, false)
	case key.Matches(msg, k.SetDNF):
		return s.changeStatus(model.StatusDidNotFinish, true)
	case key.Matches(msg, k.SetIgnored):
		return s.changeStatus(model.StatusIgnored, true)
	case key.Matches(msg, k.Timer):
		return request(reqTimerToggle{book: s.selected(), reading: s.tab == tabReading})
	}
	return nil
}

// progress asks for a relative page update on the selection.
func (s *librarySection) progress(delta int) tea.Cmd {
	b := s.selected()
	if b == nil {
		return nil
	}
	return request(reqProgress{book: *b, delta: delta})
}

// changeStatus asks to move the selection to another shelf. Ignoring a book
// takes it out of the library and a DNF closes the read; U can put the
// shelf back while the toast is up, but neither should happen because a
// finger slipped one key, so those two confirm first.
func (s *librarySection) changeStatus(status model.Status, confirm bool) tea.Cmd {
	b := s.selected()
	if b == nil {
		return request(reqToast{toastError, "no book selected"})
	}
	return request(reqChangeStatus{book: *b, to: status, confirm: confirm})
}

// rebuild refreshes the rows from shared, at the width the pane has now. The
// returned command must be run: with filtering enabled, SetItems reapplies
// an active filter.
func (s *librarySection) rebuild() tea.Cmd {
	books := s.books()
	items := make([]list.Item, 0, len(books))
	for _, b := range books {
		items = append(items, userBookItem{book: b, density: s.sh.density, barW: s.barWidth()})
	}
	return s.list.SetItems(items)
}

// barWidth is the room the mini progress bar has in the current pane.
func (s *librarySection) barWidth() int {
	return clampInt(s.w-miniBarRoom, miniBarMin, miniBarMax)
}

func (s *librarySection) selected() *model.UserBook {
	item := s.list.SelectedItem()
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

// View is the list, with the position badge stamped on its last row when
// there is more of the shelf below it.
func (s *librarySection) View(w, h int) string {
	return stampOverflowBadge(fitBlock(s.list.View(), w, h), s.overflowBadge(), w, s.st)
}

// Resize sizes the list and, when the pane's width has changed, redraws the
// rows to it.
func (s *librarySection) Resize(w, h int) tea.Cmd {
	s.list.SetSize(w, h)
	if w == s.w {
		return nil
	}
	s.w = w
	return s.rebuild()
}

func (s *librarySection) Keys(k *keyMap) {
	tabHint := hint("tab", k.PrevSection, k.NextSection)
	k.Up.SetHelp("k", "navigate")
	k.Down.SetHelp("j", "navigate")
	if s.sh.timer != nil {
		k.Timer.SetHelp("t", "stop timer")
	} else if s.tab != tabReading {
		k.Timer.SetHelp("t", "timer (Reading list)")
	}
	enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.TabJump, &k.Search,
		&k.Details, &k.ProgressUp, &k.ProgressDown, &k.Update, &k.Rate,
		&k.SetReading, &k.SetWant, &k.SetFinished, &k.SetDNF, &k.SetIgnored,
		&k.Timer, &k.Sync, &k.Refresh, &k.Density)

	// Ordered by how often a key is reached for, with help first so it is
	// the one hint a narrow terminal never drops.
	k.short = []key.Binding{
		k.Help,
		hint("navigate", k.Down, k.Up),
		k.Details,
		tabHint,
		hint("status", k.SetReading, k.SetWant, k.SetFinished, k.SetDNF, k.SetIgnored),
		hint("page", k.ProgressUp, k.ProgressDown),
		hintAs("u", "update", k.Update),
	}
	if s.tab == tabReading || s.sh.timer != nil {
		k.short = append(k.short, k.Timer)
	}
	// The bar has a word per key; the modal spells them out.
	k.short = append(k.short, k.Search, hintAs("v", "rate", k.Rate), hintAs("s", "sync", k.Sync), k.Density, k.Refresh)
}

func (s *librarySection) Focus() {}
func (s *librarySection) Blur()  {}

func (s *librarySection) CapturesKeys() bool { return false }

func (s *librarySection) Title() string {
	if s.tab == tabOku {
		return fmt.Sprintf("Oku (%d)", len(s.books()))
	}
	return fmt.Sprintf("Reading (%d)", len(s.books()))
}

func (s *librarySection) Selected() selection { return selection{Book: s.selected()} }

// overflowBadge reports where the cursor sits when the pane shows fewer
// books than the shelf holds. Hiding the pagination dots took away the only
// sign that there was anything below the last visible row.
func (s *librarySection) overflowBadge() string {
	total := len(s.list.VisibleItems())
	if total == 0 || s.list.Paginator.PerPage >= total {
		return ""
	}
	return fmt.Sprintf("%d/%d", s.list.Index()+1, total)
}

// currentPage is where a book stands: the open read's progress when there
// is one, the book's own page otherwise.
func currentPage(b model.UserBook) int {
	if len(b.UserBookReads) > 0 {
		return b.UserBookReads[0].ProgressPages
	}
	return b.CurrentPage
}

// newListDelegate is the item renderer every list shares: title over
// description, the selection marked by a bar in the accent colour.
func newListDelegate(spacing int, st styles) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.SetSpacing(spacing)

	delegate.Styles.NormalTitle = st.listNormalTitle
	delegate.Styles.NormalDesc = st.listNormalDesc
	delegate.Styles.SelectedTitle = st.listSelectedTitle
	delegate.Styles.SelectedDesc = st.listSelectedDesc
	delegate.Styles.DimmedTitle = st.listDimmedTitle
	delegate.Styles.DimmedDesc = st.listDimmedDesc
	return delegate
}
