package api

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// MeUser represents a single user entry from the me query.
type MeUser struct {
	ID                      int           `json:"id"`
	Username                string        `json:"username"`
	AccountPrivacySettingID *int          `json:"account_privacy_setting_id"`
	UserBooks               []APIUserBook `json:"user_books"`
	Goals                   []APIGoal     `json:"goals"`
}

// APIGoal represents a reading goal from the API.
type APIGoal struct {
	ID        int           `json:"id"`
	Goal      int           `json:"goal"`
	Metric    string        `json:"metric"`
	Progress  FlexibleFloat `json:"progress"`
	State     string        `json:"state"`
	StartDate *string       `json:"start_date"`
	EndDate   *string       `json:"end_date"`
}

// APIReadingJournal represents a reading journal entry from the API.
type APIReadingJournal struct {
	ID       int     `json:"id"`
	ActionAt *string `json:"action_at"`
	Event    string  `json:"event"`
}

// ReadingJournalsResponse is the response shape for listing reading journals.
type ReadingJournalsResponse struct {
	ReadingJournals []APIReadingJournal `json:"reading_journals"`
}

// MeResponse is the response shape for the me query.
type MeResponse struct {
	Me []MeUser `json:"me"`
}

// UserBooksResponse is the response shape for listing user books.
type UserBooksResponse struct {
	Me []MeUser `json:"me"`
}

// APIUserBook represents a user-book relationship from the API.
type APIUserBook struct {
	ID            int               `json:"id"`
	StatusID      int               `json:"status_id"`
	Rating        *float64          `json:"rating"`
	ReviewRaw     *string           `json:"review_raw"`
	ReviewedAt    *string           `json:"reviewed_at"`
	UpdatedAt     *string           `json:"updated_at"`
	UserBookReads []APIUserBookRead `json:"user_book_reads"`
	Book          APIBook           `json:"book"`
}

// APIBook represents a book from the API.
type APIBook struct {
	ID             int               `json:"id"`
	Title          string            `json:"title"`
	Pages          int               `json:"pages"`
	Slug           string            `json:"slug"`
	Rating         float64           `json:"rating"`
	RatingsCount   int               `json:"ratings_count"`
	ReviewsCount   int               `json:"reviews_count"`
	UsersCount     int               `json:"users_count"`
	UsersReadCount int               `json:"users_read_count"`
	ReleaseDate    *string           `json:"release_date"`
	CachedTags     json.RawMessage   `json:"cached_tags"`
	Contributions  []APIContribution `json:"contributions"`
	Image          *APIImage         `json:"image"`
}

// APIContribution represents an author contribution to a book.
type APIContribution struct {
	Author struct {
		Name string `json:"name"`
	} `json:"author"`
}

// APIImage represents an image URL from the API.
type APIImage struct {
	URL string `json:"url"`
}

// APIUserBookRead represents a reading progress entry from the API.
type APIUserBookRead struct {
	ID            int     `json:"id"`
	ProgressPages int     `json:"progress_pages"`
	StartedAt     *string `json:"started_at"`
	FinishedAt    *string `json:"finished_at"`
}

// SearchResponse is the response shape for book search queries.
// The results field contains raw Typesense JSON.
type SearchResponse struct {
	Search struct {
		Results json.RawMessage `json:"results"`
	} `json:"search"`
}

// TypesenseResults represents parsed Typesense search results.
type TypesenseResults struct {
	Hits []TypesenseHit `json:"hits"`
}

// TypesenseHit represents one search hit.
type TypesenseHit struct {
	Document TypesenseBookDoc `json:"document"`
}

// UnmarshalJSON skips malformed individual hits while retaining valid results.
func (r *TypesenseResults) UnmarshalJSON(data []byte) error {
	var raw struct {
		Hits []json.RawMessage `json:"hits"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Hits = make([]TypesenseHit, 0, len(raw.Hits))
	for _, rawHit := range raw.Hits {
		var hit TypesenseHit
		if err := json.Unmarshal(rawHit, &hit); err != nil {
			continue
		}
		r.Hits = append(r.Hits, hit)
	}
	return nil
}

// TypesenseBookDoc represents a book document from Typesense search.
type TypesenseBookDoc struct {
	ID          FlexibleInt     `json:"id"`
	Title       string          `json:"title"`
	AuthorNames []string        `json:"author_names"`
	Pages       FlexibleInt     `json:"pages"`
	Slug        string          `json:"slug"`
	Rating      FlexibleFloat   `json:"rating"`
	Ratings     FlexibleInt     `json:"ratings_count"`
	Image       *TypesenseImage `json:"image"`
}

// TypesenseImage represents an image in Typesense search results.
type TypesenseImage struct {
	URL string `json:"url"`
}

// FlexibleInt handles APIs that sometimes return a number and sometimes a quoted number.
type FlexibleInt int

func (v *FlexibleInt) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		*v = 0
		return nil
	}

	// Quoted number: "123"
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		unquoted, err := strconv.Unquote(s)
		if err != nil {
			return fmt.Errorf("unquote number %q: %w", s, err)
		}
		if unquoted == "" {
			*v = 0
			return nil
		}
		n, err := parseFlexibleInt(unquoted)
		if err != nil {
			return fmt.Errorf("parse quoted number %q: %w", unquoted, err)
		}
		*v = FlexibleInt(n)
		return nil
	}

	// Raw number: 123
	n, err := parseFlexibleInt(s)
	if err != nil {
		return fmt.Errorf("parse number %q: %w", s, err)
	}
	*v = FlexibleInt(n)
	return nil
}

func parseFlexibleInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err == nil {
		return n, nil
	}
	f, floatErr := strconv.ParseFloat(s, 64)
	if floatErr != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < math.MinInt32 || f > math.MaxInt32 {
		return 0, err
	}
	return int(f), nil
}

// FlexibleFloat handles APIs that may return a number or a quoted number.
type FlexibleFloat float64

func (v *FlexibleFloat) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" {
		*v = 0
		return nil
	}

	// Quoted number: "4.23"
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		unquoted, err := strconv.Unquote(s)
		if err != nil {
			return fmt.Errorf("unquote float %q: %w", s, err)
		}
		if unquoted == "" {
			*v = 0
			return nil
		}
		n, err := strconv.ParseFloat(unquoted, 64)
		if err != nil {
			return fmt.Errorf("parse quoted float %q: %w", unquoted, err)
		}
		*v = FlexibleFloat(n)
		return nil
	}

	// Raw number: 4.23
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("parse float %q: %w", s, err)
	}
	*v = FlexibleFloat(n)
	return nil
}

// InsertUserBookResponse is the response shape for inserting a user book.
type InsertUserBookResponse struct {
	InsertUserBook struct {
		ID       *int `json:"id"`
		UserBook *struct {
			ID int `json:"id"`
		} `json:"user_book"`
		Error *string `json:"error"`
	} `json:"insert_user_book"`
}

// UpdateUserBookResponse is the response shape for updating a user book.
type UpdateUserBookResponse struct {
	UpdateUserBook struct {
		ID    *int    `json:"id"`
		Error *string `json:"error"`
	} `json:"update_user_book"`
}

// UpdateUserBookReadResponse is the response shape for updating a user book read.
type UpdateUserBookReadResponse struct {
	UpdateUserBookRead struct {
		ID    *int    `json:"id"`
		Error *string `json:"error"`
	} `json:"update_user_book_read"`
}

// InsertUserBookReadResponse is the response shape for inserting a user book read.
type InsertUserBookReadResponse struct {
	InsertUserBookRead struct {
		ID           *int             `json:"id"`
		Error        *string          `json:"error"`
		UserBookRead *APIUserBookRead `json:"user_book_read"`
	} `json:"insert_user_book_read"`
}

// InsertReadingJournalResponse is the response shape for inserting a reading journal entry.
type InsertReadingJournalResponse struct {
	InsertReadingJournal struct {
		ReadingJournal *struct {
			ID int `json:"id"`
		} `json:"reading_journal"`
	} `json:"insert_reading_journal"`
}

// UpsertUserBookReadsResponse is the response shape for upserting user book reads.
type UpsertUserBookReadsResponse struct {
	UpsertUserBookReads struct {
		Error      *string `json:"error"`
		UserBookID *int    `json:"user_book_id"`
	} `json:"upsert_user_book_reads"`
}
