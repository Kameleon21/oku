package model

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Status represents a user-book status on Hardcover.
type Status int

const (
	StatusWantToRead       Status = 1 // "oku"
	StatusCurrentlyReading Status = 2 // "reading"
	StatusRead             Status = 3 // "finished"
	StatusPaused           Status = 4
	StatusDidNotFinish     Status = 5 // "dnf"
	StatusIgnored          Status = 6
)

// StatusFromString maps CLI aliases to status IDs.
func StatusFromString(s string) (Status, error) {
	switch strings.ToLower(s) {
	case "oku", "want", "wtr", "want-to-read":
		return StatusWantToRead, nil
	case "reading":
		return StatusCurrentlyReading, nil
	case "finished", "read", "done":
		return StatusRead, nil
	case "dnf":
		return StatusDidNotFinish, nil
	default:
		return 0, fmt.Errorf("unknown status: %q (valid: reading, oku, finished, dnf)", s)
	}
}

// String returns the CLI-friendly name.
func (s Status) String() string {
	switch s {
	case StatusWantToRead:
		return "oku"
	case StatusCurrentlyReading:
		return "reading"
	case StatusRead:
		return "finished"
	case StatusPaused:
		return "paused"
	case StatusDidNotFinish:
		return "dnf"
	case StatusIgnored:
		return "ignored"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// Label returns a human-readable display label.
func (s Status) Label() string {
	switch s {
	case StatusWantToRead:
		return "Want to Read"
	case StatusCurrentlyReading:
		return "Currently Reading"
	case StatusRead:
		return "Read"
	case StatusPaused:
		return "Paused"
	case StatusDidNotFinish:
		return "Did Not Finish"
	case StatusIgnored:
		return "Ignored"
	default:
		return "Unknown"
	}
}

// Book represents a book from Hardcover.
type Book struct {
	ID                     int
	Title                  string
	Authors                []string
	Pages                  int
	Slug                   string
	ImageURL               string
	Rating                 float64
	RatingsCount           int
	ReviewsCount           int
	UsersCount             int
	UsersReadCount         int
	ReleaseDate            string
	FeaturedSeries         string
	FeaturedSeriesPosition int
}

// AuthorString returns authors joined by ", ".
func (b Book) AuthorString() string {
	return strings.Join(b.Authors, ", ")
}

// UserBook represents the user's relationship with a book.
type UserBook struct {
	ID            int
	BookID        int
	StatusID      Status
	Rating        float64
	Review        string
	ReviewedAt    *time.Time
	CurrentPage   int
	Book          Book
	UserBookReads []UserBookRead
	UpdatedAt     time.Time
}

// ValidateRating validates a user rating value.
// Allowed values are 0 (unrated) or 0.5 increments from 0.5 to 5.0.
func ValidateRating(r float64) error {
	if r == 0 {
		return nil
	}
	if r < 0.5 || r > 5.0 {
		return fmt.Errorf("rating must be 0 or between 0.5 and 5.0")
	}
	scaled := r * 2
	if math.Abs(scaled-math.Round(scaled)) > 1e-9 {
		return fmt.Errorf("rating must be in 0.5 increments")
	}
	return nil
}

// StarString renders a 5-slot rating string.
func StarString(rating float64) string {
	if rating <= 0 {
		return "☆☆☆☆☆"
	}
	if rating > 5 {
		rating = 5
	}

	full := int(math.Floor(rating))
	half := 0
	if rating-float64(full) >= 0.5 {
		half = 1
	}
	empty := 5 - full - half
	if empty < 0 {
		empty = 0
	}

	var b strings.Builder
	b.WriteString(strings.Repeat("★", full))
	if half == 1 {
		b.WriteString("½")
	}
	b.WriteString(strings.Repeat("☆", empty))
	return b.String()
}

// Progress returns current page / total pages as a string.
func (ub UserBook) Progress() string {
	page := ub.CurrentPage
	if len(ub.UserBookReads) > 0 {
		page = ub.UserBookReads[0].ProgressPages
	}
	if ub.Book.Pages > 0 {
		return fmt.Sprintf("%d/%d", page, ub.Book.Pages)
	}
	return fmt.Sprintf("%d", page)
}

// UserBookRead represents a reading session/progress entry.
type UserBookRead struct {
	ID            int
	UserBookID    int
	ProgressPages int
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

// PageUpdate represents a parsed page update instruction.
type PageUpdate struct {
	Absolute int  // target page (if not relative)
	Delta    int  // delta (if relative)
	Relative bool // true if +N or -N
}

// ParsePage parses a page string: "123", "+10", "-5".
func ParsePage(s string) (PageUpdate, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return PageUpdate{}, fmt.Errorf("empty page value")
	}

	if s[0] == '+' || s[0] == '-' {
		n, err := strconv.Atoi(s)
		if err != nil {
			return PageUpdate{}, fmt.Errorf("invalid page delta %q: %w", s, err)
		}
		return PageUpdate{Delta: n, Relative: true}, nil
	}

	n, err := strconv.Atoi(s)
	if err != nil {
		return PageUpdate{}, fmt.Errorf("invalid page number %q: %w", s, err)
	}
	if n < 0 {
		return PageUpdate{}, fmt.Errorf("page number cannot be negative: %d", n)
	}
	return PageUpdate{Absolute: n}, nil
}

// Resolve computes the final page number given the current page and total.
func (p PageUpdate) Resolve(currentPage, totalPages int) int {
	var page int
	if p.Relative {
		page = currentPage + p.Delta
	} else {
		page = p.Absolute
	}
	if page < 0 {
		page = 0
	}
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}
	return page
}

// SearchResult represents a book from the search API.
type SearchResult struct {
	ID       int
	Title    string
	Authors  []string
	Pages    int
	Slug     string
	ImageURL string
	Rating   float64
	Ratings  int
}
