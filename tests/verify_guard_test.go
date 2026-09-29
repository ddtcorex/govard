package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/verify"
)

// guardProbe captures every argv a phase run hands to the exec seam, which is
// what "the item did not run" means for a policy skip.
type guardProbe struct {
	argv [][]string
}

func installGuardProbe(t *testing.T) *guardProbe {
	t.Helper()
	probe := &guardProbe{}
	verify.SetExecGovardFakeForTest(func(_ context.Context, _ engine.Config, _ verify.VerifyOpts, args ...string) (verify.Evidence, bool) {
		probe.argv = append(probe.argv, append([]string(nil), args...))
		return verify.Evidence{ExitCode: 0, OutputExcerpt: "fake: " + strings.Join(args, " ")}, true
	})
	t.Cleanup(func() { verify.SetExecGovardFakeForTest(nil) })
	return probe
}

func (p *guardProbe) called() int { return len(p.argv) }

// sawNonPlanBootstrap reports the first `bootstrap` argv that is not a --plan
// run. The read-only remote items legitimately run `bootstrap ... --plan`; the
// two REMOTE-WRITE items are the ones that would write through the remote.
func (p *guardProbe) sawNonPlanBootstrap() (string, bool) {
	for _, args := range p.argv {
		if len(args) == 0 || args[0] != "bootstrap" {
			continue
		}
		planned := false
		for _, a := range args {
			if a == "--plan" {
				planned = true
			}
		}
		if !planned {
			return strings.Join(args, " "), true
		}
	}
	return "", false
}

// sawArgv reports whether some captured argv starts with the given words.
func (p *guardProbe) sawArgv(words ...string) (string, bool) {
	for _, args := range p.argv {
		if len(args) < len(words) {
			continue
		}
		match := true
		for i, w := range words {
			if args[i] != w {
				match = false
			}
		}
		if match {
			return strings.Join(args, " "), true
		}
	}
	return "", false
}

// copyRegistryForTest swaps in a mutable copy of the shared registry and
// restores the original on cleanup, so a relabel cannot leak into the next test.
func copyRegistryForTest(t *testing.T) []verify.Item {
	t.Helper()
	orig := verify.Registry
	copied := make([]verify.Item, len(orig))
	copy(copied, orig)
	verify.Registry = copied
	t.Cleanup(func() { verify.Registry = orig })
	return copied
}

// guardIndexForTest returns the index of id, failing when the fixture moved.
func guardIndexForTest(t *testing.T, items []verify.Item, id string) int {
	t.Helper()
	for i := range items {
		if items[i].ID == id {
			return i
		}
	}
	t.Fatalf("registry has no %s; the fixture this test relabels moved", id)
	return -1
}

// findGuardRow returns an item's single row. More than one row is a failure in
// its own right: the policy must mark the row, not add a second one beside the
// plan stub.
func findGuardRow(t *testing.T, res verify.RunResult, id string) verify.RunItem {
	t.Helper()
	rows := 0
	var row verify.RunItem
	for _, it := range res.Items {
		if it.ID != id {
			continue
		}
		rows++
		row = it
	}
	if rows != 1 {
		t.Fatalf("%s has %d rows in the %s report, want exactly 1", id, rows, res.Phase)
	}
	return row
}

// The two items that write through a remote carried no label at all, so a run
// without an opt-in pulled a live bootstrap. They now carry REMOTE-WRITE and the
// runner refuses them unless --allow-remote-write was passed.
func TestRemoteWriteItemIsSkippedWithoutOptIn(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	probe := installGuardProbe(t)

	// P2-05 and P2-08 are both phase-2 items (P2-08's Precond names a phase-4
	// snapshot; that is not where it runs), so phase 2 is the run that can show
	// the policy acting on them.
	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 2,
		verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase phase 2: %v", err)
	}

	if argv, ran := probe.sawNonPlanBootstrap(); ran {
		t.Fatalf("a REMOTE-WRITE item ran `%s` without --allow-remote-write", argv)
	}
	for _, id := range []string{"P2-05", "P2-08"} {
		row := findGuardRow(t, res, id)
		if !row.Skipped {
			t.Fatalf("%s Skipped = false: it writes through a remote and this run carried no opt-in", id)
		}
		if !strings.Contains(row.SkipReason, "--allow-remote-write") {
			t.Fatalf("%s skip reason %q does not name --allow-remote-write", id, row.SkipReason)
		}
	}
}

// Plan mode replaces every Run with a stub, so a policy decided after that
// branch would report the item as "would run" — and a policy decided outside the
// filter loop would report it twice. The row must be a single skip.
func TestRemoteWriteItemIsSkippedInPlanModeTooAndRunIsNeverCalled(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	probe := installGuardProbe(t)

	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 2,
		verify.VerifyOpts{Plan: true, ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase plan phase 2: %v", err)
	}

	for _, id := range []string{"P2-05", "P2-08"} {
		row := findGuardRow(t, res, id)
		if !row.Skipped {
			t.Fatalf("%s Skipped = false with --plan: the policy must decide before the plan stub", id)
		}
		if strings.HasPrefix(row.EvidenceExcerpt, "plan: ") {
			t.Fatalf("%s was rendered as a plan stub, not a skip: %q", id, row.EvidenceExcerpt)
		}
		if !strings.Contains(row.SkipReason, "--allow-remote-write") {
			t.Fatalf("%s skip reason %q does not name --allow-remote-write", id, row.SkipReason)
		}
	}
	if got := probe.called(); got != 0 {
		t.Fatalf("plan mode called the exec seam %d time(s), want 0", got)
	}
}

// DESTRUCTIVE-LOCAL is enforced by the label, not by a hard-coded phase-5 list:
// the same item relabelled stops running in phase 4.
func TestDestructiveLocalItemIsSkippedOutsidePhase5(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	registry := copyRegistryForTest(t)
	registry[guardIndexForTest(t, registry, "P4-02")].Guard = verify.GuardDestructiveLocal

	probe := installGuardProbe(t)
	res, err := verify.RunPhase(context.Background(), engine.Config{Framework: "magento2"}, 4,
		verify.VerifyOpts{ProjectRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("RunPhase phase 4: %v", err)
	}

	row := findGuardRow(t, res, "P4-02")
	if !row.Skipped {
		t.Fatalf("P4-02 labelled DESTRUCTIVE-LOCAL ran in phase 4: %+v", row)
	}
	if argv, ran := probe.sawArgv("remote", "audit"); ran {
		t.Fatalf("the DESTRUCTIVE-LOCAL item ran `%s` in phase 4", argv)
	}
}

// A skip must not stand in for the destructive gate: even with every phase-5
// row policy-skippable, the run still refuses to start without the opt-in.
func TestPhase5GateStillFiresWhenEveryItemWouldSkip(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")
	writePhase4(t, root, goodPhase4(root, "20260101-000000"))

	registry := copyRegistryForTest(t)
	for i := range registry {
		if registry[i].Phase == 5 {
			registry[i].Guard = verify.GuardRemoteWrite
		}
	}

	// No framework, so the composed registry is exactly the static one: every
	// phase-5 row is now policy-skippable.
	_, err := verify.RunPhase(context.Background(), engine.Config{}, 5,
		verify.VerifyOpts{ProjectRoot: root})
	if !errors.Is(err, verify.ErrNeedAllowDestructive) {
		t.Fatalf("RunPhase phase 5 = %v, want ErrNeedAllowDestructive: a skip must not mask the gate", err)
	}
}

// The label is a rule only if the whole registry obeys it, and the distribution
// is the shape the design fixed: 42 unlabelled, 13 read-only remote,
// 3 destructive local, 2 remote write.
func TestGuardValuesAreFromTheAllowedSet(t *testing.T) {
	allowed := map[string]bool{
		"":                           true,
		verify.GuardReadOnlyRemote:   true,
		verify.GuardDestructiveLocal: true,
		verify.GuardRemoteWrite:      true,
	}
	for _, it := range verify.RegistryFor(engine.Config{Framework: "magento2"}) {
		if !allowed[it.Guard] {
			t.Fatalf("item %q has Guard %q, want \"\", %q, %q or %q",
				it.ID, it.Guard, verify.GuardReadOnlyRemote, verify.GuardDestructiveLocal, verify.GuardRemoteWrite)
		}
	}

	// The distribution covers the 60 static rows: the framework-composed items
	// RegistryFor appends carry the empty guard, because they run
	// `govard tool <binary> ...` locally.
	counts := map[string]int{}
	for _, it := range verify.Registry {
		counts[it.Guard]++
	}
	for guard, want := range map[string]int{
		"":                           42,
		verify.GuardReadOnlyRemote:   13,
		verify.GuardDestructiveLocal: 3,
		verify.GuardRemoteWrite:      2,
	} {
		if counts[guard] != want {
			t.Fatalf("registry has %d %q items, want %d (distribution %v)", counts[guard], guard, want, counts)
		}
	}
}

// DecideGuard is the runner's only policy reader. phase 0 means "every phase",
// so an item is judged by its OWN phase — reading it as "outside phase 5" would
// skip every destructive item in an all-phases run.
func TestDecideGuardTreatsAllPhasesAsTheItemsOwnPhase(t *testing.T) {
	destructive := verify.Item{ID: "P5-01", Phase: 5, Guard: verify.GuardDestructiveLocal}
	destructiveElsewhere := verify.Item{ID: "P4-02", Phase: 4, Guard: verify.GuardDestructiveLocal}

	if d := verify.DecideGuard(destructive, 5, verify.VerifyOpts{}); !d.Run {
		t.Fatalf("phase-5 destructive item in its own phase: Run = false (%s)", d.Reason)
	}
	if d := verify.DecideGuard(destructive, 0, verify.VerifyOpts{}); !d.Run {
		t.Fatalf("phase 0 must read as the item's own phase 5: Run = false (%s)", d.Reason)
	}
	if d := verify.DecideGuard(destructiveElsewhere, 4, verify.VerifyOpts{}); d.Run {
		t.Fatal("a DESTRUCTIVE-LOCAL item ran outside phase 5")
	}
	if d := verify.DecideGuard(destructiveElsewhere, 0, verify.VerifyOpts{}); d.Run {
		t.Fatal("phase 0 ran a destructive item whose own phase is 4")
	}

	if d := verify.DecideGuard(verify.Item{ID: "P4-13", Phase: 4, Guard: verify.GuardReadOnlyRemote}, 4, verify.VerifyOpts{}); !d.Run {
		t.Fatalf("READ-ONLY-REMOTE has no runtime gate, got a skip: %s", d.Reason)
	}

	remoteWrite := verify.Item{ID: "P2-05", Phase: 2, Title: "govard bootstrap -e {{REMOTE}} --no-noise", Guard: verify.GuardRemoteWrite}
	skipped := verify.DecideGuard(remoteWrite, 2, verify.VerifyOpts{})
	if skipped.Run {
		t.Fatal("REMOTE-WRITE ran without --allow-remote-write")
	}
	if !strings.Contains(skipped.Reason, "--allow-remote-write") || !strings.Contains(skipped.Reason, "bootstrap") {
		t.Fatalf("skip reason %q must name both the opt-in flag and the manual command", skipped.Reason)
	}
	if d := verify.DecideGuard(remoteWrite, 2, verify.VerifyOpts{AllowRemoteWrite: true}); !d.Run {
		t.Fatalf("REMOTE-WRITE must run with --allow-remote-write, got a skip: %s", d.Reason)
	}
}

// The policy is only reachable if the flag is registered AND read into
// VerifyOpts; both halves are pinned here, without the root command's capability
// gate (which would make the test depend on the host having Docker). The run
// names a remote too: --allow-remote-write opts into writing through one, it
// does not supply the target (see TestRemoteItemsSkipWithoutAnExplicitRemote).
func TestAllowRemoteWriteFlagReachesTheRunner(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	probe := installGuardProbe(t)

	command := cmd.VerifyCommandForTest()
	if command.Flags().Lookup("allow-remote-write") == nil {
		t.Fatal("the verify command has no --allow-remote-write flag")
	}
	for name, value := range map[string]string{
		"allow-remote-write": "true",
		"remote":             "sandbox",
		"phase":              "2",
		"json":               "true",
		"project":            t.TempDir(),
	} {
		if err := command.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%s: %v", name, value, err)
		}
	}
	t.Cleanup(func() {
		// These flags and writers live on the shared command object.
		command.SetOut(nil)
		command.SetErr(nil)
		for name, value := range map[string]string{
			"allow-remote-write": "false",
			"remote":             "",
			"phase":              "0",
			"json":               "false",
			"project":            "",
		} {
			_ = command.Flags().Set(name, value)
			if f := command.Flags().Lookup(name); f != nil {
				f.Changed = false
			}
		}
	})

	stdout := &bytes.Buffer{}
	command.SetOut(stdout)
	command.SetErr(io.Discard)

	if err := command.RunE(command, nil); err != nil {
		t.Fatalf("verify --phase 2 --allow-remote-write --remote sandbox: %v", err)
	}
	if _, ran := probe.sawNonPlanBootstrap(); !ran {
		t.Fatalf("--allow-remote-write reached the policy but no bootstrap argv ran (captured %v)", probe.argv)
	}

	var payload struct {
		Items []struct {
			ID              string `json:"id"`
			Skipped         bool   `json:"skipped"`
			EvidenceExcerpt string `json:"evidence_excerpt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not a single JSON object: %v\nstdout=%q", err, stdout.String())
	}
	for _, it := range payload.Items {
		if it.ID != "P2-05" {
			continue
		}
		if it.Skipped {
			t.Fatal("P2-05 was skipped with --allow-remote-write: the flag is not reaching VerifyOpts")
		}
		if !strings.Contains(it.EvidenceExcerpt, "bootstrap") {
			t.Fatalf("P2-05 evidence = %q, want the bootstrap argv it ran", it.EvidenceExcerpt)
		}
		return
	}
	t.Fatalf("P2-05 left no row in the phase-2 report (items=%d)", len(payload.Items))
}

// The phase-5 gate is the one thing that authorises the destructive phase, so it
// must not read a skipped P4-08 row as evidence that a snapshot exists.
func TestSkippedPhase4RowCannotSatisfyThePhase5Gate(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	root := t.TempDir()
	makeSnapshot(t, root, "20260101-000000", "2026-01-01T00:00:00Z")

	// The row keeps the artifact name and a zero exit code; only Skipped marks
	// that P4-08 itself never ran.
	res := goodPhase4(root, "20260101-000000")
	res.Items[0].Skipped = true
	res.Items[0].SkipReason = "P4-08 was skipped in this run"
	writePhase4(t, root, res)

	if name, ok := verify.GateSatisfyingSnapshot(verify.VerifyOpts{ProjectRoot: root}); ok {
		t.Fatalf("the phase-5 gate opened on a skipped P4-08 row (%q): a snapshot the phase-4 run never created cannot authorise phase 5", name)
	}
}
