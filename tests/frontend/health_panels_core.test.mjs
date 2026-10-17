import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  auditDisabledRows,
  createHealthController,
  deriveAuditBanner,
  deriveDoctorBanner,
  doctorFixControl,
  normalizeAuditResult,
  normalizeDoctorCheck,
  normalizeDoctorReport,
  pendingPreviewForTest,
  selectAuditEntry,
  selectDoctorEntry,
} from "../../desktop/frontend/modules/health.js";

const islandSource = (name) =>
  readFile(
    new URL(`../../desktop/frontend/islands/${name}`, import.meta.url),
    "utf8",
  );

const failingReport = () => ({
  checks: [
    { id: "host.govard.home", title: "Govard home", status: "fail", message: "home not ready", fixKind: "safe", fixLabel: "Fix", needsPreview: false },
    { id: "project.config.drift", title: "Config drift", status: "warn", message: "drifted", fixKind: "risky", fixLabel: "Preview fix", needsPreview: true },
    { id: "host.port.80", title: "Host port 80", status: "fail", message: "port in use", fixKind: "none" },
    { id: "docker.daemon", title: "Docker daemon", status: "pass", message: "running", fixKind: "none" },
  ],
  passed: 1,
  warnings: 1,
  failures: 2,
  opId: "doctor:1",
});

const failedAuditResult = () => ({
  projectId: "project-test",
  sessionId: "sess-1",
  runId: "2",
  status: "failed",
  findingCount: 1,
  checks: [
    { id: "integrity", title: "Integrity (container-free)", enabled: true },
    { id: "lint", title: "Lint", enabled: false, disabledReason: "Requires Docker and is not run from the desktop" },
    { id: "profiler", title: "Profiler", enabled: false, disabledReason: "Requires Docker and is not run from the desktop" },
  ],
  findings: [{ tool: "composer", rule: "hash", path: "composer.json", line: 0, message: "stale hash" }],
  opId: "audit:1",
});

const fakeStore = () => {
  const state = {
    healthDoctor: { byProject: {}, scope: "" },
    healthAudit: { byProject: {}, scope: "" },
    healthPreview: { open: false, checkId: "", summary: "", actions: [] },
  };
  return {
    state,
    getState: () => state,
    setState: (patch) => Object.assign(state, patch),
  };
};

// A red banner offers a fix action: the banner derives from the latest
// report, names the actionable checks, and the safe one carries a 1-click
// Fix control.
test("red banner offers a fix action for a failing safe check", () => {
  const banner = deriveDoctorBanner(failingReport());
  assert.ok(banner, "failing report derives no banner, want an error banner");
  assert.equal(banner.tone, "error");
  assert.equal(banner.total, 3);
  const safe = banner.actionable.find((check) => check.id === "host.govard.home");
  assert.ok(safe, "safe failing check missing from banner actions");
  assert.deepEqual(doctorFixControl(safe), { kind: "fix", label: "Fix" });
});

// A risky fix always opens a preview first: the control is preview-gated,
// the controller previews without applying, and an apply without a matching
// preview never reaches the bridge.
test("risky fix opens a preview before it can apply", async () => {
  const risky = failingReport().checks[1];
  assert.deepEqual(doctorFixControl(risky), { kind: "preview", label: "Preview fix" });

  const calls = [];
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      previewDoctorFix: async (...args) => {
        calls.push(["preview", ...args]);
        return { previewId: "token-1", checkId: risky.id, summary: "plan", actions: ["do it"], requiresConfirm: true };
      },
      applyDoctorFix: async (...args) => {
        calls.push(["apply", ...args]);
        return { checkId: risky.id, status: "applied", message: "done", actions: [] };
      },
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: () => {},
    onToast: () => {},
  });

  await controller.previewFix("sample-project", risky);
  assert.deepEqual(calls, [["preview", "sample-project", risky.id]]);
  assert.equal(store.state.healthPreview.open, true);
  // The token lives in module-local controller state, never the store.
  assert.equal(pendingPreviewForTest(controller)?.previewId, "token-1");
  assert.ok(!JSON.stringify(store.state).includes("token-1"));

  // No preview for this check: the bridge must not be reached.
  calls.length = 0;
  const fresh = createHealthController({
    bridge: {
      applyDoctorFix: async () => {
        calls.push("apply");
        return {};
      },
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: () => {},
    onToast: () => {},
  });
  await fresh.applyFix("sample-project", risky, "", false);
  assert.deepEqual(calls, [], "apply without a preview reached the bridge, want a refusal first");

  // The matching preview plus an explicit confirm applies.
  await controller.applyFix("sample-project", risky, "token-1", true);
  assert.deepEqual(calls[calls.length - 1], ["apply", "sample-project", risky.id, "token-1", true]);
});

// A stale banner clears on the next clean run: banner state derives from the
// latest results, never from cached state.
test("stale banner clears after a clean re-run", async () => {
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      runDoctor: async () => failingReport(),
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: () => {},
    onToast: () => {},
  });
  await controller.runDoctor("sample-project");
  assert.ok(deriveDoctorBanner(selectDoctorEntry(store.state).report), "red run shows no banner");

  const clean = createHealthController({
    bridge: {
      runDoctor: async () => ({ checks: [], passed: 3, warnings: 0, failures: 0, opId: "doctor:2" }),
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: () => {},
    onToast: () => {},
  });
  await clean.runDoctor("sample-project");
  assert.equal(deriveDoctorBanner(selectDoctorEntry(store.state).report), null);
});

// Docker-backed audit rows render disabled with the reason, never hidden;
// a failed result banners, a clean passed result clears it.
test("docker-backed audit rows stay visible but disabled with a reason", () => {
  const banner = deriveAuditBanner(failedAuditResult());
  assert.ok(banner, "failed audit derives no banner, want one");
  assert.equal(banner.findingCount, 1);

  const disabled = auditDisabledRows(failedAuditResult());
  assert.deepEqual(
    disabled.map((row) => row.id).sort(),
    ["lint", "profiler"],
  );
  for (const row of disabled) {
    assert.equal(row.enabled, false);
    assert.ok(String(row.disabledReason || "").length > 0, `${row.id} has no disabled reason`);
  }

  const clean = { ...failedAuditResult(), status: "passed", findingCount: 0, findings: [] };
  assert.equal(deriveAuditBanner(clean), null);
});

// A check with fixKind none shows no fix control at all.
test("fixKind none exposes no fix control", () => {
  assert.deepEqual(doctorFixControl({ id: "host.port.80", fixKind: "none" }), { kind: "none", label: "" });
  assert.deepEqual(doctorFixControl({ id: "unknown" }), { kind: "none", label: "" });
  // The mapping is not a constant: known kinds resolve to their controls.
  assert.deepEqual(doctorFixControl({ id: "host.govard.home", fixKind: "safe" }), { kind: "fix", label: "Fix" });
});

// All UI-bound strings go through the sanitizer: ANSI escapes and control
// bytes are stripped and over-long text is truncated with a marker.
test("health strings are sanitized before render", () => {
  const check = normalizeDoctorCheck({
    id: "host.govard.home",
    title: "Govard \x1b[31mhome\x00",
    status: "fail",
    message: "x".repeat(600),
    fixKind: "safe",
  });
  assert.ok(!check.title.includes("\x1b"), "ANSI escape survived sanitizing");
  assert.ok(!check.title.includes("\x00"), "control byte survived sanitizing");
  assert.ok(check.message.endsWith("(truncated)"), "over-long message not truncated with a marker");

  const report = normalizeDoctorReport(failingReport());
  assert.equal(report.checks.length, 4);
  const result = normalizeAuditResult(failedAuditResult());
  assert.equal(result.findings.length, 1);
});

// Secrets never reach the DOM: the panels render no preview token and use no
// raw HTML injection.
test("health panels keep preview tokens out of the DOM", async () => {
  const source = await islandSource("HealthPanels.tsx");
  assert.ok(!source.includes("dangerouslySetInnerHTML"), "panels inject raw HTML");
  assert.ok(!source.includes("{preview.previewId}"), "panels render the preview token");
  assert.ok(!source.includes("{healthPreview.previewId}"), "panels render the preview token");
});
