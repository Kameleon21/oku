package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/tui"
)

// The CLI output shares the dashboard's palette, so a book looks the same in
// `oku reading` as it does in the TUI.
var (
	th = tui.DefaultTheme()

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(th.Accent)
	authorStyle = lipgloss.NewStyle().Foreground(th.TextMuted)
	pageStyle   = lipgloss.NewStyle().Foreground(th.Success)
	statusStyle = lipgloss.NewStyle().Foreground(th.Heading).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(th.TextDim)
)

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
		fmt.Println(dimStyle.Render("No books found."))
		return nil
	}
	density := currentOutputDensity()
	for i, ub := range books {
		num := dimStyle.Render(fmt.Sprintf("%d.", i+1))
		title := titleStyle.Render(ub.Book.Title)
		author := authorStyle.Render(ub.Book.AuthorString())
		progress := pageStyle.Render(ub.Progress())
		meta := format.BookMeta(ub.Book)
		detail := bookDetailLine(ub.Book)

		if density == tui.DensityCompact {
			if meta != "" {
				fmt.Printf("%s %s  %s  %s\n", num, title, progress, dimStyle.Render(meta))
			} else {
				fmt.Printf("%s %s  %s\n", num, title, progress)
			}
			continue
		}

		fmt.Printf("%s %s\n", num, title)
		if author != "" && density != tui.DensityCompact {
			fmt.Printf("   %s\n", author)
		}
		fmt.Printf("   %s\n", progress)
		if meta != "" {
			fmt.Printf("   %s\n", dimStyle.Render(meta))
		}
		if density == tui.DensityVerbose && detail != "" {
			fmt.Printf("   %s\n", dimStyle.Render(detail))
		}
	}
	return nil
}

func printSearchResults(results []model.SearchResult) error {
	if jsonOutput {
		return printJSON(results)
	}
	if len(results) == 0 {
		fmt.Println(dimStyle.Render("No results found."))
		return nil
	}
	for i, r := range results {
		num := dimStyle.Render(fmt.Sprintf("%d.", i+1))
		title := titleStyle.Render(r.Title)
		author := authorStyle.Render(strings.Join(r.Authors, ", "))
		pages := ""
		if r.Pages > 0 {
			pages = pageStyle.Render(fmt.Sprintf("(%d pages)", r.Pages))
		}
		id := dimStyle.Render(fmt.Sprintf("[ID: %d]", r.ID))

		fmt.Printf("%s %s %s %s\n", num, title, pages, id)
		if author != "" {
			fmt.Printf("   %s\n", author)
		}
	}
	return nil
}

func printActiveBooks(books []model.UserBook) error {
	if jsonOutput {
		return printJSON(books)
	}

	if len(books) == 0 {
		fmt.Println(dimStyle.Render("No active books."))
		return nil
	}

	density := currentOutputDensity()
	fmt.Println(statusStyle.Render("Active Books"))
	for i, ub := range books {
		num := dimStyle.Render(fmt.Sprintf("%d.", i+1))
		progress := pageStyle.Render(ub.Progress())
		meta := format.BookMeta(ub.Book)
		detail := bookDetailLine(ub.Book)

		if density == tui.DensityCompact {
			if meta != "" {
				fmt.Printf("%s %s  %s  %s\n", num, titleStyle.Render(ub.Book.Title), progress, dimStyle.Render(meta))
			} else {
				fmt.Printf("%s %s  %s\n", num, titleStyle.Render(ub.Book.Title), progress)
			}
			continue
		}

		fmt.Printf("%s %s\n", num, titleStyle.Render(ub.Book.Title))
		if a := ub.Book.AuthorString(); a != "" {
			fmt.Printf("      %s\n", authorStyle.Render(a))
		}
		fmt.Printf("      %s  %s\n",
			progress,
			statusStyle.Render(ub.StatusID.Label()))
		if meta != "" {
			fmt.Printf("      %s\n", dimStyle.Render(meta))
		}
		if density == tui.DensityVerbose && detail != "" {
			fmt.Printf("      %s\n", dimStyle.Render(detail))
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
