package app

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/model"
)

// TransferBook is a portable library row. IDs are Hardcover IDs, never Goodreads IDs.
type TransferBook struct {
	BookID     int                   `json:"book_id"`
	ISBN       string                `json:"isbn,omitempty"`
	Title      string                `json:"title"`
	Status     string                `json:"status"`
	Rating     float64               `json:"rating"`
	Review     string                `json:"review,omitempty"`
	ReviewedAt string                `json:"reviewed_at,omitempty"`
	Reads      []api.APIUserBookRead `json:"reads,omitempty"`
}
type LibraryExport struct {
	Version int            `json:"version"`
	Books   []TransferBook `json:"books"`
}

func (a *App) ExportBooks(ctx context.Context) (LibraryExport, error) {
	rows, err := a.API.ExportLibrary(ctx)
	if err != nil {
		return LibraryExport{}, err
	}
	out := LibraryExport{Version: 1, Books: []TransferBook{}}
	for _, r := range rows {
		b := TransferBook{BookID: r.Book.ID, Title: r.Book.Title, Status: statusName(model.Status(r.StatusID)), Reads: r.UserBookReads}
		if r.Rating != nil {
			b.Rating = *r.Rating
		}
		if r.ReviewRaw != nil {
			b.Review = *r.ReviewRaw
		}
		if r.ReviewedAt != nil {
			b.ReviewedAt = *r.ReviewedAt
		}
		out.Books = append(out.Books, b)
	}
	return out, nil
}
func statusName(s model.Status) string {
	switch s {
	case model.StatusWantToRead:
		return "oku"
	case model.StatusCurrentlyReading:
		return "reading"
	case model.StatusRead:
		return "finished"
	case model.StatusPaused:
		return "paused"
	case model.StatusDidNotFinish:
		return "dnf"
	case model.StatusIgnored:
		return "ignored"
	}
	return "unknown"
}
func WriteExport(w io.Writer, data LibraryExport, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}
	if format != "csv" {
		return fmt.Errorf("format must be csv or json")
	}
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"book_id", "isbn", "title", "status", "rating", "review", "reviewed_at", "reads_json"}); err != nil {
		return err
	}
	for _, b := range data.Books {
		reads, err := json.Marshal(b.Reads)
		if err != nil {
			return err
		}
		if err := cw.Write([]string{strconv.Itoa(b.BookID), b.ISBN, b.Title, b.Status, strconv.FormatFloat(b.Rating, 'f', -1, 64), b.Review, b.ReviewedAt, string(reads)}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
func ReadImport(r io.Reader, format string) ([]TransferBook, error) {
	if format == "json" {
		var data LibraryExport
		dec := json.NewDecoder(r)
		if err := dec.Decode(&data); err != nil {
			return nil, err
		}
		var extra any
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("unexpected data after export")
		}
		if data.Version != 1 {
			return nil, fmt.Errorf("unsupported export version %d", data.Version)
		}
		return validateImport(data.Books)
	}
	if format != "csv" && format != "goodreads" {
		return nil, fmt.Errorf("format must be csv, json, or goodreads")
	}
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i, h := range header {
		h = strings.TrimPrefix(h, "\ufeff")
		if _, ok := index[h]; ok {
			return nil, fmt.Errorf("duplicate CSV column %q", h)
		}
		index[h] = i
	}
	required := []string{"book_id", "title", "status", "rating"}
	if format == "goodreads" {
		required = []string{"Title", "Exclusive Shelf", "My Rating"}
	}
	for _, h := range required {
		if _, ok := index[h]; !ok {
			return nil, fmt.Errorf("missing CSV column %q", h)
		}
	}
	out := []TransferBook{}
	for line := 2; ; line++ {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("row %d: %w", line, err)
		}
		get := func(k string) string {
			if i, ok := index[k]; ok && i < len(row) {
				return row[i]
			}
			return ""
		}
		b := TransferBook{}
		if format == "goodreads" {
			b.Title = get("Title")
			b.ISBN = cleanISBN(get("ISBN13"))
			if b.ISBN == "" {
				b.ISBN = cleanISBN(get("ISBN"))
			}
			b.Review = get("My Review")
			switch get("Exclusive Shelf") {
			case "to-read":
				b.Status = "oku"
			case "currently-reading":
				b.Status = "reading"
			case "read":
				b.Status = "finished"
			default:
				return nil, fmt.Errorf("row %d: unsupported Goodreads shelf %q", line, get("Exclusive Shelf"))
			}
			b.Rating, err = parseRating(get("My Rating"))
			if err != nil {
				return nil, fmt.Errorf("row %d: %w", line, err)
			}
			if raw := get("Date Read"); raw != "" {
				t, e := time.Parse("2006/01/02", raw)
				if e != nil {
					return nil, fmt.Errorf("row %d: invalid Date Read", line)
				}
				date := t.Format("2006-01-02")
				b.Reads = []api.APIUserBookRead{{FinishedAt: &date}}
			}
		} else {
			b.Title = get("title")
			b.ISBN = cleanISBN(get("isbn"))
			b.Status = get("status")
			b.Review = get("review")
			b.ReviewedAt = get("reviewed_at")
			b.BookID, err = strconv.Atoi(get("book_id"))
			if err != nil {
				return nil, fmt.Errorf("row %d: invalid Hardcover book_id", line)
			}
			b.Rating, err = parseRating(get("rating"))
			if err != nil {
				return nil, fmt.Errorf("row %d: %w", line, err)
			}
			if raw := get("reads_json"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &b.Reads); err != nil {
					return nil, fmt.Errorf("row %d: invalid reads_json: %w", line, err)
				}
			}
		}
		out = append(out, b)
	}
	return validateImport(out)
}
func cleanISBN(s string) string {
	return strings.NewReplacer("=", "", "\"", "", "-", "", " ", "").Replace(strings.TrimSpace(s))
}
func parseRating(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	r, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid rating %q", s)
	}
	return r, nil
}
func validateImport(rows []TransferBook) ([]TransferBook, error) {
	for i, b := range rows {
		if b.BookID < 0 {
			return nil, fmt.Errorf("row %d: invalid book ID", i+1)
		}
		if _, err := model.StatusFromString(b.Status); err != nil {
			return nil, fmt.Errorf("row %d: %w", i+1, err)
		}
		if math.IsNaN(b.Rating) || math.IsInf(b.Rating, 0) || b.Rating < 0 || b.Rating > 5 {
			return nil, fmt.Errorf("row %d: rating must be between 0 and 5", i+1)
		}

		for j := range b.Reads {
			r := &b.Reads[j]
			if r.ProgressPages < 0 {
				return nil, fmt.Errorf("row %d: negative progress", i+1)
			}
			for _, d := range []**string{&r.StartedAt, &r.FinishedAt} {
				if *d != nil {
					date, err := transferDate(**d)
					if err != nil {
						return nil, fmt.Errorf("row %d: invalid read date: %w", i+1, err)
					}
					*d = &date
				}
			}
			if r.StartedAt != nil && r.FinishedAt != nil && *r.FinishedAt < *r.StartedAt {
				return nil, fmt.Errorf("row %d: finish precedes start", i+1)
			}
		}
		if b.ReviewedAt != "" {
			date, err := transferDate(b.ReviewedAt)
			if err != nil {
				return nil, fmt.Errorf("row %d: invalid review date: %w", i+1, err)
			}
			b.ReviewedAt = date
		}
		rows[i] = b

	}
	return rows, nil
}

type ImportResult struct {
	Title  string `json:"title"`
	BookID int    `json:"book_id"`
	Action string `json:"action"`
	Detail string `json:"detail,omitempty"`
}

// ImportBooks previews by default. Existing books are always skipped, preserving
// ratings, progress and reviews. A failed partially-added row is explicitly reported.
func (a *App) ImportBooks(ctx context.Context, rows []TransferBook, apply bool) ([]ImportResult, error) {
	if _, err := validateImport(rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ImportResult{}, nil
	}
	existing, err := a.API.ExportLibrary(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	for _, r := range existing {
		seen[r.Book.ID] = true
	}
	results := make([]ImportResult, len(rows))
	resolved := make([]int, len(rows))
	// Resolve all rows before any mutation, so preview and apply use the same rules.
	for i, b := range rows {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		r := ImportResult{Title: b.Title, BookID: b.BookID}
		id := b.BookID
		if id == 0 {
			if b.ISBN == "" {
				r.Action = "unmatched"
				r.Detail = "no ISBN; Goodreads IDs are not Hardcover IDs"
				results[i] = r
				continue
			}
			id, err = a.API.LookupISBN(ctx, b.ISBN)
			if err != nil {
				var matchErr *api.ISBNMatchError
				if !errors.As(err, &matchErr) {
					return results, err
				}
				r.Action = "unmatched"
				r.Detail = err.Error()
				results[i] = r
				continue
			}
		}
		r.BookID = id
		if seen[id] {
			r.Action = "skip"
			r.Detail = "already in library or duplicated in input"
		} else {
			detail, e := a.API.BookDetail(ctx, id)
			if e != nil {
				return results, fmt.Errorf("validate book %d: %w", id, e)
			}
			r.Title = detail.Title
			r.Action = "add"
			resolved[i] = id
			seen[id] = true
		}
		results[i] = r
	}
	if !apply {
		return results, nil
	}
	for i := range results {
		if results[i].Action == "add" {
			results[i].Action = "not_attempted"
		}
	}
	for i, b := range rows {
		if resolved[i] == 0 {
			continue
		}
		status, _ := model.StatusFromString(b.Status)
		uid, e := a.API.InsertUserBook(ctx, resolved[i], int(status))
		if e != nil {
			results[i].Action = "failed"
			results[i].Detail = e.Error()
			return results, fmt.Errorf("import stopped at %q: %w", b.Title, e)
		}
		results[i].Action = "added"
		if b.Review != "" {
			date := b.ReviewedAt
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			e = a.API.UpdateUserBookReviewAndRating(ctx, uid, b.Rating, b.Review, date)
		} else if b.Rating > 0 {
			e = a.API.UpdateUserBookRating(ctx, uid, b.Rating)
		}
		if e == nil {
			e = a.API.ImportReads(ctx, uid, b.Reads)
		}
		if e != nil {
			results[i].Action = "partial"
			results[i].Detail = "book added; metadata incomplete: " + e.Error()
			return results, fmt.Errorf("import stopped after adding %q; repair metadata before retrying: %w", b.Title, e)
		}
	}
	if err := a.SyncAll(ctx); err != nil {
		return results, fmt.Errorf("import saved on Hardcover; refresh local cache with oku sync: %w", err)
	}
	return results, nil
}

// Hardcover review timestamps can omit the timezone. Date-valued mutations
// retain the calendar date in the source value, without converting its offset.
func transferDate(raw string) (string, error) {
	for _, layout := range []string{"2006-01-02", time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("unsupported date %q", raw)
}
