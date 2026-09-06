package cli

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/Kameleon21/oku/internal/config"
	"github.com/Kameleon21/oku/internal/tui"
)

// swatchBlock is one role in a preview row. A full block shows the colour at
// its own weight; the row sits on the palette's Surface, which is the
// background the roles were chosen against.
const swatchBlock = "██"

// themeNameWidth lines the preview rows up under each other. The longest
// name is "catppuccin-mocha".
const themeNameWidth = 18

func newConfigThemeCmd() *cobra.Command {
	var preview bool

	cmd := &cobra.Command{
		Use:   "theme [name]",
		Short: "Show, preview or set the colour theme",
		Long: "Without an argument, list the themes and mark the one in your config.\n" +
			"With a name, write it to the config file.\n" +
			"With --preview, draw a swatch of every palette so you can pick one.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return setTheme(args[0])
			}
			if preview {
				return previewThemes()
			}
			return listThemes()
		},
	}
	cmd.Flags().BoolVar(&preview, "preview", false, "draw a colour swatch of every theme")
	return cmd
}

// setTheme validates the name against the same rules the config loader uses
// and writes it, so a value that is accepted here always starts.
func setTheme(name string) error {
	if err := tui.ApplyThemeSetting(name); err != nil {
		return err
	}
	// The canonical spelling goes into the file, whatever the user typed.
	canonical := tui.ActiveThemeName()
	if canonical == "" {
		canonical = strings.ToLower(strings.TrimSpace(name))
		if canonical == "" {
			canonical = "auto"
		}
	}
	if err := config.SetTheme(canonical); err != nil {
		return err
	}
	path, err := config.FilePath()
	if err != nil {
		return err
	}
	outPrintf("Theme set to %s in %s\n", titleStyle().Render(canonical), path)
	return nil
}

// listThemes prints every value the `theme` key takes, with the current one
// marked.
func listThemes() error {
	current := currentThemeSetting()
	outPrintln(statusStyle().Render("Themes"))
	for _, name := range tui.ThemeSettings() {
		marker := "  "
		if name == current {
			marker = titleStyle().Render("* ")
		}
		outPrintf("%s%s\n", marker, name)
	}
	outPrintln("")
	outPrintln(dimStyle().Render("oku config theme --preview   draw them"))
	outPrintln(dimStyle().Render("oku config theme <name>      set one"))
	return nil
}

// previewThemes draws one row per palette: the fifteen roles as blocks on the
// palette's own surface, so the ramps and the accents can be compared side by
// side. Everything goes through the colour-profile writer, so a pipe or
// NO_COLOR gets the names and plain blocks rather than escape sequences.
func previewThemes() error {
	current := currentThemeSetting()

	outPrintln(statusStyle().Render("Theme preview"))

	// The built-in palette is previewable too: "dark" and "light" are the
	// two sides of it, and "auto" is whichever the terminal reports.
	rows := []struct {
		name  string
		theme tui.Theme
	}{
		{"dark", tui.NewTheme(true)},
		{"light", tui.NewTheme(false)},
	}
	for _, nt := range tui.NamedThemes() {
		rows = append(rows, struct {
			name  string
			theme tui.Theme
		}{nt.Name, nt.Theme})
	}

	for _, row := range rows {
		marker := " "
		if row.name == current {
			marker = "*"
		}
		outPrintf("%s %-*s %s\n", marker, themeNameWidth, row.name, swatchRow(row.theme))
	}
	outPrintln("")
	outPrintln(dimStyle().Render("roles, left to right: accent heading · text muted dim · " +
		"border focused · success warning error · heat 1-4"))
	outPrintln(dimStyle().Render("oku config theme <name>      set one"))
	return nil
}

// swatchRow renders one palette as blocks, grouped the way the legend reads:
// the two accents, the text ramp, the borders, the three states and the
// activity ramp.
func swatchRow(th tui.Theme) string {
	groups := [][]color.Color{
		{th.Accent, th.Heading},
		{th.Text, th.TextMuted, th.TextDim},
		{th.Border, th.BorderFocused},
		{th.Success, th.Warning, th.Error},
		{th.Heat1, th.Heat2, th.Heat3, th.Heat4},
	}

	// The whole row carries the palette's surface, which is the background
	// its foregrounds were picked for.
	on := lipgloss.NewStyle().Background(th.Surface)
	var b strings.Builder
	b.WriteString(on.Render(" "))
	for i, group := range groups {
		if i > 0 {
			b.WriteString(on.Render(" "))
		}
		for _, c := range group {
			b.WriteString(on.Foreground(c).Render(swatchBlock))
		}
	}
	b.WriteString(on.Render(" "))
	return b.String()
}

// currentThemeSetting is the `theme` value in the config file, normalised the
// way ApplyThemeSetting normalises it, or "auto" when the config is missing
// or unreadable — which is what the dashboard would fall back to as well.
func currentThemeSetting() string {
	cfg, err := config.Load()
	if err != nil {
		return "auto"
	}
	return normalizeThemeSetting(cfg.Theme)
}

// describeTheme is the one-line answer `oku config show` gives for the theme,
// which names what the setting resolves to as well as what it says.
func describeTheme(setting string) string {
	switch normalised := normalizeThemeSetting(setting); normalised {
	case "auto":
		return "auto (the terminal is asked for its background)"
	case "dark", "light":
		return fmt.Sprintf("%s (the built-in palette, pinned)", normalised)
	default:
		return normalised
	}
}

// normalizeThemeSetting spells a `theme` value the way the TUI matches it:
// lower case, hyphens for underscores, "auto" for empty.
func normalizeThemeSetting(setting string) string {
	normalised := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(setting)), "_", "-")
	if normalised == "" {
		return "auto"
	}
	return normalised
}
