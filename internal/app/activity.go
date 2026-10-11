package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

const (
	// Journal event names mirror Hardcover's reading_journals events.
	journalEventProgress = "progress_updated"
	journalEventFinished = "status_read"

	privacySettingKey = "account_privacy_setting_id"

	// privacyPublic is Hardcover's default privacy setting (1=Public,
	// 2=Followers, 3=Private), used when the account setting can't be fetched.
	privacyPublic = 1
)

// logLocalJournal mirrors a Hardcover journal event locally so the activity
// heatmap reflects it immediately; the server-side entry replaces it on the
// next sync. Failures are non-fatal: the remote update already succeeded.
func (a *App) logLocalJournal(event string) {
	_ = a.store.InsertLocalJournal(time.Now(), event)
}

// statusCountsAsActivity reports whether changing to the given status counts
// as reading activity for the heatmap. Only finishing a book qualifies.
func statusCountsAsActivity(s model.Status) bool {
	return s == model.StatusRead
}

// logRemoteProgress creates a Hardcover reading journal entry so the website's
// activity heatmap reflects progress logged through oku. Best-effort: the
// progress update itself already succeeded, and the journal only affects the
// website's activity display.
func (a *App) logRemoteProgress(ctx context.Context, bookID, page, totalPages int) {
	if !a.Online() {
		return
	}
	privacy, _ := a.privacy(ctx, false)
	_ = a.hc.InsertProgressJournal(ctx, bookID, page, totalPages, privacy)
}

// privacy returns the account's default privacy setting for journal entries
// and goals. Strict lookups, for anything the user writes, always ask
// Hardcover and fail when it can't answer: a personal note must never fall
// back to public. Lenient lookups, for progress telemetry, take the value
// cached by an earlier fetch, then ask Hardcover, then fall back to public,
// and never fail. Every successful fetch refreshes the cached value.
func (a *App) privacy(ctx context.Context, strict bool) (int, error) {
	if !strict {
		if val, err := a.store.GetState(privacySettingKey); err == nil {
			if id, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && id > 0 {
				return id, nil
			}
		}
	}

	id, err := a.fetchPrivacy(ctx)
	if err != nil {
		if strict {
			return 0, err
		}
		return privacyPublic, nil
	}
	_ = a.store.SetState(privacySettingKey, strconv.Itoa(id))
	return id, nil
}

func (a *App) fetchPrivacy(ctx context.Context) (int, error) {
	// Every caller checks this already; it stays so fetchPrivacy is safe on its own.
	if err := a.requireOnline(); err != nil {
		return 0, err
	}
	id, err := a.hc.GetAccountPrivacySetting(ctx)
	if err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, fmt.Errorf("invalid account privacy setting %d", id)
	}
	return id, nil
}
