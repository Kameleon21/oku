package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/colorprofile"
)

// captureOutput points the styled output at a buffer, with env as the
// environment colorprofile reads. The buffer is not a terminal, so this is
// what `oku reading > file` and `oku reading | less` get.
func captureOutput(t *testing.T, env ...string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := stdout
	stdout = colorprofile.NewWriter(&buf, env)
	t.Cleanup(func() { stdout = prev })
	return &buf
}

// TestPrintBooksIsPlainTextOffATerminal is the regression that v2 introduced
// and this PR fixes: a lipgloss v2 style always writes its colour, so without
// a profile writer in front of stdout `oku reading | less` was full of escape
// sequences and NO_COLOR did nothing.
func TestPrintBooksIsPlainTextOffATerminal(t *testing.T) {
	books := []model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune", Pages: 412, Authors: []string{"Frank Herbert"}},
			CurrentPage: 120},
	}

	for _, tc := range []struct {
		name string
		env  []string
	}{
		{"pipe", []string{"TERM=xterm-256color"}},
		{"NO_COLOR", []string{"TERM=xterm-256color", "NO_COLOR=1"}},
		{"no environment at all", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureOutput(t, tc.env...)
			if err := printBooks(books); err != nil {
				t.Fatalf("printBooks: %v", err)
			}
			got := buf.String()
			if strings.ContainsRune(got, 0x1b) {
				t.Fatalf("escape sequences off a terminal:\n%q", got)
			}
			want := "1. Dune\n   Frank Herbert\n   120/412\n"
			if got != want {
				t.Fatalf("printBooks =\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// TestPrintBooksSkipsAnAuthorlessRow: the emptiness test used to be on the
// styled string, which is never empty, so a book with no author printed a
// blank indented line.
func TestPrintBooksSkipsAnAuthorlessRow(t *testing.T) {
	buf := captureOutput(t, "TERM=xterm-256color")
	if err := printBooks([]model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune", Pages: 412}, CurrentPage: 120},
	}); err != nil {
		t.Fatalf("printBooks: %v", err)
	}
	if got, want := buf.String(), "1. Dune\n   120/412\n"; got != want {
		t.Fatalf("printBooks =\n%q\nwant\n%q", got, want)
	}
}

// TestPrintSearchResultsSkipsAnAuthorlessRow is the same bug in the other
// printer.
func TestPrintSearchResultsSkipsAnAuthorlessRow(t *testing.T) {
	buf := captureOutput(t, "TERM=xterm-256color")
	if err := printSearchResults([]model.SearchResult{{ID: 7, Title: "Dune"}}); err != nil {
		t.Fatalf("printSearchResults: %v", err)
	}
	got := buf.String()
	if strings.ContainsRune(got, 0x1b) {
		t.Fatalf("escape sequences off a terminal:\n%q", got)
	}
	if want := "1. Dune  [ID: 7]\n"; got != want {
		t.Fatalf("printSearchResults =\n%q\nwant\n%q", got, want)
	}
}

// TestOutputKeepsColourWhenItIsForced: the writer downsamples for a pipe, but
// CLICOLOR_FORCE is how a caller says it wants the colour anyway.
func TestOutputKeepsColourWhenItIsForced(t *testing.T) {
	buf := captureOutput(t, "TERM=xterm-256color", "CLICOLOR_FORCE=1")
	if err := printBooks([]model.UserBook{
		{Book: model.Book{ID: 1, Title: "Dune", Pages: 412}, CurrentPage: 120},
	}); err != nil {
		t.Fatalf("printBooks: %v", err)
	}
	if !strings.ContainsRune(buf.String(), 0x1b) {
		t.Fatalf("CLICOLOR_FORCE should keep the colour:\n%q", buf.String())
	}
}

func TestFinishWrite(t *testing.T) {
	var warned bytes.Buffer
	prev := warnOut
	warnOut = &warned
	t.Cleanup(func() { warnOut = prev })

	cases := []struct {
		name     string
		err      error
		wantErr  bool
		reported bool
		warning  string
	}{
		{"success", nil, false, true, ""},
		{"plain failure", errors.New("update status: rejected"), true, false, ""},
		{"cache refresh failure", fmt.Errorf("%w: database is closed", app.ErrCacheRefresh), false, true, "Warning: " + app.ErrCacheRefresh.Error() + ": database is closed\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warned.Reset()
			reported := false
			err := finishWrite(tc.err, func() { reported = true })
			if (err != nil) != tc.wantErr || reported != tc.reported {
				t.Fatalf("err = %v, reported = %v; want error %v, reported %v", err, reported, tc.wantErr, tc.reported)
			}
			if got := warned.String(); got != tc.warning {
				t.Fatalf("warnings = %q, want %q", got, tc.warning)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "rejected") {
				t.Fatalf("err = %v, want the original error", err)
			}
		})
	}
}
