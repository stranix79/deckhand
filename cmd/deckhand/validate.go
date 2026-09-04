package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/stranix/deckhand/internal/deck"
)

func validateCmd() *cobra.Command {
	var quiet bool
	c := &cobra.Command{
		Use:   "validate <deck>",
		Short: "Check a deck (directory, .zip or .tar.gz) and report every problem",
		Long: `validate loads a deck exactly like present and serve would, and prints a
report: title, ratio, slides, then warnings and errors. Exit code 0 when the
deck is usable, 1 when it is not.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := deck.Load(args[0])
			out := cmd.OutOrStdout()
			if err != nil {
				rep := deck.AsReport(err)
				printReport(out, args[0], nil, rep, quiet)
				os.Exit(1)
			}
			defer func() { _ = d.Close() }()
			printReport(out, args[0], d, &deck.Report{Warnings: d.Warnings}, quiet)
			return nil
		},
	}
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "only print errors and warnings")
	return c
}

// printReport is the human-readable output of validate. Kept boring on
// purpose: one line per fact, symbols first, colours only on a terminal.
func printReport(w io.Writer, path string, d *deck.Deck, rep *deck.Report, quiet bool) {
	if d != nil && !quiet {
		fmt.Fprintf(w, "%s %s\n", ui.ok("✓"), ui.bold(d.Title))
		fmt.Fprintf(w, "  %s %s\n", ui.dim("source"), path)
		fmt.Fprintf(w, "  %s %s (%d×%d)\n", ui.dim("ratio "), d.Ratio, d.Width, d.Height)
		fmt.Fprintf(w, "  %s %d\n", ui.dim("slides"), len(d.Slides))
		for _, s := range d.Slides {
			mark := " "
			switch {
			case s.Public:
				mark = ui.ok("◎")
			case s.Notes != "":
				mark = ui.ok("●")
			}
			fmt.Fprintf(w, "    %2d  %s  %s\n", s.Index+1, mark, s.File)
		}
		if len(d.Slides) > 0 {
			fmt.Fprintf(w, "  %s\n", ui.dim("● has notes   ◎ has public notes"))
		}
	}
	for _, m := range rep.Warnings {
		fmt.Fprintf(w, "%s %s\n", ui.warn("!"), m)
	}
	for _, m := range rep.Errors {
		fmt.Fprintf(w, "%s %s\n", ui.err("✗"), m)
	}
	switch {
	case len(rep.Errors) > 0:
		fmt.Fprintf(w, "%s %d error(s): the deck cannot be presented\n", ui.err("✗"), len(rep.Errors))
	case len(rep.Warnings) > 0 && !quiet:
		fmt.Fprintf(w, "%s deck is usable (%d warning(s))\n", ui.ok("✓"), len(rep.Warnings))
	case !quiet:
		fmt.Fprintf(w, "%s deck is valid\n", ui.ok("✓"))
	}
}
