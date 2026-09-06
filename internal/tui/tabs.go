package tui

import (
	"fmt"
	"strings"

	"github.com/Kameleon21/oku/internal/format"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type focusSection int

const (
	sectionIntro focusSection = iota
	sectionReading
	sectionOku
	sectionSearch
	sectionStats
	sectionTimer
	sectionCount // sentinel
)

type sectionDef struct {
	id    focusSection
	label string
	count int // -1 = no count
}

const okuASCII = `
   ____  __ __  __  __
  / __ \/ //_/ / / / /
 / / / / ,<   / / / / 
/ /_/ / /| | / /_/ /  
\____/_/ |_| \____/   
`

// introSection is the welcome page: the logo, the library at a glance and
// the keys to get going.
type introSection struct {
	sh *shared
	st styles
}

func newIntroSection(sh *shared, st styles) *introSection {
	return &introSection{sh: sh, st: st}
}

func (s *introSection) Update(msg tea.Msg) tea.Cmd {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	k := keysFor(s)
	switch {
	case key.Matches(keyMsg, k.Down):
		return request(reqSwitchTab{step: +1})
	case key.Matches(keyMsg, k.Up):
		return request(reqSwitchTab{step: -1})
	}
	return nil
}

func (s *introSection) View(int, int) string {
	st := s.st
	var sb strings.Builder

	sb.WriteString(st.head.Render(okuASCII))
	sb.WriteString("\n")
	sb.WriteString(st.dim.Render("  a reading companion"))
	sb.WriteString("\n\n")

	writeField := func(label, value string) {
		sb.WriteString(st.label.Render(fmt.Sprintf("  %-10s ", label)))
		sb.WriteString(st.value.Render(value))
		sb.WriteString("\n")
	}

	writeField("Reading", fmt.Sprintf("%d books", len(s.sh.reading)))
	writeField("Oku", fmt.Sprintf("%d books", len(s.sh.oku)))

	if s.sh.stats != nil && s.sh.stats.Year.BooksFinished > 0 {
		writeField("This year", fmt.Sprintf("%d books", s.sh.stats.Year.BooksFinished))
	}

	if s.sh.timer != nil {
		elapsed := s.sh.now().Sub(s.sh.timer.StartedAt)
		bookTitle := ""
		if s.sh.timerBook != nil {
			bookTitle = s.sh.timerBook.Title
		}

		if bookTitle != "" {
			writeField("Timer", fmt.Sprintf("%s (%s)", format.Duration(elapsed), bookTitle))
		} else {
			writeField("Timer", format.Duration(elapsed))
		}
	} else {
		writeField("Timer", "not running")
	}

	sb.WriteString("\n")
	sb.WriteString(st.dim.Render("  j/k navigate   h/l section   ? help"))

	return sb.String()
}

func (s *introSection) Resize(int, int) {}

func (s *introSection) Keys(k *keyMap) {
	sectionHint := hint("section", k.PrevSection, k.NextSection)
	k.Up.SetHelp("k", "section")
	k.Down.SetHelp("j", "section")
	enable(&k.Quit, &k.Help, &k.Up, &k.Down, &k.NextSection, &k.PrevSection, &k.Search)
	k.short = []key.Binding{k.Help, sectionHint, hintAs("Tab", "next", k.NextSection), k.Search, k.Quit}
}

func (s *introSection) Focus() {}
func (s *introSection) Blur()  {}

func (s *introSection) CapturesKeys() bool { return false }

func (s *introSection) Title() string { return "Intro" }

func (s *introSection) Selected() selection { return selection{} }
