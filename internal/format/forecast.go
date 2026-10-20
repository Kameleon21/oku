package format

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// ReadPace is the pace-and-estimate line for an open read, e.g.
// "30 pages/day · ~3 days left (≈ Thursday)". The estimate is left off
// whenever model.ForecastRead declined to make one.
func ReadPace(p model.ReadPace, now time.Time) string {
	if p.Days <= 0 {
		return "started today"
	}
	pace := "<1 page/day"
	if p.PagesPerDay >= 1 {
		pace = Plural(int(math.Round(p.PagesPerDay)), "page") + "/day"
	}
	if p.Stalled() {
		return pace + " · pace too low to estimate"
	}
	if p.Finish.IsZero() {
		return pace
	}
	left, ok := HumanDays(p.DaysLeft)
	if !ok {
		return pace
	}
	return fmt.Sprintf("%s · ~%s left (≈ %s)", pace, left, FinishDay(p.Finish, now))
}

// FinishDay names the day an estimate lands on as a reader would say it:
// "today", "tomorrow", the weekday while it is still this week, and the date
// after that, with the year only once it leaves this one.
func FinishDay(t, now time.Time) string {
	switch d := model.DaysBetween(now, t); {
	case d <= 0:
		return "today"
	case d == 1:
		return "tomorrow"
	case d < 7:
		return t.Local().Weekday().String()
	}
	if t.Local().Year() != now.Local().Year() {
		return t.Local().Format("2 Jan 2006")
	}
	return t.Local().Format("2 Jan")
}

// GoalPace is where a goal stands and what it takes from here, e.g.
// "2 books behind · 1.4 books a week gets you to 24". Ahead of schedule it
// says where the current pace lands instead, since no rate is needed.
func GoalPace(p model.GoalPace) string {
	if p.Reached() {
		return "goal reached"
	}
	status := "on track"
	switch d := int(math.Round(p.Ahead)); {
	case d <= -1:
		status = metricCount(-d, p.Metric) + " behind"
	case d >= 1:
		status = metricCount(d, p.Metric) + " ahead"
		if p.Projected > 0 {
			return status + " · on pace for " + Thousands(p.Projected)
		}
	}
	if p.DaysLeft <= 7 {
		more, unit := int(math.Ceil(p.Remaining())), p.Metric
		if more == 1 {
			unit = singular(unit)
		}
		return fmt.Sprintf("%s · %s more %s by %s", status, Thousands(more), unit, p.End.Format("2 Jan"))
	}
	return fmt.Sprintf("%s · %s a week gets you to %s", status, metricRate(p.PerWeek, p.Metric), Thousands(p.Target))
}

// metricCount is n of a goal's unit: "1 book", "3 books", "1,200 pages".
func metricCount(n int, metric string) string {
	if n == 1 {
		return "1 " + singular(metric)
	}
	return Thousands(n) + " " + metric
}

// metricRate is a weekly rate in a goal's unit, rounded up so that keeping
// to it never falls short. A book rate under ten keeps a decimal, since 1.4
// a week and 1 a week are different plans; anything else is a whole number,
// since a tenth of a page is not. The epsilon keeps float noise, 0.3 a week
// stored as 0.30000000000000004, from rounding up a whole tenth.
func metricRate(rate float64, metric string) string {
	const eps = 1e-9
	if rate >= 10 || metric != "books" {
		return metricCount(int(math.Ceil(rate-eps)), metric)
	}
	r := math.Ceil(rate*10-eps) / 10
	if r == 1 {
		return "1 " + singular(metric)
	}
	return strconv.FormatFloat(r, 'f', -1, 64) + " " + metric
}

func singular(metric string) string { return strings.TrimSuffix(metric, "s") }

// HumanDays renders a span at the coarsest unit that is still honest about
// it. Past model.MaxForecastDays the estimate says nothing worth printing,
// so ok is false.
func HumanDays(d int) (string, bool) {
	switch {
	case d <= 0:
		return "<1 day", true
	case d == 1:
		return "1 day", true
	case d <= 14:
		return fmt.Sprintf("%d days", d), true
	case d <= 70:
		return Plural(int(math.Round(float64(d)/7)), "week"), true
	case d <= model.MaxForecastDays:
		return Plural(int(math.Round(float64(d)/30)), "month"), true
	default:
		return "", false
	}
}

// Plural is n of unit with an English plural s.
func Plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
