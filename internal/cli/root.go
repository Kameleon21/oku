package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var jsonOutput bool

func newRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "oku",
		Short:   "A fast CLI for Hardcover",
		Version: version,
	}

	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

// Execute runs the root command and returns an exit code.
func Execute(version string) int {
	cmd := newRootCmd(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), err)
		return 1
	}
	return 0
}
