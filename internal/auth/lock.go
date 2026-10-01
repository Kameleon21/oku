package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kameleon21/oku/internal/config"
	"github.com/gofrs/flock"
)

const (
	lockFileName = "refresh.lock"
	// lockTimeout must outlast the refresh request's own 10s HTTP timeout.
	lockTimeout   = 15 * time.Second
	lockPollEvery = 50 * time.Millisecond
)

var refreshMu sync.Mutex

// lockRefresh serializes token refreshes across goroutines and oku processes.
// If the file lock can't be taken it falls back to the in-process mutex only,
// so a broken data dir never blocks the user.
func lockRefresh(ctx context.Context) (unlock func()) {
	refreshMu.Lock()
	unlockFile, err := lockFile(ctx)
	if err != nil {
		return refreshMu.Unlock
	}
	return func() {
		unlockFile()
		refreshMu.Unlock()
	}
}

func lockFile(ctx context.Context) (unlock func(), err error) {
	dir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, lockTimeout)
	defer cancel()

	fl := flock.New(filepath.Join(dir, lockFileName))
	locked, err := fl.TryLockContext(ctx, lockPollEvery)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, fmt.Errorf("refresh lock not acquired")
	}
	return func() { _ = fl.Unlock() }, nil
}
