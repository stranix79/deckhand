package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/stranix79/deckhand/internal/deck"
	"github.com/stranix79/deckhand/internal/pptx"
)

func importCmd() *cobra.Command {
	var out string
	var dark, force bool
	c := &cobra.Command{
		Use:   "import <file.pptx>",
		Short: "Convert a PowerPoint file into a deck: one HTML file per slide",
		Long: `import reads a .pptx and writes a Deckhand deck next to it: one HTML file
per slide, deck.json with your presenter notes, and assets/ with the pictures,
the videos and one stylesheet you can edit.

Text slides become clean HTML: titles, lists, tables, pictures, laid out by
that stylesheet instead of the PowerPoint theme. Slides drawn by hand, like a
diagram with boxes and arrows, keep their layout and their colours.

The theme, animations, charts and SmartArt are not converted. Everything that
was left out is listed, and stays visible in the slide as a dashed box until
you replace it.`,
		Example: `  deckhand import talk.pptx
  deckhand import talk.pptx -o talk-html --dark
  deckhand present talk`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]
			if strings.EqualFold(filepath.Ext(src), ".ppt") {
				return fmt.Errorf("%s is an old binary .ppt file: open it in PowerPoint or LibreOffice and save it as .pptx first", src)
			}
			base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
			if out == "" {
				out = base
			}
			if filepath.Clean(out) == filepath.Clean(src) {
				return fmt.Errorf("%s has no extension: give the output directory with -o", src)
			}

			p, err := pptx.Open(src)
			if err != nil {
				return err
			}
			defer func() { _ = p.Close() }()
			if err := p.Write(out, pptx.Options{Title: base, Dark: dark, Force: force}); err != nil {
				return err
			}

			// The loader has the last word: what it accepts presents everywhere.
			d, err := deck.Load(out)
			if err != nil {
				return fmt.Errorf("the converted deck does not validate, please report it: %w", err)
			}
			defer func() { _ = d.Close() }()

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s %s\n", ui.ok("✓"), ui.bold(d.Title))
			fmt.Fprintf(w, "  %s %s\n", ui.dim("source"), src)
			fmt.Fprintf(w, "  %s %s\n", ui.dim("deck  "), out)
			fmt.Fprintf(w, "  %s %s (%d×%d)\n", ui.dim("ratio "), d.Ratio, d.Width, d.Height)
			slides := fmt.Sprintf("%d", p.Slides())
			if p.Hidden > 0 {
				slides += fmt.Sprintf(" (%d hidden slide(s) skipped)", p.Hidden)
			}
			fmt.Fprintf(w, "  %s %s\n", ui.dim("slides"), slides)
			fmt.Fprintf(w, "  %s %d slide(s) with presenter notes\n", ui.dim("notes "), p.WithNotes())
			fmt.Fprintf(w, "  %s %d picture(s) and video(s)\n", ui.dim("media "), p.Assets())
			for _, m := range p.Warnings {
				fmt.Fprintf(w, "%s %s\n", ui.warn("!"), m)
			}
			if n := len(p.Warnings); n > 0 {
				fmt.Fprintf(w, "%s converted with %d thing(s) to redo by hand\n", ui.ok("✓"), n)
			} else {
				fmt.Fprintf(w, "%s converted\n", ui.ok("✓"))
			}
			fmt.Fprintf(w, "  %s deckhand present %s\n", ui.dim("next  "), shellQuote(out))
			return nil
		},
	}
	c.Flags().StringVarP(&out, "output", "o", "", "directory to write the deck to (default: the file name without .pptx)")
	c.Flags().BoolVar(&dark, "dark", false, "use the dark theme instead of the light one")
	c.Flags().BoolVar(&force, "force", false, "write into a directory that is not empty (existing files are overwritten)")
	return c
}

// shellQuote makes the "next" hint copy-pastable when the path has spaces.
func shellQuote(s string) string {
	if strings.ContainsAny(s, " '\"$&()") {
		return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	}
	return s
}
