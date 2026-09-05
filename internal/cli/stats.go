package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/charts"
	"github.com/Kameleon21/oku/internal/format"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

const (
	// defaultHeatmapWeeks matches the range GetReadingStats already fetches.
	defaultHeatmapWeeks = 26
	// maxHeatmapWeeks is a year: SyncStats only fetches a year of journals, so
	// a wider grid adds empty columns the terminal has to fit.
	maxHeatmapWeeks = 52
)

func newStatsCmd() *cobra.Command {
	var weeks int
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show your Hardcover reading stats and activity heatmap",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCount("weeks", weeks, maxHeatmapWeeks); err != nil {
				return err
			}

			a, err := initLocalApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			stats, err := a.GetReadingStats()
			if err != nil {
				return err
			}
			if weeks != defaultHeatmapWeeks {
				heatmap, err := a.GetHeatmap(weeks)
				if err != nil {
					return err
				}
				stats.Heatmap = heatmap
			}

			if jsonOutput {
				return printJSON(stats)
			}

			fmt.Printf("\nReading Stats · %d\n\n", stats.Year.Year)
			fmt.Printf("  Books finished:  %d\n", stats.Year.BooksFinished)
			fmt.Printf("  Pages read:      %s\n", format.Thousands(stats.Year.PagesRead))
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

	cmd.Flags().IntVar(&weeks, "weeks", defaultHeatmapWeeks, fmt.Sprintf("Number of weeks to show in the heatmap (1-%d)", maxHeatmapWeeks))
	return cmd
}

// renderHeatmap prints a GitHub-style heatmap of reading activity.
func renderHeatmap(activities []model.DayActivity, weeks int) {
	fmt.Println(charts.Heatmap(activities, weeks, charts.Plain))
}
