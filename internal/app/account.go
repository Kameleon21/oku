package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Kameleon21/oku/internal/model"
)

const userIDKey = "user_id"

// accountStateKeys are the state entries that belong to one Hardcover
// account rather than to this device: the per-status sync stamps, the
// reading queue order, the account's privacy setting and the active books.
func accountStateKeys() []string {
	keys := []string{queueStateKey, privacySettingKey, activeBookIDKey, activeBookIDsKey}
	for _, status := range model.AllStatuses {
		keys = append(keys, syncStateKey(status))
	}
	return keys
}

// RememberUser records the Hardcover account the client is signed in as.
// When a different account was recorded before, everything cached for it is
// dropped first, so its library is never shown as the new account's. With
// no account recorded yet nothing is dropped. The new ID is stored last: a
// clear that fails is retried the next time.
func (a *App) RememberUser(id int) error {
	prev, err := a.store.GetState(userIDKey)
	if err != nil {
		return err
	}
	if p, err := strconv.Atoi(strings.TrimSpace(prev)); err == nil && p > 0 && p != id {
		if err := a.store.ClearAccount(accountStateKeys()); err != nil {
			return fmt.Errorf("clear the previous account's cache: %w", err)
		}
	}
	if err := a.store.SetState(userIDKey, strconv.Itoa(id)); err != nil {
		return fmt.Errorf("record the signed-in account: %w", err)
	}
	return nil
}
