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
	errLockTimeout = errors.New("timed out waiting for refresh lock")
)

// lockRefresh serializes token refreshes across goroutines and processes.
// unlock is always safe to call. An error other than errLockTimeout or a
// context error means the file lock is unavailable; the caller may refresh
// under the in-process mutex alone.
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
	_, err = fl.TryLockContext(tCtx, lockPollEvery)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return nil, errLockTimeout
		}
		return nil, err
	}

	// locked
	return func() { _ = fl.Unlock() }, nil
}
