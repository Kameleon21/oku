package cli

import (
	"fmt"
	"image/color"
	"slices"
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
			"With --preview, draw a swatch of every palette so you can pick one,\n" +
			"or of one named palette to see it before you set it.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if preview {
					return previewTheme(args[0])
				}
				return setTheme(args[0])
			}
			if preview {
				return previewThemes()
			}
			return listThemes()
		},
	}
	cmd.Flags().BoolVar(&preview, "preview", false, "draw a colour swatch instead of setting anything")
	return cmd
}

// setTheme validates the name against the same rules the config loader uses
// and writes it, so a value that is accepted here always starts. The palette
// in force is left alone: this command sets the next run's theme, not its
// own output's.
func setTheme(name string) error {
	// The canonical spelling goes into the file, whatever the user typed.
	canonical, err := tui.ResolveThemeSetting(name)
	if err != nil {
		return err
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
	current, ok := currentTheme()
	outPrintln(statusStyle().Render("Themes"))
	printUnsetCurrent(current, ok)
	for _, name := range tui.ThemeSettings() {
		marker := "  "
		if ok && name == current {
			marker = titleStyle().Render("* ")
		}
		outPrintf("%s%s\n", marker, name)
	}
	outPrintln("")
	outPrintln(dimStyle().Render("oku config theme --preview   draw them"))
	outPrintln(dimStyle().Render("oku config theme <name>      set one"))
	return nil
}

// themeRow is one palette under the name that selects it.
type themeRow struct {
	name  string
	theme tui.Theme
}

// themeRows is every palette a preview can draw: the built-in one under each
// of its two names, then the named palettes in listing order. "auto" has no
// row of its own — it is whichever of the first two the terminal reports.
func themeRows() []themeRow {
	rows := []themeRow{
		{"dark", tui.NewTheme(true)},
		{"light", tui.NewTheme(false)},
	}
	for _, nt := range tui.NamedThemes() {
		rows = append(rows, themeRow{nt.Name, nt.Theme})
	}
	return rows
}

// previewThemes draws one row per palette: the fifteen roles as blocks on the
// palette's own surface, so the ramps and the accents can be compared side by
// side.
func previewThemes() error {
	return printPreview(themeRows())
}

// previewTheme draws the swatch for one name, which is what `oku config theme
// <name> --preview` asks for: a look at a palette before it is written.
// "auto" is drawn as the two sides it chooses between.
func previewTheme(name string) error {
	canonical, err := tui.ResolveThemeSetting(name)
	if err != nil {
		return err
	}
	// "auto" has no row of its own: it is whichever side of the built-in
	// palette the terminal reports, so both are drawn.
	wanted := func(r themeRow) bool { return r.name == canonical }
	if canonical == "auto" {
		wanted = func(r themeRow) bool { return r.name == "dark" || r.name == "light" }
	}
	rows := slices.DeleteFunc(themeRows(), func(r themeRow) bool { return !wanted(r) })
	return printPreview(rows)
}

// printPreview draws the given palettes, marking the one in the config.
// Everything goes through the colour-profile writer, so a pipe or NO_COLOR
// gets the names and plain blocks rather than escape sequences.
func printPreview(rows []themeRow) error {
	current, ok := currentTheme()

	outPrintln(statusStyle().Render("Theme preview"))
	printUnsetCurrent(current, ok)
	for _, row := range rows {
		marker := " "
		if ok && row.name == current {
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

// printUnsetCurrent says so when the config's `theme` value is not one of
// ours: without it the listing marks nothing and reads as though the default
// were in force, when in fact every coloured command is refusing to start.
func printUnsetCurrent(current string, ok bool) {
	if !ok {
		outPrintf("%s\n", dimStyle().Render(fmt.Sprintf("current: %q (not a theme)", current)))
	}
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

// currentTheme is the `theme` value in the config file under its canonical
// spelling, and whether it names a theme at all. A missing or unreadable
// config reads as "auto", which is what the dashboard would fall back to as
// well; a value that resolves to nothing comes back as it was written.
func currentTheme() (name string, ok bool) {
	cfg, err := config.Load()
	if err != nil {
		return "auto", true
	}
	canonical, err := tui.ResolveThemeSetting(cfg.Theme)
	if err != nil {
		return strings.TrimSpace(cfg.Theme), false
	}
	return canonical, true
}

// describeTheme is the one-line answer `oku config show` gives for the theme,
// which names what the setting resolves to as well as what it says. A value
// that resolves to nothing is reported here rather than refused: `config` is
// where a bad one is found and fixed.
func describeTheme(setting string) string {
	name, err := tui.ResolveThemeSetting(setting)
	if err != nil {
		return fmt.Sprintf("%q (not a theme — see oku config theme)", strings.TrimSpace(setting))
	}
	switch name {
	case "auto":
		return "auto (the terminal is asked for its background)"
	case "dark", "light":
		return fmt.Sprintf("%s (the built-in palette, pinned)", name)
	default:
		return name
	}
}
