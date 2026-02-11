package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/machinebox/graphql"
)

// GetMe returns the authenticated user's ID and username.
func (c *Client) GetMe(ctx context.Context) (int, string, error) {
	req := graphql.NewRequest(`query { me { id, username } }`)

	var resp MeResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return 0, "", fmt.Errorf("GetMe: %w", err)
	}
	if len(resp.Me) == 0 {
		return 0, "", fmt.Errorf("GetMe: empty response")
	}

	return resp.Me[0].ID, resp.Me[0].Username, nil
}

// ListUserBooks returns the user's books filtered by status ID.
func (c *Client) ListUserBooks(ctx context.Context, statusID int) ([]APIUserBook, error) {
	q := userBooksQuery(statusID, true)

	req := graphql.NewRequest(q)

	var resp UserBooksResponse
	if err := c.do(ctx, req, &resp); err != nil {
		// Schema can differ between API versions; fall back to a minimal stable query.
		if isSchemaFieldError(err) {
			q = userBooksQuery(statusID, false)
			req = graphql.NewRequest(q)
			if err := c.do(ctx, req, &resp); err != nil {
				return nil, fmt.Errorf("ListUserBooks: %w", err)
			}
		} else {
			return nil, fmt.Errorf("ListUserBooks: %w", err)
		}
	}
	if len(resp.Me) == 0 {
		return nil, nil
	}

	return resp.Me[0].UserBooks, nil
}

func userBooksQuery(statusID int, extended bool) string {
	if !extended {
		return fmt.Sprintf(`query {
  me {
    user_books(where: { status_id: { _eq: %d } }) {
      id
      status_id
      user_book_reads(order_by: { id: desc }, limit: 1) {
        id
        progress_pages
        started_at
        finished_at
      }
      book {
        id
        title
        pages
        slug
        contributions {
          author {
            name
          }
        }
        image {
          url
        }
      }
    }
  }
}`, statusID)
	}

	return fmt.Sprintf(`query {
  me {
    user_books(where: { status_id: { _eq: %d } }) {
      id
      status_id
      updated_at
      user_book_reads(order_by: { id: desc }, limit: 1) {
        id
        progress_pages
        started_at
        finished_at
      }
      book {
        id
        title
        pages
        slug
        rating
        ratings_count
        reviews_count
        users_count
        users_read_count
        release_date
        series_names
        contributions {
          author {
            name
          }
        }
        image {
          url
        }
      }
    }
  }
}`, statusID)
}

func isSchemaFieldError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cannot query field") ||
		strings.Contains(msg, "unknown field") ||
		strings.Contains(msg, "field ") && strings.Contains(msg, " not found")
}

// SearchBooks searches for books by query string and returns parsed results.
func (c *Client) SearchBooks(ctx context.Context, query string, perPage int) ([]model.SearchResult, error) {
	// Escape double quotes and backslashes in user input for safe embedding in the query string.
	sanitized := strings.ReplaceAll(query, `\`, `\\`)
	sanitized = strings.ReplaceAll(sanitized, `"`, `\"`)

	q := fmt.Sprintf(`query {
  search(query: "%s", query_type: "Book", per_page: %d, page: 1) {
    results
  }
}`, sanitized, perPage)

	req := graphql.NewRequest(q)

	var resp SearchResponse
	if err := c.do(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("SearchBooks: %w", err)
	}

	var tsResults TypesenseResults
	if err := json.Unmarshal(resp.Search.Results, &tsResults); err != nil {
		return nil, fmt.Errorf("SearchBooks: parsing results: %w", err)
	}

	results := make([]model.SearchResult, 0, len(tsResults.Hits))
	for _, hit := range tsResults.Hits {
		doc := hit.Document
		sr := model.SearchResult{
			ID:      int(doc.ID),
			Title:   doc.Title,
			Authors: doc.AuthorNames,
			Pages:   int(doc.Pages),
			Slug:    doc.Slug,
		}
		if doc.Image != nil {
			sr.ImageURL = doc.Image.URL
		}
		results = append(results, sr)
	}

	return results, nil
}
