package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
      rating
      review_raw
      reviewed_at
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
		strings.Contains(msg, "unknown argument") ||
		strings.Contains(msg, "field ") && strings.Contains(msg, " not found")
}

// SearchBooks searches for books by query string and returns parsed results.
// User input travels as a GraphQL variable, never interpolated into the
// query document, so no escaping is needed.
func (c *Client) SearchBooks(ctx context.Context, query string, perPage int, mode model.SearchMode) ([]model.SearchResult, error) {
	config := searchConfigForMode(mode)

	newSearchRequest := func(extraArgs string) *graphql.Request {
		q := fmt.Sprintf(`query($query: String!, $perPage: Int!) {
  search(query: $query, per_page: $perPage, page: 1%s) {
    results
  }
}`, extraArgs)
		req := graphql.NewRequest(q)
		req.Var("query", query)
		req.Var("perPage", perPage)
		return req
	}

	req := newSearchRequest(config.fieldsArg + config.weightsArg)

	var resp SearchResponse
	if err := c.do(ctx, req, &resp); err != nil {
		// If a search-specific argument/value is unsupported, transparently fall back.
		if isSearchFallbackError(err) {
			if err := c.do(ctx, newSearchRequest(""), &resp); err != nil {
				return nil, fmt.Errorf("SearchBooks: %w", err)
			}
		} else {
			return nil, fmt.Errorf("SearchBooks: %w", err)
		}
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
			Rating:  float64(doc.Rating),
			Ratings: int(doc.Ratings),
		}
		if doc.Image != nil {
			sr.ImageURL = doc.Image.URL
		}
		results = append(results, sr)
	}

	return results, nil
}

func isSearchFallbackError(err error) bool {
	if isSchemaFieldError(err) {
		return true
	}
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "book does not exist") ||
		strings.Contains(msg, "query_type") && strings.Contains(msg, "does not exist")
}

// GetBookRatingsByIDs fetches rating metadata for a set of book IDs.
func (c *Client) GetBookRatingsByIDs(ctx context.Context, ids []int) (map[int]model.Book, error) {
	if len(ids) == 0 {
		return map[int]model.Book{}, nil
	}

	seen := make(map[int]struct{}, len(ids))
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		parts = append(parts, strconv.Itoa(id))
	}
	if len(parts) == 0 {
		return map[int]model.Book{}, nil
	}

	q := fmt.Sprintf(`query {
  books(where: { id: { _in: [%s] } }) {
    id
    rating
    ratings_count
    reviews_count
    users_count
    users_read_count
    release_date
  }
}`, strings.Join(parts, ","))

	req := graphql.NewRequest(q)
	var resp struct {
		Books []APIBook `json:"books"`
	}
	if err := c.do(ctx, req, &resp); err != nil {
		return nil, fmt.Errorf("GetBookRatingsByIDs: %w", err)
	}

	out := make(map[int]model.Book, len(resp.Books))
	for _, b := range resp.Books {
		mb := model.Book{
			ID:             b.ID,
			Rating:         b.Rating,
			RatingsCount:   b.RatingsCount,
			ReviewsCount:   b.ReviewsCount,
			UsersCount:     b.UsersCount,
			UsersReadCount: b.UsersReadCount,
		}
		if b.ReleaseDate != nil {
			mb.ReleaseDate = *b.ReleaseDate
		}
		out[b.ID] = mb
	}
	return out, nil
}

type searchQueryConfig struct {
	fieldsArg  string
	weightsArg string
}

func searchConfigForMode(mode model.SearchMode) searchQueryConfig {
	switch mode {
	case model.SearchModeAuthor:
		return searchQueryConfig{
			fieldsArg:  `, fields: "author_names,title"`,
			weightsArg: `, weights: "8,2"`,
		}
	case model.SearchModeGenre:
		return searchQueryConfig{
			fieldsArg:  `, fields: "genres,tags,title,author_names"`,
			weightsArg: `, weights: "8,5,2,1"`,
		}
	default:
		return searchQueryConfig{}
	}
}
