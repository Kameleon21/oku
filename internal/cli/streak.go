package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

func newStreakCmd() *cobra.Command {
	var weeks int
	cmd := &cobra.Command{
		Use:   "streak",
		Short: "Show your reading streak and activity heatmap",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := initLocalApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			streak, err := a.GetStreak()
			if err != nil {
				return err
			}
			heatmap, err := a.GetHeatmap(weeks)
			if err != nil {
				return err
			}

			if jsonOutput {
				printJSON(map[string]interface{}{
					"streak":  streak,
					"heatmap": heatmap,
				})
				return nil
			}

			fmt.Print("\nReading Streak\n\n")
			fmt.Printf("  Current:  %d days\n", streak.Current)
			fmt.Printf("  Longest:  %d days\n", streak.Longest)
			fmt.Printf("  Total:    %d reading days\n", streak.Total)

			if !streak.ReadToday && streak.Current > 0 {
				fmt.Println()
				fmt.Println("  Read today to keep your streak!")
			}

			if len(heatmap) > 0 {
				fmt.Println()
				fmt.Printf("  Activity (last %d weeks)\n\n", weeks)
				renderHeatmap(heatmap, weeks)
			}

			fmt.Println()
			return nil
		},
	}

	cmd.Flags().IntVar(&weeks, "weeks", 26, "Number of weeks to show in the heatmap")
	return cmd
}

// renderHeatmap prints a GitHub-style heatmap of reading activity.
func renderHeatmap(activities []model.DayActivity, weeks int) {
	// Build a map of date -> minutes.
	actMap := make(map[string]int, len(activities))
	for _, a := range activities {
		actMap[a.Date.Format("2006-01-02")] = a.Minutes
	}

	// Find the max minutes for intensity scaling.
	maxMin := 1
	for _, a := range activities {
		if a.Minutes > maxMin {
			maxMin = a.Minutes
		}
	}

	now := time.Now()
	// Go back `weeks` weeks from the end of the current week.
	// Find the start: the Monday of (weeks) weeks ago.
	weekday := (int(now.Weekday()) + 6) % 7 // Mon=0, Sun=6
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startDate := endDate.AddDate(0, 0, -weeks*7-weekday+1)

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
			mins := actMap[key]
			row += intensityChar(mins, maxMin) + " "
		}
		fmt.Println(row)
	}

	// Legend.
	fmt.Println()
	fmt.Printf("  Less %s %s %s %s More\n",
		intensityChar(0, maxMin),
		intensityChar(maxMin/4, maxMin),
		intensityChar(maxMin/2, maxMin),
		intensityChar(maxMin, maxMin),
	)
}

// intensityChar returns a Unicode character representing reading intensity.
func intensityChar(minutes, maxMinutes int) string {
	if minutes <= 0 {
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

func intensityCellTUI(minutes, maxMinutes int) string {
	if minutes <= 0 {
		return heatmapEmptyStyle.Render("·")
	}
	ratio := float64(minutes) / float64(maxMinutes)
	switch {
	case ratio > 0.75:
		return heatmapLevel4Style.Render("█")
	case ratio > 0.5:
		return heatmapLevel3Style.Render("▓")
	case ratio > 0.25:
		return heatmapLevel2Style.Render("▒")
	default:
		return heatmapLevel1Style.Render("░")
	}
}

// renderHeatmapTUI returns a heatmap string for the TUI (no printing to stdout).
func renderHeatmapTUI(activities []model.DayActivity, weeks, availWidth int) string {
	actMap := make(map[string]int, len(activities))
	for _, a := range activities {
		actMap[a.Date.Format("2006-01-02")] = a.Minutes
	}

	maxMin := 1
	for _, a := range activities {
		if a.Minutes > maxMin {
			maxMin = a.Minutes
		}
	}

	// Adapt weeks to available width: each week takes ~3 chars.
	maxWeeks := (availWidth - 8) / 3
	if maxWeeks < 4 {
		maxWeeks = 4
	}
	if weeks > maxWeeks {
		weeks = maxWeeks
	}

	now := time.Now()
	weekday := (int(now.Weekday()) + 6) % 7
	endDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startDate := endDate.AddDate(0, 0, -weeks*7-weekday+1)

	var sb strings.Builder

	// Month labels.
	sb.WriteString("       ")
	lastMonth := -1
	for w := 0; w < weeks; w++ {
		d := startDate.AddDate(0, 0, w*7)
		m := int(d.Month())
		if m != lastMonth {
			sb.WriteString(fmt.Sprintf("%-3s", d.Format("Jan")))
			lastMonth = m
		} else {
			sb.WriteString("   ")
		}
	}
	sb.WriteString("\n")

	// Rows.
	dayLabels := [7]string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	displayRows := []int{0, 2, 4, 6}
	for _, dayIdx := range displayRows {
		sb.WriteString(fmt.Sprintf("  %s   ", dayLabels[dayIdx]))
		for w := 0; w < weeks; w++ {
			d := startDate.AddDate(0, 0, w*7+dayIdx)
			if d.After(endDate) {
				sb.WriteString("  ")
				continue
			}
			key := d.Format("2006-01-02")
			mins := actMap[key]
			sb.WriteString(intensityCellTUI(mins, maxMin) + " ")
		}
		sb.WriteString("\n")
	}

	// Legend.
	sb.WriteString("\n  Less ")
	sb.WriteString(intensityCellTUI(0, maxMin))
	sb.WriteString(" ")
	sb.WriteString(intensityCellTUI(maxMin/4, maxMin))
	sb.WriteString(" ")
	sb.WriteString(intensityCellTUI(maxMin/2, maxMin))
	sb.WriteString(" ")
	sb.WriteString(intensityCellTUI(maxMin, maxMin))
	sb.WriteString(" More")

	return sb.String()
}
