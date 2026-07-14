package app

import (
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/model"
)

func TestUnfinishedReadRejectsFinishedRead(t *testing.T) {
	finishedAt := time.Now()
	finished := &model.UserBookRead{ID: 1, ProgressPages: 300, FinishedAt: &finishedAt}
	if got := unfinishedRead(finished); got != nil {
		t.Fatalf("unfinishedRead(finished) = %+v, want nil so UpdateProgress creates a new read", got)
	}

	active := &model.UserBookRead{ID: 2, ProgressPages: 50}
	if got := unfinishedRead(active); got != active {
		t.Fatalf("unfinishedRead(active) = %+v, want original read", got)
	}
}
