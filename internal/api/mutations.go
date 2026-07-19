package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/machinebox/graphql"
)

// InsertUserBook adds a book to the user's library with the given status and returns the user_book ID.
func (c *Client) InsertUserBook(ctx context.Context, bookID int, statusID int) (int, error) {
	q := fmt.Sprintf(`mutation {
  insert_user_book(object: { book_id: %d, status_id: %d }) {
    id
    user_book {
      id
    }
    error
  }
}`, bookID, statusID)

	req := graphql.NewRequest(q)

	var resp InsertUserBookResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return 0, fmt.Errorf("InsertUserBook: %w", err)
	}

	r := resp.InsertUserBook
	if r.Error != nil {
		return 0, fmt.Errorf("InsertUserBook: API error: %s", *r.Error)
	}
	if r.UserBook == nil {
		return 0, fmt.Errorf("InsertUserBook: no user_book returned")
	}

	return r.UserBook.ID, nil
}

// UpdateUserBookStatus changes the status of an existing user book.
func (c *Client) UpdateUserBookStatus(ctx context.Context, userBookID int, statusID int) error {
	q := fmt.Sprintf(`mutation {
  update_user_book(id: %d, object: { status_id: %d }) {
    id
    error
  }
}`, userBookID, statusID)

	req := graphql.NewRequest(q)

	var resp UpdateUserBookResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return fmt.Errorf("UpdateUserBookStatus: %w", err)
	}

	r := resp.UpdateUserBook
	if r.Error != nil {
		return fmt.Errorf("UpdateUserBookStatus: API error: %s", *r.Error)
	}
	if r.ID == nil {
		return fmt.Errorf("UpdateUserBookStatus: no updated record returned")
	}

	return nil
}

// UpdateUserBookRating updates a user-book rating.
func (c *Client) UpdateUserBookRating(ctx context.Context, userBookID int, rating float64) error {
	req := graphql.NewRequest(`mutation($id: Int!, $rating: numeric!) {
  update_user_book(id: $id, object: { rating: $rating }) {
    id
    error
  }
}`)
	req.Var("id", userBookID)
	req.Var("rating", rating)

	var resp UpdateUserBookResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return fmt.Errorf("UpdateUserBookRating: %w", err)
	}

	r := resp.UpdateUserBook
	if r.Error != nil {
		return fmt.Errorf("UpdateUserBookRating: API error: %s", *r.Error)
	}
	if r.ID == nil {
		return fmt.Errorf("UpdateUserBookRating: no updated record returned")
	}

	return nil
}

// UpdateUserBookReviewAndRating updates review text, reviewed timestamp, and rating.
func (c *Client) UpdateUserBookReviewAndRating(
	ctx context.Context,
	userBookID int,
	rating float64,
	review string,
	reviewedAt string,
) error {
	review = strings.TrimSpace(review)
	if review == "" {
		return c.UpdateUserBookRating(ctx, userBookID, rating)
	}

	attempts := []bool{true, false}
	var lastErr error
	for _, includeReviewedAt := range attempts {
		if err := c.updateUserBookReviewAndRatingAttempt(
			ctx,
			userBookID,
			rating,
			reviewTextToSlate(review),
			reviewedAt,
			includeReviewedAt,
		); err != nil {
			if !isSchemaFieldError(err) {
				return err
			}
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (c *Client) updateUserBookReviewAndRatingAttempt(
	ctx context.Context,
	userBookID int,
	rating float64,
	reviewSlate []map[string]any,
	reviewedAt string,
	includeReviewedAt bool,
) error {
	req := graphql.NewRequest(updateUserBookReviewMutation(includeReviewedAt))
	req.Var("id", userBookID)
	req.Var("rating", rating)
	req.Var("reviewSlate", reviewSlate)
	if includeReviewedAt {
		req.Var("reviewedAt", reviewedAt)
	}

	var resp UpdateUserBookResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return fmt.Errorf("UpdateUserBookReviewAndRating: %w", err)
	}

	r := resp.UpdateUserBook
	if r.Error != nil {
		return fmt.Errorf("UpdateUserBookReviewAndRating: API error: %s", *r.Error)
	}
	if r.ID == nil {
		return fmt.Errorf("UpdateUserBookReviewAndRating: no updated record returned")
	}

	return nil
}

func updateUserBookReviewMutation(includeReviewedAt bool) string {
	if includeReviewedAt {
		return `mutation($id: Int!, $rating: numeric!, $reviewSlate: jsonb!, $reviewedAt: date!) {
  update_user_book(
    id: $id,
    object: { rating: $rating, review_slate: $reviewSlate, reviewed_at: $reviewedAt }
  ) {
    id
    error
  }
}`
	}

	return `mutation($id: Int!, $rating: numeric!, $reviewSlate: jsonb!) {
  update_user_book(
    id: $id,
    object: { rating: $rating, review_slate: $reviewSlate }
  ) {
    id
    error
  }
}`
}

func reviewTextToSlate(review string) []map[string]any {
	paragraphs := strings.Split(strings.ReplaceAll(review, "\r\n", "\n"), "\n")
	out := make([]map[string]any, 0, len(paragraphs))
	for _, p := range paragraphs {
		out = append(out, map[string]any{
			"type": "paragraph",
			"children": []map[string]any{
				{"text": p},
			},
		})
	}
	return out
}

// UpdateReadProgress updates the progress pages on an existing user book read.
func (c *Client) UpdateReadProgress(ctx context.Context, userBookReadID int, progressPages int) error {
	q := fmt.Sprintf(`mutation {
  update_user_book_read(id: %d, object: { progress_pages: %d }) {
    id
    error
  }
}`, userBookReadID, progressPages)

	req := graphql.NewRequest(q)

	var resp UpdateUserBookReadResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return fmt.Errorf("UpdateReadProgress: %w", err)
	}

	r := resp.UpdateUserBookRead
	if r.Error != nil {
		return fmt.Errorf("UpdateReadProgress: API error: %s", *r.Error)
	}

	return nil
}

// InsertUserBookRead creates a new reading progress entry for the given user book.
func (c *Client) InsertUserBookRead(ctx context.Context, userBookID int, progressPages int) (*APIUserBookRead, error) {
	startedAt := time.Now().UTC().Format("2006-01-02")
	q := fmt.Sprintf(`mutation {
  insert_user_book_read(
    user_book_id: %d,
    user_book_read: { progress_pages: %d, started_at: "%s" }
  ) {
    id
    error
    user_book_read {
      id
      progress_pages
      started_at
      finished_at
    }
  }
}`, userBookID, progressPages, startedAt)

	req := graphql.NewRequest(q)

	var resp InsertUserBookReadResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("InsertUserBookRead: %w", err)
	}

	r := resp.InsertUserBookRead
	if r.Error != nil {
		return nil, fmt.Errorf("InsertUserBookRead: API error: %s", *r.Error)
	}
	if r.UserBookRead == nil {
		return nil, fmt.Errorf("InsertUserBookRead: no user_book_read returned")
	}

	return r.UserBookRead, nil
}

// InsertProgressJournal creates a "progress_updated" reading journal entry.
// Hardcover's website builds its activity heatmap from reading_journals, which
// the site creates on every progress update; update_user_book_read alone never
// produces one, so API-driven updates stay invisible on the calendar without this.
func (c *Client) InsertProgressJournal(ctx context.Context, bookID, progressPages, totalPages, privacySettingID int) error {
	actionAt := time.Now().Format("2006-01-02")
	req := graphql.NewRequest(`mutation($object: ReadingJournalCreateType!) {
  insert_reading_journal(object: $object) {
    reading_journal {
      id
    }
  }
}`)
	req.Var("object", progressJournalObject(bookID, progressPages, totalPages, privacySettingID, actionAt))

	var resp InsertReadingJournalResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return fmt.Errorf("InsertProgressJournal: %w", err)
	}
	if resp.InsertReadingJournal.ReadingJournal == nil {
		return fmt.Errorf("InsertProgressJournal: no reading_journal returned")
	}

	return nil
}

func progressJournalObject(bookID, progressPages, totalPages, privacySettingID int, actionAt string) map[string]any {
	position := map[string]any{
		"type":  "pages",
		"value": progressPages,
	}
	if totalPages > 0 {
		position["possible"] = totalPages
	}
	return map[string]any{
		"book_id":            bookID,
		"event":              "progress_updated",
		"action_at":          actionAt,
		"privacy_setting_id": privacySettingID,
		"tags":               []any{},
		"metadata":           map[string]any{"position": position},
	}
}

// UpsertUserBookReads creates or updates a reading progress entry for the given user book.
func (c *Client) UpsertUserBookReads(ctx context.Context, userBookID int, progressPages int) error {
	q := fmt.Sprintf(`mutation {
  upsert_user_book_reads(user_book_id: %d, datesRead: [{ progress_pages: %d }]) {
    error
    user_book_id
  }
}`, userBookID, progressPages)

	req := graphql.NewRequest(q)

	var resp UpsertUserBookReadsResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return fmt.Errorf("UpsertUserBookReads: %w", err)
	}
	if resp.UpsertUserBookReads.Error != nil {
		return fmt.Errorf("UpsertUserBookReads: API error: %s", *resp.UpsertUserBookReads.Error)
	}

	return nil
}
