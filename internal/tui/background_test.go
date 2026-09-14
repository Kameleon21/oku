package tui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func frameCanvas(m *Model) *lipgloss.Canvas {
	return lipgloss.NewCanvas(m.lay.W, m.lay.H).Compose(lipgloss.NewLayer(m.View().Content))
}

func requireBackground(t *testing.T, canvas *lipgloss.Canvas, x, y int, want color.Color) {
	t.Helper()
	cell := canvas.CellAt(x, y)
	if cell == nil || !sameColor(cell.Style.Bg, want) {
		t.Fatalf("cell (%d,%d) = %+v, want background %v", x, y, cell, want)
	}
}

func TestThemeBackgroundsFillEveryTab(t *testing.T) {
	old := currentThemeSetting()
	t.Cleanup(func() { _ = ApplyThemeSetting(old) })
	for _, name := range ThemeSettings()[1:] {
		for _, size := range [][2]int{{40, 16}, {80, 24}, {120, 40}} {
			for tb := tabReading; tb < tabCount; tb++ {
				t.Run(fmt.Sprintf("%s/%dx%d/%s", name, size[0], size[1], tb.name()), func(t *testing.T) {
					_ = ApplyThemeSetting(name)
					m := newGoldenModel(t, size[0], size[1], tb)
					canvas := frameCanvas(m)
					requireBackground(t, canvas, 0, size[1]-1, m.st.th.Background)
					for y := 0; y < size[1]; y++ {
						for x := 0; x < size[0]; x++ {
							cell := canvas.CellAt(x, y)
							if cell == nil || cell.Style.Bg == nil {
								t.Fatalf("unpainted cell (%d,%d): %+v", x, y, cell)
							}
						}
					}
				})
			}
		}
	}
}

func TestHeaderBackgroundHasNoGaps(t *testing.T) {
	old := currentThemeSetting()
	t.Cleanup(func() { _ = ApplyThemeSetting(old) })
	for _, name := range ThemeSettings() {
		_ = ApplyThemeSetting(name)
		for _, width := range []int{20, 40, 80, 120} {
			m := newGoldenModel(t, width, 24, tabReading, withRunningTimer)
			m.syncing = true
			canvas := lipgloss.NewCanvas(width, 1).Compose(lipgloss.NewLayer(m.header(m.lay)))
			for x := 0; x < width; x++ {
				requireBackground(t, canvas, x, 0, m.st.th.Surface)
			}
		}
	}
}

func TestThemeBackgroundPreviewCancelSaveAndAuto(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := pickerModel(t, "auto")
	m.Update(backgroundMsg(false))
	assertBody := func(want color.Color) {
		t.Helper()
		requireBackground(t, frameCanvas(m), 0, m.lay.H-1, want)
		if m.View().BackgroundColor != nil {
			t.Fatal("themes must paint the app without changing terminal defaults")
		}
	}
	assertBody(nil)
	typeTheme(m, "nord")
	assertBody(hex("#2e3440"))
	panel := m.topModal().View(m.lay, m.st)
	_, px, py := overlayModal(m.lay, m.frame(), panel)
	requireBackground(t, frameCanvas(m), px+2, py+1, m.st.th.Surface)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	assertBody(hex("#2e3440"))
	m.Update(keyMsgFor(t, "esc"))
	assertBody(nil)
	if m.isDark {
		t.Fatal("preview changed the detected terminal background")
	}
	m.openThemePicker()
	typeTheme(m, "solarized-light")
	assertBody(hex("#fdf6e3"))
	m.Update(keyMsgFor(t, "enter"))
	assertBody(hex("#fdf6e3"))
	m.openThemePicker()
	typeTheme(m, "nord")
	assertBody(hex("#2e3440"))
	m.Update(keyMsgFor(t, "esc"))
	assertBody(hex("#fdf6e3"))
	m.shared.loaded = false
	canvas := frameCanvas(m)
	requireBackground(t, canvas, m.lay.W-1, m.lay.H-1, hex("#fdf6e3"))
}

func TestBackgroundFillPreservesStyledUnicodeAndLinks(t *testing.T) {
	bg, button := hex("#2e3440"), hex("#bf616a")
	link := ansi.SetHyperlink("https://example.com/book", "") + "本" + ansi.ResetHyperlink()
	input := " " + lipgloss.NewStyle().Bold(true).Background(button).Render("Save") + " " + link + " e\u0301  "
	fg := hex("#eceff4")
	out := fillColors(input, fg, bg)
	if ansi.Strip(out) != ansi.Strip(input) || lipgloss.Width(out) != lipgloss.Width(input) {
		t.Fatalf("background fill changed content: %q", out)
	}
	if !strings.Contains(out, "https://example.com/book") {
		t.Fatal("background fill dropped the link")
	}
	canvas := lipgloss.NewCanvas(lipgloss.Width(out), 1).Compose(lipgloss.NewLayer(out))
	for _, x := range []int{0, 5, 6, 8, 9, 10, 11} {
		requireBackground(t, canvas, x, 0, bg)
		if !sameColor(canvas.CellAt(x, 0).Style.Fg, fg) {
			t.Fatalf("unstyled text at %d did not inherit the theme foreground", x)
		}
	}
	for x := 1; x <= 4; x++ {
		requireBackground(t, canvas, x, 0, button)
	}
}
