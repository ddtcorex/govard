// Package render turns benchmark run files into a summary and a markdown table.
//
// It is a library package with a thin main under scripts/bench/report so the
// published table can be tested like any other behaviour in this repository.
package render

import (
	"fmt"
	"sort"
	"strings"
)

// Machine describes the host a run was measured on.
type Machine struct {
	CPU   string `json:"cpu"`
	Cores int    `json:"cores"`
	RAMGB int    `json:"ram_gb"`
	Disk  string `json:"disk"`
}

// Meta is the provenance block every published table carries.
type Meta struct {
	Machine       Machine `json:"machine"`
	OS            string  `json:"os"`
	DockerVersion string  `json:"docker_version"`
	GovardVersion string  `json:"govard_version"`
	Commit        string  `json:"commit"`
	Fixture       string  `json:"fixture"`
	StartedAt     string  `json:"started_at"`
}

// Run is one measurement of one target.
type Run struct {
	Run            int     `json:"run"`
	StartMS        float64 `json:"start_ms"`
	StopMS         float64 `json:"stop_ms"`
	TTFBP50MS      float64 `json:"ttfb_p50_ms"`
	TTFBP95MS      float64 `json:"ttfb_p95_ms"`
	StaticDeployMS float64 `json:"static_deploy_ms"`
	DICompileMS    float64 `json:"di_compile_ms"`
	PeakMemMB      float64 `json:"peak_mem_mb"`
}

// RunsFile is one adapter's output.
type RunsFile struct {
	Target     string `json:"target"`
	Stack      string `json:"stack,omitempty"`
	ImagesWarm bool   `json:"images_warm"`
	Meta       Meta   `json:"meta"`
	Runs       []Run  `json:"runs"`
}

// Summary is the median of one adapter's runs.
type Summary struct {
	Target     string             `json:"target"`
	Stack      string             `json:"stack,omitempty"`
	ImagesWarm bool               `json:"images_warm"`
	Runs       int                `json:"runs"`
	Meta       Meta               `json:"meta"`
	Metrics    map[string]float64 `json:"metrics"`
}

// metricOrder is the row order of the published table, so two runs of the tool
// never reorder the page.
var metricOrder = []struct {
	key   string
	label string
	unit  string
}{
	{"start_ms", "Start (up to first 200)", "ms"},
	{"stop_ms", "Stop", "ms"},
	{"ttfb_p50_ms", "TTFB p50", "ms"},
	{"ttfb_p95_ms", "TTFB p95", "ms"},
	{"static_deploy_ms", "Static deploy", "ms"},
	{"di_compile_ms", "DI compile", "ms"},
	{"peak_mem_mb", "Peak memory", "MB"},
}

// Summarize reduces the runs to their median, which is what the page publishes.
func Summarize(data RunsFile) Summary {
	summary := Summary{
		Target:     data.Target,
		Stack:      data.Stack,
		ImagesWarm: data.ImagesWarm,
		Runs:       len(data.Runs),
		Meta:       data.Meta,
		Metrics:    map[string]float64{},
	}

	for _, metric := range metricOrder {
		values := make([]float64, 0, len(data.Runs))
		for _, run := range data.Runs {
			switch metric.key {
			case "start_ms":
				values = append(values, run.StartMS)
			case "stop_ms":
				values = append(values, run.StopMS)
			case "ttfb_p50_ms":
				values = append(values, run.TTFBP50MS)
			case "ttfb_p95_ms":
				values = append(values, run.TTFBP95MS)
			case "static_deploy_ms":
				values = append(values, run.StaticDeployMS)
			case "di_compile_ms":
				values = append(values, run.DICompileMS)
			case "peak_mem_mb":
				values = append(values, run.PeakMemMB)
			}
		}
		if len(values) > 0 {
			summary.Metrics[metric.key] = median(values)
		}
	}

	return summary
}

func median(values []float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}

	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// Latest keeps one summary per target: the one measured most recently. Without
// this, running the harness twice into the same results directory would render
// two columns with the same headline, which reads as two different tools.
func Latest(summaries []Summary) []Summary {
	byTarget := map[string]Summary{}
	order := []string{}

	for _, summary := range summaries {
		previous, seen := byTarget[summary.Target]
		if !seen {
			order = append(order, summary.Target)
			byTarget[summary.Target] = summary
			continue
		}
		if summary.Meta.StartedAt >= previous.Meta.StartedAt {
			byTarget[summary.Target] = summary
		}
	}

	// govard leads the table; every other target follows alphabetically, so the
	// column order never depends on directory listing order.
	sort.Slice(order, func(i, j int) bool {
		if order[i] == "govard" {
			return true
		}
		if order[j] == "govard" {
			return false
		}
		return order[i] < order[j]
	})

	latest := make([]Summary, 0, len(order))
	for _, target := range order {
		latest = append(latest, byTarget[target])
	}

	return latest
}

// Render builds the markdown table. A target with no measured metric is left
// out entirely, so a page never shows an empty column for a tool whose adapter
// has not run yet.
func Render(summaries []Summary) string {
	measured := make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		if len(summary.Metrics) > 0 {
			measured = append(measured, summary)
		}
	}
	if len(measured) == 0 {
		return "No benchmark results yet.\n"
	}

	var b strings.Builder
	b.WriteString("| Metric |")
	for _, summary := range measured {
		fmt.Fprintf(&b, " %s |", summary.Target)
	}
	b.WriteString("\n| --- |")
	for range measured {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")

	for _, metric := range metricOrder {
		b.WriteString("| " + metric.label + " |")
		wrote := false
		for _, summary := range measured {
			value, ok := summary.Metrics[metric.key]
			if !ok {
				b.WriteString(" n/a |")
				continue
			}
			wrote = true
			fmt.Fprintf(&b, " %s %s |", formatValue(value), metric.unit)
		}
		if !wrote {
			continue
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	for _, summary := range measured {
		fmt.Fprintf(&b, "- **%s**: %d run(s), images %s%s. %s, %d cores, %d GiB RAM, disk %s, %s, Docker %s, govard %s at commit %s, measured %s.\n",
			summary.Target,
			summary.Runs,
			warmLabel(summary.ImagesWarm),
			stackLabel(summary.Stack),
			summary.Meta.Machine.CPU,
			summary.Meta.Machine.Cores,
			summary.Meta.Machine.RAMGB,
			summary.Meta.Machine.Disk,
			summary.Meta.OS,
			summary.Meta.DockerVersion,
			summary.Meta.GovardVersion,
			summary.Meta.Commit,
			summary.Meta.StartedAt,
		)
	}

	return b.String()
}

// stackLabel names the services an arm actually ran, because the two tools do
// not start the same stack by default and the difference is part of the result.
func stackLabel(stack string) string {
	if stack == "" {
		return ""
	}

	return "; stack: " + stack
}

func warmLabel(warm bool) string {
	if warm {
		return "warm (pulls excluded)"
	}

	return "cold (pulls included)"
}

func formatValue(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%d", int64(value))
	}

	return fmt.Sprintf("%.1f", value)
}
