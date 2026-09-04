package picker

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Kameleon21/oku/internal/model"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170"))
)

// bookItem implements list.Item for the Bubbles list.
type bookItem struct {
	book model.UserBook
}

func (i bookItem) Title() string {
	title := i.book.Book.Title
	if i.book.Book.Pages > 0 {
		title += fmt.Sprintf(" (%s)", i.book.Progress())
	}
	return title
}

func (i bookItem) Description() string {
	return i.book.Book.AuthorString()
}

func (i bookItem) FilterValue() string {
	return i.book.Book.Title
}

type pickerModel struct {
	list     list.Model
	choice   int
	quitting bool
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		// While the user types a filter, keys like "q" are input and "esc"
		// exits filter mode — let the list handle them.
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "enter":
			if item, ok := m.list.SelectedItem().(bookItem); ok {
				m.choice = item.book.Book.ID
			}
			m.quitting = true
			return m, tea.Quit
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m pickerModel) View() string {
	if m.quitting {
		return ""
	}
	return m.list.View()
}

// PickBook presents an interactive picker for user books. Returns the book ID or 0 if cancelled.
func PickBook(books []model.UserBook, title string) (int, error) {
	items := make([]list.Item, len(books))
	for i, b := range books {
		items[i] = bookItem{book: b}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedStyle
	l := list.New(items, delegate, 60, min(len(items)*3+6, 20))
	l.Title = title
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	m := pickerModel{list: l}
	p := tea.NewProgram(m)

	final, err := p.Run()
	if err != nil {
		return 0, fmt.Errorf("picker error: %w", err)
	}

	result := final.(pickerModel)
	return result.choice, nil
}
