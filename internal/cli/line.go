package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"unicode"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/config"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/store"
	"github.com/Kameleon21/oku/internal/tui"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

// defaultLineWidth keeps the segment short enough to share a prompt line with
// the directory and git status.
const defaultLineWidth = 24

// lineJSON is `oku line --json`: the full title rather than the cut one
// (cleaned the same way), and a null percent when the book has no page count.
type lineJSON struct {
	Title   string `json:"title"`
	Page    int    `json:"page"`
	Pages   int    `json:"pages"`
	Percent *int   `json:"percent"`
	Others  int    `json:"others"`
}

// newLineCmd is the shell prompt and status-line segment. A prompt runs it
// on every redraw, so it reads only the local cache: no auth, no network, no
// config, no styled output (styles query the terminal's background first).
// It prints nothing when nothing is in progress, so the segment disappears.
func newLineCmd() *cobra.Command {
	var width int
	cmd := &cobra.Command{
		Use:   "line",
		Short: "Print the book you're reading as one line, for a shell prompt or tmux",
		Args:  cobra.NoArgs,
		// Replaces the root's hook, which loads the config to apply its
		// theme: line draws nothing, and a bad config would otherwise print
		// an error into every prompt. The flag check it also does stays.
		PersistentPreRunE: func(*cobra.Command, []string) error {
			_, err := tui.ParseDensity(outputView)
			return err
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCount("width", width, 0); err != nil {
				return err
			}

			books, err := cachedReading()
			if err != nil {
				return err
			}

			var summary *lineJSON
			if len(books) > 0 {
				b := books[0]
				summary = &lineJSON{Title: cleanTitle(b.Book.Title), Page: b.Page(), Pages: b.Book.Pages, Others: len(books) - 1}
				if b.Book.Pages > 0 {
					pct := int(model.Fraction(b.Page(), b.Book.Pages) * 100)
					summary.Percent = &pct
				}
			}
			return readerOutput(cmd, summary, func() error {
				if line := formatLine(books, width); line != "" {
					fmt.Fprintln(cmd.OutOrStdout(), line)
				}
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&width, "width", defaultLineWidth, "Cut the title to this many columns")
	return cmd
}

// cachedReading is the cached currently-reading list, the book worked on
// most recently first. Before oku has ever run there is no cache, and opening
// one would create it, so a missing cache is simply no books. The store is
// opened directly rather than through initLocalApp, which loads the config
// this command has no use for.
func cachedReading() ([]model.UserBook, error) {
	dbPath, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	db, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	a := app.New(nil, db, config.Defaults())
	defer a.Close()
	return a.RecentlyReading()
}

// cleanTitle makes a title from Hardcover safe for one line of a prompt:
// escape sequences, control characters and bidi overrides (which would
// reverse the rest of the prompt) dropped, runs of whitespace (newlines and
// tabs included) folded to one space. Zero-width joiners stay: emoji and
// some scripts need them.
func cleanTitle(title string) string {
	title = strings.Join(strings.Fields(ansi.Strip(title)), " ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || isBidiControl(r) {
			return -1
		}
		return r
	}, title)
}

// isBidiControl reports the embedding, override and isolate characters.
func isBidiControl(r rune) bool {
	return (r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069')
}

// formatLine is the prompt segment for the currently-reading books, newest
// first: the first book's title cut to width cells, its percent when the page
// count is known, and how many other books are in progress. No books, no line.
func formatLine(books []model.UserBook, width int) string {
	if len(books) == 0 {
		return ""
	}
	b := books[0]

	parts := []string{"📖"}
	title := cleanTitle(b.Book.Title)
	if ansi.StringWidth(title) > width {
		title = strings.TrimRight(ansi.Truncate(title, width-1, ""), " ") + "…"
	}
	if title != "" {
		parts = append(parts, title)
	}
	if b.Book.Pages > 0 {
		parts = append(parts, fmt.Sprintf("%d%%", int(model.Fraction(b.Page(), b.Book.Pages)*100)))
	}
	if others := len(books) - 1; others > 0 {
		parts = append(parts, fmt.Sprintf("+%d", others))
	}
	return strings.Join(parts, " ")
}
