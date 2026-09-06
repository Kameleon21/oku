package tui

import (
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/model"
)

// section is one tab's content pane. Sections are pointers and mutate in
// place: a size set on a discarded copy was the value-semantics failure mode
// this avoids, and there is no write-back plumbing in the root.
//
// Sections never call the app and never touch the in-flight counter. A key
// that needs work done answers with a request (see request), which the root
// handles in updateCommon; a key that only moves a cursor is handled here.
type section interface {
	// Update takes the keys while the section is focused and every message
	// the root broadcasts. Sections ignore what they do not own.
	Update(msg tea.Msg) tea.Cmd
	// View is the section's own content, drawn into w columns and h rows:
	// the list for a library section, the results for search, the page for
	// stats and timer. The root decides where it goes.
	View(w, h int) string
	// Resize is called by the root from the layout; lists SetSize here. It
	// returns a command because rows drawn to the pane's own width have to
	// be rebuilt when it changes, and a rebuild's SetItems command must be
	// run or an active filter goes blank.
	Resize(w, h int) tea.Cmd
	// Keys enables and relabels the bindings this focus understands, and
	// sets k.short, the help bar order.
	Keys(k *keyMap)
	Focus()
	Blur()
	// CapturesKeys reports that a text input owns the keyboard, so the root
	// keys (q, ?, h/l, ...) are letters here.
	CapturesKeys() bool
	// Title is the pane title, drawn in the pane's top border: "Reading (3)",
	// "Search", "Stats · 2026".
	Title() string
	// Selected is what the detail pane should show; zero is the empty state.
	Selected() selection
}

// inputSection is a section with a text input the root can put the cursor
// in — the Search tab, today the only one. It is one named interface rather
// than two anonymous asserts because both halves have to move together: the
// cursor-blink command textinput.Focus answers with has to be carried back
// to the root, so every way in returns one.
type inputSection interface {
	section
	// focusInput puts the cursor in the input, at the end of what is typed.
	focusInput() tea.Cmd
	// focusInputIfEmpty does the same, but only when the pane has nothing to
	// read yet.
	focusInputIfEmpty() tea.Cmd
	// inputCursor is where the terminal's cursor belongs while the input has
	// the keyboard, relative to the section's own content box, or nil.
	inputCursor() *tea.Cursor
}

type selection struct {
	Book   *model.UserBook     // Reading / Oku
	Result *model.SearchResult // Search
}

// ── Shared data ────────────────────────────────────────────────────────────

// shared is the data every section reads. The root owns it and writes to it
// from updateCommon only; sections treat it as read-only and learn of
// changes through dataChangedMsg. nextToken is the one exception: a counter
// a modal bumps to stamp its own operation, safe from anywhere on the update
// loop.
type shared struct {
	reading, oku []model.UserBook
	// shelf is every cached user book by book id, across all statuses, so
	// the search detail can say whether the library already has a result,
	// and what it was rated.
	shelf  map[int]model.UserBook
	stats  *model.ReadingStats
	weekly model.WeeklyStats
	// sessions is the recent timer history, newest first. The Timer pane
	// shows five of them; the rest are for a per-book view.
	sessions []model.ReadingSession
	timer    *model.TimerState
	// timerBook is the running timer's book, resolved when local data loads
	// so that View never queries the store.
	timerBook  *model.Book
	lastSyncAt time.Time

	recentSearches []string
	density        Density

	// now is the clock every render reads, so a test can pin it.
	now func() time.Time

	loaded, localLoaded bool

	// spin is the one spinner: the root ticks it while work is in flight,
	// the status bar and the search pane draw it.
	spin spinner.Model

	token int
}

// nextToken mints the id a modal stamps on the operation it starts, so it
// can tell its own result from any other.
func (s *shared) nextToken() int {
	s.token++
	return s.token
}

// dataKind says which part of shared changed.
type dataKind int

const (
	dataLibrary  dataKind = iota // reading and oku
	dataLocal                    // stats, sessions, timer, history
	dataSearches                 // recentSearches
	dataDensity                  // density
)

// dataChangedMsg is broadcast by the root after it has written to shared,
// so a section can rebuild what it derives from the data.
type dataChangedMsg struct{ kind dataKind }

// stylesChangedMsg is broadcast when the terminal reports its background and
// the palette is rebuilt for it. lipgloss v2 resolves a colour once rather
// than at render time, so every style a section is holding — a list's
// delegate, a memoised page — has to be replaced rather than re-read.
type stylesChangedMsg struct{ st styles }

// ── Requests (section → root) ──────────────────────────────────────────────

// request wraps r as a command that delivers it to the root. A command
// that only returns a message resolves at once, so it lands before the next
// key press in practice.
func request(r any) tea.Cmd {
	return requestCmd{r}.deliver
}

// requestCmd is a request on its way to the root. It is a named type with a
// method rather than a closure so that one function delivers every request,
// whatever the call site: a test can tell a request apart from real work by
// that function alone.
type requestCmd struct{ r any }

func (c requestCmd) deliver() tea.Msg { return c.r }

// reqChangeStatus moves a library book to another shelf, asking first when
// confirm is set.
type reqChangeStatus struct {
	book    model.UserBook
	to      model.Status
	confirm bool
}

// reqProgress is + / -: a relative page update, refused while work is in
// flight since UpdateProgress is read-modify-write.
type reqProgress struct {
	book  model.UserBook
	delta int
}

// reqSetPage is the page prompt's answer; token marks the result as the
// prompt's own.
type reqSetPage struct {
	bookID   int
	title    string
	prevPage int
	raw      string
	token    int
}

// reqReview saves a rating and review; token marks the result as the
// modal's own.
type reqReview struct {
	bookID int
	rating float64
	review string
	token  int
}

// reqAddFromSearch shelves a search result.
type reqAddFromSearch struct {
	result model.SearchResult
	to     model.Status
}

// reqSearch runs a search; seq is the section's own stamp for the result.
type reqSearch struct {
	query string
	mode  model.SearchMode
	seq   int
}

// reqSearchDone is a search that came back: the query goes into the history
// and the outcome into the status bar.
type reqSearchDone struct {
	query   string
	mode    model.SearchMode
	results int
}

// reqTimer stops the running timer, or starts one for bookID.
type reqTimer struct {
	start  bool
	bookID int
}

// reqTimerToggle is t over a library list: stop the running timer, or start
// one for the selection. Only the Reading list holds books a timer can
// track — over any other list the key opens the picker instead — and the
// request is refused while work is in flight, since the timer state only
// catches up when the load lands.
type reqTimerToggle struct {
	book    *model.UserBook
	reading bool
}

// reqTimerPick opens the book picker: t in the Timer section with no timer
// running, and t over a list that is not the Reading one. prefer is the
// book to open it on when the Reading list holds it; the Reading list's own
// selection otherwise.
type reqTimerPick struct{ prefer *model.UserBook }

// reqOpenModal pushes a modal a section built: the page prompt, the review
// editor.
type reqOpenModal struct{ m modal }

// reqHelp opens the help modal over whatever has focus.
type reqHelp struct{}

// reqSwitchTab shows another tab: to when abs is set, the one the reader
// came from when back is, otherwise step tabs along from the current one,
// wrapping.
type reqSwitchTab struct {
	to   tab
	abs  bool
	back bool
	step int
}

// reqToast puts a message in the status bar.
type reqToast struct {
	level toastLevel
	text  string
}

// reqRunOp starts a command the root prepared: a confirmed operation.
type reqRunOp struct{ cmd tea.Cmd }

// reqSync syncs the library with Hardcover; reqRefresh reloads the library
// (or, for local, the stats and timer data) from the cache.
type reqSync struct{}
type reqRefresh struct{ local bool }

// reqUndo reverses the change the current toast offers to.
type reqUndo struct{}

// reqDensity cycles the row density.
type reqDensity struct{}

// keysFor is the keymap of one focus on its own: what f enables and
// relabels over a fresh map. Sections and modals dispatch on it.
func keysFor(f interface{ Keys(*keyMap) }) keyMap {
	k := newKeyMap()
	enable(&k.ForceQuit)
	f.Keys(&k)
	return k
}
