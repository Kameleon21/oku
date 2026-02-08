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

func printBooks(books []model.UserBook) {
	if jsonOutput {
		printJSON(books)
		return
	}
	if len(books) == 0 {
		fmt.Println(dimStyle.Render("No books found."))
		return
	}
	for i, ub := range books {
		num := dimStyle.Render(fmt.Sprintf("%d.", i+1))
		title := titleStyle.Render(ub.Book.Title)
		author := authorStyle.Render(ub.Book.AuthorString())
		progress := pageStyle.Render(ub.Progress())

		fmt.Printf("%s %s\n", num, title)
		if author != "" {
			fmt.Printf("   %s\n", author)
		}
		fmt.Printf("   %s\n", progress)
	}
}

func printSearchResults(results []model.SearchResult) {
	if jsonOutput {
		printJSON(results)
		return
	}
	if len(results) == 0 {
		fmt.Println(dimStyle.Render("No results found."))
		return
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
}

func printActiveBooks(books []model.UserBook) {
	if jsonOutput {
		printJSON(books)
		return
	}

	if len(books) == 0 {
		fmt.Println(dimStyle.Render("No active books."))
		return
	}

	fmt.Println(statusStyle.Render("Active Books"))
	for i, ub := range books {
		num := dimStyle.Render(fmt.Sprintf("%d.", i+1))
		fmt.Printf("%s %s\n", num, titleStyle.Render(ub.Book.Title))
		if a := ub.Book.AuthorString(); a != "" {
			fmt.Printf("      %s\n", authorStyle.Render(a))
		}
		fmt.Printf("      %s  %s\n",
			pageStyle.Render(ub.Progress()),
			statusStyle.Render(ub.StatusID.Label()))
	}
}

func printJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
