package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/tui"
)

// stdout is where every styled line goes. A lipgloss v2 style always writes
// its colour and nothing downsamples it on the way out — v1's renderer used
// to — so the CLI owns the writer that does. colorprofile answers NO_COLOR,
// CLICOLOR/CLICOLOR_FORCE and TERM, and drops to the no-TTY profile when
// stdout is a pipe or a file, which is what keeps `oku reading | less` plain
// text. Every fmt.Printf that renders a style has to go through outPrintf.
var stdout = colorprofile.NewWriter(os.Stdout, os.Environ())

func outPrintf(format string, a ...any) { fmt.Fprintf(stdout, format, a...) }
func outPrintln(a ...any)               { fmt.Fprintln(stdout, a...) }

// The CLI output shares the dashboard's palette, so a book looks the same in
// `oku reading` as it does in the TUI. lipgloss v2 resolves a colour when the
// style is built rather than when it renders, so the styles are built once
// the background is known — see outputStyles — rather than as adaptive
// constants.
var (
	out struct {
		title, author, page, status, dim lipgloss.Style
	}

	// outStylesOnce guards the one background query the CLI makes. The
	// styles are reached through the five accessors below rather than
	// directly, so the query happens the first time a command actually
	// prints something coloured and never otherwise: it is an OSC 11 round
	// trip with a timeout, and `oku tui`, `oku login`, `oku config` and
	// `oku timer` have no use for the answer. The dashboard asks for itself,
	// from the program's own input loop.
	outStylesOnce sync.Once
)

// The five output styles. Each one settles the palette first, so the
// detection cannot be forgotten at a new call site.
func titleStyle() lipgloss.Style  { outStyles(); return out.title }
func authorStyle() lipgloss.Style { outStyles(); return out.author }
func pageStyle() lipgloss.Style   { outStyles(); return out.page }
func statusStyle() lipgloss.Style { outStyles(); return out.status }
func dimStyle() lipgloss.Style    { outStyles(); return out.dim }

// outStyles settles the background the CLI's colours are chosen for: the
// `theme` config key when it pins one, the terminal's own answer when there
// is a terminal to ask, and dark when the output is a pipe or a file and
// nothing can be asked.
func outStyles() {
	outStylesOnce.Do(func() {
		isDark := true
		if pinned, ok := tui.PinnedDark(); ok {
			isDark = pinned
		} else if isInteractiveTerminal(os.Stdin) && isInteractiveTerminal(os.Stdout) {
			isDark = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
		}
		applyOutputTheme(tui.NewTheme(isDark))
	})
}

// applyOutputTheme builds the five output styles from one palette.
func applyOutputTheme(th tui.Theme) {
	out.title = lipgloss.NewStyle().Bold(true).Foreground(th.Accent)
	out.author = lipgloss.NewStyle().Foreground(th.TextMuted)
	out.page = lipgloss.NewStyle().Foreground(th.Success)
	out.status = lipgloss.NewStyle().Foreground(th.Heading).Bold(true)
	out.dim = lipgloss.NewStyle().Foreground(th.TextDim)
}

// currentOutputDensity is the --view flag, already validated by the root
// command's PersistentPreRunE, so a value that does not parse here can only be
// a programming error and falls back to the default.
func currentOutputDensity() tui.Density {
	d, err := tui.ParseDensity(outputView)
	if err != nil {
		return tui.DensityDefault
	}
	return d
}

func printBooks(books []model.UserBook) error {
	if jsonOutput {
		return printJSON(books)
	}
	if len(books) == 0 {
		outPrintln(dimStyle().Render("No books found."))
		return nil
	}
	density := currentOutputDensity()
	for i, ub := range books {
		num := dimStyle().Render(fmt.Sprintf("%d.", i+1))
		title := titleStyle().Render(ub.Book.Title)
		// The rendered string is never empty — a style wraps even "" in its
		// escapes — so the emptiness test is on the text itself, or a book
		// with no author prints a blank row.
		authorText := ub.Book.AuthorString()
		author := authorStyle().Render(authorText)
		progress := pageStyle().Render(ub.Progress())
		meta := format.BookMeta(ub.Book)
		detail := bookDetailLine(ub.Book)

		if density == tui.DensityCompact {
			if meta != "" {
				outPrintf("%s %s  %s  %s\n", num, title, progress, dimStyle().Render(meta))
			} else {
				outPrintf("%s %s  %s\n", num, title, progress)
			}
			continue
		}

		outPrintf("%s %s\n", num, title)
		if authorText != "" && density != tui.DensityCompact {
			outPrintf("   %s\n", author)
		}
		outPrintf("   %s\n", progress)
		if meta != "" {
			outPrintf("   %s\n", dimStyle().Render(meta))
		}
		if density == tui.DensityVerbose && detail != "" {
			outPrintf("   %s\n", dimStyle().Render(detail))
		}
	}
	return nil
}

func printSearchResults(results []model.SearchResult) error {
	if jsonOutput {
		return printJSON(results)
	}
	if len(results) == 0 {
		outPrintln(dimStyle().Render("No results found."))
		return nil
	}
	for i, r := range results {
		num := dimStyle().Render(fmt.Sprintf("%d.", i+1))
		title := titleStyle().Render(r.Title)
		// As above: the test is on the text, not on what the style made of it.
		authorText := strings.Join(r.Authors, ", ")
		author := authorStyle().Render(authorText)
		pages := ""
		if r.Pages > 0 {
			pages = pageStyle().Render(fmt.Sprintf("(%d pages)", r.Pages))
		}
		id := dimStyle().Render(fmt.Sprintf("[ID: %d]", r.ID))

		outPrintf("%s %s %s %s\n", num, title, pages, id)
		if authorText != "" {
			outPrintf("   %s\n", author)
		}
	}
	return nil
}

func printActiveBooks(books []model.UserBook) error {
	if jsonOutput {
		return printJSON(books)
	}

	if len(books) == 0 {
		outPrintln(dimStyle().Render("No active books."))
		return nil
	}

	density := currentOutputDensity()
	outPrintln(statusStyle().Render("Active Books"))
	for i, ub := range books {
		num := dimStyle().Render(fmt.Sprintf("%d.", i+1))
		progress := pageStyle().Render(ub.Progress())
		meta := format.BookMeta(ub.Book)
		detail := bookDetailLine(ub.Book)

		if density == tui.DensityCompact {
			if meta != "" {
				outPrintf("%s %s  %s  %s\n", num, titleStyle().Render(ub.Book.Title), progress, dimStyle().Render(meta))
			} else {
				outPrintf("%s %s  %s\n", num, titleStyle().Render(ub.Book.Title), progress)
			}
			continue
		}

		outPrintf("%s %s\n", num, titleStyle().Render(ub.Book.Title))
		if a := ub.Book.AuthorString(); a != "" {
			outPrintf("      %s\n", authorStyle().Render(a))
		}
		outPrintf("      %s  %s\n",
			progress,
			statusStyle().Render(ub.StatusID.Label()))
		if meta != "" {
			outPrintf("      %s\n", dimStyle().Render(meta))
		}
		if density == tui.DensityVerbose && detail != "" {
			outPrintf("      %s\n", dimStyle().Render(detail))
		}
	}
	return nil
}

func bookDetailLine(b model.Book) string {
	parts := make([]string, 0, 3)
	if b.Slug != "" {
		parts = append(parts, "slug:"+b.Slug)
	}
	if b.Pages > 0 {
		parts = append(parts, fmt.Sprintf("%d pages", b.Pages))
	}
	if b.ID > 0 {
		parts = append(parts, fmt.Sprintf("id:%d", b.ID))
	}
	return strings.Join(parts, " · ")
}

// printJSON writes v to stdout. The encode error is returned so a redirected
// stdout that fails mid-write (ENOSPC, EIO) exits non-zero instead of looking
// like a successful empty run. A closed pipe never reaches here: Go's runtime
// leaves SIGPIPE fatal for fds 1 and 2, so `| head` kills the process first.
func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}
	return nil
}
