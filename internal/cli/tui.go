package cli

import (
	"github.com/spf13/cobra"

	"github.com/Kameleon21/oku/internal/tui"
)

// runDashboard opens the store the dashboard reads and hands it to
// internal/tui. Everything the dashboard draws lives there; this file is the
// cobra seam.
func runDashboard(version string) error {
	a, err := initApp()
	if err != nil {
		return err
	}
	defer a.Store.Close()

	return tui.Run(ctx(), a, currentOutputDensity(), version)
}

func newTUICmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive Oku dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDashboard(version)
		},
	}
}
