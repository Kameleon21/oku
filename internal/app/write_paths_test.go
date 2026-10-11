package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/model"
)

func assertCalls(t *testing.T, hc *fakeHardcover, method string, want ...string) {
	t.Helper()
	if got := hc.called(method); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s calls = %q, want %q", method, got, want)
	}
}

func assertActiveIDs(t *testing.T, a *App, want []int) {
	t.Helper()
	ids, err := a.GetActiveBookIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("active IDs = %v, want %v", ids, want)
	}
}

func cachedUserBook(t *testing.T, a *App, bookID int) *model.UserBook {
	t.Helper()
	ub, err := a.CachedUserBook(bookID)
	if err != nil {
		t.Fatal(err)
	}
	if ub == nil {
		t.Fatalf("book %d missing from cache", bookID)
	}
	return ub
}

// localJournalCount counts journal rows dated within a day of now.
func localJournalCount(t *testing.T, a *App) int {
	t.Helper()
	now := time.Now()
	days, err := a.store.GetJournalDays(now.AddDate(0, 0, -1), now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range days {
		n += d.Count
	}
	return n
}

func TestChangeStatusInsertsBookMissingFromLibrary(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)

	if err := a.ChangeStatus(context.Background(), 42, model.StatusCurrentlyReading); err != nil {
		t.Fatal(err)
	}

	assertCalls(t, hc, "InsertUserBook", "InsertUserBook(42, 2)")
	assertCalls(t, hc, "UpdateUserBookStatus")
	// The insert is followed by a sync of the target status so the cache
	// holds Hardcover's full row.
	assertCalls(t, hc, "ListUserBooks", "ListUserBooks(2)")
	ub := cachedUserBook(t, a, 42)
	if ub.ID != 1001 || ub.StatusID != model.StatusCurrentlyReading {
		t.Fatalf("cached user book = %+v, want Hardcover's row 1001 in reading", ub)
	}
	if _, stale, _ := a.ListCachedBooks(model.StatusCurrentlyReading); stale {
		t.Fatal("reading cache still stale after the post-insert sync")
	}
	assertActiveIDs(t, a, []int{42})
}

func TestChangeStatusUpdatesCachedBookAndActiveList(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusWantToRead, "Dune")
	before := time.Now().UTC().Add(-time.Second)

	if err := a.ChangeStatus(context.Background(), 1, model.StatusCurrentlyReading); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "UpdateUserBookStatus", "UpdateUserBookStatus(100, 2)")
	assertCalls(t, hc, "InsertUserBook")
	assertCalls(t, hc, "ListUserBooks")
	ub := cachedUserBook(t, a, 1)
	if ub.StatusID != model.StatusCurrentlyReading || ub.UpdatedAt.Before(before) {
		t.Fatalf("cached user book = %+v, want reading with a fresh updated_at", ub)
	}
	assertActiveIDs(t, a, []int{1})
	if n := localJournalCount(t, a); n != 0 {
		t.Fatalf("starting a book logged %d activity entries, want 0", n)
	}

	if err := a.ChangeStatus(context.Background(), 1, model.StatusRead); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "UpdateUserBookStatus", "UpdateUserBookStatus(100, 2)", "UpdateUserBookStatus(100, 3)")
	if ub := cachedUserBook(t, a, 1); ub.StatusID != model.StatusRead {
		t.Fatalf("status = %v, want read", ub.StatusID)
	}
	assertActiveIDs(t, a, nil)
	if n := localJournalCount(t, a); n != 1 {
		t.Fatalf("finishing a book logged %d activity entries, want 1", n)
	}
}

func TestChangeStatusUsesSingleActiveBook(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	upsertReadingBook(t, a, 100, 1, "Dune")
	if err := a.AddActiveBook(1); err != nil {
		t.Fatal(err)
	}

	if err := a.ChangeStatus(context.Background(), 0, model.StatusPaused); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "UpdateUserBookStatus", "UpdateUserBookStatus(100, 4)")
	assertActiveIDs(t, a, nil)
}

func TestChangeStatusReportsCacheRefreshFailureAfterInsert(t *testing.T) {
	hc := newFakeHardcover()
	hc.errs = map[string]error{"ListUserBooks": errors.New("network down")}
	a := newTestAppWith(t, hc)

	err := a.ChangeStatus(context.Background(), 42, model.StatusCurrentlyReading)
	if !errors.Is(err, ErrCacheRefresh) || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("err = %v, want ErrCacheRefresh carrying the sync failure", err)
	}
	assertCalls(t, hc, "InsertUserBook", "InsertUserBook(42, 2)")
	if ub, err := a.CachedUserBook(42); err != nil || ub != nil {
		t.Fatalf("cached user book = %+v, %v; want none after the failed sync", ub, err)
	}
	// The remote insert stands, so the book still becomes active.
	assertActiveIDs(t, a, []int{42})
}

func TestChangeStatusInsertFailureIsPlainError(t *testing.T) {
	hc := newFakeHardcover()
	hc.errs = map[string]error{"InsertUserBook": errors.New("rejected")}
	a := newTestAppWith(t, hc)

	err := a.ChangeStatus(context.Background(), 42, model.StatusCurrentlyReading)
	if err == nil || errors.Is(err, ErrCacheRefresh) {
		t.Fatalf("err = %v, want a plain failure", err)
	}
	assertCalls(t, hc, "ListUserBooks")
	assertActiveIDs(t, a, nil)
}

func TestChangeStatusRejectedUpdateLeavesCache(t *testing.T) {
	hc := newFakeHardcover()
	hc.errs = map[string]error{"UpdateUserBookStatus": errors.New("rejected")}
	a := newTestAppWith(t, hc)
	upsertReadingBook(t, a, 100, 1, "Dune")
	if err := a.AddActiveBook(1); err != nil {
		t.Fatal(err)
	}

	if err := a.ChangeStatus(context.Background(), 1, model.StatusRead); err == nil {
		t.Fatal("expected error")
	}
	if ub := cachedUserBook(t, a, 1); ub.StatusID != model.StatusCurrentlyReading {
		t.Fatalf("status = %v, want reading unchanged", ub.StatusID)
	}
	assertActiveIDs(t, a, []int{1})
	if n := localJournalCount(t, a); n != 0 {
		t.Fatalf("failed update logged %d activity entries", n)
	}
}

func TestUpdateProgressUpdatesOpenRead(t *testing.T) {
	hc := newFakeHardcover()
	hc.privacy = 3
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusCurrentlyReading, "Dune")
	started := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := a.store.UpsertUserBookRead(model.UserBookRead{ID: 500, UserBookID: 100, ProgressPages: 20, StartedAt: &started}); err != nil {
		t.Fatal(err)
	}

	page, err := a.UpdateProgress(context.Background(), 1, model.PageUpdate{Delta: 15, Relative: true})
	if err != nil {
		t.Fatal(err)
	}
	if page != 35 {
		t.Fatalf("page = %d, want 35", page)
	}
	assertCalls(t, hc, "UpdateReadProgress", "UpdateReadProgress(500, 35)")
	assertCalls(t, hc, "InsertUserBookRead")
	assertCalls(t, hc, "InsertProgressJournal", "InsertProgressJournal(1, 35, 100, 3)")
	read, err := a.store.GetLatestRead(100)
	if err != nil || read == nil || read.ID != 500 || read.ProgressPages != 35 {
		t.Fatalf("latest read = %+v, %v; want read 500 at page 35", read, err)
	}
	if v, _ := a.store.GetState(privacySettingKey); v != "3" {
		t.Fatalf("privacy setting cached as %q, want 3", v)
	}
	if n := localJournalCount(t, a); n != 1 {
		t.Fatalf("logged %d activity entries, want 1", n)
	}
}

func TestUpdateProgressStartsReadAfterFinishedOne(t *testing.T) {
	hc := newFakeHardcover()
	startedAt := "2026-10-01"
	hc.readStartedAt = &startedAt
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusCurrentlyReading, "Dune")
	oldStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	oldEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if err := a.store.UpsertUserBookRead(model.UserBookRead{ID: 400, UserBookID: 100, ProgressPages: 100, StartedAt: &oldStart, FinishedAt: &oldEnd}); err != nil {
		t.Fatal(err)
	}

	// Relative to the new read, not the finished one's 100 pages.
	page, err := a.UpdateProgress(context.Background(), 1, model.PageUpdate{Delta: 10, Relative: true})
	if err != nil {
		t.Fatal(err)
	}
	if page != 10 {
		t.Fatalf("page = %d, want 10", page)
	}
	assertCalls(t, hc, "InsertUserBookRead", "InsertUserBookRead(100, 10)")
	assertCalls(t, hc, "UpdateReadProgress")
	read, err := a.store.GetLatestRead(100)
	if err != nil || read == nil {
		t.Fatalf("latest read = %+v, %v", read, err)
	}
	if read.ID != 1001 || read.ProgressPages != 10 || read.StartedAt == nil || !read.StartedAt.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("latest read = %+v, want new read 1001 at page 10 started 2026-10-01", read)
	}
}

func TestUpdateProgressRequiresCachedBook(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	if _, err := a.UpdateProgress(context.Background(), 5, model.PageUpdate{Absolute: 10}); err == nil {
		t.Fatal("expected error for uncached book")
	}
	if len(hc.calls) != 0 {
		t.Fatalf("reached Hardcover: %v", hc.calls)
	}
}

func TestUpdateProgressRejectedLeavesCache(t *testing.T) {
	hc := newFakeHardcover()
	hc.errs = map[string]error{"UpdateReadProgress": errors.New("rejected")}
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusCurrentlyReading, "Dune")
	if err := a.store.UpsertUserBookRead(model.UserBookRead{ID: 500, UserBookID: 100, ProgressPages: 20}); err != nil {
		t.Fatal(err)
	}

	if _, err := a.UpdateProgress(context.Background(), 1, model.PageUpdate{Absolute: 50}); err == nil {
		t.Fatal("expected error")
	}
	if read, _ := a.store.GetLatestRead(100); read == nil || read.ProgressPages != 20 {
		t.Fatalf("latest read = %+v, want page 20 unchanged", read)
	}
	assertCalls(t, hc, "InsertProgressJournal")
}

func TestRateBookUpdatesHardcoverThenCache(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusRead, "Dune")

	if err := a.RateBook(context.Background(), 1, 4.5); err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "UpdateUserBookRating", "UpdateUserBookRating(100, 4.5)")
	if ub := cachedUserBook(t, a, 1); ub.Rating != 4.5 {
		t.Fatalf("cached rating = %v, want 4.5", ub.Rating)
	}

	if err := a.RateBook(context.Background(), 1, 4.3); err == nil {
		t.Fatal("accepted a rating off the half-star grid")
	}
	hc.errs = map[string]error{"UpdateUserBookRating": errors.New("rejected")}
	if err := a.RateBook(context.Background(), 1, 2); err == nil {
		t.Fatal("expected error")
	}
	assertCalls(t, hc, "UpdateUserBookRating", "UpdateUserBookRating(100, 4.5)", "UpdateUserBookRating(100, 2)")
	if ub := cachedUserBook(t, a, 1); ub.Rating != 4.5 {
		t.Fatalf("cached rating = %v after rejected update, want 4.5", ub.Rating)
	}
}

func TestReviewBookUpdatesHardcoverThenCache(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusRead, "Dune")

	if err := a.ReviewBook(context.Background(), 1, 4, "Spice must flow"); err != nil {
		t.Fatal(err)
	}
	ub := cachedUserBook(t, a, 1)
	if ub.ReviewedAt == nil {
		t.Fatal("reviewed_at not cached")
	}
	date := ub.ReviewedAt.UTC().Format(time.DateOnly)
	assertCalls(t, hc, "UpdateUserBookReviewAndRating", "UpdateUserBookReviewAndRating(100, 4, Spice must flow, "+date+")")
	if ub.Rating != 4 || ub.Review != "Spice must flow" {
		t.Fatalf("cached user book = %+v, want rating 4 and the review", ub)
	}

	hc.errs = map[string]error{"UpdateUserBookReviewAndRating": errors.New("rejected")}
	if err := a.ReviewBook(context.Background(), 1, 2, "Changed my mind"); err == nil {
		t.Fatal("expected error")
	}
	if ub := cachedUserBook(t, a, 1); ub.Review != "Spice must flow" || ub.Rating != 4 {
		t.Fatalf("cached user book = %+v after rejected update", ub)
	}
}

func TestMoveQueueBookQueuesMissingShelfBooksThenSwaps(t *testing.T) {
	hc := newFakeHardcover()
	hc.queueListID = 77
	hc.queue = []api.RankedBook{{ID: 11, BookID: 1, Position: 1}, {ID: 12, BookID: 2, Position: 2}}
	a := newTestAppWith(t, hc)
	for i, id := range []int{1, 2, 3} {
		upsertBookWithStatus(t, a, 100+i, id, model.StatusWantToRead, "Book")
	}

	ids, err := a.MoveQueueBook(context.Background(), 3, -1)
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "ReadingQueue", "ReadingQueue(true)", "ReadingQueue(false)")
	assertCalls(t, hc, "QueueBooks", "QueueBooks(77)")
	if want := [][]api.RankedBook{{{BookID: 3, Position: 3}}}; !reflect.DeepEqual(hc.queued, want) {
		t.Fatalf("queued %+v, want %+v", hc.queued, want)
	}
	assertCalls(t, hc, "SetQueuePositions", "SetQueuePositions(77)")
	if want := [][]api.RankedBook{{{ID: 1001, BookID: 3, Position: 2}, {ID: 12, BookID: 2, Position: 3}}}; !reflect.DeepEqual(hc.positionUpdates, want) {
		t.Fatalf("position updates %+v, want %+v", hc.positionUpdates, want)
	}
	if !reflect.DeepEqual(ids, []int{1, 3, 2}) {
		t.Fatalf("ids = %v, want [1 3 2]", ids)
	}
	if cached, err := a.CachedQueueOrder(); err != nil || !reflect.DeepEqual(cached, ids) {
		t.Fatalf("cached order = %v, %v; want %v", cached, err, ids)
	}
}

func TestMoveQueueBookAtEdgeChangesNothingRemotely(t *testing.T) {
	hc := newFakeHardcover()
	hc.queueListID = 77
	hc.queue = []api.RankedBook{{ID: 11, BookID: 1, Position: 1}, {ID: 12, BookID: 2, Position: 2}}
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusWantToRead, "One")
	upsertBookWithStatus(t, a, 101, 2, model.StatusWantToRead, "Two")

	ids, err := a.MoveQueueBook(context.Background(), 1, -1)
	if err != nil {
		t.Fatal(err)
	}
	assertCalls(t, hc, "QueueBooks")
	assertCalls(t, hc, "SetQueuePositions")
	if !reflect.DeepEqual(ids, []int{1, 2}) {
		t.Fatalf("ids = %v, want [1 2]", ids)
	}
}

func TestMoveQueueBookRejectsBadInput(t *testing.T) {
	hc := newFakeHardcover()
	hc.queueListID = 77
	hc.queue = []api.RankedBook{{ID: 11, BookID: 1, Position: 1}, {ID: 12, BookID: 2, Position: 1}}
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusWantToRead, "One")
	upsertBookWithStatus(t, a, 101, 2, model.StatusWantToRead, "Two")

	if _, err := a.MoveQueueBook(context.Background(), 1, 2); err == nil {
		t.Fatal("accepted delta 2")
	}
	if _, err := a.MoveQueueBook(context.Background(), 99, 1); err == nil {
		t.Fatal("moved a book that is not on the shelf")
	}
	if len(hc.calls) != 0 {
		t.Fatalf("invalid moves reached Hardcover: %v", hc.calls)
	}
	if _, err := a.MoveQueueBook(context.Background(), 1, 1); err == nil || !strings.Contains(err.Error(), "duplicate ranks") {
		t.Fatalf("err = %v, want duplicate ranks refusal", err)
	}
	assertCalls(t, hc, "SetQueuePositions")
}

func TestSyncAllReplacesCacheAndRefreshesStats(t *testing.T) {
	hc := newFakeHardcover()
	rating := 4.0
	started := "2026-09-01"
	actionAt := "2026-10-01T10:00:00Z"
	hc.userBooks[model.StatusCurrentlyReading] = []api.APIUserBook{{
		ID: 200, StatusID: 2, Rating: &rating,
		Book:          api.APIBook{ID: 2, Title: "Remote", Pages: 300},
		UserBookReads: []api.APIUserBookRead{{ID: 300, ProgressPages: 50, StartedAt: &started}},
	}}
	hc.userBooks[model.StatusRead] = []api.APIUserBook{{ID: 201, StatusID: 3, Book: api.APIBook{ID: 3, Title: "Done"}}}
	hc.goals = []api.APIGoal{{ID: 9, Goal: 20, Metric: "books", State: "active"}}
	hc.journals = []api.APIReadingJournal{{ID: 1, ActionAt: &actionAt, Event: journalEventProgress}}
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusCurrentlyReading, "Gone remotely")

	if err := a.SyncAll(context.Background()); err != nil {
		t.Fatal(err)
	}

	want := make([]string, 0, len(model.AllStatuses))
	for _, s := range model.AllStatuses {
		want = append(want, "ListUserBooks("+strconv.Itoa(int(s))+")")
	}
	assertCalls(t, hc, "ListUserBooks", want...)
	if ub, _ := a.CachedUserBook(1); ub != nil {
		t.Fatalf("book removed remotely is still cached: %+v", ub)
	}
	ub := cachedUserBook(t, a, 2)
	if ub.ID != 200 || ub.Rating != 4 || ub.Book.Title != "Remote" {
		t.Fatalf("cached user book = %+v", ub)
	}
	if read, _ := a.store.GetLatestRead(200); read == nil || read.ID != 300 || read.ProgressPages != 50 {
		t.Fatalf("latest read = %+v, want read 300 at page 50", read)
	}
	if finished, _ := a.CachedBooks(model.StatusRead); len(finished) != 1 || finished[0].BookID != 3 {
		t.Fatalf("finished shelf = %+v", finished)
	}
	if a.LastSyncAt().IsZero() {
		t.Fatal("sync time not recorded")
	}

	assertCalls(t, hc, "ListReadingJournals", "ListReadingJournals(1)")
	if v, _ := a.store.GetState("user_id"); v != "1" {
		t.Fatalf("user_id cached as %q, want 1", v)
	}
	goals, err := a.store.ListGoals()
	if err != nil || len(goals) != 1 || goals[0].ID != 9 || goals[0].Target != 20 {
		t.Fatalf("goals = %+v, %v", goals, err)
	}
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if days, _ := a.store.GetJournalDays(day.AddDate(0, 0, -1), day.AddDate(0, 0, 1)); len(days) != 1 {
		t.Fatalf("journal days = %+v, want the synced entry", days)
	}
}

func TestSyncAllFailureKeepsPreviousCache(t *testing.T) {
	hc := newFakeHardcover()
	hc.errs = map[string]error{"ListUserBooks": errors.New("network down")}
	a := newTestAppWith(t, hc)
	upsertBookWithStatus(t, a, 100, 1, model.StatusWantToRead, "Cached")

	err := a.SyncAll(context.Background())
	if err == nil || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("err = %v", err)
	}
	cachedUserBook(t, a, 1)
	assertCalls(t, hc, "ListGoals")
}

func TestSyncAllIgnoresStatsFailure(t *testing.T) {
	hc := newFakeHardcover()
	hc.errs = map[string]error{"ListGoals": errors.New("goals down")}
	a := newTestAppWith(t, hc)
	if err := a.SyncAll(context.Background()); err != nil {
		t.Fatalf("stats failure failed the library sync: %v", err)
	}
}

func TestSaveGoalCreatesWithAccountPrivacy(t *testing.T) {
	hc := newFakeHardcover()
	hc.privacy = 2
	a := newTestAppWith(t, hc)
	g := api.GoalInput{Goal: 24, Metric: "book", StartDate: "2026-01-01", EndDate: "2026-12-31", PrivacySettingID: 3}

	id, err := a.SaveGoal(context.Background(), 0, g, func(old, next api.GoalInput) api.GoalInput {
		t.Fatal("merge called when creating")
		return next
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != 1001 {
		t.Fatalf("id = %d, want 1001", id)
	}
	assertCalls(t, hc, "GoalSettings")
	g.PrivacySettingID = 2
	if !reflect.DeepEqual(hc.savedGoals, []api.GoalInput{g}) {
		t.Fatalf("saved %+v, want %+v", hc.savedGoals, g)
	}
}

func TestSaveGoalMergesIntoExistingSettings(t *testing.T) {
	hc := newFakeHardcover()
	hc.goalSettings = map[int]api.GoalInput{5: {Goal: 10, Metric: "book", Description: "old", StartDate: "2026-01-01", EndDate: "2026-12-31", PrivacySettingID: 3}}
	a := newTestAppWith(t, hc)

	id, err := a.SaveGoal(context.Background(), 5, api.GoalInput{Goal: 30, Metric: "page"}, func(old, next api.GoalInput) api.GoalInput {
		old.Goal = next.Goal
		return old
	})
	if err != nil || id != 5 {
		t.Fatalf("id %d, err %v", id, err)
	}
	assertCalls(t, hc, "GetAccountPrivacySetting")
	want := api.GoalInput{Goal: 30, Metric: "book", Description: "old", StartDate: "2026-01-01", EndDate: "2026-12-31", PrivacySettingID: 3}
	if !reflect.DeepEqual(hc.savedGoals, []api.GoalInput{want}) {
		t.Fatalf("saved %+v, want %+v", hc.savedGoals, want)
	}
}

func TestOfflineAppReturnsErrOffline(t *testing.T) {
	for name, a := range map[string]*App{
		"nil interface":   newTestAppWith(t, nil),
		"nil *api.Client": newTestAppWith(t, (*api.Client)(nil)),
	} {
		t.Run(name, func(t *testing.T) {
			if a.Online() {
				t.Fatal("Online() = true without a client")
			}
			upsertBookWithStatus(t, a, 100, 1, model.StatusCurrentlyReading, "Dune")
			upsertBookWithStatus(t, a, 101, 2, model.StatusWantToRead, "Queued")
			ctx := context.Background()
			newRows := []TransferBook{{BookID: 3, Status: "oku"}}
			ops := map[string]func() error{
				"ChangeStatus":   func() error { return a.ChangeStatus(ctx, 1, model.StatusRead) },
				"UpdateProgress": func() error { _, err := a.UpdateProgress(ctx, 1, model.PageUpdate{Absolute: 5}); return err },
				"RateBook":       func() error { return a.RateBook(ctx, 1, 3) },
				"ReviewBook":     func() error { return a.ReviewBook(ctx, 1, 3, "ok") },
				"SyncAll":        func() error { return a.SyncAll(ctx) },
				"ListBooks":      func() error { _, err := a.ListBooks(ctx, model.StatusPaused, false); return err },
				"SearchBooks":    func() error { _, err := a.SearchBooks(ctx, "dune", 5, ""); return err },
				"BookJournal":    func() error { _, err := a.BookJournal(ctx, 1); return err },
				"ListGoals":      func() error { _, err := a.ListGoals(ctx); return err },
				"SaveGoal":       func() error { _, err := a.SaveGoal(ctx, 0, api.GoalInput{}, nil); return err },
				"GetBookDetail":  func() error { _, err := a.GetBookDetail(ctx, 1, false); return err },
				"TrendingBooks":  func() error { _, err := a.TrendingBooks(ctx, "week", 5, false); return err },
				"RefreshQueue":   func() error { _, err := a.RefreshQueue(ctx); return err },
				"MoveQueueBook":  func() error { _, err := a.MoveQueueBook(ctx, 2, 1); return err },
				"AddJournal":     func() error { _, err := a.AddJournalEntry(ctx, 1, "note", "hi"); return err },
				"ExportBooks":    func() error { _, err := a.ExportBooks(ctx); return err },
				"ImportBooks":    func() error { _, err := a.ImportBooks(ctx, newRows, false); return err },
			}
			for op, fn := range ops {
				if err := fn(); !errors.Is(err, ErrOffline) {
					t.Errorf("%s: err = %v, want ErrOffline", op, err)
				}
			}
			if ub := cachedUserBook(t, a, 1); ub.StatusID != model.StatusCurrentlyReading || ub.Rating != 0 {
				t.Fatalf("offline calls changed the cache: %+v", ub)
			}
		})
	}
}

func TestSaveGoalNilMergeSavesInputAsGiven(t *testing.T) {
	hc := newFakeHardcover()
	hc.goalSettings = map[int]api.GoalInput{5: {Goal: 10, Metric: "book", Description: "old"}}
	a := newTestAppWith(t, hc)
	g := api.GoalInput{Goal: 30, Metric: "page"}

	if _, err := a.SaveGoal(context.Background(), 5, g, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hc.savedGoals, []api.GoalInput{g}) {
		t.Fatalf("saved %+v, want %+v", hc.savedGoals, g)
	}
}

func TestSaveGoalUnknownGoalSavesNothing(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	_, err := a.SaveGoal(context.Background(), 5, api.GoalInput{Goal: 30}, func(old, next api.GoalInput) api.GoalInput { return next })
	if err == nil || !strings.Contains(err.Error(), "goal 5 not found") {
		t.Fatalf("err = %v, want goal not found", err)
	}
	assertCalls(t, hc, "SaveGoal")
}

func TestRefreshQueueWithoutListCachesEmptyOrder(t *testing.T) {
	hc := newFakeHardcover()
	a := newTestAppWith(t, hc)
	if err := a.saveQueue([]int{4, 5}); err != nil {
		t.Fatal(err)
	}

	ids, err := a.RefreshQueue(context.Background())
	if err != nil || len(ids) != 0 {
		t.Fatalf("ids = %v, err = %v; want an empty queue", ids, err)
	}
	assertCalls(t, hc, "ReadingQueue", "ReadingQueue(false)")
	if cached, _ := a.CachedQueueOrder(); len(cached) != 0 {
		t.Fatalf("cached order = %v, want empty", cached)
	}
}

func TestOfflineAppReportsBadInputFirst(t *testing.T) {
	a := newTestApp(t)
	upsertBookWithStatus(t, a, 100, 1, model.StatusRead, "Dune")
	ctx := context.Background()
	checks := map[string]error{
		"RateBook":      a.RateBook(ctx, 1, 9),
		"ReviewBook":    a.ReviewBook(ctx, 1, 9, "x"),
		"MoveQueueBook": func() error { _, err := a.MoveQueueBook(ctx, 1, 1); return err }(),
		"ImportBooks": func() error {
			_, err := a.ImportBooks(ctx, []TransferBook{{BookID: 1, Status: "invented"}}, false)
			return err
		}(),
	}
	for op, err := range checks {
		if err == nil || errors.Is(err, ErrOffline) {
			t.Errorf("%s: err = %v, want the validation error", op, err)
		}
	}
	if ids, err := a.ImportBooks(ctx, nil, false); err != nil || len(ids) != 0 {
		t.Errorf("empty import offline = %v, %v; want no results and no error", ids, err)
	}
}

// TestWriteThroughFailurePolicy pins the one policy every write-through
// mutation follows: a failure before or from the remote write is a plain
// error that leaves the cache alone, and a local failure after a successful
// remote write is ErrCacheRefresh.
func TestWriteThroughFailurePolicy(t *testing.T) {
	ctx := context.Background()
	reading := func(t *testing.T, a *App) {
		upsertBookWithStatus(t, a, 100, 1, model.StatusCurrentlyReading, "Dune")
	}
	cases := []struct {
		name   string
		method string
		setup  func(t *testing.T, a *App)
		run    func(a *App) (page int, err error)
		// wantPage is what UpdateProgress reports; 0 for the others.
		wantPage int
		prefix   string
	}{
		{
			name: "ChangeStatus update", method: "UpdateUserBookStatus", setup: reading,
			run:    func(a *App) (int, error) { return 0, a.ChangeStatus(ctx, 1, model.StatusRead) },
			prefix: "update status: ",
		},
		{
			name: "ChangeStatus insert", method: "InsertUserBook", setup: func(*testing.T, *App) {},
			run:    func(a *App) (int, error) { return 0, a.ChangeStatus(ctx, 42, model.StatusCurrentlyReading) },
			prefix: "add book to library: ",
		},
		{
			name: "UpdateProgress open read", method: "UpdateReadProgress",
			setup: func(t *testing.T, a *App) {
				reading(t, a)
				if err := a.store.UpsertUserBookRead(model.UserBookRead{ID: 500, UserBookID: 100, ProgressPages: 20}); err != nil {
					t.Fatal(err)
				}
			},
			run:      func(a *App) (int, error) { return a.UpdateProgress(ctx, 1, model.PageUpdate{Absolute: 50}) },
			wantPage: 50, prefix: "update progress: ",
		},
		{
			name: "UpdateProgress new read", method: "InsertUserBookRead", setup: reading,
			run:      func(a *App) (int, error) { return a.UpdateProgress(ctx, 1, model.PageUpdate{Absolute: 50}) },
			wantPage: 50, prefix: "update progress: ",
		},
		{
			name: "RateBook", method: "UpdateUserBookRating", setup: reading,
			run:    func(a *App) (int, error) { return 0, a.RateBook(ctx, 1, 4.5) },
			prefix: "update rating: ",
		},
		{
			name: "ReviewBook", method: "UpdateUserBookReviewAndRating", setup: reading,
			run:    func(a *App) (int, error) { return 0, a.ReviewBook(ctx, 1, 4, "Spice") },
			prefix: "update review: ",
		},
	}

	// snapshot is everything a mutation could change locally.
	snapshot := func(t *testing.T, a *App) string {
		t.Helper()
		ub, err := a.store.GetUserBookByBookID(1)
		if err != nil {
			t.Fatal(err)
		}
		inserted, err := a.store.GetUserBookByBookID(42)
		if err != nil {
			t.Fatal(err)
		}
		read, err := a.store.GetLatestRead(100)
		if err != nil {
			t.Fatal(err)
		}
		ids, err := a.GetActiveBookIDs()
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%+v %+v %+v %v %d", ub, inserted, read, ids, localJournalCount(t, a))
	}

	for _, tc := range cases {
		t.Run(tc.name+"/remote write fails", func(t *testing.T) {
			hc := newFakeHardcover()
			hc.errs = map[string]error{tc.method: errors.New("rejected")}
			a := newTestAppWith(t, hc)
			tc.setup(t, a)
			before := snapshot(t, a)

			_, err := tc.run(a)
			if err == nil || errors.Is(err, ErrCacheRefresh) || !strings.HasPrefix(err.Error(), tc.prefix) {
				t.Fatalf("err = %v, want a plain %q error", err, tc.prefix)
			}
			if after := snapshot(t, a); after != before {
				t.Fatalf("a rejected write changed the cache:\nbefore %s\nafter  %s", before, after)
			}
		})

		t.Run(tc.name+"/cache fails after remote write", func(t *testing.T) {
			hc := newFakeHardcover()
			a := newTestAppWith(t, hc)
			tc.setup(t, a)
			hc.afterWrite = func() { _ = a.store.Close() }

			page, err := tc.run(a)
			if !errors.Is(err, ErrCacheRefresh) {
				t.Fatalf("err = %v, want ErrCacheRefresh", err)
			}
			if got := hc.called(tc.method); len(got) != 1 {
				t.Fatalf("%s calls = %q, want the one remote write", tc.method, got)
			}
			if page != tc.wantPage {
				t.Fatalf("page = %d, want %d alongside the warning", page, tc.wantPage)
			}
		})
	}
}
