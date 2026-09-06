package tui

import (
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/lipgloss"
)

// detailsView renders the selected library book into the right pane, at
// the row detail density asks for.
func detailsView(b *model.UserBook, density Density, w int, st styles) string {
	if b == nil {
		return st.dim.Render("  No book selected")
	}

	var sb strings.Builder

	sb.WriteString(st.head.Render(b.Book.Title))
	sb.WriteString("\n")
	author := fallback(b.Book.AuthorString(), "Unknown author")
	sb.WriteString(st.dim.Render(author))
	sb.WriteString("\n\n")

	writeField := func(label, value string) {
		sb.WriteString(st.label.Render(fmt.Sprintf("  %-10s ", label)))
		sb.WriteString(st.value.Render(value))
		sb.WriteString("\n")
	}

	writeField("Status", b.StatusID.Label())

	page := b.CurrentPage
	if len(b.UserBookReads) > 0 {
		page = b.UserBookReads[0].ProgressPages
	}
	progressText := b.Progress()
	if b.Book.Pages > 0 {
		// The field is 13 columns of label, then the text, two spaces, the bar
		// and " 100%". Size the bar to what is left so the row is never cut.
		barW := clampInt(w-20-lipgloss.Width(progressText), 8, 20)
		progressText += "  " + progressBar(page, b.Book.Pages, barW, st)
	}
	writeField("Progress", progressText)

	if density != DensityCompact {
		writeField("Book ID", fmt.Sprintf("%d", b.Book.ID))
		if b.Book.Pages > 0 {
			writeField("Pages", fmt.Sprintf("%d", b.Book.Pages))
		}
		if b.Book.Rating > 0 {
			rating := fmt.Sprintf("%.2f", b.Book.Rating)
			if b.Book.RatingsCount > 0 {
				rating += fmt.Sprintf(" (%s ratings)", format.Count(b.Book.RatingsCount))
			}
			writeField("Rating", rating)
		}
		if b.Book.ReviewsCount > 0 {
			writeField("Reviews", format.Count(b.Book.ReviewsCount))
		}
		if b.Book.UsersReadCount > 0 || b.Book.UsersCount > 0 {
			readers := ""
			if b.Book.UsersReadCount > 0 {
				readers = format.Count(b.Book.UsersReadCount) + " read"
			}
			if b.Book.UsersCount > 0 {
				if readers != "" {
					readers += " · "
				}
				readers += format.Count(b.Book.UsersCount) + " shelved"
			}
			writeField("Readers", readers)
		}
		if b.Book.ReleaseDate != "" {
			writeField("Released", b.Book.ReleaseDate)
		}
		if b.Book.FeaturedSeries != "" {
			series := b.Book.FeaturedSeries
			if b.Book.FeaturedSeriesPosition > 0 {
				series += fmt.Sprintf(" #%d", b.Book.FeaturedSeriesPosition)
			}
			writeField("Series", series)
		}
	}

	if density == DensityVerbose {
		if b.Book.Slug != "" {
			writeField("Slug", b.Book.Slug)
		}
		if len(b.UserBookReads) > 0 {
			if b.UserBookReads[0].StartedAt != nil {
				writeField("Started", b.UserBookReads[0].StartedAt.Format("2006-01-02"))
			}
			if b.UserBookReads[0].FinishedAt != nil {
				writeField("Finished", b.UserBookReads[0].FinishedAt.Format("2006-01-02"))
			}
		}
	}

	return sb.String()
}
