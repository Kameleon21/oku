package store

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

// localJournalMaxAgeDays bounds how long optimistic local journal rows
// (negative IDs, written before a sync confirms the server copy) are kept.
const localJournalMaxAgeDays = 90

// ReplaceReadingJournals atomically swaps the cached server journal entries
// with the given set. Local optimistic rows (negative IDs) are kept, except
// those old enough that a sync would long since have covered them.
func (s *Store) ReplaceReadingJournals(entries []model.JournalEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM reading_journals WHERE id >= 0`); err != nil {
		return fmt.Errorf("delete journals: %w", err)
	}
	cutoff := time.Now().AddDate(0, 0, -localJournalMaxAgeDays).Format("2006-01-02")
	if _, err := tx.Exec(`DELETE FROM reading_journals WHERE id < 0 AND date(action_at) < date(?)`, cutoff); err != nil {
		return fmt.Errorf("prune local journals: %w", err)
	}

	const insert = `INSERT OR REPLACE INTO reading_journals (id, action_at, event) VALUES (?, ?, ?)`
	for _, e := range entries {
		if _, err := tx.Exec(insert, e.ID, e.ActionAt.Format(time.RFC3339), e.Event); err != nil {
			return fmt.Errorf("insert journal %d: %w", e.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit journals: %w", err)
	}
	return nil
}

// InsertLocalJournal records an optimistic local journal entry (negative ID)
// so the heatmap reflects activity immediately, before the next sync pulls the
// server-side entry. Duplicate days are harmless: activity is counted per day.
func (s *Store) InsertLocalJournal(actionAt time.Time, event string) error {
	const query = `INSERT OR REPLACE INTO reading_journals (id, action_at, event) VALUES (?, ?, ?)`
	id := -actionAt.UnixNano()
	if id >= 0 {
		id = -1
	}
	if _, err := s.db.Exec(query, id, actionAt.Format(time.RFC3339), event); err != nil {
		return fmt.Errorf("insert local journal: %w", err)
	}
	return nil
}

// GetJournalDays returns the distinct calendar days with at least one journal
// entry in the given range. Date-only timestamps (midnight) are taken as-is;
// timestamps with a time component are converted to local time first.
func (s *Store) GetJournalDays(from, to time.Time) ([]time.Time, error) {
	const query = `
SELECT action_at FROM reading_journals
WHERE date(action_at) >= date(?) AND date(action_at) <= date(?)
`
	rows, err := s.db.Query(query, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("get journal days: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	var days []time.Time
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan journal day: %w", err)
		}
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			if t, err = time.ParseInLocation("2006-01-02", raw, time.Local); err != nil {
				continue
			}
		} else if t.Hour() != 0 || t.Minute() != 0 {
			t = t.In(time.Local)
		}
		key := t.Format("2006-01-02")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		day, _ := time.ParseInLocation("2006-01-02", key, time.Local)
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	return days, rows.Err()
}

// ReplaceGoals atomically swaps the cached reading goals.
func (s *Store) ReplaceGoals(goals []model.Goal) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM goals`); err != nil {
		return fmt.Errorf("delete goals: %w", err)
	}
	const insert = `
INSERT INTO goals (id, metric, target, progress, state, start_date, end_date)
VALUES (?, ?, ?, ?, ?, ?, ?)
`
	for _, g := range goals {
		var start, end *string
		if !g.StartDate.IsZero() {
			v := g.StartDate.Format("2006-01-02")
			start = &v
		}
		if !g.EndDate.IsZero() {
			v := g.EndDate.Format("2006-01-02")
			end = &v
		}
		if _, err := tx.Exec(insert, g.ID, g.Metric, g.Target, g.Progress, g.State, start, end); err != nil {
			return fmt.Errorf("insert goal %d: %w", g.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit goals: %w", err)
	}
	return nil
}

// ListGoals returns all cached reading goals.
func (s *Store) ListGoals() ([]model.Goal, error) {
	const query = `
SELECT id, metric, target, progress, state, start_date, end_date
FROM goals
ORDER BY end_date DESC, id DESC
`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	defer rows.Close()

	var goals []model.Goal
	for rows.Next() {
		var g model.Goal
		var start, end sql.NullString
		if err := rows.Scan(&g.ID, &g.Metric, &g.Target, &g.Progress, &g.State, &start, &end); err != nil {
			return nil, fmt.Errorf("scan goal: %w", err)
		}
		if start.Valid {
			if t, err := time.ParseInLocation("2006-01-02", start.String, time.Local); err == nil {
				g.StartDate = t
			}
		}
		if end.Valid {
			if t, err := time.ParseInLocation("2006-01-02", end.String, time.Local); err == nil {
				g.EndDate = t
			}
		}
		goals = append(goals, g)
	}
	return goals, rows.Err()
}

// GetYearSummary aggregates finished books, pages read, and the user's average
// rating for one calendar year. Finished dates from Hardcover are date-only,
// so bucketing uses the stored date verbatim (no timezone conversion).
func (s *Store) GetYearSummary(year int) (model.YearSummary, error) {
	out := model.YearSummary{Year: year}
	y := fmt.Sprintf("%04d", year)

	const totals = `
SELECT COUNT(*),
       COALESCE(SUM(COALESCE(NULLIF(b.pages, 0), r.progress_pages)), 0)
FROM user_book_reads r
JOIN user_books ub ON ub.id = r.user_book_id
JOIN books b ON b.id = ub.book_id
WHERE r.finished_at IS NOT NULL AND strftime('%Y', r.finished_at) = ?
`
	if err := s.db.QueryRow(totals, y).Scan(&out.BooksFinished, &out.PagesRead); err != nil {
		return out, fmt.Errorf("year summary totals: %w", err)
	}

	const rating = `
SELECT COALESCE(AVG(ub.rating), 0), COUNT(*)
FROM user_books ub
WHERE ub.rating > 0
  AND EXISTS (
    SELECT 1 FROM user_book_reads r
    WHERE r.user_book_id = ub.id
      AND r.finished_at IS NOT NULL
      AND strftime('%Y', r.finished_at) = ?
  )
`
	if err := s.db.QueryRow(rating, y).Scan(&out.AvgRating, &out.RatedCount); err != nil {
		return out, fmt.Errorf("year summary rating: %w", err)
	}
	return out, nil
}

// GetBooksPerMonth returns books finished per month (Jan=0) for one year.
func (s *Store) GetBooksPerMonth(year int) ([12]int, error) {
	var months [12]int
	const query = `
SELECT CAST(strftime('%m', finished_at) AS INTEGER), COUNT(*)
FROM user_book_reads
WHERE finished_at IS NOT NULL AND strftime('%Y', finished_at) = ?
GROUP BY 1
`
	rows, err := s.db.Query(query, fmt.Sprintf("%04d", year))
	if err != nil {
		return months, fmt.Errorf("books per month: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var m, n int
		if err := rows.Scan(&m, &n); err != nil {
			return months, fmt.Errorf("scan books per month: %w", err)
		}
		if m >= 1 && m <= 12 {
			months[m-1] = n
		}
	}
	return months, rows.Err()
}

// GetBooksPerYear returns books finished per year, ascending.
func (s *Store) GetBooksPerYear() ([]model.LabelCount, error) {
	const query = `
SELECT strftime('%Y', finished_at), COUNT(*)
FROM user_book_reads
WHERE finished_at IS NOT NULL
GROUP BY 1
ORDER BY 1
`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("books per year: %w", err)
	}
	defer rows.Close()

	var out []model.LabelCount
	for rows.Next() {
		var lc model.LabelCount
		if err := rows.Scan(&lc.Label, &lc.Count); err != nil {
			return nil, fmt.Errorf("scan books per year: %w", err)
		}
		out = append(out, lc)
	}
	return out, rows.Err()
}

// GetRatingsDistribution returns counts per half-star bucket:
// index i holds books rated (i+1)*0.5 stars.
func (s *Store) GetRatingsDistribution() ([10]int, error) {
	var buckets [10]int
	const query = `
SELECT CAST(ROUND(rating * 2) AS INTEGER), COUNT(*)
FROM user_books
WHERE rating > 0
GROUP BY 1
`
	rows, err := s.db.Query(query)
	if err != nil {
		return buckets, fmt.Errorf("ratings distribution: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var b, n int
		if err := rows.Scan(&b, &n); err != nil {
			return buckets, fmt.Errorf("scan ratings distribution: %w", err)
		}
		if b >= 1 && b <= 10 {
			buckets[b-1] = n
		}
	}
	return buckets, rows.Err()
}

// GetGenreBreakdown returns the most common genres across the user's finished
// books, descending, limited to `limit` entries.
func (s *Store) GetGenreBreakdown(limit int) ([]model.LabelCount, error) {
	if limit <= 0 {
		limit = 6
	}
	const query = `
SELECT b.cached_tags
FROM user_books ub
JOIN books b ON b.id = ub.book_id
WHERE ub.status_id = ? AND b.cached_tags != ''
`
	rows, err := s.db.Query(query, int(model.StatusRead))
	if err != nil {
		return nil, fmt.Errorf("genre breakdown: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan genre breakdown: %w", err)
		}
		for _, tag := range model.TagsForCategory(raw, "Genre") {
			counts[tag]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]model.LabelCount, 0, len(counts))
	for tag, n := range counts {
		out = append(out, model.LabelCount{Label: tag, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
