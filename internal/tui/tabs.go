package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kameleon21/oku/internal/format"
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

// ── Navigation helpers ─────────────────────────────────────────────────────

// setSection focuses a section and re-sizes the lists. leftSectionHeights
// gives the focused list extra rows, so the sizes have to follow the focus and
// not only a window resize.
func (m *Model) setSection(s focusSection) {
	m.section = s
	m.resize()
}

func (m *Model) nextSection() {
	m.searchInput.Blur()
	m.setSection((m.section + 1) % sectionCount)
}

func (m *Model) prevSection() {
	m.searchInput.Blur()
	m.setSection((m.section - 1 + sectionCount) % sectionCount)
}

const okuASCII = `
   ____  __ __  __  __
  / __ \/ //_/ / / / /
 / / / / ,<   / / / / 
/ /_/ / /| | / /_/ /  
\____/_/ |_| \____/   
`

// introView renders the intro/welcome right panel.
func (m Model) introView(w int) string {
	var sb strings.Builder

	sb.WriteString(m.st.head.Render(okuASCII))
	sb.WriteString("\n")
	sb.WriteString(m.st.dim.Render("  a reading companion"))
	sb.WriteString("\n\n")

	writeField := func(label, value string) {
		sb.WriteString(m.st.label.Render(fmt.Sprintf("  %-10s ", label)))
		sb.WriteString(m.st.value.Render(value))
		sb.WriteString("\n")
	}

	if m.version != "" {
		writeField("Version", m.version)
	}
	writeField("Reading", fmt.Sprintf("%d books", len(m.readingBooks)))
	writeField("Oku", fmt.Sprintf("%d books", len(m.okuBooks)))

	if m.readingStats != nil && m.readingStats.Year.BooksFinished > 0 {
		writeField("This year", fmt.Sprintf("%d books", m.readingStats.Year.BooksFinished))
	}

	if m.timerState != nil {
		elapsed := time.Since(m.timerState.StartedAt)
		bookTitle := ""
		if m.timerBook != nil {
			bookTitle = m.timerBook.Title
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
	sb.WriteString(m.st.dim.Render("  j/k navigate   h/l section   ? help"))

	return sb.String()
}
