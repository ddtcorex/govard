package tests

import (
	"strings"
	"testing"

	"github.com/pterm/pterm"

	"govard/internal/ui"
)

// The table style is pinned, not inherited: pterm v0.12.84 changed its defaults
// from ASCII pipes to Unicode box characters and added a header rule, which
// would have changed every govard table's output under a dependency bump.
func TestTableKeepsTheAsciiSeparators(t *testing.T) {
	pterm.DisableColor()
	table := ui.Table().WithData(pterm.TableData{
		{"NAME", "STATUS"},
		{"example", "running"},
	})
	out, err := table.Srender()
	if err != nil {
		t.Fatalf("render table: %v", err)
	}
	if strings.ContainsRune(out, '│') || strings.ContainsRune(out, '─') {
		t.Fatalf("table uses Unicode box characters:\n%s", out)
	}
	if !strings.Contains(out, " | ") {
		t.Fatalf("table does not use the ASCII column separator:\n%s", out)
	}
	if strings.Count(out, "\n") != 2 {
		t.Fatalf("table gained a header rule or extra row:\n%s", out)
	}
}
