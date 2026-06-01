package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Kameleon21/oku/internal/model"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	if book.Rating > 0 {
		ratingIn.SetValue(fmt.Sprintf("%.1f", book.Rating))
	}

	reviewIn := textarea.New()
	reviewIn.Placeholder = "Write your review..."
	reviewIn.SetWidth(72)
	reviewIn.SetHeight(10)
	reviewIn.SetValue(book.Review)
	reviewIn.ShowLineNumbers = false

	m := reviewFormModel{
		book:        book,
		ratingInput: ratingIn,
		reviewInput: reviewIn,
		focus:       reviewFocusRating,
	}
	m.focusRatingField()
	return m
}

func (m reviewFormModel) Init() tea.Cmd {
	return textinput.Blink
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
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "tab":
			if m.focus == reviewFocusRating {
				m.focusReviewField()
			} else {
				m.focusRatingField()
			}
			return m, nil
		case "shift+tab":
			if m.focus == reviewFocusText {
				m.focusRatingField()
			} else {
				m.focusReviewField()
			}
			return m, nil
		case "ctrl+s":
			rating, err := parseReviewRatingInput(m.ratingInput.Value())
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

func (m reviewFormModel) View() string {
	rating, err := parseReviewRatingInput(m.ratingInput.Value())
	stars := "☆☆☆☆☆"
	ratingLine := "unrated"
	if err != nil {
		ratingLine = "invalid rating"
	} else {
		stars = model.StarString(rating)
		ratingLine = fmt.Sprintf("%.1f", rating)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Review & Rating\n\n%s\n%s\n\n", m.book.Book.Title, fallback(m.book.Book.AuthorString(), "Unknown author")))
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

func (m *reviewFormModel) focusRatingField() {
	m.focus = reviewFocusRating
	m.ratingInput.Focus()
	m.reviewInput.Blur()
}

func (m *reviewFormModel) focusReviewField() {
	m.focus = reviewFocusText
	m.ratingInput.Blur()
	m.reviewInput.Focus()
}

func parseReviewRatingInput(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	rating, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("rating must be a number between 0 and 5")
	}
	if err := model.ValidateRating(rating); err != nil {
		return 0, err
	}
	return rating, nil
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
