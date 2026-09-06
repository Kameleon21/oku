package tui

import (
	"image/color"
	"math"
	"strings"
	"testing"
)

// The contrast floors every named palette is held to. They are the WCAG
// ratios for the three roles that carry meaning rather than decoration:
// body text has to be readable (AA for normal text), the dim text used for
// counts and hints has to be legible without competing with the body, and
// the accent — borders, key hints, the selection bar — has to be visible as
// a non-text element (AA for graphics).
const (
	minTextContrast    = 4.5
	minTextDimContrast = 2.0
	minAccentContrast  = 3.0
)

// relativeLuminance is the WCAG 2.x luminance of a colour. color.Color hands
// back premultiplied 16-bit channels; the palettes are all opaque, so the
// high byte of each is the sRGB value.
func relativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v>>8) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrastRatio is the WCAG ratio between two colours, 1 (identical) to 21
// (black on white).
func contrastRatio(a, b color.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// TestNamedThemeRoles checks that every palette fills every role, and that
// the roles that have to stand apart from the panel background do.
func TestNamedThemeRoles(t *testing.T) {
	for _, nt := range NamedThemes() {
		t.Run(nt.Name, func(t *testing.T) {
			th := nt.Theme
			roles := map[string]color.Color{
				"Accent": th.Accent, "Heading": th.Heading, "Text": th.Text,
				"TextMuted": th.TextMuted, "TextDim": th.TextDim,
				"Border": th.Border, "BorderFocused": th.BorderFocused,
				"Surface": th.Surface, "Success": th.Success,
				"Warning": th.Warning, "Error": th.Error,
				"Heat1": th.Heat1, "Heat2": th.Heat2, "Heat3": th.Heat3, "Heat4": th.Heat4,
			}
			if len(roles) != 15 {
				t.Fatalf("the role table lists %d roles, want the Theme's 15", len(roles))
			}
			for name, c := range roles {
				if c == nil {
					t.Errorf("%s is unset", name)
				}
			}
			if t.Failed() {
				return
			}
			if renderColour(th.Text) == renderColour(th.Surface) {
				t.Error("Text and Surface are the same colour: body text would be invisible")
			}

			for _, c := range []struct {
				role string
				fg   color.Color
				min  float64
			}{
				{"Text", th.Text, minTextContrast},
				{"TextDim", th.TextDim, minTextDimContrast},
				{"Accent", th.Accent, minAccentContrast},
			} {
				if got := contrastRatio(c.fg, th.Surface); got < c.min {
					t.Errorf("%s on Surface = %.2f:1, want >= %.1f:1", c.role, got, c.min)
				}
			}
		})
	}
}

// TestNamedThemeHeatRamp: the activity ramp has to read as a ramp. On a dark
// palette it climbs away from the background and on a light one it descends
// towards it, so the test is on the direction the palette's own side implies.
func TestNamedThemeHeatRamp(t *testing.T) {
	for _, nt := range NamedThemes() {
		t.Run(nt.Name, func(t *testing.T) {
			ramp := []color.Color{nt.Theme.Heat1, nt.Theme.Heat2, nt.Theme.Heat3, nt.Theme.Heat4}
			for i := 1; i < len(ramp); i++ {
				prev, cur := relativeLuminance(ramp[i-1]), relativeLuminance(ramp[i])
				if nt.IsDark && cur <= prev {
					t.Errorf("Heat%d (%.3f) is no lighter than Heat%d (%.3f)", i+1, cur, i, prev)
				}
				if !nt.IsDark && cur >= prev {
					t.Errorf("Heat%d (%.3f) is no darker than Heat%d (%.3f)", i+1, cur, i, prev)
				}
			}
		})
	}
}

// TestApplyThemeSettingNames: every palette can be named in the config, in
// any case and with underscores, and naming one pins the background the
// palette was drawn for.
func TestApplyThemeSettingNames(t *testing.T) {
	t.Cleanup(func() { _ = ApplyThemeSetting("auto") })

	for _, nt := range NamedThemes() {
		for _, spelling := range []string{
			nt.Name,
			strings.ToUpper(nt.Name),
			" " + strings.ReplaceAll(nt.Name, "-", "_") + " ",
		} {
			if err := ApplyThemeSetting(spelling); err != nil {
				t.Fatalf("ApplyThemeSetting(%q) error = %v", spelling, err)
			}
			isDark, pinned := PinnedDark()
			if !pinned || isDark != nt.IsDark {
				t.Fatalf("theme = %q: isDark=%v pinned=%v, want isDark=%v pinned=true",
					spelling, isDark, pinned, nt.IsDark)
			}
			if got := ActiveThemeName(); got != nt.Name {
				t.Fatalf("theme = %q: active name = %q, want %q", spelling, got, nt.Name)
			}
			if got, want := renderColour(ActiveTheme(true).Accent), renderColour(nt.Theme.Accent); got != want {
				t.Fatalf("theme = %q: accent = %q, want the palette's %q", spelling, got, want)
			}
			// A named palette is an answer, so the background it was drawn
			// for survives a terminal reporting the other one.
			if got, want := renderColour(ActiveTheme(!nt.IsDark).Accent), renderColour(nt.Theme.Accent); got != want {
				t.Fatalf("theme = %q: the palette moved with the terminal background", spelling)
			}
		}
	}

	// auto, dark and light still choose the built-in palette rather than a
	// named one.
	for _, setting := range []string{"auto", "dark", "light"} {
		if err := ApplyThemeSetting(setting); err != nil {
			t.Fatalf("ApplyThemeSetting(%q) error = %v", setting, err)
		}
		if name := ActiveThemeName(); name != "" {
			t.Fatalf("theme = %q named the palette %q", setting, name)
		}
		if got, want := renderColour(ActiveTheme(true).Accent), renderColour(NewTheme(true).Accent); got != want {
			t.Fatalf("theme = %q: accent = %q, want the built-in %q", setting, got, want)
		}
	}
}

// TestApplyThemeSettingRejectsUnknown: a typo is reported with the list of
// values that would have worked, which is the only place that list is on
// screen when the dashboard refuses to start.
func TestApplyThemeSettingRejectsUnknown(t *testing.T) {
	if err := ApplyThemeSetting("auto"); err != nil {
		t.Fatalf("ApplyThemeSetting(auto) error = %v", err)
	}
	t.Cleanup(func() { _ = ApplyThemeSetting("auto") })

	for _, bad := range []string{"solarized", "gruvbox", "catppuccin", "nord-light", "tokyonight"} {
		err := ApplyThemeSetting(bad)
		if err == nil {
			t.Fatalf("ApplyThemeSetting(%q) = nil, want an error", bad)
		}
		for _, want := range ThemeSettings() {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("ApplyThemeSetting(%q) error %q does not list %q", bad, err, want)
			}
		}
		if _, pinned := PinnedDark(); pinned {
			t.Fatalf("ApplyThemeSetting(%q) pinned a background despite failing", bad)
		}
	}
}
