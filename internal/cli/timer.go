package cli

import (
	"fmt"
	"time"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/picker"
	"github.com/spf13/cobra"
)

func newTimerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timer",
		Short: "Track reading sessions with a start/stop timer",
	}

	cmd.AddCommand(newTimerStartCmd())
	cmd.AddCommand(newTimerStopCmd())
	cmd.AddCommand(newTimerStatusCmd())
	cmd.AddCommand(newTimerStatsCmd())
	cmd.AddCommand(newTimerListCmd())

	return cmd
}

func newTimerStartCmd() *cobra.Command {
	var bookID int
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a reading timer",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initLocalApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			if bookID == 0 {
				// Always require explicit selection from currently reading books.
				books, err := a.Store.ListUserBooks(model.StatusCurrentlyReading)
				if err != nil {
					return err
				}
				if len(books) == 0 {
					return fmt.Errorf("no currently reading books found; add one first or pass --book")
				}

				picked, err := picker.PickBook(books, "Select a book for this session")
				if err != nil {
					return err
				}
				if picked == 0 {
					return fmt.Errorf("no book selected")
				}
				bookID = picked
			}

			if err := a.TimerStart(bookID); err != nil {
				return err
			}

			now := time.Now()
			fmt.Printf("\nTimer started — %s\n", now.Format("3:04 PM"))

			if bookID > 0 {
				if b, err := a.Store.GetBookByID(bookID); err == nil && b != nil {
					fmt.Printf("  Book: %s\n", b.Title)
				}
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&bookID, "book", 0, "Book ID to track (skips picker)")
	return cmd
}

func newTimerStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running timer and record the session",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initLocalApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			session, err := a.TimerStop()
			if err != nil {
				return err
			}

			fmt.Printf("\nSession complete — %s\n", format.Duration(session.Duration()))
			if session.BookTitle != "" {
				fmt.Printf("  Book: %s\n", session.BookTitle)
			}

			return nil
		},
	}
}

func newTimerStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current timer status",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initLocalApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			state, err := a.TimerStatus()
			if err != nil {
				return err
			}
			if state == nil {
				fmt.Println("\nNo timer running.")
				return nil
			}

			elapsed := time.Since(state.StartedAt)
			fmt.Printf("\nTimer running — %s elapsed\n", format.Duration(elapsed))
			if state.BookID > 0 {
				if b, err := a.Store.GetBookByID(state.BookID); err == nil && b != nil {
					fmt.Printf("  Book: %s\n", b.Title)
				}
			}
			fmt.Printf("  Started: %s\n", state.StartedAt.Local().Format("3:04 PM"))

			return nil
		},
	}
}

func newTimerStatsCmd() *cobra.Command {
	var weeks int
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show reading time statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCount("weeks", weeks, 0); err != nil {
				return err
			}

			a, err := initLocalApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			stats, err := a.TimerStats(weeks)
			if err != nil {
				return err
			}

			if jsonOutput {
				return printJSON(stats)
			}

			label := "this week"
			if weeks > 1 {
				label = fmt.Sprintf("last %d weeks", weeks)
			}
			fmt.Printf("\nReading Stats (%s)\n\n", label)

			dayNames := [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

			// Find max for scaling.
			maxMin := 1
			for _, m := range stats.Days {
				if m > maxMin {
					maxMin = m
				}
			}

			barWidth := 20
			for i, m := range stats.Days {
				filled := m * barWidth / maxMin
				if m > 0 && filled == 0 {
					filled = 1
				}
				empty := barWidth - filled

				bar := ""
				for j := 0; j < filled; j++ {
					bar += "█"
				}
				for j := 0; j < empty; j++ {
					bar += "░"
				}

				timeStr := "    —"
				if m > 0 {
					timeStr = fmt.Sprintf("%5s", format.Duration(time.Duration(m)*time.Minute))
				}
				fmt.Printf("  %s  %s %s\n", dayNames[i], bar, timeStr)
			}

			fmt.Println()
			avg := 0
			if stats.Sessions > 0 {
				avg = stats.Total / stats.Sessions
			}
			fmt.Printf("  Total: %s    Avg: %s    Sessions: %d\n",
				format.Duration(time.Duration(stats.Total)*time.Minute),
				format.Duration(time.Duration(avg)*time.Minute),
				stats.Sessions,
			)

			return nil
		},
	}

	cmd.Flags().IntVar(&weeks, "weeks", 1, "Number of weeks to show stats for")
	return cmd
}

func newTimerListCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent reading sessions",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCount("limit", limit, 0); err != nil {
				return err
			}

			a, err := initLocalApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			sessions, err := a.TimerList(limit)
			if err != nil {
				return err
			}

			if jsonOutput {
				return printJSON(sessions)
			}

			if len(sessions) == 0 {
				fmt.Println("\nNo reading sessions yet.")
				return nil
			}

			fmt.Print("\nRecent Sessions\n\n")
			for i, s := range sessions {
				dateStr := s.StartedAt.Local().Format("Jan 02")
				startStr := s.StartedAt.Local().Format("3:04 PM")
				endStr := ""
				dur := ""
				if s.EndedAt != nil {
					endStr = s.EndedAt.Local().Format("3:04 PM")
					dur = format.Duration(s.Duration())
				}

				bookTitle := s.BookTitle
				if bookTitle == "" {
					bookTitle = "(no book)"
				}

				fmt.Printf("  %d. %s  %s — %s  (%s)    %s\n",
					i+1, dateStr, startStr, endStr, dur, bookTitle)
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "Number of sessions to show")
	return cmd
}
