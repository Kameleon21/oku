package app

import (
	"errors"
	"fmt"

	"github.com/Kameleon21/oku/internal/model"
)

// ErrCacheRefresh reports that a write reached Hardcover but a local step
// after it failed: the cache upsert, the active-books bookkeeping or the
// post-insert sync. Every write-through mutation (ChangeStatus,
// UpdateProgress, RateBook, ReviewBook) wraps such failures in it, and only
// those: an error before or from the remote write is returned plain. The
// remote change stands, so callers should report success and warn rather
// than fail; a retry could repeat the change, an insert most of all.
var ErrCacheRefresh = errors.New("book updated on Hardcover but the local cache could not be refreshed; run `oku sync`")

// errNotCached marks a write to a book that is missing from the cached
// library.
var errNotCached = errors.New("not found in cache. Run: oku sync")

// loadForWrite is the common start of a write-through mutation: it resolves
// bookID (0 means the single active book), requires Hardcover and loads the
// cached user book. A book missing from the cache is an error wrapping
// errNotCached, returned with the resolved ID so ChangeStatus can add it.
func (a *App) loadForWrite(bookID int) (int, *model.UserBook, error) {
	resolvedID, err := a.ResolveBookID(bookID)
	if err != nil {
		return 0, nil, err
	}
	if err := a.requireOnline(); err != nil {
		return 0, nil, err
	}
	ub, err := a.store.GetUserBookByBookID(resolvedID)
	if err != nil {
		return 0, nil, err
	}
	if ub == nil {
		return resolvedID, nil, fmt.Errorf("book ID %d %w", resolvedID, errNotCached)
	}
	return resolvedID, ub, nil
}

// cacheRefreshed wraps a local failure that followed a successful remote
// write in ErrCacheRefresh. A nil err stays nil.
func cacheRefreshed(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrCacheRefresh, err)
}
