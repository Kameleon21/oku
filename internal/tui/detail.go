package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const okuASCII = `   ____  __ __  __  __
  / __ \/ //_/ / / / /
 / / / / ,<   / / / /
/ /_/ / /| | / /_/ /
\____/_/ |_| \____/   `

// detailLabelW is the label column every field row lines up against.
const detailLabelW = 13

// detailStackWidth is the width below which the two-column grid stacks: two
// label columns and their values do not fit in less.
const detailStackWidth = 56

// maxDetailSessions caps the session list: the rest of a book's history is
// the Timer tab's business, not the detail pane's.
const maxDetailSessions = 4

// detailPane is the right-hand pane: everything known about whatever the
// content pane has selected. It renders into a viewport, so a long review
// scrolls instead of pushing the rows under it out of the box.
//
// The content is rebuilt only when the selection, the data behind it or the
// pane's size actually changes; a render otherwise costs a viewport slice.
type detailPane struct {
	sh *shared
	st styles
	vp viewport.Model

	w, h int
	// key is what the rendered content was built from. A different key is
	// the one reason to build it again.
	key detailKey
	// stamp is bumped by a data change and copied into the key at each
	// render. It is deliberately not part of the key itself: a counter the
	// comparison could see would advance on both sides at once and never
	// register as a difference.
	stamp int
	// title is the pane title for the content on screen.
	title string
}

// detailKey identifies the content in the viewport: what it is, which one,
// and everything about the frame that would change how it is drawn.
type detailKey struct {
	kind    string
	id      int
	updated time.Time
	density Density
	w       int
	// stamp is the pane's data counter as it stood when the content was
	// built, so a book whose sessions or shelf moved under it is rendered
	// again even though its id has not changed.
	stamp int
}

func newDetailPane(sh *shared, st styles) *detailPane {
	return &detailPane{sh: sh, st: st, vp: viewport.New(1, 1)}
}

// Update takes the broadcast messages the pane cares about: a data change
// invalidates what is on screen.
func (d *detailPane) Update(msg tea.Msg) tea.Cmd {
	if _, ok := msg.(dataChangedMsg); ok {
		d.stamp++
	}
	return nil
}

// handleKey scrolls the pane while it has the focus. It reports whether it
// took the key: everything else falls through to the section, so the shelf
// and progress keys still act on the selection whose detail is on screen.
//
// g and G are deliberately not scroll keys here: they set a shelf in the
// library, and a detail pane that stole them would be a trap.
func (d *detailPane) handleKey(msg tea.KeyMsg, k keyMap) bool {
	switch {
	case key.Matches(msg, k.Down):
		d.vp.LineDown(1)
	case key.Matches(msg, k.Up):
		d.vp.LineUp(1)
	case key.Matches(msg, k.HalfPageDown):
		d.vp.HalfViewDown()
	case key.Matches(msg, k.HalfPageUp):
		d.vp.HalfViewUp()
	default:
		return false
	}
	return true
}

// Resize follows the pane's box. The width is part of the render, so the
// content is rebuilt at the next View.
func (d *detailPane) Resize(w, h int) {
	d.w, d.h = w, h
	d.vp.Width, d.vp.Height = max(1, w), max(1, h)
}

// Keys relabels the bindings for a focused detail pane: j/k scroll it, Esc
// gives the keyboard back to the list. The section's own action keys stay
// enabled, since they still act on the selection.
func (d *detailPane) Keys(k *keyMap) {
	k.Up.SetHelp("k", "scroll")
	k.Down.SetHelp("j", "scroll")
	k.Back.SetHelp("Esc", "back to list")
	enable(&k.Up, &k.Down, &k.Back, &k.HalfPageUp, &k.HalfPageDown)
	// Enter is what got here; it has nowhere left to go.
	k.Details.SetEnabled(false)

	// The scroll and the way out come first; the actions the section still
	// answers for keep their places behind them. The bar holds copies of the
	// bindings, so the hints that no longer apply are dropped by hand rather
	// than by disabling the originals.
	details := k.Details.Help()
	rest := make([]key.Binding, 0, len(k.short))
	for _, b := range k.short {
		if h := b.Help(); h.Key == "?" || h.Key == "j/k" || h.Key == "k/j" || h == details {
			continue
		}
		rest = append(rest, b)
	}
	k.short = append([]key.Binding{k.Help, hint("scroll", k.Down, k.Up), k.Back}, rest...)
}

// Title is the pane title for a selection: the book or result on screen.
func (d *detailPane) Title(sel selection) string {
	switch {
	case sel.Book != nil:
		return sel.Book.Book.Title
	case sel.Result != nil:
		return sel.Result.Title
	default:
		return "Details"
	}
}

// View renders the selection into the pane's box, rebuilding the content
// only when something about it has changed.
func (d *detailPane) View(sel selection, t tab) string {
	k := detailKey{density: d.sh.density, w: d.w, stamp: d.stamp}
	switch {
	case sel.Book != nil:
		k.kind, k.id, k.updated = "book", sel.Book.Book.ID, sel.Book.UpdatedAt
	case sel.Result != nil:
		k.kind, k.id = "result", sel.Result.ID
	default:
		k.kind = "empty:" + t.name()
	}

	if k != d.key {
		d.key = k
		d.vp.SetContent(d.render(sel, t))
		d.vp.GotoTop()
	}
	d.title = d.Title(sel)
	return d.vp.View()
}

func (d *detailPane) render(sel selection, t tab) string {
	switch {
	case sel.Book != nil:
		return renderUserBook(*sel.Book, d.sh.sessions, d.sh.now(), d.w, d.sh.density, d.st)
	case sel.Result != nil:
		return renderSearchResult(*sel.Result, d.sh.shelf, d.w, d.st)
	case t == tabSearch:
		return d.searchEmptyState()
	default:
		return d.emptyState()
	}
}

// emptyState is what the pane shows with nothing selected: the logo, and
// what to press to fill it.
func (d *detailPane) emptyState() string {
	st := d.st
	return "\n" + st.head.Render(okuASCII) + "\n\n" +
		st.dim.Render("  j/k to pick a book")
}

// searchEmptyState is the Search tab's version: what to type, and what has
// been typed before.
func (d *detailPane) searchEmptyState() string {
	st := d.st
	out := "\n" + st.head.Render(okuASCII) + "\n\n" + st.dim.Render("  Type a query")
	if recent := d.sh.recentSearches; len(recent) > 0 {
		if len(recent) > 3 {
			recent = recent[:3]
		}
		out += "\n\n" + st.dim.Render(ansi.Truncate("  Recent: "+strings.Join(recent, " · "), max(1, d.w), "…"))
	}
	return out
}

// ── Library book ───────────────────────────────────────────────────────────

// renderUserBook is everything the dashboard knows about a book on a shelf.
// A row with no data behind it is left out rather than filled with a dash,
// except in the two-column grid, where an empty cell would break the shape.
func renderUserBook(ub model.UserBook, sessions []model.ReadingSession, now time.Time, w int, density Density, st styles) string {
	if w <= 0 {
		return ""
	}
	b := ub.Book
	var sb strings.Builder
	line := func(s string) { sb.WriteString(s + "\n") }

	line(st.dim.Render(cut(authorLine(b), w)))
	line(st.value.Render(cut(statusLine(ub), w)))
	line("")

	page := currentPage(ub)
	line(progressRow(page, b.Pages, w, st))
	if pace := paceLine(ub, page, now); pace != "" {
		line(st.dim.Render(cut(pace, w)))
	}
	line("")

	// Two columns of labelled fields, stacked when the pane is too narrow
	// for the second one to start halfway across.
	rating, community := "", ""
	if ub.Rating > 0 {
		rating = fmt.Sprintf("%s %.1f", model.StarString(ub.Rating), ub.Rating)
	}
	if b.Rating > 0 {
		community = fmt.Sprintf("★ %.2f", b.Rating)
		if b.RatingsCount > 0 {
			community += fmt.Sprintf(" (%s)", format.Count(b.RatingsCount))
		}
	}
	series := "—"
	if b.FeaturedSeries != "" {
		series = b.FeaturedSeries
		if b.FeaturedSeriesPosition > 0 {
			series += fmt.Sprintf(" #%d", b.FeaturedSeriesPosition)
		}
	}
	for _, row := range gridRows(w, st,
		[2]string{"Your rating", rating}, [2]string{"Community", community},
		[2]string{"Series", series}, [2]string{"Released", releaseYear(b.ReleaseDate)},
	) {
		line(row)
	}
	if genres := model.TagsForCategory(b.CachedTags, "Genre"); len(genres) > 0 {
		if len(genres) > 5 {
			genres = genres[:5]
		}
		line(field("Genres", strings.Join(genres, " · "), w, st))
	}

	if block := sessionsBlock(ub, sessions, now, w, st); block != "" {
		line("")
		sb.WriteString(block)
	}
	if block := reviewBlock(ub.Review, w, st); block != "" {
		line("")
		sb.WriteString(block)
	}

	if density == DensityVerbose {
		line("")
		meta := fmt.Sprintf("id %d", b.ID)
		if b.Slug != "" {
			meta = "hardcover.app/books/" + b.Slug + " · " + meta
		}
		line(st.dim.Render(cut(meta, w)))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// authorLine names the authors, folding a long list so one book cannot take
// the whole row: two names and how many more there are.
func authorLine(b model.Book) string {
	switch n := len(b.Authors); {
	case n == 0:
		return "Unknown author"
	case n > 3:
		return fmt.Sprintf("%s, %s +%d", b.Authors[0], b.Authors[1], n-2)
	default:
		return b.AuthorString()
	}
}

// statusLine is the shelf the book is on, and the dates of the open read.
func statusLine(ub model.UserBook) string {
	out := ub.StatusID.Label()
	if len(ub.UserBookReads) == 0 {
		return out
	}
	read := ub.UserBookReads[0]
	if read.StartedAt != nil {
		out += " · started " + read.StartedAt.Local().Format("2 Jan 2006")
	}
	if read.FinishedAt != nil {
		out += " · finished " + read.FinishedAt.Local().Format("2 Jan 2006")
	}
	return out
}

// progressRow is the bar and the page count on one row, the bar sized to
// whatever the numbers leave: the numbers are the point, so they are never
// the part that is cut.
func progressRow(page, pages, w int, st styles) string {
	if pages <= 0 {
		return st.dim.Render(cut(fmt.Sprintf("p.%d", page), w))
	}
	suffix := fmt.Sprintf("  p.%d / %d · %d%%", page, pages, int(percent(page, pages)*100))
	barW := clampInt(w-lipgloss.Width(suffix), 0, 48)
	if barW == 0 {
		return st.dim.Render(cut(strings.TrimSpace(suffix), w))
	}
	return bar(page, pages, barW, st) + st.dim.Render(suffix)
}

// paceLine is how fast the book is being read and when it would be finished
// at that rate. It is only shown for an open read with a start date and a
// page count: anything else would be a guess presented as an estimate.
func paceLine(ub model.UserBook, page int, now time.Time) string {
	if ub.StatusID != model.StatusCurrentlyReading || page <= 0 || ub.Book.Pages <= 0 {
		return ""
	}
	if len(ub.UserBookReads) == 0 || ub.UserBookReads[0].StartedAt == nil {
		return ""
	}
	return paceAndETA(*ub.UserBookReads[0].StartedAt, now, page, ub.Book.Pages)
}

// paceAndETA is the pace-and-estimate row for a read started at started and
// standing at page of pages.
//
// It assumes the read began at page 0: user_book_reads records where a read
// started in time but not in pages, and neither the timer sessions nor the
// journal entries carry a page locally. A book picked up at its halfway mark
// therefore reads as faster than it was.
func paceAndETA(started, now time.Time, page, pages int) string {
	days := daysBetween(started, now)
	if days <= 0 {
		return "started today"
	}
	pace := float64(page) / float64(days)

	paceStr := "<1 page/day"
	if pace >= 1 {
		paceStr = plural(int(math.Round(pace)), "page") + "/day"
	}

	remaining := pages - page
	if remaining <= 0 {
		return paceStr
	}
	if pace < 0.5 {
		return paceStr + " · pace too low to estimate"
	}
	etaDays := int(math.Ceil(float64(remaining) / pace))
	left, ok := humanDays(etaDays)
	if !ok {
		return paceStr
	}
	// The year is only worth a column when the estimate leaves this one.
	eta := now.AddDate(0, 0, etaDays)
	layout := "2 Jan"
	if eta.Year() != now.Year() {
		layout = "2 Jan 2006"
	}
	return fmt.Sprintf("%s · ~%s left (≈ %s)", paceStr, left, eta.Format(layout))
}

// daysBetween counts whole days from a to b, bucketed to the reader's own
// midnight: a book started last night at 23:50 is a day old this morning,
// not an hour.
func daysBetween(a, b time.Time) int {
	from := midnight(a.Local())
	to := midnight(b.Local())
	return int(math.Round(to.Sub(from).Hours() / 24))
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// humanDays renders a span at the coarsest unit that is still honest about
// it. Past two years the estimate says nothing worth printing, so it is not
// printed at all.
func humanDays(d int) (string, bool) {
	switch {
	case d <= 0:
		return "<1 day", true
	case d == 1:
		return "1 day", true
	case d <= 14:
		return fmt.Sprintf("%d days", d), true
	case d <= 70:
		return plural(int(math.Round(float64(d)/7)), "week"), true
	case d <= 730:
		return plural(int(math.Round(float64(d)/30)), "month"), true
	default:
		return "", false
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// releaseYear keeps only the year of a release date, which is all a full
// ISO date says that is worth a column here.
func releaseYear(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 4 && raw[:4] >= "0000" && raw[:4] <= "9999" {
		return raw[:4]
	}
	return raw
}

// sessionsBlock lists this book's own timer sessions, newest first.
func sessionsBlock(ub model.UserBook, sessions []model.ReadingSession, now time.Time, w int, st styles) string {
	rows := make([]string, 0, maxDetailSessions)
	for _, s := range sessions {
		if s.BookID != ub.Book.ID || len(rows) == maxDetailSessions {
			continue
		}
		rows = append(rows, st.value.Render(cut(fmt.Sprintf("  %-6s %s   %s",
			format.DayLabel(s.StartedAt, now), format.Clock(s.StartedAt), format.Duration(s.Duration())), w)))
	}
	if len(rows) == 0 {
		if ub.StatusID != model.StatusCurrentlyReading {
			return ""
		}
		return st.dim.Render(cut("No timer sessions yet · t to start", w)) + "\n"
	}
	return st.label.Render("Sessions (this book)") + "\n" + strings.Join(rows, "\n") + "\n"
}

// reviewBlock is the user's own review, wrapped to the pane. It is not cut:
// a review is the one thing here that is worth reading in full, and the
// viewport scrolls whatever does not fit.
func reviewBlock(review string, w int, st styles) string {
	review = strings.TrimSpace(review)
	if review == "" {
		return ""
	}
	lines := strings.Split(lipgloss.NewStyle().Width(max(1, w-2)).Render(review), "\n")
	for i, l := range lines {
		lines[i] = st.value.Render("  " + l)
	}
	return st.label.Render("Your review") + "\n" + strings.Join(lines, "\n") + "\n"
}

// ── Search result ──────────────────────────────────────────────────────────

// renderSearchResult is what is known about a book that is not on a shelf
// yet: what Hardcover's search returns, plus whether the library already has
// it. Series, release date and genres arrive with the detail fetch; the rows
// switch on as soon as the data does.
func renderSearchResult(r model.SearchResult, shelf map[int]model.UserBook, w int, st styles) string {
	if w <= 0 {
		return ""
	}
	var sb strings.Builder
	line := func(s string) { sb.WriteString(s + "\n") }

	author := strings.Join(r.Authors, ", ")
	if author == "" {
		author = "Unknown author"
	}
	line(st.dim.Render(cut(author, w)))
	line("")

	if r.Rating > 0 {
		community := fmt.Sprintf("★ %.2f", r.Rating)
		if r.Ratings > 0 {
			community += fmt.Sprintf(" (%s ratings)", format.Count(r.Ratings))
		}
		line(field("Community", community, w, st))
	}
	if r.Pages > 0 {
		line(field("Pages", fmt.Sprintf("%d", r.Pages), w, st))
	}

	if ub, ok := shelf[r.ID]; ok {
		on := "On your shelf: " + ub.StatusID.Label()
		if ub.Rating > 0 {
			on += fmt.Sprintf("  %s %.1f", model.StarString(ub.Rating), ub.Rating)
		}
		if ub.StatusID == model.StatusCurrentlyReading {
			on += "  p." + ub.Progress()
		}
		line("")
		line(st.value.Render(cut(on, w)))
	}

	if r.Slug != "" {
		line("")
		line(st.dim.Render(cut("hardcover.app/books/"+r.Slug, w)))
	}
	line("")
	line(st.dim.Render(cut("↵ add as reading · w want to read · f finished · d dnf", w)))
	return strings.TrimRight(sb.String(), "\n")
}

// ── Row helpers ────────────────────────────────────────────────────────────

// field is one labelled row: a fixed label column, then the value in what is
// left of the pane.
func field(label, value string, w int, st styles) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return st.label.Render(fmt.Sprintf("%-*s", detailLabelW, label)) +
		st.value.Render(cut(value, max(1, w-detailLabelW)))
}

// gridRows lays labelled cells out two to a row, the second starting halfway
// across, and stacks them when the pane is too narrow for that. Cells with no
// value are dropped, but a pair that keeps one cell keeps the row's shape.
func gridRows(w int, st styles, cells ...[2]string) []string {
	rows := make([]string, 0, len(cells)/2)
	for i := 0; i < len(cells); i += 2 {
		left := cells[i]
		right := [2]string{}
		if i+1 < len(cells) {
			right = cells[i+1]
		}
		if w < detailStackWidth {
			for _, c := range [][2]string{left, right} {
				if row := field(c[0], c[1], w, st); row != "" {
					rows = append(rows, row)
				}
			}
			continue
		}

		half := w / 2
		leftRow := field(left[0], left[1], half, st)
		rightRow := field(right[0], right[1], w-half, st)
		switch {
		case leftRow == "" && rightRow == "":
			continue
		case rightRow == "":
			rows = append(rows, leftRow)
		default:
			if pad := half - lipgloss.Width(leftRow); pad > 0 {
				leftRow += strings.Repeat(" ", pad)
			}
			rows = append(rows, leftRow+rightRow)
		}
	}
	return rows
}

// cut trims a value to the columns it has, on a glyph boundary.
func cut(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}
