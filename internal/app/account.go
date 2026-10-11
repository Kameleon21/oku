package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Kameleon21/oku/internal/model"
)

const (
	userIDKey   = "user_id"
	usernameKey = "username"
)

// accountStateKeys are the state entries that belong to one Hardcover
// account rather than to this device.
var accountStateKeys = []string{
	usernameKey,
	queueStateKey,
	privacySettingKey,
	activeBookIDKey,
	activeBookIDsKey,
}

// rememberUser records the Hardcover account the client is signed in as.
// When a different account was recorded before, everything cached for it is
// dropped first, so its library is never shown as the new account's. The new
// ID is stored last: a clear that fails is retried the next time.
func (a *App) rememberUser(id int) error {
	prev, err := a.store.GetState(userIDKey)
	if err != nil {
		return err
	}
	if p, err := strconv.Atoi(strings.TrimSpace(prev)); err == nil && p > 0 && p != id {
		if err := a.clearAccountCache(); err != nil {
			return fmt.Errorf("clear the previous account's cache: %w", err)
		}
	}
	return a.store.SetState(userIDKey, strconv.Itoa(id))
}

// clearAccountCache removes the cached library, its sync stamps, the synced
// journals and goals and the per-account state. Device-local data (timer
// sessions, private notes, search history) stays.
func (a *App) clearAccountCache() error {
	for _, status := range model.AllStatuses {
		if err := a.store.DeleteUserBooksByStatus(status); err != nil {
			return err
		}
		if err := a.store.DeleteState(syncStateKey(status)); err != nil {
			return err
		}
	}
	for _, key := range accountStateKeys {
		if err := a.store.DeleteState(key); err != nil {
			return err
		}
	}
	if err := a.store.ReplaceReadingJournals(nil); err != nil {
		return err
	}
	return a.store.ReplaceGoals(nil)
}
