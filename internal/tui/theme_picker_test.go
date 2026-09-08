package tui

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/Kameleon21/oku/internal/config"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/exp/golden"
)

func pickerModel(t *testing.T, setting string) *Model {
	t.Helper()
	old := currentThemeSetting()
	t.Cleanup(func() { _ = ApplyThemeSetting(old) })
	if err := ApplyThemeSetting(setting); err != nil {
		t.Fatal(err)
	}
	m := newGoldenModel(t, 80, 24, tabReading)
	m.Update(runeKey('T'))
	if _, ok := m.topModal().(*themePicker); !ok {
		t.Fatal("T did not open picker")
	}
	return m
}
func typeTheme(m *Model, s string) {
	for _, r := range s {
		m.Update(runeKey(r))
	}
}
func TestThemePickerPreviewCancelAndSave(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.SetTheme("nord"); err != nil {
		t.Fatal(err)
	}
	m := pickerModel(t, "nord")
	original := m.st.th
	typeTheme(m, "drc")
	if currentThemeSetting() != "dracula" || m.st.th == original {
		t.Fatal("fuzzy search did not preview Dracula")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "nord" {
		t.Fatal("preview wrote config")
	}
	m.Update(keyMsgFor(t, "esc"))
	if m.topModal() != nil || m.st.th != original || currentThemeSetting() != "nord" {
		t.Fatal("cancel did not restore theme")
	}
	m.openThemePicker()
	typeTheme(m, "tkn")
	m.Update(keyMsgFor(t, "enter"))
	cfg, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if m.topModal() != nil || cfg.Theme != "tokyo-night" || currentThemeSetting() != cfg.Theme {
		t.Fatal("selection not saved/applied")
	}
}
func TestThemePickerNoMatchAndSaveFailure(t *testing.T) {
	m := pickerModel(t, "light")
	p := m.topModal().(*themePicker)
	typeTheme(m, "zzzz")
	before := currentThemeSetting()
	m.Update(keyMsgFor(t, "down"))
	m.Update(keyMsgFor(t, "enter"))
	if m.topModal() != p || currentThemeSetting() != before {
		t.Fatal("empty result changed theme")
	}
	p.input.SetValue("")
	p.cursor = 0
	p.save = func(string) error { return errors.New("read-only config") }
	m.Update(keyMsgFor(t, "down"))
	m.Update(keyMsgFor(t, "enter"))
	if m.topModal() != p || p.err == "" {
		t.Fatal("save failure must remain visible")
	}
	m.Update(keyMsgFor(t, "esc"))
	if currentThemeSetting() != "light" {
		t.Fatal("failed save lost original theme")
	}
}
func TestThemePickerAutoAndRouting(t *testing.T) {
	m := pickerModel(t, "auto")
	m.applyBackground(false)
	typeTheme(m, "nord")
	if !m.themePinned {
		t.Fatal("named preview must pin background")
	}
	m.Update(keyMsgFor(t, "esc"))
	if m.themePinned || m.isDark || m.st.th != NewTheme(false) {
		t.Fatal("auto cancellation lost terminal background")
	}
	for tb := tabReading; tb < tabCount; tb++ {
		m.setTab(tb)
		if m.section().CapturesKeys() {
			m.Update(runeKey('T'))
			if m.topModal() != nil {
				t.Fatal("picker stole text input")
			}
		} else {
			m.Update(runeKey('T'))
			if m.topModal() == nil {
				t.Fatalf("tab %v cannot open picker", tb)
			}
			m.Update(keyMsgFor(t, "esc"))
		}
	}
}
func TestThemePickerFrames(t *testing.T) {
	old := currentThemeSetting()
	t.Cleanup(func() { _ = ApplyThemeSetting(old) })
	for _, size := range [][2]int{{80, 24}, {120, 40}, {40, 16}} {
		for _, name := range []string{"nord", "light"} {
			t.Run(fmt.Sprintf("%dx%d_%s", size[0], size[1], name), func(t *testing.T) {
				_ = ApplyThemeSetting(name)
				m := newGoldenModel(t, size[0], size[1], tabReading)
				m.openThemePicker()
				p := m.topModal().(*themePicker)
				panel := p.View(m.lay, m.st)
				if lipgloss.Width(panel) > size[0] || lipgloss.Height(panel) > size[1] {
					t.Fatalf("panel exceeds terminal: %dx%d", lipgloss.Width(panel), lipgloss.Height(panel))
				}
				golden.RequireEqual(t, []byte(frameAt(m, colorprofile.TrueColor)))
				m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			})
		}
	}
}

func TestThemePickerNavigationAndResize(t *testing.T) {
	m := pickerModel(t, "auto")
	p := m.topModal().(*themePicker)
	previous := []tea.KeyPressMsg{{Code: tea.KeyUp}, {Code: 'p', Mod: tea.ModCtrl}, {Code: tea.KeyTab, Mod: tea.ModShift}}
	next := []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: 'n', Mod: tea.ModCtrl}, {Code: tea.KeyTab}}
	for i := range previous {
		p.cursor = 0
		m.Update(previous[i])
		if p.cursor != len(ThemeSettings())-1 || currentThemeSetting() != "catppuccin-mocha" {
			t.Fatal("previous did not wrap and preview")
		}
		m.Update(next[i])
		if p.cursor != 0 || currentThemeSetting() != "auto" {
			t.Fatal("next did not wrap and preview")
		}
	}
	typeTheme(m, "cat")
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 16})
	cur := p.Cursor()
	if cur == nil || cur.Y != modalContentY+1 || cur.X >= 40 {
		t.Fatalf("invalid resized input cursor: %+v", cur)
	}
	m.Update(keyMsgFor(t, "esc"))
}
