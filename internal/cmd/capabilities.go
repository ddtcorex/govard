package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

// CapabilityRow is one command's declared requirement and its status on this
// host.
type CapabilityRow struct {
	Command   string `json:"command"`
	Requires  string `json:"requires"`
	Satisfied bool   `json:"satisfied"`
}

var capabilitiesJSONFlag bool

var capabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "List every command's runtime requirements and whether this host meets them",
	Long: `List every runnable command with the runtime capabilities it declares and
whether this host satisfies them. The requirement-free commands are the
Docker-free core: they work on a host with no container runtime at all.`,
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		rows := capabilityRows()
		if capabilitiesJSONFlag {
			raw, err := capabilitiesJSON(rows)
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
			return nil
		}
		fmt.Print(capabilitiesText(rows))
		return nil
	},
}

func init() {
	capabilitiesCmd.Flags().BoolVar(&capabilitiesJSONFlag, "json", false, "Print machine-readable output")
}

// walkCommandTree visits a command and every descendant in registration order.
func walkCommandTree(cmd *cobra.Command, visit func(*cobra.Command)) {
	visit(cmd)
	for _, child := range cmd.Commands() {
		walkCommandTree(child, visit)
	}
}

// capabilityRows derives the command manifest: the Docker-free core set is
// whatever resolves to no requirement, never a hand-written list.
func capabilityRows() []CapabilityRow {
	rows := []CapabilityRow{}
	satisfiedByRequirement := map[string]bool{}
	walkCommandTree(rootCmd, func(cmd *cobra.Command) {
		if cmd == rootCmd || (cmd.RunE == nil && cmd.Run == nil) {
			return
		}
		capabilities := runtime.Requires(cmd)
		names := make([]string, 0, len(capabilities))
		for _, capability := range capabilities {
			names = append(names, string(capability))
		}
		if len(names) == 0 {
			names = append(names, string(runtime.CapNone))
		}
		requires := strings.Join(names, ",")
		satisfied, probed := satisfiedByRequirement[requires]
		if !probed {
			// One probe per distinct requirement set keeps this fast even
			// though it covers every command.
			satisfied = runtime.Probe(capabilities...) == nil
			satisfiedByRequirement[requires] = satisfied
		}
		rows = append(rows, CapabilityRow{Command: cmd.CommandPath(), Requires: requires, Satisfied: satisfied})
	})
	sort.Slice(rows, func(i, j int) bool { return rows[i].Command < rows[j].Command })
	return rows
}

func capabilitiesText(rows []CapabilityRow) string {
	var builder strings.Builder
	for _, row := range rows {
		status := "ok"
		if !row.Satisfied {
			status = "missing"
		}
		fmt.Fprintf(&builder, "%s\t%s\t%s\n", row.Command, row.Requires, status)
	}
	return builder.String()
}

func capabilitiesJSON(rows []CapabilityRow) ([]byte, error) {
	payload := struct {
		SchemaVersion int             `json:"schema_version"`
		Commands      []CapabilityRow `json:"commands"`
	}{SchemaVersion: 1, Commands: rows}
	return json.MarshalIndent(payload, "", "  ")
}
