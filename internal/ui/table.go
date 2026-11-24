package ui

import "github.com/pterm/pterm"

// Table returns a table printer with govard's own separators: an ASCII pipe
// between columns and no header rule.
//
// The style is stated here rather than inherited from pterm's defaults, because
// pterm changed them in v0.12.84 (ASCII " | " became the Unicode " │ " and a
// "─" header rule appeared), which would otherwise change the output of every
// table under a dependency bump. A table is output people read and sometimes
// parse, so it stays put until someone changes it here on purpose.
func Table() *pterm.TablePrinter {
	printed := pterm.DefaultTable.WithHasHeader()
	printed.Separator = " | "
	printed.HeaderRowSeparator = ""
	return printed
}
