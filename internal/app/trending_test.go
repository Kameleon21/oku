package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/Kameleon21/oku/internal/api"
)

func trendingBook(id, series int) api.APIBook {
	b := api.APIBook{ID: id}
	if series != 0 {
		b.FeaturedSeries = &api.APIBookSeries{}
		b.FeaturedSeries.Series.ID = series
	}
	return b
}

func trendingIDs(books []api.APIBook) []int {
	ids := []int{}
	for _, b := range books {
		ids = append(ids, b.ID)
	}
	return ids
}

func TestCurateTrending(t *testing.T) {
	// Rank order as Hardcover returns it: a series dominates the top.
	raw := []api.APIBook{
		trendingBook(1, 0),  // standalone, owned
		trendingBook(2, 10), // series 10, book 1, owned
		trendingBook(3, 10), // series 10, book 2: first unowned of its series
		trendingBook(4, 10), // series 10, book 3: dropped as a repeat
		trendingBook(5, 0),  // standalone
		trendingBook(6, 20), // series 20
		trendingBook(7, 0),  // standalone
	}
	owned := map[int]bool{1: true, 2: true}

	tests := []struct {
		name  string
		owned map[int]bool
		limit int
		want  []int
	}{
		{name: "drops owned and repeated series", owned: owned, limit: 20, want: []int{3, 5, 6, 7}},
		{name: "stops at limit", owned: owned, limit: 2, want: []int{3, 5}},
		{name: "empty library still dedupes series", owned: map[int]bool{}, limit: 20, want: []int{1, 2, 5, 6, 7}},
		{name: "everything filtered", owned: map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true}, limit: 20, want: []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trendingIDs(curateTrending(raw, tt.owned, tt.limit))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrendingBooksRejectsBadLimit(t *testing.T) {
	a := &App{}
	for _, limit := range []int{-1, 0, 101} {
		if _, err := a.TrendingBooks(context.Background(), "week", limit, false); err == nil {
			t.Errorf("limit %d accepted", limit)
		}
	}
}
