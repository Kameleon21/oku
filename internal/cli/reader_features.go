package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/app"
	"github.com/spf13/cobra"
)

func readerOutput(cmd *cobra.Command, v any, human func() error) error {
	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	return human()
}
func newJournalCmd(event string) *cobra.Command {
	var id int
	cmd := &cobra.Command{Use: event + " <text>", Short: "Save a " + event + " to your Hardcover reading journal", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(args[0]) == "" {
			return fmt.Errorf("text cannot be empty")
		}
		a, err := initApp()
		if err != nil {
			return err
		}
		defer a.Store.Close()
		entryID, err := a.AddJournalEntry(cmd.Context(), id, event, args[0])
		if err != nil {
			return err
		}
		return readerOutput(cmd, map[string]any{"id": entryID, "event": event}, func() error { _, err := fmt.Fprintf(cmd.OutOrStdout(), "Saved %s #%d.\n", event, entryID); return err })
	}}
	cmd.Flags().IntVar(&id, "book", 0, "Hardcover book ID (defaults to the single active book)")
	return cmd
}
func newJournalListCmd() *cobra.Command {
	var id int
	cmd := &cobra.Command{Use: "journal", Short: "Read a book's notes and quotes", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		a, err := initApp()
		if err != nil {
			return err
		}
		defer a.Store.Close()
		id, err = a.ResolveBookID(id)
		if err != nil {
			return err
		}
		rows, err := a.API.BookJournal(cmd.Context(), id)
		if err != nil {
			return err
		}
		return readerOutput(cmd, rows, func() error {
			for _, r := range rows {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s · %s #%d\n%s\n\n", r.ActionAt, r.Event, r.ID, r.Entry); err != nil {
					return err
				}
			}
			return nil
		})
	}}
	cmd.Flags().IntVar(&id, "book", 0, "Hardcover book ID")
	return cmd
}
func newBookDetailCmd() *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{Use: "book <id>", Short: "Show description, community ratings and editions", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := positiveID(args[0])
		if err != nil {
			return err
		}
		a, err := initApp()
		if err != nil {
			return err
		}
		defer a.Store.Close()
		b, err := a.GetBookDetail(cmd.Context(), id, refresh)
		if err != nil {
			return err
		}
		return readerOutput(cmd, b, func() error {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s (#%d)\n%s\n\n%s\n\nCommunity: %.2f (%d ratings) · %d editions\n", b.Title, b.ID, b.Headline, b.Description, b.Rating, b.RatingsCount, b.EditionsCount); err != nil {
				return err
			}
			rows, err := api.ParseRatingDistribution(b.RatingsDistribution)
			if err != nil {
				return fmt.Errorf("ratings distribution: %w", err)
			}
			maximum := 0
			for _, r := range rows {
				maximum = max(maximum, r.Count)
			}
			for _, r := range rows {
				n := 0
				if maximum > 0 {
					n = r.Count * 30 / maximum
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%4g ★ %-30s %d\n", r.Rating, strings.Repeat("█", n), r.Count); err != nil {
					return err
				}
			}
			return nil
		})
	}}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Refresh cached book detail")
	return cmd
}
func positiveID(s string) (int, error) {
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid ID %q", s)
	}
	return id, nil
}
func newGoalsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "goals", Short: "List and set Hardcover reading goals", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		a, err := initApp()
		if err != nil {
			return err
		}
		defer a.Store.Close()
		goals, err := a.API.ListGoals(cmd.Context())
		if err != nil {
			return err
		}
		return readerOutput(cmd, goals, func() error {
			for _, g := range goals {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d · %.0f/%d %s · %s\n", g.ID, float64(g.Progress), g.Goal, g.Metric, g.State); err != nil {
					return err
				}
			}
			return nil
		})
	}}
	var id, target, year int
	var metric, description, start, end string
	set := &cobra.Command{Use: "set", Short: "Create a goal, or update selected settings with --id", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if id < 0 {
			return fmt.Errorf("--id must be positive")
		}
		if year < 1 || year > 9999 {
			return fmt.Errorf("invalid year")
		}
		if start == "" {
			start = fmt.Sprintf("%04d-01-01", year)
		}
		if end == "" {
			end = fmt.Sprintf("%04d-12-31", year)
		}
		g := api.GoalInput{Goal: target, Metric: metric, Description: description, StartDate: start, EndDate: end, PrivacySettingID: 3}
		if id == 0 {
			if err := g.Validate(); err != nil {
				return err
			}
		}
		a, err := initApp()
		if err != nil {
			return err
		}
		defer a.Store.Close()
		if id > 0 {
			old, err := a.API.GoalSettings(cmd.Context(), id)
			if err != nil {
				return err
			}
			g = mergeGoalSettings(cmd, old, g)
		} else {
			privacy, err := a.API.GetAccountPrivacySetting(cmd.Context())
			if err != nil {
				return err
			}
			g.PrivacySettingID = privacy
		}
		goalID, err := a.API.SaveGoal(cmd.Context(), id, g)
		if err != nil {
			return err
		}
		return readerOutput(cmd, map[string]int{"id": goalID}, func() error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Saved goal #%d. Run oku sync to refresh Stats.\n", goalID)
			return err
		})
	}}
	set.Flags().IntVar(&id, "id", 0, "Existing goal ID to update (omit to create)")
	set.Flags().IntVar(&target, "target", 0, "Target amount")
	set.Flags().IntVar(&year, "year", time.Now().Year(), "Goal year")
	set.Flags().StringVar(&metric, "metric", "book", "Goal metric: book or page")
	set.Flags().StringVar(&description, "description", "", "Goal description")
	set.Flags().StringVar(&start, "start", "", "Start date YYYY-MM-DD (default: January 1)")
	set.Flags().StringVar(&end, "end", "", "End date YYYY-MM-DD (default: December 31)")
	cmd.AddCommand(set)
	return cmd
}
func newTrendingCmd() *cobra.Command {
	var period string
	var limit int
	var week bool
	cmd := &cobra.Command{Use: "trending", Short: "Discover books trending on Hardcover", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if week {
			period = "week"
		}
		if err := validateCount("limit", limit, 100); err != nil {
			return err
		}
		a, err := initApp()
		if err != nil {
			return err
		}
		defer a.Store.Close()
		rows, err := a.TrendingBooks(cmd.Context(), period, limit)
		if err != nil {
			return err
		}
		return readerOutput(cmd, rows, func() error {
			for i, b := range rows {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%d. %s — %s (#%d)\n", i+1, b.Title, strings.Join(b.Authors, ", "), b.ID); err != nil {
					return err
				}
			}
			return nil
		})
	}}
	cmd.Flags().StringVar(&period, "period", "week", "week, month, three_month, one_year, all")
	cmd.Flags().BoolVar(&week, "week", false, "Show this week's trending books")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum results (1-100)")
	return cmd
}
func newExportCmd() *cobra.Command {
	var format, path string
	cmd := &cobra.Command{Use: "export", Short: "Export the full Hardcover library, including reading history", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput {
			format = "json"
		}
		if format != "json" && format != "csv" {
			return fmt.Errorf("format must be json or csv")
		}
		a, err := initApp()
		if err != nil {
			return err
		}
		defer a.Store.Close()
		data, err := a.ExportBooks(cmd.Context())
		if err != nil {
			return err
		}
		if path == "" {
			return app.WriteExport(cmd.OutOrStdout(), data, format)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		err = app.WriteExport(f, data, format)
		closeErr := f.Close()
		if err != nil {
			os.Remove(path)
			return err
		}
		if closeErr != nil {
			os.Remove(path)
			return closeErr
		}
		_, err = fmt.Fprintf(cmd.ErrOrStderr(), "Exported %d books to %s\n", len(data.Books), path)
		return err
	}}
	cmd.Flags().StringVar(&format, "format", "json", "json or csv")
	cmd.Flags().StringVar(&path, "output", "", "New output file (default: stdout; existing files are not overwritten)")
	return cmd
}
func newImportCmd() *cobra.Command {
	var format string
	var apply bool
	cmd := &cobra.Command{Use: "import <file>", Short: "Preview an Oku export or Goodreads CSV import; use --apply to add books", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if format == "" {
			if strings.EqualFold(filepath.Ext(args[0]), ".json") {
				format = "json"
			} else {
				format = "csv"
			}
		}
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		rows, err := app.ReadImport(f, format)
		f.Close()
		if err != nil {
			return err
		}
		a, err := initApp()
		if err != nil {
			return err
		}
		defer a.Store.Close()
		results, importErr := a.ImportBooks(cmd.Context(), rows, apply)
		printErr := readerOutput(cmd, results, func() error {
			for _, r := range results {
				if r.Action == "" {
					continue
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s · %s (#%d) %s\n", r.Action, r.Title, r.BookID, r.Detail); err != nil {
					return err
				}
			}
			if !apply {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), "Preview only. Use --apply to add matched books; existing books are skipped.")
				return err
			}
			return nil
		})
		if importErr != nil {
			return importErr
		}
		return printErr
	}}
	cmd.Flags().StringVar(&format, "format", "", "json, csv (Oku), or goodreads")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply the import (default: preview without changes)")
	return cmd
}

func mergeGoalSettings(cmd *cobra.Command, old, next api.GoalInput) api.GoalInput {
	if cmd.Flags().Changed("target") {
		old.Goal = next.Goal
	}
	if cmd.Flags().Changed("metric") {
		old.Metric = next.Metric
	}
	if cmd.Flags().Changed("description") {
		old.Description = next.Description
	}
	if cmd.Flags().Changed("start") || cmd.Flags().Changed("year") {
		old.StartDate = next.StartDate
	}
	if cmd.Flags().Changed("end") || cmd.Flags().Changed("year") {
		old.EndDate = next.EndDate
	}
	return old
}
