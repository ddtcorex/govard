// Package depsaudit reports Go module drift for govard, and checks that every
// dependency the repository pins on purpose is still pinned for a reason.
//
// The repository keeps its stack at the newest stable release, which is only
// honest if the two exceptions are visible and self-expiring: a hold whose
// module no longer needs holding is reported as stale, so the table cannot rot
// into a quiet downgrade.
package depsaudit

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Module is one entry of `go list -m -u -json all`. Update is empty when the
// module is already at its latest release.
type Module struct {
	Path     string
	Version  string
	Update   string
	Indirect bool
}

// Hold is a module pinned below its latest release, with the reason it cannot
// move and the condition that retires the hold.
type Hold struct {
	Path    string
	Version string
	Reason  string
	Retry   string
}

// Report is the audit result. UpstreamOwned is filled by the caller (the script
// uses `go mod why`), because deciding it needs the module graph, not just the
// list this package parses.
type Report struct {
	Movable       []Module
	Held          []Module
	UpstreamOwned []Module
	StaleHolds    []Hold
	// Unverified is set when the module list carried no update information at
	// all: `go list -m -u` answers successfully with empty Update fields when it
	// cannot reach the module proxy. Every hold then looks stale, so the audit
	// says it could not check instead of recommending an edit to holds.json.
	Unverified bool
}

// Exit codes, kept distinct so a caller can tell a stale hold from a run that
// simply could not reach the proxy.
const (
	ExitOK         = 0
	ExitStaleHolds = 1
	ExitUnverified = 2
)

// ExitCode maps a report onto the process exit code.
func ExitCode(report Report) int {
	switch {
	case report.Unverified:
		return ExitUnverified
	case len(report.StaleHolds) > 0:
		return ExitStaleHolds
	default:
		return ExitOK
	}
}

//go:embed holds.json
var holdsJSON []byte

// Holds returns the holds declared in the repository.
func Holds() ([]Hold, error) {
	return ParseHolds(holdsJSON)
}

// ParseHolds decodes and validates a holds document. Every field is required:
// a hold without a reason or a retry trigger is a downgrade nobody can review,
// and a hold with no version suppresses nothing.
func ParseHolds(data []byte) ([]Hold, error) {
	var holds []Hold
	if err := json.Unmarshal(data, &holds); err != nil {
		return nil, fmt.Errorf("parse holds: %w", err)
	}
	for _, hold := range holds {
		if strings.TrimSpace(hold.Path) == "" || strings.TrimSpace(hold.Version) == "" ||
			strings.TrimSpace(hold.Reason) == "" || strings.TrimSpace(hold.Retry) == "" {
			return nil, fmt.Errorf("hold %q needs path, version, reason and retry", hold.Path)
		}
	}
	return holds, nil
}

type listModule struct {
	Path     string
	Version  string
	Indirect bool
	Update   *struct {
		Path    string
		Version string
	}
}

// ParseModules decodes the concatenated JSON stream `go list -m -u -json all`
// writes, one object per module.
func ParseModules(r io.Reader) ([]Module, error) {
	dec := json.NewDecoder(r)
	var modules []Module
	for {
		var entry listModule
		err := dec.Decode(&entry)
		if err == io.EOF {
			return modules, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		module := Module{Path: entry.Path, Version: entry.Version, Indirect: entry.Indirect}
		if entry.Update != nil {
			module.Update = entry.Update.Version
		}
		modules = append(modules, module)
	}
}

// Analyze splits the modules into held, movable and stale holds. A hold is stale
// when its module is not at the held version, or when the module is no longer
// outdated: both mean the hold suppresses nothing and the table needs re-reading.
func Analyze(mods []Module, holds []Hold) Report {
	byPath := make(map[string]Module, len(mods))
	for _, module := range mods {
		byPath[module.Path] = module
	}
	heldByPath := make(map[string]Hold, len(holds))
	var report Report
	updates := 0
	for _, module := range mods {
		if module.Update != "" {
			updates++
		}
	}
	if updates == 0 && len(holds) > 0 {
		// A hold is by definition outdated, so a graph with update data but no
		// outdated hold is an expired hold; a graph with no update data anywhere
		// is a run that could not ask the proxy. They are different answers.
		report.Unverified = true
		return report
	}
	for _, hold := range holds {
		heldByPath[hold.Path] = hold
		module, known := byPath[hold.Path]
		if !known || module.Version != hold.Version || module.Update == "" {
			report.StaleHolds = append(report.StaleHolds, hold)
			continue
		}
		report.Held = append(report.Held, module)
	}
	for _, module := range mods {
		if module.Update == "" {
			continue
		}
		if _, isHeld := heldByPath[module.Path]; isHeld {
			continue
		}
		report.Movable = append(report.Movable, module)
	}
	for _, bucket := range [][]Module{report.Movable, report.Held} {
		sort.Slice(bucket, func(i, j int) bool { return bucket[i].Path < bucket[j].Path })
	}
	sort.Slice(report.StaleHolds, func(i, j int) bool { return report.StaleHolds[i].Path < report.StaleHolds[j].Path })
	return report
}

// Classify splits the movable rows into the ones this module does not import
// (their version belongs to the module that requires them) and the ones it does,
// using the caller's import probe. A probe failure is returned, never folded
// into a classification: an unclassified module must not read as upstream-owned.
func Classify(report Report, imported func(path string) (bool, error)) (Report, []error) {
	var failures []error
	for _, module := range report.Movable {
		used, err := imported(module.Path)
		if err != nil {
			failures = append(failures, fmt.Errorf("classify %s: %w", module.Path, err))
			continue
		}
		if !used {
			report.UpstreamOwned = append(report.UpstreamOwned, module)
		}
	}
	sort.Slice(report.UpstreamOwned, func(i, j int) bool {
		return report.UpstreamOwned[i].Path < report.UpstreamOwned[j].Path
	})
	return report, failures
}
