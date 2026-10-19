package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/Kameleon21/oku/internal/config"
	"github.com/Kameleon21/oku/internal/model"
	"github.com/Kameleon21/oku/internal/store"
)

func reading(title string, page, pages int) model.UserBook {
	return model.UserBook{
		Book:          model.Book{Title: title, Pages: pages},
		UserBookReads: []model.UserBookRead{{ProgressPages: page}},
	}
}

func TestFormatLine(t *testing.T) {
	tests := []struct {
		name  string
		books []model.UserBook
		width int
		want  string
	}{
		{"nothing in progress", nil, 24, ""},
		{"one book", []model.UserBook{reading("Dune", 256, 412)}, 24, "📖 Dune 62%"},
		{"percent truncates like the TUI", []model.UserBook{reading("Dune", 411, 412)}, 24, "📖 Dune 99%"},
		{"just started", []model.UserBook{reading("Dune", 0, 412)}, 24, "📖 Dune 0%"},
		{"unknown page count drops the percent", []model.UserBook{reading("Dune", 120, 0)}, 24, "📖 Dune"},
		{"others counted", []model.UserBook{reading("Dune", 256, 412), reading("Piranesi", 30, 272), reading("Emma", 1, 474)}, 24, "📖 Dune 62% +2"},
		{"others counted without a percent", []model.UserBook{reading("Dune", 120, 0), reading("Emma", 1, 474)}, 24, "📖 Dune +1"},
		{"exactly width is kept", []model.UserBook{reading("The Left Hand of Darkness", 10, 304)}, 25, "📖 The Left Hand of Darkness 3%"},
		{"long title is cut", []model.UserBook{reading("The Name of the Wind: The Kingkiller Chronicle", 331, 662)}, 24, "📖 The Name of the Wind: T… 50%"},
		{"no space left before the ellipsis", []model.UserBook{reading("Piranesi and the House", 10, 272)}, 10, "📖 Piranesi… 3%"},
		{"wide characters count by cell", []model.UserBook{reading("ノルウェイの森", 10, 100)}, 7, "📖 ノルウ… 10%"},
		{"surrounding whitespace ignored", []model.UserBook{reading("  Dune \n", 256, 412)}, 24, "📖 Dune 62%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLine(tt.books, tt.width); got != tt.want {
				t.Errorf("formatLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

// runLine runs `oku line args...` through the root command, as a prompt would.
func runLine(t *testing.T, args ...string) string {
	t.Helper()
	t.Cleanup(func() { jsonOutput = false })
	var out bytes.Buffer
	cmd := newRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"line"}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("oku line %v: %v", args, err)
	}
	return out.String()
}

func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

// seedReading caches books as currently reading, the first one most recently
// updated.
func seedReading(t *testing.T, books ...model.UserBook) {
	t.Helper()
	if err := config.EnsureDataDir(); err != nil {
		t.Fatal(err)
	}
	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	for i, b := range books {
		id := i + 1
		b.ID, b.BookID, b.Book.ID = id, id, id
		b.StatusID = model.StatusCurrentlyReading
		b.UpdatedAt = now.Add(-time.Duration(i) * time.Hour)
		if err := db.UpsertUserBook(b); err != nil {
			t.Fatal(err)
		}
		for _, r := range b.UserBookReads {
			r.ID, r.UserBookID, r.StartedAt = id, id, &now
			if err := db.UpsertUserBookRead(r); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestLineBeforeFirstRun: a prompt runs oku line before oku has ever been set
// up. It prints nothing and leaves no cache behind.
func TestLineBeforeFirstRun(t *testing.T) {
	isolateConfig(t)

	if out := runLine(t); out != "" {
		t.Fatalf("output = %q, want nothing", out)
	}
	dbPath, err := config.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("cache created at %s (stat err %v)", dbPath, err)
	}
}

func TestLineNothingInProgress(t *testing.T) {
	isolateConfig(t)
	seedReading(t)

	if out := runLine(t); out != "" {
		t.Fatalf("output = %q, want nothing", out)
	}
}

func TestLineShowsTheLatestBook(t *testing.T) {
	isolateConfig(t)
	seedReading(t, reading("Dune", 256, 412), reading("Piranesi", 30, 272))

	if out, want := runLine(t), "📖 Dune 62% +1\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestLineWidthFlag(t *testing.T) {
	isolateConfig(t)
	seedReading(t, reading("Piranesi", 30, 272))

	if out, want := runLine(t, "--width", "4"), "📖 Pir… 11%\n"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestLineRejectsAZeroWidth(t *testing.T) {
	isolateConfig(t)
	t.Cleanup(func() { jsonOutput = false })
	cmd := newRootCmd("test")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"line", "--width", "0"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("--width 0 accepted")
	}
}

func TestLineJSON(t *testing.T) {
	isolateConfig(t)
	seedReading(t, reading("Dune", 256, 412), reading("Piranesi", 30, 272))

	want := `{
  "title": "Dune",
  "page": 256,
  "pages": 412,
  "percent": 62,
  "others": 1
}
`
	if out := runLine(t, "--json"); out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestLineJSONUnknownPagesAndNothingInProgress(t *testing.T) {
	isolateConfig(t)
	if out := runLine(t, "--json"); out != "null\n" {
		t.Fatalf("before first run: output = %q, want null", out)
	}

	seedReading(t, reading("Dune", 120, 0))
	want := `{
  "title": "Dune",
  "page": 120,
  "pages": 0,
  "percent": null,
  "others": 0
}
`
	if out := runLine(t, "--json"); out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}
