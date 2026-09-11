package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCapabilitiesTextListsCoreCommands(t *testing.T) {
	rows := capabilityRows()
	if len(rows) == 0 {
		t.Fatal("capabilityRows returned no commands")
	}
	found := false
	for _, row := range rows {
		if row.Command == "govard version" {
			found = true
			if row.Requires != "none" {
				t.Fatalf("govard version requires %q, want none", row.Requires)
			}
		}
	}
	if !found {
		t.Fatal("govard version missing from capability rows")
	}
	if !strings.Contains(capabilitiesText(rows), "govard version\tnone\t") {
		t.Fatalf("text output missing version row:\n%s", capabilitiesText(rows))
	}
}

func TestCapabilitiesJSONIsVersioned(t *testing.T) {
	raw, err := capabilitiesJSON(capabilityRows())
	if err != nil {
		t.Fatalf("capabilitiesJSON error = %v", err)
	}
	var payload struct {
		SchemaVersion int `json:"schema_version"`
		Commands      []struct {
			Command   string `json:"command"`
			Requires  string `json:"requires"`
			Satisfied bool   `json:"satisfied"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if payload.SchemaVersion != 1 || len(payload.Commands) == 0 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestCapabilitiesCoversEveryRunnableCommand(t *testing.T) {
	rows := capabilityRows()
	seen := map[string]bool{}
	for _, row := range rows {
		seen[row.Command] = true
	}
	missing := []string{}
	walkCommandTree(rootCmd, func(cmd *cobra.Command) {
		if cmd == rootCmd || (cmd.RunE == nil && cmd.Run == nil) {
			return
		}
		if !seen[cmd.CommandPath()] {
			missing = append(missing, cmd.CommandPath())
		}
	})
	if len(missing) > 0 {
		t.Fatalf("commands missing from capabilities: %v", missing)
	}
}

func TestCapabilitiesRowsAreSorted(t *testing.T) {
	rows := capabilityRows()
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Command > rows[i].Command {
			t.Fatalf("rows not sorted: %q before %q", rows[i-1].Command, rows[i].Command)
		}
	}
}
