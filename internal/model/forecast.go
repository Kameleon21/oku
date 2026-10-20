package model

import (
	"math"
	"sort"
	"time"
)

// MaxForecastDays is the furthest out a finish date is still worth stating:
// past two years the pace says nothing a reader could plan around.
const MaxForecastDays = 730

// minForecastPace is the slowest pace, in pages a day, that still yields a
// finish date. Below it the read has stalled rather than slowed.
const minForecastPace = 0.5

// ReadPace is how fast an open read is going and when it would end at that
// rate.
//
// The pace assumes the read began at page 0: user_book_reads records where a
// read started in time but not in pages, and neither the timer sessions nor
// the journal entries carry a page locally. A book picked up at its halfway
// mark therefore reads as faster than it was.
type ReadPace struct {
	Page        int
	Pages       int
	Days        int     // whole days since the read started; 0 = started today
	PagesPerDay float64 // 0 when the read started today
	DaysLeft    int     // days to the last page at this pace; 0 without a Finish
	// Finish is the day the last page falls on at this pace, zero when there
	// is no honest estimate: the read started today, is already on its last
	// page, has stalled, or would run past MaxForecastDays.
	Finish time.Time `json:",omitzero"`
}

// Stalled reports a pace too slow to put a date on.
func (p ReadPace) Stalled() bool {
	return p.Days > 0 && p.Page < p.Pages && p.PagesPerDay < minForecastPace
}

// ForecastRead is the pace of a read started at started and standing at page
// of pages, as of now, which is in the reader's zone. started is taken as a
// calendar date (see DaysBetween).
func ForecastRead(started, now time.Time, page, pages int) ReadPace {
	p := ReadPace{Page: page, Pages: pages, Days: DaysBetween(started, now)}
	if p.Days <= 0 {
		return p
	}
	p.PagesPerDay = float64(page) / float64(p.Days)
	remaining := pages - page
	if remaining <= 0 || p.PagesPerDay < minForecastPace {
		return p
	}
	left := int(math.Ceil(float64(remaining) / p.PagesPerDay))
	if left > MaxForecastDays {
		return p
	}
	p.DaysLeft = left
	p.Finish = calendarDay(now, now.Location()).AddDate(0, 0, left)
	return p
}

// ForecastBook is ForecastRead for a library entry. ok is false unless the
// book is being read, has a page count and a start date, and has been opened
// past its first page: anything else would be a guess presented as an
// estimate.
func ForecastBook(ub UserBook, now time.Time) (ReadPace, bool) {
	page := ub.Page()
	if ub.StatusID != StatusCurrentlyReading || page <= 0 || ub.Book.Pages <= 0 {
		return ReadPace{}, false
	}
	if len(ub.UserBookReads) == 0 || ub.UserBookReads[0].StartedAt == nil {
		return ReadPace{}, false
	}
	return ForecastRead(*ub.UserBookReads[0].StartedAt, now, page, ub.Book.Pages), true
}

// BookForecast is one open read's pace, under the book it belongs to.
type BookForecast struct {
	BookID int
	Title  string
	ReadPace
}

// ForecastShelf is the pace of every book in books that ForecastBook can
// measure, the soonest finish first and the reads without a date after them
// in the order given.
func ForecastShelf(books []UserBook, now time.Time) []BookForecast {
	out := make([]BookForecast, 0, len(books))
	for _, ub := range books {
		if p, ok := ForecastBook(ub, now); ok {
			out = append(out, BookForecast{BookID: ub.BookID, Title: ub.Book.Title, ReadPace: p})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Finish, out[j].Finish
		if a.IsZero() || b.IsZero() {
			return !a.IsZero() && b.IsZero()
		}
		return a.Before(b)
	})
	return out
}

// GoalPace is where a reading goal stands against an even pace through its
// window.
type GoalPace struct {
	Metric   string
	Target   int
	Progress float64
	// End is the goal's last day; the window runs to the end of it.
	End time.Time
	// Expected is where an even pace from the start would stand now, and
	// Ahead is Progress less that: negative when behind.
	Expected float64
	Ahead    float64
	// PerWeek is the rate from now that lands exactly on Target by End.
	PerWeek float64
	// Projected is where the pace kept so far lands by End, 0 until a day of
	// the window has passed.
	Projected int
	// DaysLeft counts the days still in the window, today included.
	DaysLeft int
}

// Reached reports a goal already met.
func (g GoalPace) Reached() bool { return g.Progress >= float64(g.Target) }

// Remaining is what is still to read to meet the goal, never negative.
func (g GoalPace) Remaining() float64 { return math.Max(0, float64(g.Target)-g.Progress) }

// ForecastGoal measures g against an even pace as of now, a day at a time:
// a day counts once it has passed, so every reading of the goal on one day
// agrees, whichever screen it is read on. ok is false when the goal has no
// target or no window, or today falls outside the window.
//
// The window is the goal's calendar days, start and end inclusive, read as
// the reader's dates (see DaysBetween): a goal ending on 31 Dec runs to the
// reader's midnight, not UTC's.
func ForecastGoal(g Goal, now time.Time) (GoalPace, bool) {
	if g.Target <= 0 || g.StartDate.IsZero() || g.EndDate.IsZero() {
		return GoalPace{}, false
	}
	total := DaysBetween(g.StartDate, g.EndDate) + 1
	elapsed := DaysBetween(g.StartDate, now)
	if total <= 0 || elapsed < 0 || elapsed >= total {
		return GoalPace{}, false
	}

	fraction := float64(elapsed) / float64(total)
	p := GoalPace{
		Metric:   g.Metric,
		Target:   g.Target,
		Progress: g.Progress,
		End:      calendarDay(g.EndDate, now.Location()),
		Expected: float64(g.Target) * fraction,
		DaysLeft: total - elapsed,
	}
	p.Ahead = p.Progress - p.Expected
	p.PerWeek = p.Remaining() / (float64(p.DaysLeft) / 7)
	if elapsed > 0 {
		p.Projected = int(math.Round(p.Progress / fraction))
	}
	return p, true
}

// Ended reports a goal whose last day is behind the reader, read as
// calendar dates like ForecastGoal's window. A goal without an end date
// never ends.
func (g Goal) Ended(now time.Time) bool {
	return !g.EndDate.IsZero() && DaysBetween(g.EndDate, now) > 0
}

// Covers reports a goal whose window includes now's date. A missing start
// or end leaves that side of the window open.
func (g Goal) Covers(now time.Time) bool {
	started := g.StartDate.IsZero() || DaysBetween(g.StartDate, now) >= 0
	return started && !g.Ended(now)
}

// DaysBetween counts calendar days from a's date to b's, each read in its
// own zone. Hardcover sends bare dates, which parse as UTC midnight and mean
// that day wherever the reader is, so they must not be converted: west of
// Greenwich the instant is the evening before. now, for its part, is
// already in the reader's zone, so a book started last night at 23:50 is a
// day old this morning, not an hour.
func DaysBetween(a, b time.Time) int {
	da := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	db := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}

// calendarDay is t's own calendar date as midnight in loc.
func calendarDay(t time.Time, loc *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
