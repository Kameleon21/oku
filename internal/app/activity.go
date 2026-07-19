package app

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

const (
	activityProgress = "progress"
	activityFinished = "finished"

	privacySettingKey = "account_privacy_setting_id"

	// privacyPublic is Hardcover's default privacy setting (1=Public,
	// 2=Followers, 3=Private), used when the account setting can't be fetched.
	privacyPublic = 1
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

// logRemoteProgress creates a Hardcover reading journal entry so the website's
// activity heatmap reflects progress logged through oku. Best-effort: the
// progress update itself already succeeded, and the journal only affects the
// website's activity display.
func (a *App) logRemoteProgress(ctx context.Context, bookID, page, totalPages int) {
	_ = a.API.InsertProgressJournal(ctx, bookID, page, totalPages, a.accountPrivacySettingID(ctx))
}

// accountPrivacySettingID returns the user's default privacy setting for
// journal entries, cached in local state after the first fetch. Falls back to
// public when the setting can't be determined.
func (a *App) accountPrivacySettingID(ctx context.Context) int {
	if val, err := a.Store.GetState(privacySettingKey); err == nil {
		if id, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && id > 0 {
			return id
		}
	}

	id, err := a.API.GetAccountPrivacySetting(ctx)
	if err != nil || id <= 0 {
		return privacyPublic
	}
	_ = a.Store.SetState(privacySettingKey, strconv.Itoa(id))
	return id
}
