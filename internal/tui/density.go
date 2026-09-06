package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Density is how much of a book a row shows. It is the CLI's `--view` setting
// and the dashboard's `z` key at once: both list the same books, so they read
// the same value.
type Density int

const (
	DensityCompact Density = iota
	DensityDefault
	DensityVerbose
)

// ParseDensity reads the `--view` flag. An empty value is the default, so a
// command that never sets the flag gets the middle density.
func ParseDensity(raw string) (Density, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "default":
		return DensityDefault, nil
	case "compact":
		return DensityCompact, nil
	case "verbose":
		return DensityVerbose, nil
	default:
		return DensityDefault, fmt.Errorf("invalid --view value %q (valid: compact, default, verbose)", raw)
	}
}

// Label names the density for a toast or a flag error.
func (d Density) Label() string {
	switch d {
	case DensityCompact:
		return "compact"
	case DensityVerbose:
		return "verbose"
	default:
		return "default"
	}
}

// cycleDensity steps to the next density and tells the sections, whose
// rows are rebuilt from it.
func (m *Model) cycleDensity() tea.Cmd {
	switch m.shared.density {
	case DensityCompact:
		m.shared.density = DensityDefault
	case DensityDefault:
		m.shared.density = DensityVerbose
	default:
		m.shared.density = DensityCompact
	}
	cmd := m.broadcast(dataChangedMsg{dataDensity})
	return tea.Batch(cmd, m.showToast(toastInfo, "Density: "+m.shared.density.Label()))
}
