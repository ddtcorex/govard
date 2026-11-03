package tests

import (
	"strings"
	"testing"

	"govard/scripts/bench/report/render"
)

func sampleRuns() render.RunsFile {
	return render.RunsFile{
		Target:     "govard",
		ImagesWarm: true,
		Meta: render.Meta{
			Machine:       render.Machine{CPU: "Test CPU", Cores: 12, RAMGB: 30, Disk: "ssd"},
			OS:            "Linux 6.8",
			DockerVersion: "29.9.2",
			GovardVersion: "v1.82.0",
			Commit:        "8964d620",
			Fixture:       "/tmp/fixture",
			StartedAt:     "2026-11-03T04:00:00Z",
		},
		Runs: []render.Run{
			{Run: 1, StartMS: 1000, StopMS: 300, TTFBP50MS: 10, TTFBP95MS: 20, StaticDeployMS: 21000, DICompileMS: 9000, PeakMemMB: 900},
			{Run: 2, StartMS: 3000, StopMS: 500, TTFBP50MS: 30, TTFBP95MS: 40, StaticDeployMS: 23000, DICompileMS: 11000, PeakMemMB: 1100},
			{Run: 3, StartMS: 2000, StopMS: 400, TTFBP50MS: 20, TTFBP95MS: 30, StaticDeployMS: 22000, DICompileMS: 10000, PeakMemMB: 1000},
		},
	}
}

func TestSummarizeUsesTheMedianOfRuns(t *testing.T) {
	summary := render.Summarize(sampleRuns())

	if summary.Runs != 3 {
		t.Fatalf("runs = %d, want 3", summary.Runs)
	}
	if got := summary.Metrics["start_ms"]; got != 2000 {
		t.Fatalf("start_ms median = %v, want 2000", got)
	}
	if got := summary.Metrics["ttfb_p50_ms"]; got != 20 {
		t.Fatalf("ttfb_p50_ms median = %v, want 20", got)
	}
	if !summary.ImagesWarm {
		t.Fatalf("ImagesWarm = false, want the summary to keep the flag")
	}
}

func TestRenderProducesOneColumnPerTool(t *testing.T) {
	govard := render.Summarize(sampleRuns())

	ddev := govard
	ddev.Target = "ddev"
	ddev.Metrics = map[string]float64{"start_ms": 9000, "ttfb_p50_ms": 40}

	table := render.Render([]render.Summary{govard, ddev})

	for _, want := range []string{"| Metric | govard | ddev |", "| Start", "| TTFB p50", "Test CPU", "8964d620"} {
		if !strings.Contains(table, want) {
			t.Fatalf("table is missing %q:\n%s", want, table)
		}
	}
}

func TestRenderOmitsAToolWithNoMeasuredMetric(t *testing.T) {
	govard := render.Summarize(sampleRuns())
	empty := render.Summary{Target: "ddev", Meta: govard.Meta, Metrics: map[string]float64{}}

	table := render.Render([]render.Summary{govard, empty})
	if strings.Contains(table, "| ddev |") {
		t.Fatalf("table rendered a column for a target with no metrics:\n%s", table)
	}
}

func TestRenderStatesThatImagesWereWarm(t *testing.T) {
	table := render.Render([]render.Summary{render.Summarize(sampleRuns())})

	if !strings.Contains(strings.ToLower(table), "warm") {
		t.Fatalf("table does not say whether images were warm:\n%s", table)
	}
}

func TestLatestKeepsOneSummaryPerTarget(t *testing.T) {
	older := render.Summarize(sampleRuns())
	older.Meta.StartedAt = "2026-11-03T04:00:00Z"

	newer := older
	newer.Meta.StartedAt = "2026-11-03T06:00:00Z"
	newer.Runs = 5

	latest := render.Latest([]render.Summary{older, newer})
	if len(latest) != 1 {
		t.Fatalf("Latest kept %d summaries, want 1", len(latest))
	}
	if latest[0].Runs != 5 {
		t.Fatalf("Latest kept the summary with %d runs, want the most recent (5)", latest[0].Runs)
	}
}
