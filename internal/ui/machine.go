package ui

import (
	"io"

	"github.com/pterm/pterm"
)

// RouteHumanOutputTo points every pterm printer at w. Callers use it when the
// command was asked for machine-readable output: stdout then belongs to the
// document the caller parses, and a banner or a progress line in front of it
// makes the stream unparseable.
//
// pterm's default writer is captured at package init and every Info/Success/
// Warning line, plus the printers built from them, resolves it at write time, so
// one call covers the whole command tree. Machine documents are printed with
// fmt and the command's own stdout, never through pterm, which is what makes the
// split safe.
func RouteHumanOutputTo(w io.Writer) {
	if w == nil {
		w = io.Discard
	}
	pterm.SetDefaultOutput(w)
}
