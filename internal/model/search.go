package model

import (
	"fmt"
	"strings"
)

// SearchMode represents a discovery intent in search.
type SearchMode string

const (
	SearchModeBook   SearchMode = "book"
	SearchModeAuthor SearchMode = "author"
	SearchModeGenre  SearchMode = "genre"
)

// ParseSearchMode parses a user-facing search mode value.
func ParseSearchMode(raw string) (SearchMode, error) {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "book", "books", "title":
		return SearchModeBook, nil
	case "author", "authors":
		return SearchModeAuthor, nil
	case "genre", "genres", "tag", "tags":
		return SearchModeGenre, nil
	default:
		return "", fmt.Errorf("unknown search mode: %q (valid: book, author, genre)", raw)
	}
}

// Label returns a compact uppercase label suitable for badges.
func (m SearchMode) Label() string {
	switch m {
	case SearchModeAuthor:
		return "AUTHOR"
	case SearchModeGenre:
		return "GENRE"
	default:
		return "BOOK"
	}
}

// Description returns the applied discovery strategy.
func (m SearchMode) Description() string {
	switch m {
	case SearchModeAuthor:
		return "author-weighted"
	case SearchModeGenre:
		return "genre/tag-weighted"
	default:
		return "Hardcover default"
	}
}

// Next cycles through modes in a deterministic order.
func (m SearchMode) Next() SearchMode {
	switch m {
	case SearchModeBook:
		return SearchModeAuthor
	case SearchModeAuthor:
		return SearchModeGenre
	default:
		return SearchModeBook
	}
}
