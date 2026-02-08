package api

import (
	"context"
	"fmt"

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

	return nil
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

// UpsertUserBookReads creates or updates a reading progress entry for the given user book.
func (c *Client) UpsertUserBookReads(ctx context.Context, userBookID int, progressPages int) error {
	q := fmt.Sprintf(`mutation {
  upsert_user_book_reads(user_book_id: %d, datesRead: [{ progress_pages: %d }]) {
    user_book_reads {
      id
    }
  }
}`, userBookID, progressPages)

	req := graphql.NewRequest(q)

	var resp UpsertUserBookReadsResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return fmt.Errorf("UpsertUserBookReads: %w", err)
	}

	return nil
}
