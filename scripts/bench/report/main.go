// Command report is the thin CLI over scripts/bench/report/render.
//
//	summarize <runs.json> <summary.json>   reduce runs to their median
//	render <results-dir> <docs-file>       splice the table into a docs page
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/scripts/bench/report/render"
)

const (
	startMarker = "<!-- bench:start -->"
	endMarker   = "<!-- bench:end -->"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: report summarize <runs.json> <summary.json> | report render <results-dir> <docs-file>")
	}

	switch os.Args[1] {
	case "summarize":
		if len(os.Args) != 4 {
			fail("summarize needs <runs.json> <summary.json>")
		}
		summarize(os.Args[2], os.Args[3])
	case "render":
		if len(os.Args) != 4 {
			fail("render needs <results-dir> <docs-file>")
		}
		renderDocs(os.Args[2], os.Args[3])
	default:
		fail("unknown mode: " + os.Args[1])
	}
}

func summarize(runsPath, summaryPath string) {
	var data render.RunsFile
	readJSON(runsPath, &data)
	if len(data.Runs) == 0 {
		fail("no runs in " + runsPath)
	}

	summary := render.Summarize(data)
	writeJSON(summaryPath, summary)
	fmt.Fprintf(os.Stderr, "report: %d run(s) -> %s\n", summary.Runs, summaryPath)
}

func renderDocs(resultsDir, docsPath string) {
	matches, err := filepath.Glob(filepath.Join(resultsDir, "*-summary.json"))
	if err != nil {
		fail("scan " + resultsDir + ": " + err.Error())
	}

	summaries := make([]render.Summary, 0, len(matches))
	for _, path := range matches {
		var summary render.Summary
		readJSON(path, &summary)
		summaries = append(summaries, summary)
	}

	table := render.Render(summaries)
	page, err := os.ReadFile(docsPath)
	if err != nil {
		fail("read " + docsPath + ": " + err.Error())
	}

	updated, err := splice(string(page), table)
	if err != nil {
		fail(err.Error())
	}
	if err := os.WriteFile(docsPath, []byte(updated), 0o644); err != nil {
		fail("write " + docsPath + ": " + err.Error())
	}

	fmt.Fprintf(os.Stderr, "report: %d summary file(s) -> %s\n", len(matches), docsPath)
}

func splice(page, table string) (string, error) {
	start := strings.Index(page, startMarker)
	end := strings.Index(page, endMarker)
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("docs page is missing the %s and %s markers", startMarker, endMarker)
	}

	head := page[:start+len(startMarker)]
	tail := page[end:]

	return head + "\n\n" + table + "\n" + tail, nil
}

func readJSON(path string, into any) {
	data, err := os.ReadFile(path)
	if err != nil {
		fail("read " + path + ": " + err.Error())
	}
	if err := json.Unmarshal(data, into); err != nil {
		fail("parse " + path + ": " + err.Error())
	}
}

func writeJSON(path string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fail("encode " + path + ": " + err.Error())
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		fail("write " + path + ": " + err.Error())
	}
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "report: "+message)
	os.Exit(1)
}
