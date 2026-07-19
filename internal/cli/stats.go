package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	var weeks int
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show your Hardcover reading stats and activity heatmap",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initLocalApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			stats, err := a.GetReadingStats()
			if err != nil {
				return err
			}
			if weeks != 26 {
				if heatmap, err := a.GetHeatmap(weeks); err == nil {
					stats.Heatmap = heatmap
				}
			}

			if jsonOutput {
				printJSON(stats)
				return nil
			}

			fmt.Printf("\nReading Stats · %d\n\n", stats.Year.Year)
			fmt.Printf("  Books finished:  %d\n", stats.Year.BooksFinished)
			fmt.Printf("  Pages read:      %s\n", groupThousands(stats.Year.PagesRead))
			if stats.Year.AvgRating > 0 {
				fmt.Printf("  Average rating:  %.1f (%d rated)\n", stats.Year.AvgRating, stats.Year.RatedCount)
			}

			if g := stats.Goal; g != nil && g.Target > 0 {
				fmt.Printf("\n  Goal: %d/%d %s (%d%%)\n", int(g.Progress), g.Target, g.Metric, g.Percent())
			}

			if len(stats.Heatmap) > 0 {
				fmt.Printf("\n  Activity (last %d weeks)\n\n", weeks)
				renderHeatmap(stats.Heatmap, weeks)
			}

			printBarChart := func(title string, rows []model.LabelCount) {
				if len(rows) == 0 {
					return
				}
				maxCount := 1
				labelW := 0
				for _, r := range rows {
					if r.Count > maxCount {
						maxCount = r.Count
					}
					if len(r.Label) > labelW {
						labelW = len(r.Label)
					}
				}
				fmt.Printf("\n  %s\n\n", title)
				for _, r := range rows {
					f := r.Count * 20 / maxCount
					if r.Count > 0 && f == 0 {
						f = 1
					}
					fmt.Printf("  %-*s %s%s %d\n", labelW, r.Label,
						strings.Repeat("█", f), strings.Repeat("░", 20-f), r.Count)
				}
			}

			months := make([]model.LabelCount, 0, 12)
			upto := 12
			if stats.Year.Year == time.Now().Year() {
				upto = int(time.Now().Month())
			}
			for i := 0; i < upto; i++ {
				months = append(months, model.LabelCount{Label: time.Month(i + 1).String()[:3], Count: stats.Months[i]})
			}
			printBarChart("Books per month", months)

			if len(stats.Years) > 1 {
				printBarChart("Books per year", stats.Years)
			}

			ratings := make([]model.LabelCount, 0, 10)
			for i := 9; i >= 0; i-- {
				if stats.Ratings[i] == 0 {
					continue
				}
				ratings = append(ratings, model.LabelCount{
					Label: fmt.Sprintf("★%.1f", float64(i+1)/2),
					Count: stats.Ratings[i],
				})
			}
			printBarChart("Ratings", ratings)
			printBarChart("Top genres", stats.Genres)

			fmt.Println()
			return nil
		},
	}

	cmd.Flags().IntVar(&weeks, "weeks", 26, "Number of weeks to show in the heatmap")
	return cmd
}

// renderHeatmap prints a GitHub-style heatmap of reading activity.
func renderHeatmap(activities []model.DayActivity, weeks int) {
	// Build a map of date -> activity.
	actMap := make(map[string]model.DayActivity, len(activities))
	for _, a := range activities {
		actMap[a.Date.Format("2006-01-02")] = a
	}

	// Find the max minutes for intensity scaling.
	maxMin := 1
	for _, a := range activities {
		if a.Minutes > maxMin {
			maxMin = a.Minutes
		}
	}

	now := time.Now()
	// Start at the Monday of (weeks-1) weeks ago so the grid's last column
	// is the current week.
	weekday := (int(now.Weekday()) + 6) % 7 // Mon=0, Sun=6
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startDate := endDate.AddDate(0, 0, -(weeks-1)*7-weekday)

	// Render month labels.
	monthRow := "       "
	lastMonth := -1
	for w := 0; w < weeks; w++ {
		d := startDate.AddDate(0, 0, w*7)
		m := int(d.Month())
		if m != lastMonth {
			monthRow += fmt.Sprintf("%-3s", d.Format("Jan"))
			lastMonth = m
		} else {
			monthRow += "   "
		}
	}
	fmt.Println(monthRow)

	// Render rows for Mon, Wed, Fri (indices 0, 2, 4).
	dayLabels := [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	displayRows := []int{0, 2, 4}

	for _, dayIdx := range displayRows {
		row := fmt.Sprintf("  %s  ", dayLabels[dayIdx])
		for w := 0; w < weeks; w++ {
			d := startDate.AddDate(0, 0, w*7+dayIdx)
			if d.After(endDate) {
				row += " "
				continue
			}
			key := d.Format("2006-01-02")
			act := actMap[key]
			row += intensityChar(act.Minutes, maxMin, act.HasActivity) + " "
		}
		fmt.Println(row)
	}

	// Legend.
	fmt.Println()
	fmt.Printf("  Less %s %s %s %s More\n",
		intensityChar(0, maxMin, false),
		intensityChar(maxMin/4, maxMin, false),
		intensityChar(maxMin/2, maxMin, false),
		intensityChar(maxMin, maxMin, false),
	)
}

// intensityChar returns a Unicode character representing reading intensity.
// Days with journal activity but no timer minutes get the lightest shade.
func intensityChar(minutes, maxMinutes int, active bool) string {
	if minutes <= 0 {
		if active {
			return "░"
		}
		return "·"
	}
	ratio := float64(minutes) / float64(maxMinutes)
	switch {
	case ratio > 0.75:
		return "█"
	case ratio > 0.5:
		return "▓"
	case ratio > 0.25:
		return "▒"
	default:
		return "░"
	}
}
