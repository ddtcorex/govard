package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"govard/internal/testbudget"
)

func overrunReport() result {
	return result{
		testCount: 3,
		violations: []testbudget.Violation{
			{Test: "TestSlow", Actual: 12 * time.Second, Budget: 7 * time.Second},
		},
	}
}

func TestParseHostLoadReadsTheThreeAveragesAndTheCPUCount(t *testing.T) {
	load, ok := parseHostLoad("5.96 4.49 6.15 3/3778 594425\n", 12)
	if !ok {
		t.Fatal("a well-formed /proc/loadavg line was rejected")
	}
	if load.One != 5.96 || load.Five != 4.49 || load.Fifteen != 6.15 || load.CPUs != 12 {
		t.Fatalf("load = %+v, want 5.96/4.49/6.15 on 12 CPUs", load)
	}
	if _, ok := parseHostLoad("not a load average", 12); ok {
		t.Fatal("garbage was accepted as a load average")
	}
}

// The load average is what separates a regression from a busy laptop, so it has
// to sit on the very line the developer reads when the gate goes red.
func TestAnOverrunLinePrintsTheHostLoadAndTheCPUCount(t *testing.T) {
	r := overrunReport()
	r.load = hostLoad{One: 5.96, Five: 4.49, Fifteen: 6.15, CPUs: 12, known: true}
	var out bytes.Buffer
	r.print(&out)

	var overrun string
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.Contains(line, "TestSlow") && strings.Contains(line, "budget 7s") {
			overrun = line
		}
	}
	if overrun == "" {
		t.Fatalf("no overrun line for TestSlow:\n%s", out.String())
	}
	for _, want := range []string{"load 5.96/4.49/6.15", "12 CPUs"} {
		if !strings.Contains(overrun, want) {
			t.Errorf("overrun line %q does not carry %q", overrun, want)
		}
	}
}

func TestAnOverrunStillFailsWhenTheHostLoadIsUnknown(t *testing.T) {
	r := overrunReport()
	var out bytes.Buffer
	r.print(&out)
	if !strings.Contains(out.String(), "1 test(s) exceeded their time budget") || !strings.Contains(out.String(), "TestSlow") {
		t.Fatalf("an unknown load must not hide the overrun:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "load unknown") {
		t.Fatalf("the report should say the load could not be read:\n%s", out.String())
	}
}

func TestABusyHostGetsARerunHintButNeverAPass(t *testing.T) {
	r := overrunReport()
	r.load = hostLoad{One: 11.5, Five: 9, Fifteen: 8, CPUs: 12, known: true}
	var out bytes.Buffer
	r.print(&out)
	if !strings.Contains(out.String(), "host was busy") {
		t.Fatalf("a loaded host should be called out:\n%s", out.String())
	}
	if len(r.violations) != 1 {
		t.Fatal("the load hint must not change the verdict")
	}

	idle := overrunReport()
	idle.load = hostLoad{One: 0.4, Five: 0.4, Fifteen: 0.4, CPUs: 12, known: true}
	out.Reset()
	idle.print(&out)
	if strings.Contains(out.String(), "host was busy") {
		t.Fatalf("an idle host must not be blamed:\n%s", out.String())
	}
}

func TestACachedPackageIsReportedAsAReplayNotAMeasurement(t *testing.T) {
	var cached = []string{"govard/tests", "govard/internal/deploy"}
	r := result{testCount: 2, cachedPackages: cached}
	var out bytes.Buffer
	r.print(&out)
	for _, want := range []string{"replayed from the go test cache", "not a fresh measurement", "2 package(s)", "-count=1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("replay notice missing %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	(result{testCount: 2}).print(&out)
	if strings.Contains(out.String(), "replayed") {
		t.Fatalf("a fresh run must not claim a replay:\n%s", out.String())
	}
}

func TestCachedPackagesAreDetectedFromThePackageResultLine(t *testing.T) {
	if pkg, ok := cachedPackage(event{Action: "output", Package: "govard/tests", Output: "ok  \tgovard/tests\t(cached)\n"}); !ok || pkg != "govard/tests" {
		t.Fatalf("cachedPackage = (%q, %v), want govard/tests", pkg, ok)
	}
	if _, ok := cachedPackage(event{Action: "output", Package: "govard/internal/deploy", Output: "ok  \tgovard/internal/deploy\t1.2s\n"}); ok {
		t.Fatal("a freshly measured package was reported as cached")
	}
	if _, ok := cachedPackage(event{Action: "output", Package: "p", Test: "TestX", Output: "(cached) in a test log\n"}); ok {
		t.Fatal("a test's own output must not be mistaken for the package result")
	}
}
