package model

import "testing"

func TestParseSearchMode(t *testing.T) {
	tests := []struct {
		in   string
		want SearchMode
		ok   bool
	}{
		{"book", SearchModeBook, true},
		{"author", SearchModeAuthor, true},
		{"genre", SearchModeGenre, true},
		{"tags", SearchModeGenre, true},
		{"unknown", "", false},
	}

	for _, tt := range tests {
		got, err := ParseSearchMode(tt.in)
		if tt.ok && err != nil {
			t.Fatalf("ParseSearchMode(%q) unexpected error: %v", tt.in, err)
		}
		if !tt.ok && err == nil {
			t.Fatalf("ParseSearchMode(%q) expected error", tt.in)
		}
		if tt.ok && got != tt.want {
			t.Fatalf("ParseSearchMode(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSearchModeNext(t *testing.T) {
	if SearchModeBook.Next() != SearchModeAuthor {
		t.Fatal("book should cycle to author")
	}
	if SearchModeAuthor.Next() != SearchModeGenre {
		t.Fatal("author should cycle to genre")
	}
	if SearchModeGenre.Next() != SearchModeBook {
		t.Fatal("genre should cycle to book")
	}
}
