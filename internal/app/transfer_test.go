package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
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

type transferTransport func(*http.Request) (*http.Response, error)

func (f transferTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func installTransferAPI(t *testing.T, a *App, respond func(string) string) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = transferTransport(func(r *http.Request) (*http.Response, error) {
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(respond(body.Query))), Request: r}, nil
	})
	a.API = api.NewClient("test-token")
	http.DefaultTransport = original
}
func TestImportPreviewNeverMutatesAndSkipsExistingDuplicates(t *testing.T) {
	a := newTestApp(t)
	installTransferAPI(t, a, func(q string) string {
		if strings.Contains(q, "mutation") {
			t.Fatal("preview issued mutation")
		}
		switch {
		case strings.Contains(q, "me {"):
			return `{"data":{"me":[{"id":1}]}}`
		case strings.Contains(q, "user_books("):
			return `{"data":{"user_books":[{"id":5,"book":{"id":9}}]}}`
		case strings.Contains(q, "books_by_pk"):
			return `{"data":{"books_by_pk":{"id":10,"title":"New"}}}`
		}
		t.Fatal(q)
		return ""
	})
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
}
func TestImportReportsPartialAndStopsAfterMetadataFailure(t *testing.T) {
	a := newTestApp(t)
	inserts := 0
	installTransferAPI(t, a, func(q string) string {
		switch {
		case strings.Contains(q, "me {"):
			return `{"data":{"me":[{"id":1}]}}`
		case strings.Contains(q, "query") && strings.Contains(q, "user_books("):
			return `{"data":{"user_books":[]}}`
		case strings.Contains(q, "books_by_pk"):
			return `{"data":{"books_by_pk":{"id":10,"title":"New"}}}`
		case strings.Contains(q, "insert_user_book("):
			inserts++
			return `{"data":{"insert_user_book":{"id":100,"user_book":{"id":100}}}}`
		case strings.Contains(q, "update_user_book("):
			return `{"data":{"update_user_book":{"id":null,"error":"rating refused"}}}`
		}
		t.Fatal(q)
		return ""
	})
	result, err := a.ImportBooks(context.Background(), []TransferBook{{BookID: 10, Status: "reading", Rating: 4}, {BookID: 11, Status: "oku"}}, true)
	if err == nil || inserts != 1 || result[0].Action != "partial" {
		t.Fatalf("result %#v inserts %d err %v", result, inserts, err)
	}
}
func TestPersonalJournalDoesNotFallBackToPublic(t *testing.T) {
	a := newTestApp(t)
	installTransferAPI(t, a, func(q string) string {
		if strings.Contains(q, "mutation") {
			t.Fatal("saved without knowing privacy")
		}
		return `{"data":{"me":[{"id":1,"account_privacy_setting_id":null}]}}`
	})
	if _, err := a.AddJournalEntry(context.Background(), 1, "note", "Private thought"); err == nil {
		t.Fatal("expected privacy lookup error")
	}
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
