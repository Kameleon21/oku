package tui

import (
	"strings"
	"testing"
	"time"
)

// TestStatsFinishingNextFollowsTheLibrary: the forecast block is measured on
// the Reading list, which arrives with the library rather than the local
// data, so a library change has to rebuild the memoised page.
func TestStatsFinishingNextFollowsTheLibrary(t *testing.T) {
	m := newGoldenModel(t, 120, 40, tabStats)
	s := statsOf(m)
	if page := stripANSI(s.View(118, 36)); !strings.Contains(page, "Money") {
		t.Fatalf("the demo's open reads should be forecast:\n%s", page)
	}

	m.shared.reading = nil
	m.broadcast(dataChangedMsg{dataLibrary})
	if page := stripANSI(s.View(118, 36)); strings.Contains(page, "Finishing next") {
		t.Fatalf("an empty Reading list should drop the block:\n%s", page)
	}
}

// TestStatsPageRebuildsAtMidnight: the forecasts are dated against today,
// so the memo keys on the reader's date.
func TestStatsPageRebuildsAtMidnight(t *testing.T) {
	m := newGoldenModel(t, 120, 40, tabStats)
	s := statsOf(m)
	before := stripANSI(s.View(118, 36))

	m.shared.now = func() time.Time { return fixedNow.AddDate(0, 0, 1) }
	if after := stripANSI(s.View(118, 36)); after == before {
		t.Fatal("a new day should rebuild the page with new estimates")
	}
}
