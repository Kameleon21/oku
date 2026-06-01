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

	all, err := listCachedBooksForReview(a)
	if err != nil {
		return 0, err
	}

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

func listCachedBooksForReview(a *app.App) ([]model.UserBook, error) {
	statuses := []model.Status{
		model.StatusCurrentlyReading,
		model.StatusWantToRead,
		model.StatusRead,
		model.StatusDidNotFinish,
		model.StatusIgnored,
		model.StatusPaused,
	}

	seen := map[int]struct{}{}
	all := make([]model.UserBook, 0, 64)
	for _, status := range statuses {
		books, err := a.Store.ListUserBooks(status)
		if err != nil {
			return nil, err
		}
		for _, b := range books {
			if _, ok := seen[b.Book.ID]; ok {
				continue
			}
			seen[b.Book.ID] = struct{}{}
			all = append(all, b)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return strings.ToLower(all[i].Book.Title) < strings.ToLower(all[j].Book.Title)
	})
	return all, nil
}
