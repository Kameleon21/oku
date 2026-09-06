package tui

import (
	"image/color"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// ── Named palettes ──────────────────────────────────────────────────────────

// A named palette is a whole Theme written out in true colour rather than a
// pair of 256-colour ramps: the point of asking for "nord" is to get Nord's
// own hexes, not the built-in palette nudged towards them. Naming one also
// pins the background — a scheme is drawn for the terminal it was designed
// for — so lipgloss.LightDark never has to guess and PinnedDark answers with
// the palette's own side.
//
// Every colour below is quoted from the scheme's published palette; the
// per-theme comment names the source and the deviations, of which there are
// a handful: a scheme's own "comment" grey is sometimes too dark to serve as
// TextDim on the scheme's own panel background, and the nearest palette
// colour that clears the contrast floor is used instead (see
// TestNamedThemeRoles, which holds every palette to those floors).

// NamedTheme is one palette under the name the `theme` config key selects it
// with.
type NamedTheme struct {
	Name   string
	IsDark bool
	Theme  Theme
}

// hex is a true-colour palette entry. Named palettes never go through
// lipgloss.LightDark: the scheme picked its background already.
func hex(s string) color.Color { return lipgloss.Color(s) }

// namedThemes is every palette the `theme` key can name, in the order the
// listing prints them: dark schemes first, each light companion beside its
// dark one.
var namedThemes = []NamedTheme{
	// Nord — nordtheme.com/docs/colors-and-palettes. Polar Night nord0
	// #2E3440 … nord3 #4C566A, Snow Storm nord4 #D8DEE9 … nord6 #ECEFF4,
	// Frost nord7 #8FBCBB … nord10 #5E81AC, Aurora nord11 #BF616A,
	// nord13 #EBCB8B, nord14 #A3BE8C.
	//
	// Deviation: nord3 #4C566A, the palette's comment grey, reaches only
	// 1.7:1 on nord1, so the dim role takes the Frost nord9 instead — three
	// near-white Snow Storm greys would be no ramp at all — and nord3 stays
	// where it reads, at the empty end of the activity ramp.
	{
		Name:   "nord",
		IsDark: true,
		Theme: Theme{
			Accent:        hex("#88C0D0"), // nord8, Frost's primary
			Heading:       hex("#8FBCBB"), // nord7
			Text:          hex("#ECEFF4"), // nord6
			TextMuted:     hex("#D8DEE9"), // nord4
			TextDim:       hex("#81A1C1"), // nord9
			Border:        hex("#434C5E"), // nord2
			BorderFocused: hex("#88C0D0"), // nord8
			Surface:       hex("#3B4252"), // nord1
			Success:       hex("#A3BE8C"), // nord14
			Warning:       hex("#EBCB8B"), // nord13
			Error:         hex("#BF616A"), // nord11
			Heat1:         hex("#4C566A"), // nord3
			Heat2:         hex("#5E81AC"), // nord10
			Heat3:         hex("#81A1C1"), // nord9
			Heat4:         hex("#A3BE8C"), // nord14
		},
	},

	// Tokyo Night — folke/tokyonight.nvim, the "night" variant's palette.lua:
	// bg #1a1b26, bg_highlight #292e42, fg #c0caf5, fg_dark #a9b1d6,
	// dark5 #737aa2, terminal_black #414868, blue #7aa2f7, blue7 #394b70,
	// magenta #bb9af7, green #9ece6a, green1 #73daca, green2 #41a6b5,
	// yellow #e0af68, red #f7768e.
	{
		Name:   "tokyo-night",
		IsDark: true,
		Theme: Theme{
			Accent:        hex("#7aa2f7"), // blue
			Heading:       hex("#bb9af7"), // magenta
			Text:          hex("#c0caf5"), // fg
			TextMuted:     hex("#a9b1d6"), // fg_dark
			TextDim:       hex("#737aa2"), // dark5
			Border:        hex("#414868"), // terminal_black
			BorderFocused: hex("#7aa2f7"), // blue
			Surface:       hex("#292e42"), // bg_highlight
			Success:       hex("#9ece6a"), // green
			Warning:       hex("#e0af68"), // yellow
			Error:         hex("#f7768e"), // red
			Heat1:         hex("#394b70"), // blue7
			Heat2:         hex("#41a6b5"), // green2
			Heat3:         hex("#9ece6a"), // green
			Heat4:         hex("#73daca"), // green1
		},
	},

	// Dracula — draculatheme.com/contribute. Background #282A36,
	// Current Line #44475A, Foreground #F8F8F2, Comment #6272A4,
	// Cyan #8BE9FD, Green #50FA7B, Orange #FFB86C, Pink #FF79C6,
	// Purple #BD93F9, Red #FF5555, Yellow #F1FA8C.
	//
	// Two deviations. Comment #6272A4 makes only 1.9:1 on Current Line, so
	// Surface takes Background #282A36 — where the comment grey clears 3:1,
	// as Dracula intends it to — and Current Line becomes the border. And
	// Dracula publishes exactly two text greys, Foreground and Comment, with
	// Current Line far too dark to carry dim text (1.5:1), so the three-step
	// text ramp is completed at the top with the ANSI bright white the
	// scheme's terminal table lists.
	{
		Name:   "dracula",
		IsDark: true,
		Theme: Theme{
			Accent:        hex("#BD93F9"), // Purple
			Heading:       hex("#FF79C6"), // Pink
			Text:          hex("#FFFFFF"), // ANSI bright white
			TextMuted:     hex("#F8F8F2"), // Foreground
			TextDim:       hex("#6272A4"), // Comment
			Border:        hex("#44475A"), // Current Line
			BorderFocused: hex("#BD93F9"), // Purple
			Surface:       hex("#282A36"), // Background
			Success:       hex("#50FA7B"), // Green
			Warning:       hex("#F1FA8C"), // Yellow
			Error:         hex("#FF5555"), // Red
			Heat1:         hex("#44475A"), // Current Line
			Heat2:         hex("#6272A4"), // Comment
			Heat3:         hex("#BD93F9"), // Purple
			Heat4:         hex("#50FA7B"), // Green
		},
	},

	// Gruvbox dark — morhetz/gruvbox, the dark palette: bg0 #282828,
	// bg1 #3c3836, bg2 #504945, fg1 #ebdbb2, fg2 #d5c4a1, fg4 #a89984,
	// bright red #fb4934, bright green #b8bb26, bright yellow #fabd2f,
	// bright orange #fe8019, neutral orange #d65d0e, dark orange #af3a03.
	{
		Name:   "gruvbox-dark",
		IsDark: true,
		Theme: Theme{
			Accent:        hex("#fe8019"), // bright orange
			Heading:       hex("#fabd2f"), // bright yellow
			Text:          hex("#ebdbb2"), // fg1
			TextMuted:     hex("#d5c4a1"), // fg2
			TextDim:       hex("#a89984"), // fg4
			Border:        hex("#504945"), // bg2
			BorderFocused: hex("#fe8019"), // bright orange
			Surface:       hex("#3c3836"), // bg1
			Success:       hex("#b8bb26"), // bright green
			Warning:       hex("#fabd2f"), // bright yellow
			Error:         hex("#fb4934"), // bright red
			// Gruvbox's greens are equiluminant with its aqua, so the ramp
			// climbs the orange the accent comes from instead.
			Heat1: hex("#af3a03"), // dark orange
			Heat2: hex("#d65d0e"), // neutral orange
			Heat3: hex("#fe8019"), // bright orange
			Heat4: hex("#fabd2f"), // bright yellow
		},
	},

	// Gruvbox light — morhetz/gruvbox, the light palette: bg0 #fbf1c7,
	// bg1 #ebdbb2, bg2 #d5c4a1, bg3 #bdae93, fg0 #282828, fg2 #504945,
	// fg4 #7c6f64, faded red #9d0006, faded green #79740e,
	// faded yellow #b57614, faded orange #af3a03.
	{
		Name:   "gruvbox-light",
		IsDark: false,
		Theme: Theme{
			Accent:        hex("#af3a03"), // faded orange
			Heading:       hex("#b57614"), // faded yellow
			Text:          hex("#282828"), // fg0
			TextMuted:     hex("#504945"), // fg2
			TextDim:       hex("#7c6f64"), // fg4
			Border:        hex("#bdae93"), // bg3
			BorderFocused: hex("#af3a03"), // faded orange
			Surface:       hex("#ebdbb2"), // bg1
			Success:       hex("#79740e"), // faded green
			Warning:       hex("#b57614"), // faded yellow
			Error:         hex("#9d0006"), // faded red
			// On a light ground the ramp darkens as the day gets busier.
			Heat1: hex("#d5c4a1"), // bg2
			Heat2: hex("#b57614"), // faded yellow
			Heat3: hex("#af3a03"), // faded orange
			Heat4: hex("#9d0006"), // faded red
		},
	},

	// Solarized dark — ethanschoonover.com/solarized: base03 #002b36,
	// base02 #073642, base01 #586e75, base00 #657b83, base0 #839496,
	// base1 #93a1a1, base2 #eee8d5, yellow #b58900, red #dc322f,
	// blue #268bd2, cyan #2aa198, green #859900.
	{
		Name:   "solarized-dark",
		IsDark: true,
		Theme: Theme{
			Accent:        hex("#268bd2"), // blue
			Heading:       hex("#2aa198"), // cyan
			Text:          hex("#93a1a1"), // base1, emphasised content
			TextMuted:     hex("#839496"), // base0, body text
			TextDim:       hex("#657b83"), // base00
			Border:        hex("#586e75"), // base01
			BorderFocused: hex("#268bd2"), // blue
			Surface:       hex("#073642"), // base02, background highlight
			Success:       hex("#859900"), // green
			Warning:       hex("#b58900"), // yellow
			Error:         hex("#dc322f"), // red
			// Solarized's accents are equiluminant by design — its own
			// selling point — so a ramp of them would be flat. This one
			// climbs the base ramp with green at the second step.
			Heat1: hex("#586e75"), // base01
			Heat2: hex("#859900"), // green
			Heat3: hex("#93a1a1"), // base1
			Heat4: hex("#eee8d5"), // base2
		},
	},

	// Solarized light — the same palette inverted, as the scheme specifies:
	// base3 #fdf6e3 background, base2 #eee8d5 highlight, base00 body,
	// base01 emphasised, base1 secondary.
	//
	// Deviation: on base2 the body base00 makes 3.6:1 and even the
	// emphasised base01 only 4.4:1, both under the 4.5:1 floor, so Text
	// takes base02 #073642 — the scheme's "optional emphasised content" —
	// and base01/base00 fall to the muted and dim roles.
	{
		Name:   "solarized-light",
		IsDark: false,
		Theme: Theme{
			Accent:        hex("#268bd2"), // blue
			Heading:       hex("#cb4b16"), // orange
			Text:          hex("#073642"), // base02
			TextMuted:     hex("#586e75"), // base01
			TextDim:       hex("#657b83"), // base00
			Border:        hex("#93a1a1"), // base1
			BorderFocused: hex("#268bd2"), // blue
			Surface:       hex("#eee8d5"), // base2, background highlight
			Success:       hex("#859900"), // green
			Warning:       hex("#b58900"), // yellow
			Error:         hex("#dc322f"), // red
			Heat1:         hex("#93a1a1"), // base1
			Heat2:         hex("#859900"), // green
			Heat3:         hex("#586e75"), // base01
			Heat4:         hex("#002b36"), // base03
		},
	},

	// Catppuccin Mocha — catppuccin/catppuccin, the Mocha flavour:
	// base #1e1e2e, surface0 #313244, surface1 #45475a, surface2 #585b70,
	// overlay2 #9399b2, subtext0 #a6adc8, subtext1 #bac2de, text #cdd6f4,
	// mauve #cba6f7, blue #89b4fa, sapphire #74c7ec, green #a6e3a1,
	// yellow #f9e2af, red #f38ba8.
	{
		Name:   "catppuccin-mocha",
		IsDark: true,
		Theme: Theme{
			Accent:        hex("#cba6f7"), // mauve
			Heading:       hex("#89b4fa"), // blue
			Text:          hex("#cdd6f4"), // text
			TextMuted:     hex("#bac2de"), // subtext1
			TextDim:       hex("#a6adc8"), // subtext0
			Border:        hex("#45475a"), // surface1
			BorderFocused: hex("#cba6f7"), // mauve
			Surface:       hex("#313244"), // surface0
			Success:       hex("#a6e3a1"), // green
			Warning:       hex("#f9e2af"), // yellow
			Error:         hex("#f38ba8"), // red
			// Mocha's green and teal sit at the same luminance, so the ramp
			// leaves the surfaces, passes through sapphire and lands on green.
			Heat1: hex("#585b70"), // surface2
			Heat2: hex("#9399b2"), // overlay2
			Heat3: hex("#74c7ec"), // sapphire
			Heat4: hex("#a6e3a1"), // green
		},
	},
}

// normalizeThemeName is how a `theme` value is matched: case does not
// matter and an underscore reads as a hyphen, so "Tokyo_Night" finds
// tokyo-night.
func normalizeThemeName(setting string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(setting)), "_", "-")
}

// NamedThemes is every named palette, in listing order.
func NamedThemes() []NamedTheme {
	out := make([]NamedTheme, len(namedThemes))
	copy(out, namedThemes)
	return out
}

// lookupNamedTheme finds a palette by an already-normalised name.
func lookupNamedTheme(name string) (NamedTheme, bool) {
	for _, nt := range namedThemes {
		if nt.Name == name {
			return nt, true
		}
	}
	return NamedTheme{}, false
}

// ThemeSettings is every value the `theme` config key accepts, the three
// background settings first and the palettes after them, sorted — this is
// the list an invalid value is reported against and the one `oku config
// theme` prints.
func ThemeSettings() []string {
	names := make([]string, 0, len(namedThemes))
	for _, nt := range namedThemes {
		names = append(names, nt.Name)
	}
	sort.Strings(names)
	return append([]string{"auto", "dark", "light"}, names...)
}
