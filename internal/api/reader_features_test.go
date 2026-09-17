package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type readerRequest struct {
	Query     string                     `json:"query"`
	Variables map[string]json.RawMessage `json:"variables"`
}

func readReaderRequest(t *testing.T, r *http.Request) readerRequest {
	t.Helper()
	var q readerRequest
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		t.Error(err)
	}
	return q
}
func TestJournalUsesVariablesAndReportsMutationErrors(t *testing.T) {
	t.Parallel()
	entry := "A quote: \"hello\"\n{ mutation }"
	for _, response := range []string{`{"data":{"insert_reading_journal":{"id":41,"errors":[]}}}`, `{"data":{"insert_reading_journal":{"id":null,"errors":["entry rejected"]}}}`} {
		c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
			q := readReaderRequest(t, r)
			if strings.Contains(q.Query, entry) {
				t.Error("entry interpolated into GraphQL")
			}
			var object map[string]any
			json.Unmarshal(q.Variables["object"], &object)
			if object["entry"] != entry || object["privacy_setting_id"] != float64(3) || object["event"] != "quote" {
				t.Errorf("wrong journal object: %#v", object)
			}
			fmt.Fprint(w, response)
		})
		id, err := c.CreateJournalEntry(context.Background(), 7, "quote", entry, 3)
		srv.Close()
		if strings.Contains(response, "rejected") {
			if err == nil || !strings.Contains(err.Error(), "rejected") {
				t.Fatalf("missing validation error: %v", err)
			}
		} else if err != nil || id != 41 {
			t.Fatalf("id %d, error %v", id, err)
		}
	}
}
func TestCreationDoesNotRetryUncertainWrites(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(502)
		fmt.Fprint(w, "upstream unavailable")
	})
	defer srv.Close()
	_, err := c.CreateJournalEntry(context.Background(), 1, "note", "personal", 3)
	if err == nil || calls.Load() != 1 {
		t.Fatalf("calls %d error %v", calls.Load(), err)
	}
}
func TestTrendingPreservesRankAndFetchesMetadata(t *testing.T) {
	t.Parallel()
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		q := readReaderRequest(t, r)
		if strings.Contains(q.Query, "books_trending") {
			if string(q.Variables["duration"]) != `"week"` {
				t.Error("wrong period")
			}
			fmt.Fprint(w, `{"data":{"books_trending":{"ids":[8,3],"error":null}}}`)
		} else {
			fmt.Fprint(w, `{"data":{"books":[{"id":3,"title":"Third"},{"id":8,"title":"First"}]}}`)
		}
	})
	defer srv.Close()
	books, err := c.Trending(context.Background(), "week", 20)
	if err != nil || len(books) != 2 || books[0].ID != 8 || books[1].ID != 3 {
		t.Fatalf("books %#v err %v", books, err)
	}
}
func TestExportPaginatesAndRequestsAllReads(t *testing.T) {
	t.Parallel()
	var pages atomic.Int32
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		q := readReaderRequest(t, r)
		if strings.Contains(q.Query, "me {") {
			fmt.Fprint(w, `{"data":{"me":[{"id":7}]}}`)
			return
		}
		if !strings.Contains(q.Query, "user_book_reads(order_by:{id:desc})") || !strings.Contains(q.Query, "order_by:{id:asc}") {
			t.Error("missing stable order or all reads")
		}
		pages.Add(1)
		var offset int
		json.Unmarshal(q.Variables["offset"], &offset)
		count := 100
		if offset == 100 {
			count = 1
		} else if offset != 0 {
			t.Errorf("unexpected offset %d", offset)
			count = 0
		}
		rows := []APIUserBook{}
		for i := 0; i < count; i++ {
			rows = append(rows, APIUserBook{ID: offset + i + 1, Book: APIBook{ID: offset + i + 1000}})
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"user_books": rows}})
	})
	defer srv.Close()
	rows, err := c.ExportLibrary(context.Background())
	if err != nil || len(rows) != 101 || pages.Load() != 2 {
		t.Fatalf("rows %d pages %d err %v", len(rows), pages.Load(), err)
	}
}
func TestQueueDiscoveryNeverCreatesOnRead(t *testing.T) {
	t.Parallel()
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		q := readReaderRequest(t, r)
		if strings.Contains(q.Query, "mutation") {
			t.Error("read created a list")
		}
		if strings.Contains(q.Query, "me {") {
			fmt.Fprint(w, `{"data":{"me":[{"id":7}]}}`)
		} else {
			fmt.Fprint(w, `{"data":{"lists":[]}}`)
		}
	})
	defer srv.Close()
	id, rows, err := c.ReadingQueue(context.Background(), false)
	if err != nil || id != 0 || len(rows) != 0 {
		t.Fatalf("%d %#v %v", id, rows, err)
	}
}
func TestQueuePositionsScopeListAndCheckEachRow(t *testing.T) {
	t.Parallel()
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		q := readReaderRequest(t, r)
		if string(q.Variables["list"]) != "12" || strings.Count(q.Query, "list_id:{_eq:$list}") != 2 {
			t.Error("updates not scoped to list")
		}
		fmt.Fprint(w, `{"data":{"p0":{"affected_rows":1},"p1":{"affected_rows":0}}}`)
	})
	defer srv.Close()
	err := c.SetQueuePositions(context.Background(), 12, []RankedBook{{ID: 5, Position: 2}, {ID: 6, Position: 1}})
	if err == nil {
		t.Fatal("silently accepted partial update")
	}
}
func TestGoalValidationAndRequiredObject(t *testing.T) {
	t.Parallel()
	g := GoalInput{Goal: 20, Metric: "book", StartDate: "2026-01-01", EndDate: "2026-12-31", PrivacySettingID: 3}
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		q := readReaderRequest(t, r)
		if !strings.Contains(q.Query, "update_goal") {
			t.Error("expected update")
		}
		var obj map[string]any
		json.Unmarshal(q.Variables["object"], &obj)
		if _, ok := obj["conditions"].(map[string]any); !ok {
			t.Error("required conditions must be object")
		}
		fmt.Fprint(w, `{"data":{"result":{"id":9,"errors":[]}}}`)
	})
	defer srv.Close()
	id, err := c.SaveGoal(context.Background(), 9, g)
	if err != nil || id != 9 {
		t.Fatal(id, err)
	}
	g.EndDate = "2025-01-01"
	if g.Validate() == nil {
		t.Fatal("accepted reversed goal dates")
	}
	g.EndDate = "2026-12-31"
	g.Goal = 0
	if g.Validate() == nil {
		t.Fatal("accepted zero goal")
	}
}
func TestLookupISBNAmbiguity(t *testing.T) {
	t.Parallel()
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"editions":[{"book_id":1},{"book_id":2}]}}`)
	})
	defer srv.Close()
	_, err := c.LookupISBN(context.Background(), "9780000000000")
	if err == nil {
		t.Fatal("ambiguous ISBN silently matched")
	}
}
func TestBookDetailNullIsError(t *testing.T) {
	t.Parallel()
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":{"books_by_pk":null}}`) })
	defer srv.Close()
	if _, err := c.BookDetail(context.Background(), 99); err == nil {
		t.Fatal("missing book accepted")
	}
}

func TestQueueSetupBatchesAtFiveFields(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		q := readReaderRequest(t, r)
		n := strings.Count(q.Query, "insert_list_book(")
		if n < 1 || n > 5 {
			t.Errorf("%d mutation fields", n)
		}
		calls.Add(1)
		data := map[string]any{}
		for i := 0; i < n; i++ {
			data[fmt.Sprintf("b%d", i)] = map[string]int{"id": i + 1}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	})
	defer srv.Close()
	rows := make([]RankedBook, 6)
	for i := range rows {
		rows[i] = RankedBook{BookID: i + 1, Position: i + 1}
	}
	if err := c.QueueBooks(context.Background(), 4, rows); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
}
func TestImportReadsKeepsDatesWithoutForeignReadIDs(t *testing.T) {
	t.Parallel()
	date := "2026-09-17"
	c, srv := testClientForServer(func(w http.ResponseWriter, r *http.Request) {
		q := readReaderRequest(t, r)
		var dates []map[string]any
		json.Unmarshal(q.Variables["dates"], &dates)
		if len(dates) != 2 || dates[0]["finished_at"] != date || dates[1]["progress_pages"] != float64(12) {
			t.Errorf("wrong reads %#v", dates)
		}
		for _, d := range dates {
			if _, ok := d["id"]; ok {
				t.Error("foreign read IDs were reused")
			}
		}
		fmt.Fprint(w, `{"data":{"upsert_user_book_reads":{"user_book_id":42,"error":null}}}`)
	})
	defer srv.Close()
	if err := c.ImportReads(context.Background(), 42, []APIUserBookRead{{ID: 300, FinishedAt: &date}, {ID: 400, ProgressPages: 12}}); err != nil {
		t.Fatal(err)
	}
}

func TestLiveRatingDistributionShape(t *testing.T) {
	rows, err := ParseRatingDistribution([]byte(`[{"count":1,"rating":3.75},{"count":1655,"rating":5.0},{"count":18,"rating":0.5}]`))
	if err != nil || len(rows) != 3 || rows[0].Rating != 5 || rows[1].Rating != 3.75 {
		t.Fatal(rows, err)
	}
	if _, err := ParseRatingDistribution([]byte(`[{"count":-1,"rating":5}]`)); err == nil {
		t.Fatal("accepted negative count")
	}
}
