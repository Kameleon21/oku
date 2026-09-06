// Package tui is the Oku dashboard: the Bubble Tea model behind `oku tui`,
// the palette it draws with, and the sections it draws. It is imported by
// internal/cli and imports nothing from it, so the CLI commands and the
// dashboard share a theme without sharing a package.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Dashboard Model ────────────────────────────────────────────────────────

// focus is which of the two panes the keyboard is aimed at. The content pane
// has it unless Enter moved it to the detail pane; the focused pane carries
// the thick border.
type focus int

const (
	focusContent focus = iota
	focusDetail
)

// Model is the root of the dashboard. It routes keys to the top modal or the
// active section, owns the data they share, and is the only place work
// starts: sections and modals answer keys with requests, updateCommon runs
// them with the in-flight guard and the spinner.
type Model struct {
	app *app.App
	// ctx is cancelled when the program exits so in-flight API calls abort
	// instead of outliving the store.
	ctx context.Context
	// version is the build the dashboard is running, shown in the help modal.
	version string

	// st is every style the dashboard draws with, derived from a Theme. It is
	// a value on the model so a theme change can rebuild it mid-run.
	st styles

	shared *shared

	// sections is one content pane per tab; tab is the one on screen and
	// focus says which of its two panes takes the keys.
	sections [tabCount]section
	tab      tab
	// prevTab is the tab the current one was reached from, which Esc in the
	// search input goes back to.
	prevTab tab
	focus   focus
	detail  *detailPane

	// modals is the stack of overlays; the top one takes the keys.
	modals []modal

	lay layout

	// inflight counts the commands that are running. A single boolean meant
	// the first completion cleared the flag for every other operation, so a
	// fast local load stopped the spinner mid-sync. Every command started
	// through beginLoading must produce exactly one message whose handler
	// calls endLoading.
	inflight int

	// spinning reports whether the spinner tick loop is armed. It only runs
	// while work is in flight, so an idle dashboard does not re-render a
	// dozen times a second.
	spinning bool
	// reconciling marks the background reconcile load; only its success
	// clears dirty.
	reconciling bool
	// syncing marks a full sync, which the header reports in place of the
	// age of the last one.
	syncing bool

	help help.Model

	dirty          bool
	lastMutationAt time.Time

	// toast is the footer's message, and undo what U would reverse while it
	// is up. Both are dropped when the toast expires.
	toast toast
	undo  *undoAction

	// timerTicking reports whether the one-second tick loop is armed; it only
	// runs while a timer is actually running.
	timerTicking bool
}

// New builds the dashboard model. density is the CLI's --view setting, which
// the list rows and the detail pane read to decide how much to show, and
// version is the build the help modal names.
func New(ctx context.Context, a *app.App, density Density, version string) *Model {
	if ctx == nil {
		ctx = context.Background()
	}

	st := newStyles(DefaultTheme())

	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = st.spinner

	sh := &shared{
		density: density,
		now:     time.Now,
		spin:    s,
	}

	hlp := help.New()
	hlp.ShortSeparator = " · "
	hlp.Styles.ShortKey = st.keyHint
	hlp.Styles.ShortDesc = st.desc
	hlp.Styles.ShortSeparator = st.dim
	hlp.Styles.Ellipsis = st.dim

	m := &Model{
		app:     a,
		ctx:     ctx,
		version: version,
		st:      st,
		shared:  sh,
		tab:     tabReading,
		detail:  newDetailPane(sh, st),
		help:    hlp,
		// Init starts the cached-library and local-data loads, so two
		// commands are already in flight.
		inflight: 2,
		spinning: true,
	}
	m.sections = [tabCount]section{
		tabReading: newLibrarySection(sh, st, tabReading),
		tabOku:     newLibrarySection(sh, st, tabOku),
		tabSearch:  newSearchSection(sh, st),
		tabStats:   newStatsSection(sh, st),
		tabTimer:   newTimerSection(sh, st),
	}
	m.section().Focus()
	return m
}

// section is the content pane of the tab on screen.
func (m *Model) section() section {
	return m.sections[m.tab]
}

// ── Init ───────────────────────────────────────────────────────────────────

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.shared.spin.Tick,
		loadCachedLibraryCmd(m.app),
		loadLocalDataCmd(m.app, m.shared.now),
		backgroundCheckCmd(),
	)
}

// ── Update ─────────────────────────────────────────────────────────────────

// Update routes a key press: ctrl+c quits from anywhere; the top modal takes
// every other key; the root keys apply unless the section's input owns the
// keyboard; the focused detail pane scrolls on j/k; the section gets the
// rest. Everything else is common handling plus a broadcast.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return m, m.updateCommon(msg)
	}

	k := m.activeKeys()
	if key.Matches(keyMsg, k.ForceQuit) {
		return m, tea.Quit
	}
	if top := m.topModal(); top != nil {
		done, cmd := top.Update(keyMsg)
		if done {
			m.pop()
		}
		return m, cmd
	}
	if !m.section().CapturesKeys() {
		if cmd, handled := m.rootKey(keyMsg, k); handled {
			return m, cmd
		}
		// The detail pane only takes the keys that scroll it; the shelf and
		// progress keys still act on the selection it is showing.
		if m.focus == focusDetail && m.detail.handleKey(keyMsg, k) {
			return m, nil
		}
	}

	cmd := m.section().Update(keyMsg)
	// A key that has just put the cursor in a text input takes the focus
	// with it. Without this the detail pane keeps the thick border while
	// the input owns the keyboard: the selection empties under it, so the
	// pane blanks, and on a terminal too narrow to split the list being
	// typed into is not drawn at all. It is enforced here rather than in
	// the section because the section cannot see the focus, and every way
	// into an input has to obey it.
	if m.section().CapturesKeys() && m.focus == focusDetail {
		cmd = tea.Batch(cmd, m.setFocus(focusContent))
	}
	return m, cmd
}

// rootKey handles the keys every tab shares. Like the sections, it only
// answers with requests: quitting, help and moving the focus are the
// exceptions, since they start no work.
func (m *Model) rootKey(msg tea.KeyMsg, k keyMap) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, k.Quit):
		return tea.Quit, true
	case key.Matches(msg, k.Help):
		m.openHelp()
		return nil, true
	case key.Matches(msg, k.Undo):
		return request(reqUndo{}), true
	case key.Matches(msg, k.TabJump):
		if t, ok := tabForKey(msg.String()); ok {
			return request(reqSwitchTab{to: t, abs: true}), true
		}
	case key.Matches(msg, k.Details):
		// Enter used to move the book to another shelf, so a stray keypress
		// silently rewrote the library. It now only moves the keyboard into
		// the detail pane; g/w/f/d still change the status.
		return m.setFocus(focusDetail), true
	case key.Matches(msg, k.Back):
		if m.focus == focusDetail {
			return m.setFocus(focusContent), true
		}
	case key.Matches(msg, k.PrevSection):
		if m.focus == focusDetail {
			// The detail pane is the right-hand one: h comes back from it.
			return m.setFocus(focusContent), true
		}
		return m.setTab((m.tab - 1 + tabCount) % tabCount), true
	case key.Matches(msg, k.NextSection):
		return m.setTab((m.tab + 1) % tabCount), true
	case key.Matches(msg, k.Search):
		return tea.Batch(m.setTab(tabSearch), m.focusSectionInput()), true
	case key.Matches(msg, k.Sync):
		return request(reqSync{}), true
	case key.Matches(msg, k.Density):
		return request(reqDensity{}), true
	}
	return nil, false
}

// focusSectionInput puts the cursor in the section's text input, for the one
// section that has one. The keyboard goes back to the content pane first: an
// input that owns the keys while the detail pane still holds the focus would
// draw the thick border around a pane no key reaches.
func (m *Model) focusSectionInput() tea.Cmd {
	s, ok := m.section().(interface{ focusInput() })
	if !ok {
		return nil
	}
	cmd := m.setFocus(focusContent)
	s.focusInput()
	return cmd
}

// openEmptySectionInput puts the cursor in the section's input when the
// section has nothing to show yet, for the one section that has one.
func (m *Model) openEmptySectionInput() {
	if s, ok := m.section().(interface{ focusInputIfEmpty() }); ok {
		s.focusInputIfEmpty()
	}
}

// updateCommon handles every message that is not a key press, whatever has
// focus, then broadcasts it to the sections and the modals.
func (m *Model) updateCommon(msg tea.Msg) tea.Cmd {
	cmd := m.handleCommon(msg)
	return tea.Batch(cmd, m.broadcast(msg))
}

// broadcast hands a message to every section, the detail pane and every
// modal. A modal that reports done is dropped from the stack. Bubbles' own
// messages are id-stamped so duplicates are harmless, except
// list.FilterMatchesMsg, which the lists themselves drop unless they are
// filtering.
func (m *Model) broadcast(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.sections)+len(m.modals)+1)
	for _, s := range m.sections {
		cmds = append(cmds, s.Update(msg))
	}
	cmds = append(cmds, m.detail.Update(msg))
	kept := m.modals[:0]
	for _, mod := range m.modals {
		done, cmd := mod.Update(msg)
		cmds = append(cmds, cmd)
		if !done {
			kept = append(kept, mod)
		}
	}
	if len(kept) != len(m.modals) {
		m.modals = kept
		m.modalsChanged()
	}
	return tea.Batch(cmds...)
}

// handleCommon is the root's own handling: async results, ticks, resizes
// and the requests the sections and modals raise.
func (m *Model) handleCommon(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !m.isLoading() {
			// Nothing in flight: let the tick loop stop.
			m.spinning = false
			return nil
		}
		var cmd tea.Cmd
		m.shared.spin, cmd = m.shared.spin.Update(msg)
		return cmd

	case timerTickMsg:
		if m.shared.timer == nil {
			// Nothing to animate: let the tick loop stop.
			m.timerTicking = false
			return nil
		}
		// Just triggers a re-render for the timer display.
		return timerTickCmd()

	case tea.WindowSizeMsg:
		m.lay.W, m.lay.H = msg.Width, msg.Height
		return m.resize()

	case libraryLoadedMsg:
		return m.applyLibraryLoaded(msg)

	case localDataLoadedMsg:
		return m.applyLocalDataLoaded(msg)

	case timerOpDoneMsg:
		m.endLoading()
		var toastCmd tea.Cmd
		if msg.err != nil {
			toastCmd = m.showToast(toastError, msg.err.Error())
		} else {
			toastCmd = m.showToast(toastSuccess, msg.info)
		}
		// Reload local data after timer operations. The toast tick is not
		// work, so it stays out of the in-flight count.
		reload := m.beginLoading(loadLocalDataCmd(m.app, m.shared.now))
		return tea.Batch(reload, toastCmd)

	case toastExpiredMsg:
		// Only the current toast's own tick clears it; a tick left over from
		// an earlier toast must not take a newer one down with it.
		if msg.seq == m.toast.seq {
			m.toast = toast{seq: m.toast.seq}
			m.undo = nil
		}
		return nil

	case searchLoadedMsg:
		// The command has finished either way, so its slot is released; the
		// search section applies the result.
		m.endLoading()
		return nil

	case backgroundCheckMsg:
		if m.dirty && !m.isLoading() && !m.reconciling && m.shared.now().Sub(m.lastMutationAt) >= backgroundSyncWindow {
			m.reconciling = true
			cmd := m.beginLoading(reconcileLibraryCmd(m.ctx, m.app))
			return tea.Batch(cmd, backgroundCheckCmd())
		}
		return backgroundCheckCmd()

	case opDoneMsg:
		return m.applyOpDone(msg)
	}
	return m.handleRequest(msg)
}

func (m *Model) applyLibraryLoaded(msg libraryLoadedMsg) tea.Cmd {
	m.endLoading()
	m.shared.loaded = true
	if msg.reconcile {
		m.reconciling = false
	}
	if msg.err != nil {
		// dirty stays set: the local mutations are still unreconciled.
		return m.showToast(toastError, msg.err.Error())
	}
	m.shared.reading = msg.reading
	m.shared.oku = msg.oku
	if msg.reconcile {
		// The pending local mutations are now reflected by the server data.
		m.dirty = false
	}
	cmd := m.broadcast(dataChangedMsg{dataLibrary})
	if msg.needsRefresh {
		refresh := m.beginLoading(loadLibraryCmd(m.ctx, m.app, true))
		return tea.Batch(cmd, refresh)
	}
	return cmd
}

func (m *Model) applyLocalDataLoaded(msg localDataLoadedMsg) tea.Cmd {
	m.endLoading()
	m.shared.localLoaded = true

	if msg.err != nil {
		return m.showToast(toastError, msg.err.Error())
	}
	m.shared.stats = msg.readingStats
	if msg.readingStats != nil {
		m.shared.weekly = msg.readingStats.Weekly
	}
	m.shared.sessions = msg.recentSessions
	m.mergeRecentSearches(msg.recentSearches)
	m.shared.timer = msg.timerState
	m.shared.timerBook = msg.timerBook
	m.shared.lastSyncAt = msg.lastSyncAt
	if msg.shelf != nil {
		m.shared.shelf = msg.shelf
	}
	return tea.Batch(m.startTimerTick(), m.broadcast(dataChangedMsg{dataLocal}))
}

// applyOpDone applies the result of a mutating operation: its slot, its
// toast, the dirty mark and the reload. The modal it belongs to, if any,
// sees it in the broadcast that follows.
func (m *Model) applyOpDone(msg opDoneMsg) tea.Cmd {
	m.endLoading()
	if msg.op == opSync {
		m.syncing = false
	}

	toastCmd := m.toastFor(msg)
	if msg.err == nil && msg.markDirty {
		m.dirty = true
		m.lastMutationAt = m.shared.now()
	}
	if msg.reload {
		reload := m.beginLoading(loadLibraryCmd(m.ctx, m.app, false), loadLocalDataCmd(m.app, m.shared.now))
		return tea.Batch(reload, toastCmd)
	}
	return toastCmd
}

// handleRequest runs what a section or modal asked for. This is the one
// place the in-flight guard applies and commands start.
func (m *Model) handleRequest(msg tea.Msg) tea.Cmd {
	switch r := msg.(type) {
	case reqToast:
		return m.showToast(r.level, r.text)

	case reqSwitchTab:
		switch {
		case r.back:
			return m.setTab(m.backTab())
		case r.abs:
			cmd := m.setTab(r.to)
			// A tab asked for by its own number is one the reader means to
			// use: a section whose pane has nothing to show yet puts the
			// cursor where they are about to type. Walking the strip never
			// does — see searchFocus.
			m.openEmptySectionInput()
			return cmd
		default:
			return m.setTab((m.tab + tab(r.step) + tabCount) % tabCount)
		}

	case reqOpenModal:
		m.push(r.m)
		return nil

	case reqHelp:
		m.openHelp()
		return nil

	case reqRunOp:
		return m.beginLoading(r.cmd)

	case reqSync:
		m.syncing = true
		return m.beginLoading(syncAllAndReloadCmd(m.ctx, m.app))

	case reqRefresh:
		if r.local {
			return m.beginLoading(loadLocalDataCmd(m.app, m.shared.now))
		}
		return m.beginLoading(loadLibraryCmd(m.ctx, m.app, true))

	case reqUndo:
		return m.runUndo()

	case reqDensity:
		return m.cycleDensity()

	case reqProgress:
		// UpdateProgress is read-modify-write, so firing a second one while
		// the first is in flight would silently lose an update.
		if m.isLoading() {
			return m.showToast(toastWarn, inFlightNotice)
		}
		b := r.book
		return m.beginLoading(quickProgressCmd(m.ctx, m.app, b.Book.ID, b.Book.Title, currentPage(b), r.delta))

	case reqSetPage:
		if m.isLoading() {
			return m.refuse(opProgress, r.token)
		}
		return m.beginLoading(stamped(updateProgressCmd(m.ctx, m.app, r.bookID, r.title, r.prevPage, r.raw), r.token))

	case reqChangeStatus:
		b := r.book
		change := changeStatusCmd(m.ctx, m.app, b.Book.ID, b.Book.Title, b.StatusID, r.to)
		if r.confirm {
			m.push(newConfirmModal(fmt.Sprintf("Mark '%s' as %s?", b.Book.Title, r.to.Label()), change))
			return nil
		}
		return m.beginLoading(change)

	case reqReview:
		if m.isLoading() {
			// The same guard the page prompt has: both modals go read-only
			// while they save, so both need the refusal to reach them.
			return m.refuse(opReview, r.token)
		}
		toastCmd := m.showToast(toastInfo, reviewSavePendingMessage(r.review))
		save := m.beginLoading(stamped(submitReviewRatingCmd(m.ctx, m.app, r.bookID, r.rating, r.review), r.token))
		return tea.Batch(save, toastCmd)

	case reqAddFromSearch:
		return m.beginLoading(addFromSearchCmd(m.ctx, m.app, r.result.ID, r.to))

	case reqSearch:
		toastCmd := m.showToast(toastInfo, fmt.Sprintf("%s mode (%s): searching for %q...",
			strings.ToLower(r.mode.Label()), r.mode.Description(), r.query))
		search := m.beginLoading(searchCmd(m.ctx, m.app, r.query, r.mode, r.seq))
		return tea.Batch(search, toastCmd)

	case reqSearchDone:
		// The label is the mode the results were fetched with; the user may
		// have switched modes since.
		label := strings.ToLower(r.mode.Label())
		var toastCmd tea.Cmd
		if r.results == 0 {
			toastCmd = m.showToast(toastInfo, fmt.Sprintf("%s mode: no results for %q", label, r.query))
		} else {
			toastCmd = m.showToast(toastSuccess, fmt.Sprintf("%s mode: loaded %d results", label, r.results))
		}
		save := m.addRecentSearchQuery(r.query)
		return tea.Batch(save, toastCmd, m.broadcast(dataChangedMsg{dataSearches}))

	case reqTimerToggle:
		if m.isLoading() {
			// timerState only catches up when the load lands, so two quick
			// presses would otherwise start two sessions.
			return m.showToast(toastWarn, inFlightNotice)
		}
		if m.shared.timer != nil {
			return m.beginLoading(stopTimerCmd(m.app))
		}
		if !r.reading {
			// The other lists hold books that are not being read, so t
			// there asks which Reading book to track rather than refusing.
			return m.handleRequest(reqTimerPick{prefer: r.book})
		}
		if r.book == nil {
			return m.showToast(toastError, "no book selected")
		}
		return m.beginLoading(startTimerForBookCmd(m.app, r.book.Book.ID))

	case reqTimerPick:
		if len(m.shared.reading) == 0 {
			return m.showToast(toastError, "no currently reading books available — add a book to Reading first")
		}
		m.push(newTimerPickerModal(m.shared, m.timerPickIndex(r.prefer)))
		return m.showToast(toastInfo, "Select a book and press Enter to start timer")

	case reqTimer:
		if r.start {
			return m.beginLoading(startTimerForBookCmd(m.app, r.bookID))
		}
		return m.beginLoading(stopTimerCmd(m.app))
	}
	return nil
}

// refuse answers a modal that asked to save while something else is still
// running. The modal is drawn over a blank screen, so the toast behind it
// cannot be read: the refusal is handed to the modal as its own failed
// result, which is the path that already shows a reason inside the panel.
// It is not an operation finishing, so it goes out as a broadcast and
// leaves the in-flight count alone.
func (m *Model) refuse(op opKind, token int) tea.Cmd {
	cmd := m.broadcast(opDoneMsg{op: op, seq: token, err: errors.New(inFlightNotice)})
	return tea.Batch(cmd, m.showToast(toastWarn, inFlightNotice))
}

// timerPickIndex is the book the picker opens on: prefer when the Reading
// list holds it — the selection t was pressed over — and the Reading list's
// own cursor otherwise.
func (m *Model) timerPickIndex(prefer *model.UserBook) int {
	wanted := []*model.UserBook{prefer, m.sections[tabReading].Selected().Book}
	for _, b := range wanted {
		if b == nil {
			continue
		}
		for i, r := range m.shared.reading {
			if r.Book.ID == b.Book.ID {
				return i
			}
		}
	}
	return 0
}

// addRecentSearchQuery puts a query at the head of the history and returns the
// command that writes it back to the store.
func (m *Model) addRecentSearchQuery(query string) tea.Cmd {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	m.shared.recentSearches = dedupeQueries(append([]string{query}, m.shared.recentSearches...))
	return saveRecentSearchesCmd(m.app, m.shared.recentSearches)
}

// mergeRecentSearches keeps this session's queries ahead of the ones read back
// from the store, so a load landing mid-session cannot drop them.
func (m *Model) mergeRecentSearches(loaded []string) {
	m.shared.recentSearches = dedupeQueries(append(append([]string(nil), m.shared.recentSearches...), loaded...))
}

// beginLoading counts one in-flight command per cmd and (re)starts the spinner
// tick loop. Each cmd must produce exactly one message whose handler calls
// endLoading.
func (m *Model) beginLoading(cmds ...tea.Cmd) tea.Cmd {
	for _, cmd := range cmds {
		if cmd != nil {
			m.inflight++
		}
	}
	return tea.Batch(append(cmds, m.startSpinner())...)
}

// endLoading records that one in-flight command has produced its result.
func (m *Model) endLoading() {
	if m.inflight > 0 {
		m.inflight--
	}
}

// isLoading reports whether any command is still running.
func (m *Model) isLoading() bool {
	return m.inflight > 0
}

// startSpinner returns the spinner tick only when work is in flight and no
// loop is armed, so overlapping operations never run two of them at once.
func (m *Model) startSpinner() tea.Cmd {
	if m.spinning || m.inflight == 0 {
		return nil
	}
	m.spinning = true
	return m.shared.spin.Tick
}

// startTimerTick arms the one-second tick that refreshes the elapsed time,
// but only while a timer runs and no tick loop is armed yet.
func (m *Model) startTimerTick() tea.Cmd {
	if m.timerTicking || m.shared.timer == nil {
		return nil
	}
	m.timerTicking = true
	return timerTickCmd()
}

// ── Focus and modals ───────────────────────────────────────────────────────

// setTab shows another tab. The keyboard goes back to the content pane: the
// detail pane of the tab being left has nothing to do with the new one.
func (m *Model) setTab(t tab) tea.Cmd {
	if t == m.tab {
		return nil
	}
	m.prevTab = m.tab
	m.section().Blur()
	m.tab = t
	m.focus = focusContent
	m.section().Focus()
	return m.resize()
}

// backTab is where Esc goes from a tab that was reached by name: the one it
// was reached from, or the first tab when that is this one.
func (m *Model) backTab() tab {
	if m.prevTab == m.tab {
		return tabReading
	}
	return m.prevTab
}

// setFocus moves the keyboard between the two panes. On a terminal too
// narrow to split, the detail pane takes the content pane's place instead of
// sitting beside it, so the sizes are recomputed either way.
func (m *Model) setFocus(f focus) tea.Cmd {
	if f == focusDetail && !m.tab.hasDetail() {
		return nil
	}
	if f == m.focus {
		return nil
	}
	m.focus = f
	return m.resize()
}

// resize recomputes the layout and pushes the two panes' sizes into the
// section and the detail pane. The library rows are drawn to the width they
// are given, so a resize rebuilds them — and a rebuild has a command that
// must be run, or an active filter goes blank.
func (m *Model) resize() tea.Cmd {
	m.lay = computeLayout(m.lay.W, m.lay.H, m.tab, m.focus == focusDetail)
	m.help.Width = m.helpBarWidth()

	cmd := m.section().Resize(m.lay.ContentInner, m.lay.InnerH)
	m.detail.Resize(m.lay.DetailInner, m.lay.InnerH)
	for _, mod := range m.modals {
		mod.Resize(m.lay)
	}
	return cmd
}

// topModal is the modal that takes the keys, or nil.
func (m *Model) topModal() modal {
	if n := len(m.modals); n > 0 {
		return m.modals[n-1]
	}
	return nil
}

// push puts a modal on top of the stack.
func (m *Model) push(mod modal) {
	m.modals = append(m.modals, mod)
	m.modalsChanged()
}

// pop drops the top modal.
func (m *Model) pop() {
	m.modals = m.modals[:len(m.modals)-1]
	m.modalsChanged()
}

// modalsChanged follows a push or a pop: a new modal is sized to the
// terminal, and an empty stack gives the section its focus back.
func (m *Model) modalsChanged() {
	if len(m.modals) == 0 {
		m.section().Focus()
	}
	for _, mod := range m.modals {
		mod.Resize(m.lay)
	}
}

// openHelp shows the help modal, scrolled back to the top.
func (m *Model) openHelp() {
	m.push(newHelpModal(m.keysBehind, m.version, m.st))
}

// ── View ───────────────────────────────────────────────────────────────────

func (m *Model) View() string {
	return m.fitToScreen(m.frame())
}

// frame renders the dashboard at its natural size. View clamps it to the
// terminal; keeping the two apart lets a test assert that the layout really
// fills the screen instead of being padded into it.
func (m *Model) frame() string {
	if !m.shared.loaded {
		return "\n  " + m.shared.spin.View() + " Loading dashboard..."
	}
	if top := m.topModal(); top != nil {
		// True compositing over the dashboard needs lipgloss v2; on v1 the
		// modal is centred over a blank screen.
		return fitBlock(overlayModal(m.lay, top.View(m.lay, m.st)),
			max(minFrameWidth, m.lay.W), max(1, m.lay.H))
	}
	return m.header(m.lay) + "\n" + m.body(m.lay) + "\n" + m.footer(m.lay)
}

// body is the row of panes between the header and the footer: the content
// pane, the detail pane beside it when the terminal is wide enough, or the
// detail pane on its own when Enter opened it on a narrow one.
func (m *Model) body(lay layout) string {
	sel := m.section().Selected()
	if lay.DetailOnly {
		return m.pane(m.detail.Title(sel), m.detail.View(sel, m.tab), lay.DetailW, lay.PaneH, true)
	}

	content := m.pane(
		m.section().Title(),
		m.section().View(lay.ContentInner, lay.InnerH),
		lay.ContentW, lay.PaneH,
		m.focus == focusContent || !lay.Split,
	)
	if !lay.Split {
		return content
	}
	detail := m.pane(
		m.detail.Title(sel),
		m.detail.View(sel, m.tab),
		lay.DetailW, lay.PaneH,
		m.focus == focusDetail,
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, content, detail)
}

// ── Run ────────────────────────────────────────────────────────────────────

// Run starts the dashboard on a, with density as the row detail the CLI's
// --view flag asked for and version as the build it reports. It returns when
// the user quits.
func Run(ctx context.Context, a *app.App, density Density, version string) error {
	// Bubble Tea runs commands in goroutines it does not track, so cancel the
	// command context as soon as the program exits: in-flight API calls abort
	// instead of racing the caller's store shutdown.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	p := tea.NewProgram(New(runCtx, a, density, version), tea.WithAltScreen())
	_, err := p.Run()
	cancel()
	return err
}
