package app

import (
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

const (
	activityProgress = "progress"
	activityFinished = "finished"
)

// logActivity records a local activity event for streak tracking.
// Failures are non-fatal: the remote update already succeeded, and the
// activity log only affects streak display.
func (a *App) logActivity(bookID int, event string) {
	_ = a.Store.InsertActivity(bookID, event, time.Now().UTC())
}

// statusCountsAsActivity reports whether changing to the given status counts
// as reading activity for the streak. Only finishing a book qualifies.
func statusCountsAsActivity(s model.Status) bool {
	return s == model.StatusRead
}
