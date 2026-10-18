package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/Kameleon21/oku/internal/api"
	"github.com/Kameleon21/oku/internal/model"
)

// Issue #59: a token without read:journal still shows the book's detail, with
// a note where the journal would be, instead of an error toast.
func TestJournalWithoutPermissionShowsANoteNotAnError(t *testing.T) {
	m, fb := loadedDashboard(t)
	m.setTab(tabReading)
	fb.errs["BookJournal"] = fmt.Errorf("BookJournal: %w", &api.StatusError{Code: 403, APIError: "insufficient_scope", Scope: "read:journal"})

	drain(t, m, send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}))

	if m.toast.level == toastError {
		t.Fatalf("toast = %q, want no error toast for a missing permission", m.toast.text)
	}
	frame := stripANSI(m.frame())
	for _, want := range []string{"A desert planet.", "Your notes & quotes", "Hardcover hasn't granted oku the read:journal", "oku auth login"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("detail pane lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "BookJournal:") {
		t.Fatalf("detail pane shows the call-site prefix:\n%s", frame)
	}

	// Once the permission is granted, the next load replaces the note.
	delete(fb.errs, "BookJournal")
	fb.journals[1] = []model.BookJournalEntry{{Event: "note", Entry: "The spice must flow."}}
	drain(t, m, send(t, m, reqBookDetail{selection: selection{Book: &dune}}))
	frame = stripANSI(m.frame())
	if strings.Contains(frame, "read:journal") || !strings.Contains(frame, "The spice must flow.") {
		t.Fatalf("after a granted reload the pane should show the journal:\n%s", frame)
	}
}

// Other journal failures still raise the error toast.
func TestJournalFailureStillToasts(t *testing.T) {
	m, fb := loadedDashboard(t)
	m.setTab(tabReading)
	fb.errs["BookJournal"] = fmt.Errorf("BookJournal: %w", &api.StatusError{Code: 500})

	drain(t, m, send(t, m, tea.KeyPressMsg{Code: tea.KeyEnter}))

	if m.toast.level != toastError || !strings.HasPrefix(m.toast.text, "Load journal:") {
		t.Fatalf("toast = %v %q, want the journal error", m.toast.level, m.toast.text)
	}
}
