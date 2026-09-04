// Command deckhand is the single binary: validate, present (local), push and
// serve (hub). Milestone 1 ships validate and version; the other commands are
// added milestone by milestone.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/stranix79/deckhand/internal/version"
)

func main() {
	root := &cobra.Command{
		Use:           "deckhand",
		Short:         "Turn a folder of HTML slides into a presentation with a stage, a remote and a live audience",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(validateCmd(), versionCmd(), importCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, ui.err("error:"), err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "deckhand", version.Version)
		},
	}
}

// importCmd is the stub the brief asks for: the PPTX converter is planned, not built.
func importCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file.pptx>",
		Short: "Convert a PowerPoint deck (planned, not available yet)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return fmt.Errorf("import is planned but not available yet: export %q as one HTML file per slide (or ask your AI to) and run `deckhand validate` on the folder", args[0])
		},
	}
}
