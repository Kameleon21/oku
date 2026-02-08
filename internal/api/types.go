package api

import "encoding/json"

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
	UserBookReads []APIUserBookRead `json:"user_book_reads"`
	Book          APIBook           `json:"book"`
}

// APIBook represents a book from the API.
type APIBook struct {
	ID            int               `json:"id"`
	Title         string            `json:"title"`
	Pages         int               `json:"pages"`
	Slug          string            `json:"slug"`
	Contributions []APIContribution `json:"contributions"`
	Image         *APIImage         `json:"image"`
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
	ID          int              `json:"id"`
	Title       string           `json:"title"`
	AuthorNames []string         `json:"author_names"`
	Pages       int              `json:"pages"`
	Slug        string           `json:"slug"`
	Image       *TypesenseImage  `json:"image"`
}

// TypesenseImage represents an image in Typesense search results.
type TypesenseImage struct {
	URL string `json:"url"`
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

// UpsertUserBookReadsResponse is the response shape for upserting user book reads.
type UpsertUserBookReadsResponse struct {
	UpsertUserBookReads struct {
		UserBookReads []struct {
			ID int `json:"id"`
		} `json:"user_book_reads"`
	} `json:"upsert_user_book_reads"`
}
