package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/model"
)

func TestLibraryTransferRoundTrip(t *testing.T) {
	date := "2026-09-17"
	data := LibraryExport{Version: 1, Books: []TransferBook{{BookID: 9, Title: "Book, \"quoted\"", Status: "paused", Rating: 3.75, Review: "First\nsecond, paragraph", ReviewedAt: date, Reads: []api.APIUserBookRead{{ID: 8, ProgressPages: 120, StartedAt: &date}, {ID: 7, FinishedAt: &date}}}}}
	for _, format := range []string{"csv", "json"} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteExport(&buf, data, format); err != nil {
				t.Fatal(err)
			}
			got, err := ReadImport(&buf, format)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, data.Books) {
				t.Fatalf("roundtrip lost data: %#v", got)
			}
		})
	}
}
func TestGoodreadsImportUsesISBNNotGoodreadsID(t *testing.T) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Write([]string{"Book Id", "Title", "ISBN13", "ISBN", "Exclusive Shelf", "My Rating", "My Review", "Date Read"})
	w.Write([]string{"999999", "Example", `="9781234567890"`, "", "read", "4", "A review", "2024/03/02"})
	w.Flush()
	rows, err := ReadImport(&buf, "goodreads")
	if err != nil {
		t.Fatal(err)
	}
	b := rows[0]
	if b.BookID != 0 || b.ISBN != "9781234567890" || b.Status != "finished" || *b.Reads[0].FinishedAt != "2024-03-02" {
		t.Fatalf("wrong mapping: %#v", b)
	}
}
func TestImportRejectsInvalidInputBeforeWrites(t *testing.T) {
	for _, raw := range []string{`{"version":2,"books":[]}`, `{"version":1,"books":[{"book_id":1,"status":"reading","rating":6}]}`, `{"version":1,"books":[{"book_id":1,"status":"invented"}]}`, `{"version":1,"books":[{"book_id":1,"status":"reading","reads":[{"progress_pages":-1}]}]}`, `{"version":1,"books":[]} {}`} {
		if _, err := ReadImport(strings.NewReader(raw), "json"); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if _, err := ReadImport(strings.NewReader("book_id,title,status,rating\n1,A,reading,NaN\n"), "csv"); err == nil {
		t.Fatal("accepted NaN")
	}
}
func TestQueueOrderPreservesUnrankedShelfOrder(t *testing.T) {
	books := []model.UserBook{{BookID: 1}, {BookID: 2}, {BookID: 3}, {BookID: 4}}
	SortQueue(books, []int{3, 99, 1})
	got := []int{}
	for _, b := range books {
		got = append(got, b.BookID)
	}
	if !reflect.DeepEqual(got, []int{3, 1, 2, 4}) {
		t.Fatal(got)
	}
}
func TestQueueCacheRoundTrip(t *testing.T) {
	a := newTestApp(t)
	want := []int{3, 9, 2}
	if err := a.saveQueue(want); err != nil {
		t.Fatal(err)
	}
	got, err := a.CachedQueueOrder()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
}

// mutatingCalls are the fake's write methods; a preview must issue none.
var mutatingCalls = []string{"InsertUserBook", "UpdateUserBookStatus", "UpdateUserBookRating", "UpdateUserBookReviewAndRating", "ImportReads", "CreateJournalEntry", "QueueBooks", "SetQueuePositions", "SaveGoal"}

func assertNoMutations(t *testing.T, hc *fakeHardcover) {
	t.Helper()
	for _, m := range mutatingCalls {
		if got := hc.called(m); len(got) > 0 {
			t.Fatalf("issued mutation %v", got)
		}
	}
}

func TestImportPreviewNeverMutatesAndSkipsExistingDuplicates(t *testing.T) {
	hc := newFakeHardcover()
	hc.library = []api.APIUserBook{{ID: 5, Book: api.APIBook{ID: 9}}}
	hc.details = map[int]*api.BookDetail{10: {APIBook: api.APIBook{ID: 10, Title: "New"}}}
	a := newTestAppWith(t, hc)
	rows := []TransferBook{{BookID: 9, Status: "reading"}, {BookID: 10, Status: "oku"}, {BookID: 10, Status: "oku"}, {Title: "No ISBN", Status: "finished"}}
	result, err := a.ImportBooks(context.Background(), rows, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"skip", "add", "skip", "unmatched"}
	for i, r := range result {
		if r.Action != want[i] {
			t.Errorf("row %d: %#v", i, r)
		}
	}
	if result[1].Title != "New" {
		t.Errorf("added row title = %q, want Hardcover's %q", result[1].Title, "New")
	}
	assertNoMutations(t, hc)
}
func TestImportReportsPartialAndStopsAfterMetadataFailure(t *testing.T) {
	hc := newFakeHardcover()
	hc.errs = map[string]error{"UpdateUserBookRating": errors.New("rating refused")}
	a := newTestAppWith(t, hc)
	result, err := a.ImportBooks(context.Background(), []TransferBook{{BookID: 10, Status: "reading", Rating: 4}, {BookID: 11, Status: "oku"}}, true)
	inserts := len(hc.called("InsertUserBook"))
	if err == nil || inserts != 1 || result[0].Action != "partial" || result[1].Action != "not_attempted" {
		t.Fatalf("result %#v inserts %d err %v", result, inserts, err)
	}
	if got := hc.called("ListUserBooks"); len(got) != 0 {
		t.Fatalf("synced after a stopped import: %v", got)
	}
}
func TestPersonalJournalDoesNotFallBackToPublic(t *testing.T) {
	hc := newFakeHardcover()
	hc.errs = map[string]error{"GetAccountPrivacySetting": errors.New("account privacy setting missing")}
	a := newTestAppWith(t, hc)
	if _, err := a.AddJournalEntry(context.Background(), 1, "note", "Private thought"); err == nil {
		t.Fatal("expected privacy lookup error")
	}
	assertNoMutations(t, hc)
}

func TestImportAcceptsLiveHardcoverReviewTimestamp(t *testing.T) {
	rows, err := ReadImport(strings.NewReader(`{"version":1,"books":[{"book_id":1,"status":"finished","reviewed_at":"2026-06-01T00:00:00","reads":[{"finished_at":"2026-06-01T23:00:00-05:00"}]}]}`), "json")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].ReviewedAt != "2026-06-01" || *rows[0].Reads[0].FinishedAt != "2026-06-01" {
		t.Fatal(rows)
	}
}
