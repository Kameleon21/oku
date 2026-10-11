package app

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/model"
)

// This file is where Hardcover's wire types become domain types: every field
// that crosses from the api package into model is decided here, and every
// Hardcover date is read by parseAPITime. The rest of App works on model
// types and hands api types only to the Hardcover seam.

// mapUserBook converts a library entry, with all the reads the query returned.
func mapUserBook(ab api.APIUserBook) model.UserBook {
	updatedAt := time.Now().UTC()
	if t, ok := parseAPITimePtr(ab.UpdatedAt); ok {
		updatedAt = t
	}

	ub := model.UserBook{
		ID:        ab.ID,
		BookID:    ab.Book.ID,
		StatusID:  model.Status(ab.StatusID),
		Book:      mapBook(ab.Book),
		UpdatedAt: updatedAt,
	}
	if ab.Rating != nil {
		ub.Rating = *ab.Rating
	}
	if ab.ReviewRaw != nil {
		ub.Review = *ab.ReviewRaw
	}
	if t, ok := parseAPITimePtr(ab.ReviewedAt); ok {
		ub.ReviewedAt = &t
	}
	for _, r := range ab.UserBookReads {
		ub.UserBookReads = append(ub.UserBookReads, mapRead(ab.ID, r))
	}
	return ub
}

// mapBook converts the book fields a query requested; the rest stay zero.
func mapBook(b api.APIBook) model.Book {
	book := model.Book{
		ID:             b.ID,
		Title:          b.Title,
		Authors:        mapAuthors(b.Contributions),
		Pages:          b.Pages,
		Slug:           b.Slug,
		Rating:         b.Rating,
		RatingsCount:   b.RatingsCount,
		ReviewsCount:   b.ReviewsCount,
		UsersCount:     b.UsersCount,
		UsersReadCount: b.UsersReadCount,
	}
	if b.Image != nil {
		book.ImageURL = b.Image.URL
	}
	if b.ReleaseDate != nil {
		book.ReleaseDate = strings.TrimSpace(*b.ReleaseDate)
	}
	book.CachedTags = rawJSON(b.CachedTags)
	if s := b.FeaturedSeries; s != nil {
		book.FeaturedSeries = strings.TrimSpace(s.Series.Name)
		if s.Position != nil {
			book.FeaturedSeriesPosition = *s.Position
		}
	}
	return book
}

func mapAuthors(contributions []api.APIContribution) []string {
	var authors []string
	for _, c := range contributions {
		authors = append(authors, c.Author.Name)
	}
	return authors
}

// mapRead converts one read of the library entry userBookID.
func mapRead(userBookID int, r api.APIUserBookRead) model.UserBookRead {
	read := model.UserBookRead{
		ID:            r.ID,
		UserBookID:    userBookID,
		ProgressPages: r.ProgressPages,
	}
	if t, ok := parseAPITimePtr(r.StartedAt); ok {
		read.StartedAt = &t
	}
	if t, ok := parseAPITimePtr(r.FinishedAt); ok {
		read.FinishedAt = &t
	}
	return read
}

// mapSearchHit converts a Typesense search document.
func mapSearchHit(doc api.TypesenseBookDoc) model.SearchResult {
	r := model.SearchResult{
		ID:      int(doc.ID),
		Title:   doc.Title,
		Authors: doc.AuthorNames,
		Pages:   int(doc.Pages),
		Slug:    doc.Slug,
		Rating:  float64(doc.Rating),
		Ratings: int(doc.Ratings),
	}
	if doc.Image != nil {
		r.ImageURL = doc.Image.URL
	}
	return r
}

// mapTrendingBook presents a trending book the way search results are shown.
func mapTrendingBook(b api.APIBook) model.SearchResult {
	return model.SearchResult{
		ID:      b.ID,
		Title:   b.Title,
		Authors: mapAuthors(b.Contributions),
		Pages:   b.Pages,
		Slug:    b.Slug,
		Rating:  b.Rating,
		Ratings: b.RatingsCount,
	}
}

func mapBookDetail(d api.BookDetail) model.BookDetail {
	return model.BookDetail{
		Book:                mapBook(d.APIBook),
		Description:         d.Description,
		Headline:            d.Headline,
		EditionsCount:       d.EditionsCount,
		RatingsDistribution: rawJSON(d.RatingsDistribution),
	}
}

func mapBookJournal(entries []api.JournalEntry) []model.BookJournalEntry {
	out := make([]model.BookJournalEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, model.BookJournalEntry{
			ID:       e.ID,
			BookID:   e.BookID,
			Event:    e.Event,
			Entry:    e.Entry,
			ActionAt: e.ActionAt,
		})
	}
	return out
}

// mapJournalActivity converts a journal row for the activity heatmap. Rows
// without a readable date are dropped.
func mapJournalActivity(j api.APIReadingJournal) (model.JournalEntry, bool) {
	t, ok := parseAPITimePtr(j.ActionAt)
	if !ok {
		return model.JournalEntry{}, false
	}
	return model.JournalEntry{ID: j.ID, ActionAt: t, Event: j.Event}, true
}

func mapGoal(g api.APIGoal) model.Goal {
	goal := model.Goal{
		ID:       g.ID,
		Metric:   g.Metric,
		Target:   g.Goal,
		Progress: float64(g.Progress),
		State:    g.State,
	}
	if t, ok := parseAPITimePtr(g.StartDate); ok {
		goal.StartDate = t
	}
	if t, ok := parseAPITimePtr(g.EndDate); ok {
		goal.EndDate = t
	}
	return goal
}

func mapGoalInput(g api.GoalInput) model.GoalInput {
	return model.GoalInput{
		Description:      g.Description,
		Goal:             g.Goal,
		Metric:           g.Metric,
		StartDate:        g.StartDate,
		EndDate:          g.EndDate,
		Conditions:       g.Conditions,
		PrivacySettingID: g.PrivacySettingID,
	}
}

// apiGoalInput is mapGoalInput's inverse, for sending a goal to Hardcover.
func apiGoalInput(g model.GoalInput) api.GoalInput {
	return api.GoalInput{
		Description:      g.Description,
		Goal:             g.Goal,
		Metric:           g.Metric,
		StartDate:        g.StartDate,
		EndDate:          g.EndDate,
		Conditions:       g.Conditions,
		PrivacySettingID: g.PrivacySettingID,
	}
}

// rawJSON keeps a JSON value the domain parses itself, such as cached_tags.
// An absent value and JSON null are both "".
func rawJSON(raw json.RawMessage) string {
	if s := strings.TrimSpace(string(raw)); s != "null" {
		return s
	}
	return ""
}

// parseAPITime reads a Hardcover timestamp or date, returning it in UTC.
func parseAPITime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// parseAPITimePtr is parseAPITime for an optional field; nil is not a time.
func parseAPITimePtr(raw *string) (time.Time, bool) {
	if raw == nil {
		return time.Time{}, false
	}
	return parseAPITime(*raw)
}
