package model

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// BookDetail is community metadata fetched on demand, outside library sync.
type BookDetail struct {
	Book
	Description   string
	Headline      string
	EditionsCount int
	// RatingsDistribution is Hardcover's raw ratings_distribution JSON; read
	// it with ParseRatingDistribution.
	RatingsDistribution string
}

// BookJournalEntry is one of the user's notes or quotes on a book. ActionAt
// is the date as Hardcover reports it, shown verbatim.
type BookJournalEntry struct {
	ID       int    `json:"id"`
	BookID   int    `json:"book_id"`
	Event    string `json:"event"`
	Entry    string `json:"entry"`
	ActionAt string `json:"action_at"`
}

// GoalInput is the editable settings of a Hardcover reading goal. Dates are
// YYYY-MM-DD; Conditions are Hardcover's goal filters, kept opaque so an
// update preserves them.
type GoalInput struct {
	Description      string
	Goal             int
	Metric           string // "book" or "page"
	StartDate        string
	EndDate          string
	Conditions       map[string]any
	PrivacySettingID int
}

// Validate reports whether Hardcover would accept g.
func (g GoalInput) Validate() error {
	if g.Goal <= 0 {
		return fmt.Errorf("goal must be greater than zero")
	}
	if g.Metric != "book" && g.Metric != "page" {
		return fmt.Errorf("metric must be book or page")
	}
	start, e := time.Parse("2006-01-02", g.StartDate)
	if e != nil {
		return fmt.Errorf("invalid start date: %w", e)
	}
	end, e := time.Parse("2006-01-02", g.EndDate)
	if e != nil || end.Before(start) {
		return fmt.Errorf("end date must be YYYY-MM-DD and on or after start date")
	}
	if g.PrivacySettingID < 1 || g.PrivacySettingID > 3 {
		return fmt.Errorf("invalid privacy setting")
	}
	return nil
}

// RatingBucket is how many community ratings a book has at one star value.
type RatingBucket struct {
	Rating float64 `json:"rating"`
	Count  int     `json:"count"`
}

// ParseRatingDistribution accepts Hardcover's array and older keyed-object
// shape, returning buckets from the highest rating down.
func ParseRatingDistribution(raw string) ([]RatingBucket, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	var rows []RatingBucket
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		var values map[string]int
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, err
		}
		for key, count := range values {
			rating, err := strconv.ParseFloat(key, 64)
			if err != nil {
				return nil, err
			}
			rows = append(rows, RatingBucket{rating, count})
		}
	}
	for _, r := range rows {
		if r.Rating < 0 || r.Rating > 5 || math.IsNaN(r.Rating) || math.IsInf(r.Rating, 0) || r.Count < 0 {
			return nil, fmt.Errorf("invalid rating distribution")
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Rating > rows[j].Rating })
	return rows, nil
}
