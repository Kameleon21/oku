package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Kameleon21/oku/internal/model"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	authorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	pageStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type outputDensity int

const (
	densityCompact outputDensity = iota
	densityDefault
	densityVerbose
)

func parseOutputDensity(raw string) (outputDensity, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default":
		return densityDefault, nil
	case "compact":
		return densityCompact, nil
	case "verbose":
		return densityVerbose, nil
	default:
		return densityDefault, fmt.Errorf("invalid --view value %q (valid: compact, default, verbose)", raw)
	}
}

func currentOutputDensity() outputDensity {
	d, err := parseOutputDensity(outputView)
	if err != nil {
		return densityDefault
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
		meta := bookMetaLine(ub.Book)
		detail := bookDetailLine(ub.Book)

		if density == densityCompact {
			if meta != "" {
				fmt.Printf("%s %s  %s  %s\n", num, title, progress, dimStyle.Render(meta))
			} else {
				fmt.Printf("%s %s  %s\n", num, title, progress)
			}
			continue
		}

		fmt.Printf("%s %s\n", num, title)
		if author != "" && density != densityCompact {
			fmt.Printf("   %s\n", author)
		}
		fmt.Printf("   %s\n", progress)
		if meta != "" {
			fmt.Printf("   %s\n", dimStyle.Render(meta))
		}
		if density == densityVerbose && detail != "" {
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
		meta := bookMetaLine(ub.Book)
		detail := bookDetailLine(ub.Book)

		if density == densityCompact {
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
		if density == densityVerbose && detail != "" {
			fmt.Printf("      %s\n", dimStyle.Render(detail))
		}
	}
	return nil
}

func bookMetaLine(b model.Book) string {
	parts := make([]string, 0, 4)

	if b.Rating > 0 {
		rating := fmt.Sprintf("★ %.2f", b.Rating)
		if b.RatingsCount > 0 {
			rating += fmt.Sprintf(" (%s ratings)", formatCount(b.RatingsCount))
		}
		parts = append(parts, rating)
	}

	if b.UsersReadCount > 0 {
		parts = append(parts, fmt.Sprintf("%s readers", formatCount(b.UsersReadCount)))
	}

	if b.ReleaseDate != "" {
		parts = append(parts, "released "+b.ReleaseDate)
	}

	if b.FeaturedSeries != "" {
		series := "series: " + b.FeaturedSeries
		if b.FeaturedSeriesPosition > 0 {
			series += fmt.Sprintf(" #%d", b.FeaturedSeriesPosition)
		}
		parts = append(parts, series)
	}

	return strings.Join(parts, " · ")
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

func formatCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// printJSON writes v to stdout. The encode error is returned so a closed pipe
// (`oku list --json | head`) or a full disk exits non-zero instead of looking
// like a successful empty run.
func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}
	return nil
}
