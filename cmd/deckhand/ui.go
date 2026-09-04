package main

import (
	"os"

	"golang.org/x/term"
)

// ui is the tiny colour helper for terminal output. Colours are only used when
// stdout is a terminal and NO_COLOR is not set, so logs and pipes stay clean.
var ui = newUI()

type palette struct{ on bool }

func newUI() palette {
	_, noColor := os.LookupEnv("NO_COLOR")
	return palette{on: !noColor && term.IsTerminal(int(os.Stdout.Fd()))}
}

func (p palette) wrap(code, s string) string {
	if !p.on {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func (p palette) ok(s string) string   { return p.wrap("32", s) } // green
func (p palette) warn(s string) string { return p.wrap("33", s) } // yellow
func (p palette) err(s string) string  { return p.wrap("31", s) } // red
func (p palette) bold(s string) string { return p.wrap("1", s) }
func (p palette) dim(s string) string  { return p.wrap("2", s) }
