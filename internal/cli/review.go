package cli

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/spf13/cobra"
)

type reviewFieldFocus int

const (
	reviewFocusRating reviewFieldFocus = iota
	reviewFocusText
)

type reviewFormModel struct {
	book        model.UserBook
	ratingInput textinput.Model
	reviewInput textarea.Model
	focus       reviewFieldFocus
	// focusCmd is what the focused field's Focus() answered with; Init runs
	// it so the cursor appears.
	focusCmd tea.Cmd

	errMsg    string
	submitted bool
	cancelled bool
	rating    float64
	review    string
}

func newReviewFormModel(book model.UserBook) reviewFormModel {
	ratingIn := textinput.New()
	ratingIn.Prompt = "Rating (0-5, 0.5 steps): "
	ratingIn.Placeholder = "4.5"
	ratingIn.CharLimit = 4
	// A v2 textinput draws nothing wider than its width, and zero is not
	// "no limit" as it was in v1: the field is as wide as it accepts.
	ratingIn.SetWidth(4)
	if book.Rating > 0 {
		ratingIn.SetValue(fmt.Sprintf("%.1f", book.Rating))
	}

	reviewIn := textarea.New()
	reviewIn.Placeholder = "Write your review..."
	reviewIn.SetWidth(72)
	reviewIn.SetHeight(10)
	reviewIn.SetValue(book.Review)
	reviewIn.ShowLineNumbers = false

	// The form draws the terminal's own cursor (see View), so neither field
	// paints a block of its own.
	ratingIn.SetVirtualCursor(false)
	reviewIn.SetVirtualCursor(false)

	m := reviewFormModel{
		book:        book,
		ratingInput: ratingIn,
		reviewInput: reviewIn,
		focus:       reviewFocusRating,
	}
	m.focusCmd = m.focusRatingField()
	return m
}

// Init runs the command the focused field's Focus() answered with.
func (m reviewFormModel) Init() tea.Cmd {
	return m.focusCmd
}

func (m reviewFormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		width := msg.Width - 14
		if width < 40 {
			width = 40
		}
		m.reviewInput.SetWidth(width)
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "tab":
			if m.focus == reviewFocusRating {
				return m, m.focusReviewField()
			}
			return m, m.focusRatingField()
		case "shift+tab":
			if m.focus == reviewFocusText {
				return m, m.focusRatingField()
			}
			return m, m.focusReviewField()
		case "ctrl+s":
			rating, err := model.ParseRating(m.ratingInput.Value())
			if err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
			m.rating = rating
			m.review = m.reviewInput.Value()
			m.errMsg = ""
			m.submitted = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	if m.focus == reviewFocusRating {
		m.ratingInput, cmd = m.ratingInput.Update(msg)
	} else {
		m.reviewInput, cmd = m.reviewInput.Update(msg)
	}
	return m, cmd
}

// The rows the two fields are drawn on, counted from the top of the form:
// the heading, a blank, the title and the author, a blank, then the rating;
// a blank and the "Review:" label separate it from the textarea.
const (
	reviewFormRatingRow = 5
	reviewFormReviewRow = 8
)

func (m reviewFormModel) View() tea.View {
	v := tea.NewView(m.body())
	v.Cursor = m.cursor()
	return v
}

// cursor is the terminal's own cursor, in whichever field has the keyboard.
func (m reviewFormModel) cursor() *tea.Cursor {
	cur, row := m.ratingInput.Cursor(), reviewFormRatingRow
	if m.focus == reviewFocusText {
		cur, row = m.reviewInput.Cursor(), reviewFormReviewRow
	}
	if cur == nil {
		return nil
	}
	cur.Y += row
	return cur
}

func (m reviewFormModel) body() string {
	rating, err := model.ParseRating(m.ratingInput.Value())
	stars := "☆☆☆☆☆"
	ratingLine := "unrated"
	if err != nil {
		ratingLine = "invalid rating"
	} else {
		stars = model.StarString(rating)
		ratingLine = fmt.Sprintf("%.1f", rating)
	}

	author := m.book.Book.AuthorString()
	if author == "" {
		author = "Unknown author"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Review & Rating\n\n%s\n%s\n\n", m.book.Book.Title, author))
	b.WriteString(m.ratingInput.View())
	b.WriteString(fmt.Sprintf("  %s (%s)\n\n", stars, ratingLine))
	b.WriteString("Review:\n")
	b.WriteString(m.reviewInput.View())
	b.WriteString("\n")

	if m.errMsg != "" {
		b.WriteString("Error: ")
		b.WriteString(m.errMsg)
		b.WriteString("\n")
	}

	b.WriteString("\nTab/Shift+Tab switch fields   Ctrl+S submit   Esc cancel\n")
	return b.String()
}

func (m *reviewFormModel) focusRatingField() tea.Cmd {
	m.focus = reviewFocusRating
	m.reviewInput.Blur()
	return m.ratingInput.Focus()
}

func (m *reviewFormModel) focusReviewField() tea.Cmd {
	m.focus = reviewFocusText
	m.ratingInput.Blur()
	return m.reviewInput.Focus()
}

func newReviewCmd() *cobra.Command {
	var bookID int

	cmd := &cobra.Command{
		Use:   "review [title]",
		Short: "Open an interactive form to edit both rating and review",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isInteractiveTerminal(os.Stdin) || !isInteractiveTerminal(os.Stdout) {
				return fmt.Errorf("interactive terminal required for `oku review`")
			}

			title := strings.TrimSpace(strings.Join(args, " "))

			a, err := initApp()
			if err != nil {
				return err
			}
			defer a.Store.Close()

			candidateBookID, err := resolveBookIDForReviewInput(a, title, bookID, "Select a book to review")
			if err != nil {
				return err
			}

			resolvedBookID, err := a.ResolveBookID(candidateBookID)
			if err != nil {
				return err
			}

			ub, err := a.Store.GetUserBookByBookID(resolvedBookID)
			if err != nil {
				return err
			}
			if ub == nil {
				return fmt.Errorf("book ID %d not found in cache. Run: oku sync", resolvedBookID)
			}

			program := tea.NewProgram(newReviewFormModel(*ub))
			final, err := program.Run()
			if err != nil {
				return err
			}

			result, ok := final.(reviewFormModel)
			if !ok {
				return fmt.Errorf("unexpected review form result type %T", final)
			}
			if result.cancelled || !result.submitted {
				fmt.Println("Cancelled.")
				return nil
			}

			if err := a.ReviewBook(ctx(), ub.Book.ID, result.rating, result.review); err != nil {
				return err
			}

			fmt.Printf("Updated review and rating for %s\n", titleStyle.Render(ub.Book.Title))
			fmt.Printf("Rating: %s (%s)\n", pageStyle.Render(fmt.Sprintf("%.1f", result.rating)), model.StarString(result.rating))
			if strings.TrimSpace(result.review) == "" {
				fmt.Println("Review cleared.")
			} else {
				fmt.Println("Review updated.")
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&bookID, "book", 0, "Book ID (skips title lookup/picker)")
	return cmd
}
