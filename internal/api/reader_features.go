package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/machinebox/graphql"
)

// BookDetail contains community metadata fetched on demand, outside library sync.
type BookDetail struct {
	APIBook
	Description         string          `json:"description"`
	Headline            string          `json:"headline"`
	EditionsCount       int             `json:"editions_count"`
	RatingsDistribution json.RawMessage `json:"ratings_distribution"`
}

func (c *Client) BookDetail(ctx context.Context, id int) (*BookDetail, error) {
	req := graphql.NewRequest(`query($id: Int!) { books_by_pk(id:$id) { id title slug pages rating ratings_count description headline editions_count ratings_distribution contributions { author { name } } } }`)
	req.Var("id", id)
	var resp struct {
		Book *BookDetail `json:"books_by_pk"`
	}
	if err := c.do(ctx, req, &resp); err != nil {
		return nil, err
	}
	if resp.Book == nil {
		return nil, fmt.Errorf("book %d not found", id)
	}
	return resp.Book, nil
}

type JournalEntry struct {
	ID       int    `json:"id"`
	BookID   int    `json:"book_id"`
	Event    string `json:"event"`
	Entry    string `json:"entry"`
	ActionAt string `json:"action_at"`
}

func (c *Client) CreateJournalEntry(ctx context.Context, bookID int, event, entry string, privacy int) (int, error) {
	if bookID <= 0 || (event != "note" && event != "quote") || strings.TrimSpace(entry) == "" {
		return 0, fmt.Errorf("a book, note/quote event, and non-empty text are required")
	}
	if privacy < 1 || privacy > 3 {
		return 0, fmt.Errorf("invalid privacy setting")
	}
	req := graphql.NewRequest(`mutation($object: ReadingJournalCreateType!) { insert_reading_journal(object:$object) { id errors } }`)
	req.Var("object", map[string]any{"book_id": bookID, "event": event, "entry": entry, "privacy_setting_id": privacy, "action_at": time.Now().Format("2006-01-02"), "tags": []any{}})
	var resp struct {
		Result mutationResult `json:"insert_reading_journal"`
	}
	if err := c.doMutation(ctx, req, &resp); err != nil {
		return 0, err
	}
	return resp.Result.checked("save journal entry")
}

func (c *Client) BookJournal(ctx context.Context, bookID int) ([]JournalEntry, error) {
	uid, _, err := c.GetMe(ctx)
	if err != nil {
		return nil, err
	}
	out := []JournalEntry{}
	for offset := 0; ; offset += 100 {
		req := graphql.NewRequest(`query($user:Int!, $book:Int!, $offset:Int!) { reading_journals(where:{user_id:{_eq:$user},book_id:{_eq:$book},event:{_in:["note","quote"]}},order_by:{id:asc},limit:100,offset:$offset) { id book_id event entry action_at } }`)
		req.Var("user", uid)
		req.Var("book", bookID)
		req.Var("offset", offset)
		var resp struct {
			Entries []JournalEntry `json:"reading_journals"`
		}
		if err := c.do(ctx, req, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Entries...)
		if len(resp.Entries) < 100 {
			return out, nil
		}
	}
}

type mutationResult struct {
	ID     *int     `json:"id"`
	Error  *string  `json:"error"`
	Errors []string `json:"errors"`
}

func (r mutationResult) checked(action string) (int, error) {
	if r.Error != nil && *r.Error != "" {
		return 0, fmt.Errorf("%s: %s", action, *r.Error)
	}
	if len(r.Errors) > 0 {
		return 0, fmt.Errorf("%s: %s", action, strings.Join(r.Errors, "; "))
	}
	if r.ID == nil || *r.ID <= 0 {
		return 0, fmt.Errorf("%s: no record returned", action)
	}
	return *r.ID, nil
}

type GoalInput struct {
	Description      string         `json:"description"`
	Goal             int            `json:"goal"`
	Metric           string         `json:"metric"`
	StartDate        string         `json:"start_date"`
	EndDate          string         `json:"end_date"`
	Conditions       map[string]any `json:"conditions"`
	PrivacySettingID int            `json:"privacy_setting_id"`
}

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
func (c *Client) SaveGoal(ctx context.Context, id int, g GoalInput) (int, error) {
	if err := g.Validate(); err != nil {
		return 0, err
	}
	if g.Conditions == nil {
		g.Conditions = map[string]any{}
	}
	query := `mutation($object:GoalInput!) { result:insert_goal(object:$object) { id errors } }`
	if id > 0 {
		query = `mutation($id:Int!, $object:GoalInput!) { result:update_goal(id:$id,object:$object) { id errors } }`
	}
	req := graphql.NewRequest(query)
	req.Var("object", g)
	if id > 0 {
		req.Var("id", id)
	}
	var resp struct {
		Result mutationResult `json:"result"`
	}
	if err := c.doMutation(ctx, req, &resp); err != nil {
		return 0, err
	}
	return resp.Result.checked("save goal")
}

func (c *Client) Trending(ctx context.Context, duration string, limit int) ([]APIBook, error) {
	switch duration {
	case "week", "month", "three_month", "one_year", "all":
	default:
		return nil, fmt.Errorf("invalid trending period %q", duration)
	}
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("limit must be 1-100")
	}
	req := graphql.NewRequest(`query($duration:TrendingDuration!, $limit:Int!) { books_trending(duration:$duration,limit:$limit) { ids error } }`)
	req.Var("duration", duration)
	req.Var("limit", limit)
	var resp struct {
		Result struct {
			IDs   []int   `json:"ids"`
			Error *string `json:"error"`
		} `json:"books_trending"`
	}
	if err := c.do(ctx, req, &resp); err != nil {
		return nil, err
	}
	if resp.Result.Error != nil {
		return nil, fmt.Errorf("trending: %s", *resp.Result.Error)
	}
	if len(resp.Result.IDs) == 0 {
		return []APIBook{}, nil
	}
	req = graphql.NewRequest(`query($ids:[Int!]!) { books(where:{id:{_in:$ids}}) { id title slug pages rating ratings_count contributions { author { name } } } }`)
	req.Var("ids", resp.Result.IDs)
	var books struct {
		Books []APIBook `json:"books"`
	}
	if err := c.do(ctx, req, &books); err != nil {
		return nil, err
	}
	byID := map[int]APIBook{}
	for _, b := range books.Books {
		byID[b.ID] = b
	}
	out := []APIBook{}
	for _, id := range resp.Result.IDs {
		if b, ok := byID[id]; ok {
			out = append(out, b)
		}
	}
	return out, nil
}

type RankedBook struct {
	ID       int `json:"id"`
	BookID   int `json:"book_id"`
	Position int `json:"position"`
}
type RankedList struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Ranked bool   `json:"ranked"`
}

const TBRListName = "Oku reading queue"

func (c *Client) ReadingQueue(ctx context.Context, create bool) (int, []RankedBook, error) {
	uid, _, err := c.GetMe(ctx)
	if err != nil {
		return 0, nil, err
	}
	req := graphql.NewRequest(`query($user:Int!, $name:String!) { lists(where:{user_id:{_eq:$user},name:{_eq:$name},ranked:{_eq:true}},order_by:{id:asc}) { id name ranked } }`)
	req.Var("user", uid)
	req.Var("name", TBRListName)
	var resp struct {
		Lists []RankedList `json:"lists"`
	}
	if err := c.do(ctx, req, &resp); err != nil {
		return 0, nil, err
	}
	id := 0
	if len(resp.Lists) > 0 {
		id = resp.Lists[0].ID
	} else if create {
		req = graphql.NewRequest(`mutation($object:ListInput!) { result:insert_list(object:$object) { id errors } }`)
		req.Var("object", map[string]any{"name": TBRListName, "ranked": true, "privacy_setting_id": 3})
		var result struct {
			Result mutationResult `json:"result"`
		}
		if err := c.doMutation(ctx, req, &result); err != nil {
			return 0, nil, err
		}
		id, err = result.Result.checked("create reading queue")
		if err != nil {
			return 0, nil, err
		}
	} else {
		return 0, []RankedBook{}, nil
	}
	out := []RankedBook{}
	for offset := 0; ; offset += 100 {
		req = graphql.NewRequest(`query($id:Int!, $offset:Int!) { list_books(where:{list_id:{_eq:$id}},order_by:[{position:asc},{id:asc}],limit:100,offset:$offset) { id book_id position } }`)
		req.Var("id", id)
		req.Var("offset", offset)
		var page struct {
			Books []RankedBook `json:"list_books"`
		}
		if err := c.do(ctx, req, &page); err != nil {
			return 0, nil, err
		}
		out = append(out, page.Books...)
		if len(page.Books) < 100 {
			return id, out, nil
		}
	}
}

// SetQueuePositions updates the affected ranks in one database mutation request.
func (c *Client) SetQueuePositions(ctx context.Context, listID int, rows []RankedBook) error {
	if len(rows) == 0 {
		return nil
	}
	if len(rows) > 5 {
		return fmt.Errorf("at most five rank changes per request")
	}
	var q strings.Builder
	q.WriteString("mutation($list:Int!) {")
	for i, r := range rows {
		fmt.Fprintf(&q, " p%d:update_list_books(where:{list_id:{_eq:$list},id:{_eq:%d}},_set:{position:%d}) { affected_rows }", i, r.ID, r.Position)
	}
	q.WriteString(" }")
	req := graphql.NewRequest(q.String())
	req.Var("list", listID)
	var resp map[string]struct {
		Affected int `json:"affected_rows"`
	}
	if err := c.doMutation(ctx, req, &resp); err != nil {
		return err
	}
	for i := range rows {
		if resp[fmt.Sprintf("p%d", i)].Affected != 1 {
			return fmt.Errorf("queue changed remotely; refresh before retrying")
		}
	}
	return nil
}

// ExportLibrary pages by primary key order and includes every read, not only the latest.
func (c *Client) ExportLibrary(ctx context.Context) ([]APIUserBook, error) {
	uid, _, err := c.GetMe(ctx)
	if err != nil {
		return nil, err
	}
	out := []APIUserBook{}
	for offset := 0; ; offset += 100 {
		req := graphql.NewRequest(`query($user:Int!, $offset:Int!) { user_books(where:{user_id:{_eq:$user}},order_by:{id:asc},limit:100,offset:$offset) { id status_id rating review_raw reviewed_at updated_at user_book_reads(order_by:{id:desc}) { id progress_pages started_at finished_at } book { id title slug pages } } }`)
		req.Var("user", uid)
		req.Var("offset", offset)
		var resp struct {
			Books []APIUserBook `json:"user_books"`
		}
		if err := c.do(ctx, req, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Books...)
		if len(resp.Books) < 100 {
			return out, nil
		}
	}
}
func (c *Client) LookupISBN(ctx context.Context, isbn string) (int, error) {
	req := graphql.NewRequest(`query($isbn:String!) { editions(where:{_or:[{isbn_13:{_eq:$isbn}},{isbn_10:{_eq:$isbn}}]},limit:2) { book_id } }`)
	req.Var("isbn", isbn)
	var resp struct {
		Editions []struct {
			BookID int `json:"book_id"`
		} `json:"editions"`
	}
	if err := c.do(ctx, req, &resp); err != nil {
		return 0, err
	}
	if len(resp.Editions) == 0 {
		return 0, &ISBNMatchError{ISBN: isbn}
	}
	id := resp.Editions[0].BookID
	for _, e := range resp.Editions {
		if e.BookID != id {
			return 0, &ISBNMatchError{ISBN: isbn, Ambiguous: true}
		}
	}
	return id, nil
}

// ImportReads replaces read history only for a newly inserted library record.
func (c *Client) ImportReads(ctx context.Context, userBookID int, reads []APIUserBookRead) error {
	if len(reads) == 0 {
		return nil
	}
	dates := make([]map[string]any, 0, len(reads))
	for _, r := range reads {
		d := map[string]any{"progress_pages": r.ProgressPages}
		if r.StartedAt != nil {
			d["started_at"] = *r.StartedAt
		}
		if r.FinishedAt != nil {
			d["finished_at"] = *r.FinishedAt
		}
		dates = append(dates, d)
	}
	req := graphql.NewRequest(`mutation($id:Int!, $dates:[DatesReadInput!]!) { upsert_user_book_reads(user_book_id:$id, datesRead:$dates) { error user_book_id } }`)
	req.Var("id", userBookID)
	req.Var("dates", dates)
	var resp struct {
		Result struct {
			Error *string `json:"error"`
			ID    int     `json:"user_book_id"`
		} `json:"upsert_user_book_reads"`
	}
	if err := c.doMutation(ctx, req, &resp); err != nil {
		return err
	}
	if resp.Result.Error != nil {
		return fmt.Errorf("import reads: %s", *resp.Result.Error)
	}
	if resp.Result.ID != userBookID {
		return fmt.Errorf("import reads: missing confirmation")
	}
	return nil
}

// GoalSettings preserves filters, dates, description and privacy when editing a target.
func (c *Client) GoalSettings(ctx context.Context, id int) (GoalInput, error) {
	uid, _, err := c.GetMe(ctx)
	if err != nil {
		return GoalInput{}, err
	}
	req := graphql.NewRequest(`query($id:Int!, $user:Int!) { goals(where:{id:{_eq:$id},user_id:{_eq:$user}}) { description goal metric start_date end_date conditions privacy_setting_id } }`)
	req.Var("id", id)
	req.Var("user", uid)
	var resp struct {
		Goals []GoalInput `json:"goals"`
	}
	if err := c.do(ctx, req, &resp); err != nil {
		return GoalInput{}, err
	}
	if len(resp.Goals) != 1 {
		return GoalInput{}, fmt.Errorf("goal %d not found in your account", id)
	}
	return resp.Goals[0], nil
}

// doMutation makes one attempt: retrying a creation after a lost response can
// create duplicate notes, goals, or library records. Reads retain normal retries.
func (c *Client) doMutation(ctx context.Context, req *graphql.Request, resp any) error {
	ctx, cancel := withRequestTimeout(ctx)
	defer cancel()
	req.Header.Set("authorization", c.token)
	req.Header.Set("User-Agent", userAgent())
	if err := c.throttle(ctx); err != nil {
		return err
	}
	if err := c.gql.Run(ctx, req, resp); err != nil {
		if isNetworkish(err) {
			return &NetworkError{Err: fmt.Errorf("write outcome uncertain; check Hardcover before retrying: %w", err)}
		}
		return err
	}
	return nil
}

// ISBNMatchError distinguishes unmatched data from API/authentication failures.
type ISBNMatchError struct {
	ISBN      string
	Ambiguous bool
}

func (e *ISBNMatchError) Error() string {
	if e.Ambiguous {
		return "ambiguous ISBN " + e.ISBN
	}
	return "no Hardcover book matches ISBN " + e.ISBN
}

// QueueBooks appends at most five entries per request, respecting Hardcover's
// top-level field limit and avoiding one round trip per book on initial setup.
func (c *Client) QueueBooks(ctx context.Context, listID int, books []RankedBook) error {
	for start := 0; start < len(books); start += 5 {
		batch := books[start:min(start+5, len(books))]
		var q strings.Builder
		q.WriteString("mutation($list:Int!) {")
		for i, b := range batch {
			if b.BookID <= 0 || b.Position < 0 {
				return fmt.Errorf("invalid queued book")
			}
			fmt.Fprintf(&q, " b%d:insert_list_book(object:{list_id:$list,book_id:%d,position:%d}) { id }", i, b.BookID, b.Position)
		}
		q.WriteString(" }")
		req := graphql.NewRequest(q.String())
		req.Var("list", listID)
		var resp map[string]mutationResult
		if err := c.doMutation(ctx, req, &resp); err != nil {
			return fmt.Errorf("queue setup may be partially saved; refresh before retrying: %w", err)
		}
		for i := range batch {
			if _, err := resp[fmt.Sprintf("b%d", i)].checked("queue book"); err != nil {
				return err
			}
		}
	}
	return nil
}

type RatingBucket struct {
	Rating float64 `json:"rating"`
	Count  int     `json:"count"`
}

// ParseRatingDistribution accepts Hardcover's array and older keyed-object shape.
func ParseRatingDistribution(raw json.RawMessage) ([]RatingBucket, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var rows []RatingBucket
	if err := json.Unmarshal(raw, &rows); err != nil {
		var values map[string]int
		if err := json.Unmarshal(raw, &values); err != nil {
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
