import test from "node:test";
import assert from "node:assert/strict";

import {
  createHealthController,
  deriveAuditBanner,
  deriveDoctorBanner,
  selectAuditEntry,
  selectDoctorEntry,
} from "../../desktop/frontend/modules/health.js";

// Binding ruling: async completions write data under their captured key ONLY
// and NEVER move the scope pointer; ONLY an explicit user project-switch
// moves scope. A late completion from an old scope must not yank the scope
// pointer back during rapid A-B-A project switches.

const failingDoctorReport = (opId) => ({
  checks: [
    { id: "host.govard.home", title: "Govard home", status: "fail", message: "home not ready", fixKind: "safe" },
  ],
  passed: 0,
  warnings: 0,
  failures: 1,
  opId,
});

const auditResult = (runId) => ({
  projectId: `project-${runId}`,
  sessionId: "sess-1",
  runId,
  status: "failed",
  findingCount: 1,
  checks: [{ id: "integrity", title: "Integrity", enabled: true }],
  findings: [{ tool: "composer", rule: "hash", path: "composer.json", line: 0, message: "stale" }],
  opId: `audit:${runId}`,
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

const noop = () => {};

// Doctor leg: A runs slow, the user switches A->B, then A's completion lands
// late. Scope must stay where the user put it (B), A's data must land under
// A's key, and B's banner must never show A's data.
test("late doctor completion from an old scope never moves the scope pointer", async () => {
  let resolveA;
  const gateA = new Promise((resolve) => {
    resolveA = resolve;
  });
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      runDoctor: () => gateA,
      lastAuditResult: async () => {
        throw new Error("no completed integrity audit yet for this project");
      },
      runningAuditOp: async () => "",
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  await controller.selectHealthProject("project-a");
  const runA = controller.runDoctor("project-a");
  await controller.selectHealthProject("project-b");
  resolveA(failingDoctorReport("doctor:a1"));
  await runA;

  assert.equal(
    store.state.healthDoctor.scope,
    "project-b",
    "late A completion yanked scope back to project-a, want it where the user put it (project-b)",
  );
  assert.equal(
    store.state.healthDoctor.byProject["project-a"]?.report?.opId,
    "doctor:a1",
    "A's late report was not stored under A's key",
  );
  assert.equal(
    deriveDoctorBanner(selectDoctorEntry(store.state).report),
    null,
    "project-b's banner shows project-a's data, want none",
  );
});

// Audit leg: rapid A->B->A switches while B's banner refresh lands late.
// Scope must stay where the user put it (A), B's data must land under B's
// key, and B's bucket must never carry A's result.
test("rapid A-B-A switches keep scope where the user put it when B lands late", async () => {
  let resolveB;
  const gateB = new Promise((resolve) => {
    resolveB = resolve;
  });
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      lastAuditResult: async (project) => {
        if (project === "project-b") {
          return gateB;
        }
        return auditResult("10");
      },
      runningAuditOp: async () => "",
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  await controller.selectHealthProject("project-a");
  const toB = controller.selectHealthProject("project-b");
  const backToA = controller.selectHealthProject("project-a");
  resolveB(auditResult("20"));
  await Promise.all([toB, backToA]);

  assert.equal(
    store.state.healthAudit.scope,
    "project-a",
    "late B refresh yanked scope back to project-b, want it where the user put it (project-a)",
  );
  assert.equal(
    store.state.healthAudit.byProject["project-b"]?.result?.runId,
    "20",
    "B's late result was not stored under B's key",
  );
  assert.equal(
    store.state.healthAudit.byProject["project-a"]?.result?.runId,
    "10",
    "A's bucket lost its own result",
  );
  assert.ok(
    deriveAuditBanner(store.state.healthAudit.byProject["project-b"]?.result),
    "B's bucket holds no banner of its own",
  );
  assert.notEqual(
    store.state.healthAudit.byProject["project-b"]?.result?.runId,
    store.state.healthAudit.byProject["project-a"]?.result?.runId,
    "B's bucket shows A's data",
  );
});
