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
	root.AddCommand(presentCmd(), validateCmd(), pushCmd(), loginCmd(), serveCmd(), versionCmd(), importCmd())

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
