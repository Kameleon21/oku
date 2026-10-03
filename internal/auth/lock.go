package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Kameleon21/oku/internal/config"
	"github.com/gofrs/flock"
)

const (
	lockFileName  = "refresh.lock"
	lockPollEvery = 50 * time.Millisecond
)

// lockTimeout must outlast the refresh request's own 10s HTTP timeout.
var lockTimeout = 15 * time.Second

var refreshMu sync.Mutex

var (
	ErrLockTimeout = errors.New("timed out waiting for refresh lock")
	ErrNoLock      = errors.New("refresh lock not acquired")
)

// lockRefresh serializes token refreshes across goroutines and oku processes.
// If the file lock can't be taken it falls back to the in-process mutex only,
// so a broken data dir never blocks the user.
func lockRefresh(ctx context.Context) (unlock func(), err error) {
	if ctx.Err() != nil {
		return func() {}, ctx.Err()
	}
	refreshMu.Lock()
	unlockFile, err := lockFile(ctx)
	if err != nil {
		return refreshMu.Unlock, err
	}
	return func() {
		unlockFile()
		refreshMu.Unlock()
	}, nil
}

func lockFile(ctx context.Context) (unlock func(), err error) {
	dir, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}

	tCtx, cancel := context.WithTimeout(ctx, lockTimeout)
	defer cancel()

	fl := flock.New(filepath.Join(dir, lockFileName))
	locked, err := fl.TryLockContext(tCtx, lockPollEvery)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return nil, ErrLockTimeout
		}
		return nil, err
	}
	if !locked {
		return nil, ErrNoLock
	}
	return func() { _ = fl.Unlock() }, nil
}
