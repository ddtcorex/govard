// Command depsaudit reports Go module drift and checks the repository's
// declared dependency holds.
//
// It exists because the stack is meant to sit at the newest stable release: an
// outdated module is fine only when it is either a declared hold or a module no
// package here imports (whose version belongs to the module that requires it).
//
// Exit codes are part of the contract, because "a hold is stale" and "this run
// could not check" must never be confused:
//
//	0  every declared hold still holds
//	1  a declared hold no longer suppresses anything: re-read holds.json
//	2  the run could not be verified (no module proxy, or a module that could not
//	   be classified); the holds were NOT evaluated and must not be edited
//
// The weekly security workflow runs it because it needs the module proxy, and it
// fails only on exit 1 or 2, so an upstream outage is reported rather than
// mistaken for a green audit.
//
// Usage:
//
//	go run ./scripts/depsaudit
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"govard/internal/depsaudit"
)

func main() {
	code, err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "depsaudit: %v\n", err)
	}
	os.Exit(code)
}

func run() (int, error) {
	out, err := exec.Command("go", "list", "-m", "-u", "-json", "all").CombinedOutput()
	if err != nil {
		return depsaudit.ExitUnverified, fmt.Errorf("go list -m -u -json all: %w: %s", err, firstLine(out))
	}
	modules, err := depsaudit.ParseModules(bytes.NewReader(out))
	if err != nil {
		return depsaudit.ExitUnverified, err
	}
	holds, err := depsaudit.Holds()
	if err != nil {
		return depsaudit.ExitUnverified, err
	}

	report := depsaudit.Analyze(modules, holds)
	if report.Unverified {
		fmt.Println("dependency audit could not be verified: `go list -m -u` returned no update information, which is what it returns when it cannot reach the module proxy.")
		fmt.Println("the declared holds were NOT evaluated; re-run this with network access before touching holds.json")
		return depsaudit.ExitUnverified, nil
	}

	report, classifyFailures := depsaudit.Classify(report, moduleIsImported)
	for _, failure := range classifyFailures {
		fmt.Fprintf(os.Stderr, "could not classify: %v\n", failure)
	}
	unclassified := len(classifyFailures)

	print := func(title string, rows []depsaudit.Module) {
		fmt.Printf("%s: %d\n", title, len(rows))
		for _, module := range rows {
			fmt.Printf("  %s %s -> %s\n", module.Path, module.Version, module.Update)
		}
	}
	print("outdated, held on purpose", report.Held)
	print("outdated, version owned by an upstream module", report.UpstreamOwned)
	print("outdated, movable", movableOnly(report))
	for _, hold := range report.StaleHolds {
		fmt.Printf("stale hold: %s pinned at %s, reason was %q, retry: %s\n",
			hold.Path, hold.Version, hold.Reason, hold.Retry)
	}

	if len(report.StaleHolds) > 0 {
		return depsaudit.ExitCode(report), fmt.Errorf("%d declared hold(s) no longer hold anything; update internal/depsaudit/holds.json", len(report.StaleHolds))
	}
	if unclassified > 0 {
		return depsaudit.ExitUnverified, fmt.Errorf("%d module(s) could not be classified, so the audit is incomplete", unclassified)
	}
	fmt.Println("all declared holds still hold")
	return depsaudit.ExitOK, nil
}

// movableOnly drops the rows the script re-classified as upstream-owned, so the
// three buckets stay disjoint.
func movableOnly(report depsaudit.Report) []depsaudit.Module {
	owned := make(map[string]bool, len(report.UpstreamOwned))
	for _, module := range report.UpstreamOwned {
		owned[module.Path] = true
	}
	var rows []depsaudit.Module
	for _, module := range report.Movable {
		if !owned[module.Path] {
			rows = append(rows, module)
		}
	}
	return rows
}

// desktopBuildTags is the build context this audit classifies in. `go mod why`
// takes no -tags flag, so the tags travel through GOFLAGS; without them a
// dependency imported only by desktop-tagged code would read as unused and be
// filed as upstream-owned, where nobody would ever bump it.
const desktopBuildTags = "desktop,production"

// moduleIsImported asks the go command whether any package in this module needs
// the module. `go mod why` prints the chain when it does, and
// "(main module does not need module X)" when it does not.
func moduleIsImported(path string) (bool, error) {
	cmd := exec.Command("go", "mod", "why", "-m", path)
	cmd.Env = append(os.Environ(), "GOFLAGS="+goFlagsWithTags())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("go mod why -m %s: %w: %s", path, err, firstLine(out))
	}
	return !strings.Contains(string(out), "does not need module"), nil
}

// goFlagsWithTags keeps any GOFLAGS the caller set and adds the desktop tags.
func goFlagsWithTags() string {
	existing := strings.TrimSpace(os.Getenv("GOFLAGS"))
	if existing == "" {
		return "-tags=" + desktopBuildTags
	}
	return existing + " -tags=" + desktopBuildTags
}

// firstLine keeps an exec error readable: the go command puts its reason on the
// first line of stderr, and the wrapped error is what a maintainer sees.
func firstLine(out []byte) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return "no output"
	}
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		text = text[:idx]
	}
	return text
}
