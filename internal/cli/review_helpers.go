package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Kameleon21/oku/internal/app"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/picker"
)

func resolveBookIDForReviewInput(a *app.App, title string, bookID int, pickerTitle string) (int, error) {
	if bookID > 0 {
		return bookID, nil
	}

	title = strings.TrimSpace(title)
	if title == "" {
		return 0, nil
	}

	if matched, err := a.GetUserBookForTitle(title); err != nil {
		return 0, err
	} else if matched != nil {
		return matched.Book.ID, nil
	}

	all, err := a.ListAllCachedUserBooks()
	if err != nil {
		return 0, err
	}
	sort.Slice(all, func(i, j int) bool {
		return strings.ToLower(all[i].Book.Title) < strings.ToLower(all[j].Book.Title)
	})

	lower := strings.ToLower(title)
	candidates := make([]model.UserBook, 0, len(all))
	for _, b := range all {
		if strings.Contains(strings.ToLower(b.Book.Title), lower) {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) == 0 {
		return 0, fmt.Errorf("book %q not found in cache. Run: oku sync", title)
	}
	if !isInteractiveTerminal(os.Stdin) || !isInteractiveTerminal(os.Stdout) {
		return 0, fmt.Errorf("multiple matches for %q. Re-run with --book <id>", title)
	}

	picked, err := picker.PickBook(candidates, pickerTitle)
	if err != nil {
		return 0, err
	}
	if picked == 0 {
		return 0, fmt.Errorf("no book selected")
	}
	return picked, nil
}
