package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MeUser represents a single user entry from the me query.
type MeUser struct {
	ID        int           `json:"id"`
	Username  string        `json:"username"`
	UserBooks []APIUserBook `json:"user_books"`
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
	Hits []struct {
		Document TypesenseBookDoc `json:"document"`
	} `json:"hits"`
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
		n, err := strconv.Atoi(unquoted)
		if err != nil {
			return fmt.Errorf("parse quoted number %q: %w", unquoted, err)
		}
		*v = FlexibleInt(n)
		return nil
	}

	// Raw number: 123
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("parse number %q: %w", s, err)
	}
	*v = FlexibleInt(n)
	return nil
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

// UpsertUserBookReadsResponse is the response shape for upserting user book reads.
type UpsertUserBookReadsResponse struct {
	UpsertUserBookReads struct {
		Error      *string `json:"error"`
		UserBookID *int    `json:"user_book_id"`
	} `json:"upsert_user_book_reads"`
}
