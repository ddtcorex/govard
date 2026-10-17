import test from "node:test";
import assert from "node:assert/strict";

import {
  createHealthController,
  deriveAuditBanner,
  deriveDoctorBanner,
  normalizeHealthScope,
  pendingPreviewForTest,
  selectAuditEntry,
  selectDoctorEntry,
  selectHealthProjectForTest,
  selectPreviewEntry,
} from "../../desktop/frontend/modules/health.js";

// Review fixtures: two projects with different health.
const failingReport = () => ({
  checks: [
    { id: "host.govard.home", title: "Govard home", status: "fail", message: "home not ready", fixKind: "safe", fixLabel: "Fix", needsPreview: false },
    { id: "project.config.drift", title: "Config drift", status: "warn", message: "drifted", fixKind: "risky", fixLabel: "Preview fix", needsPreview: true },
  ],
  passed: 1,
  warnings: 1,
  failures: 1,
  opId: "doctor:1",
});

const cleanReport = () => ({
  checks: [{ id: "host.govard.home", title: "Govard home", status: "pass", message: "ok", fixKind: "safe" }],
  passed: 1,
  warnings: 0,
  failures: 0,
  opId: "doctor:2",
});

const failedAuditResult = () => ({
  projectId: "project-a",
  sessionId: "sess-1",
  runId: "2",
  status: "failed",
  findingCount: 1,
  checks: [{ id: "integrity", title: "Integrity", enabled: true }],
  findings: [{ tool: "composer", rule: "hash", path: "composer.json", line: 0, message: "stale" }],
  opId: "audit:1",
});

const fakeStore = () => {
  const state = {
    healthDoctor: { byProject: {}, scope: "" },
    healthAudit: { byProject: {}, scope: "" },
    healthPreview: { byProject: {}, scope: "" },
  };
  return {
    state,
    getState: () => state,
    setState: (patch) => Object.assign(state, patch),
  };
};

const noop = () => {};

// Review fix 2: banners are keyed per project. Switching the selected
// project re-derives that project's banner and never shows another
// project's banner meanwhile.
test("switching project drops the previous banner and derives the new one", async () => {
  const store = fakeStore();
  const controller = createHealthController({
    bridge: { runDoctor: async () => failingReport() },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  await controller.runDoctor("project-a");
  assert.ok(
    deriveDoctorBanner(selectDoctorEntry(store.state).report),
    "project-a run shows no banner",
  );

  // Switch before project-b ever ran: loading/empty, never project-a's red.
  await controller.selectHealthProject("project-b");
  const during = selectDoctorEntry(store.state);
  assert.equal(
    deriveDoctorBanner(during.report),
    null,
    "project-b shows project-a's banner after switch, want none",
  );

  const clean = createHealthController({
    bridge: { runDoctor: async () => cleanReport() },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });
  await clean.runDoctor("project-b");
  assert.equal(deriveDoctorBanner(selectDoctorEntry(store.state).report), null);

  // Switching back restores project-a's own banner from its stored report.
  await controller.selectHealthProject("project-a");
  const back = deriveDoctorBanner(selectDoctorEntry(store.state).report);
  assert.ok(back, "switching back to project-a lost its banner");
  assert.equal(back.total, 2);
});

// Review fix 2 (audit half): the audit banner follows the same per-project
// rule through the persisted last-result refresh.
test("audit banner follows the selected project through refresh", async () => {
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      lastAuditResult: async (project) => {
        if (project === "project-a") return failedAuditResult();
        throw new Error("no completed integrity audit yet for this project");
      },
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  await controller.selectHealthProject("project-a");
  assert.ok(
    deriveAuditBanner(selectAuditEntry(store.state).result),
    "project-a refresh shows no banner",
  );

  await controller.selectHealthProject("project-b");
  assert.equal(
    deriveAuditBanner(selectAuditEntry(store.state).result),
    null,
    "project-b shows project-a's audit banner after switch, want none",
  );
});

// Review fix 2: scope keys are normalized so spellings of one project share
// one banner bucket.
test("health scopes normalize trivial spelling differences", () => {
  assert.equal(normalizeHealthScope("  project-a/ "), normalizeHealthScope("project-a"));
});

// Review fix 3: mounting during a running audit recovers the in-flight op id
// via the backend query, so Cancel is visible and targets the right op.
test("mount during a running audit recovers Cancel for the right op", async () => {
  const calls = [];
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      runningAuditOp: async (project) => {
        calls.push(["runningAuditOp", project]);
        return "audit:7";
      },
      cancelAudit: async (opID) => {
        calls.push(["cancelAudit", opID]);
        return true;
      },
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  const recovered = await controller.recoverAuditOp("project-a");
  assert.equal(recovered, "audit:7");
  assert.deepEqual(calls[0], ["runningAuditOp", "project-a"]);
  const entry = selectAuditEntry(store.state);
  assert.equal(entry.opId, "audit:7");
  assert.equal(entry.phase, "running");

  // Cancel targets the recovered op, not an empty id.
  assert.equal(await controller.cancelAudit(), true);
  assert.deepEqual(calls[calls.length - 1], ["cancelAudit", "audit:7"]);
});

// Review fix 4: preview tokens live in module-local ephemeral state, never
// in the shared store. A snapshot of the store must hold no token-shaped
// value after a preview.
test("preview tokens never land in the shared store", async () => {
  const risky = failingReport().checks[1];
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      previewDoctorFix: async () => ({
        previewId: "preview-token-abc123",
        checkId: risky.id,
        summary: "plan",
        actions: ["do it"],
        requiresConfirm: true,
      }),
      applyDoctorFix: async () => ({ message: "done" }),
      runDoctor: async () => cleanReport(),
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  await controller.previewFix("project-a", risky);
  assert.equal(selectPreviewEntry(store.state).open, true);
  const snapshot = JSON.stringify(store.state);
  assert.ok(
    !snapshot.includes("preview-token-abc123"),
    "shared store snapshot holds the preview token, want it module-local only",
  );
  assert.ok(
    !snapshot.includes("previewId"),
    "shared store snapshot carries a previewId field, want none",
  );

  // The pending token is still reachable ephemerally for the confirm path.
  assert.equal(pendingPreviewForTest(controller)?.previewId, "preview-token-abc123");
});

// Review fix 5: applySafeFix refuses anything but a safe 1-click fix
// client-side. A risky check through it never reaches the bridge (the Go
// refusal stays as the backstop, this is the client gate).
test("risky and none checks through applySafeFix never reach the bridge", async () => {
  const calls = [];
  const toasts = [];
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      applyDoctorFix: async (...args) => {
        calls.push(args);
        return { message: "done" };
      },
      runDoctor: async () => cleanReport(),
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: (msg) => toasts.push(String(msg)),
  });

  const risky = failingReport().checks[1];
  assert.equal(await controller.applySafeFix("project-a", risky), null);
  const none = { id: "host.port.80", fixKind: "none" };
  assert.equal(await controller.applySafeFix("project-a", none), null);
  assert.deepEqual(calls, [], "non-safe check through applySafeFix reached the bridge");
  assert.ok(toasts.length >= 2, "refused safe-fix applies toast no guidance");

  // The safe path still applies.
  const safe = failingReport().checks[0];
  await controller.applySafeFix("project-a", safe);
  assert.equal(calls.length, 1);
});

// The selectHealthProject test seam: switching through the helper the island
// uses keeps the same per-project rule outside the controller.
test("selectHealthProject helper scopes banners without a controller", () => {
  const store = fakeStore();
  store.state.healthDoctor.byProject["project-a"] = {
    report: failingReport(),
    loading: false,
    error: "",
  };
  selectHealthProjectForTest(store.setState, store.getState, "project-a");
  assert.ok(deriveDoctorBanner(selectDoctorEntry(store.state).report));
  selectHealthProjectForTest(store.setState, store.getState, "project-b");
  assert.equal(deriveDoctorBanner(selectDoctorEntry(store.state).report), null);
});

// I1: the doctor run reaches the bridge scoped to the selected project, so
// the backend diagnoses that project instead of the app CWD.
test("runDoctor passes the selected project to the bridge", async () => {
  const calls = [];
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      runDoctor: async (project) => {
        calls.push(project);
        return cleanReport();
      },
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  await controller.runDoctor("project-a");
  assert.deepEqual(calls, ["project-a"]);
});

// M3: mounting during a running doctor run recovers the in-flight op id via
// the backend query, so Cancel is visible and targets the right op.
test("mount during a running doctor run recovers Cancel for the right op", async () => {
  const calls = [];
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      runningDoctorOp: async (project) => {
        calls.push(["runningDoctorOp", project]);
        return "doctor:9";
      },
      cancelDoctorRun: async (opID) => {
        calls.push(["cancelDoctorRun", opID]);
        return true;
      },
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  assert.equal(
    typeof controller.recoverDoctorOp,
    "function",
    "controller has no recoverDoctorOp, want the audit-style recovery",
  );
  const recovered = await controller.recoverDoctorOp("project-a");
  assert.equal(recovered, "doctor:9");
  assert.deepEqual(calls[0], ["runningDoctorOp", "project-a"]);
  const entry = selectDoctorEntry(store.state);
  assert.equal(entry.opId, "doctor:9");
  assert.equal(entry.loading, true);

  // Cancel targets the recovered op, not an empty id.
  assert.equal(await controller.cancelDoctor(), true);
  assert.deepEqual(calls[calls.length - 1], ["cancelDoctorRun", "doctor:9"]);
});

// M6: switching project with a pending preview leaves the old preview
// unusable. The island project-switch path (standalone seam) moves the scope
// without dropping module-local state, so the confirm path must refuse a
// preview minted for another scope instead of applying it there.
test("switching project with a pending preview leaves the old preview unusable", async () => {
  const risky = failingReport().checks[1];
  const calls = [];
  const store = fakeStore();
  const controller = createHealthController({
    bridge: {
      previewDoctorFix: async () => ({
        previewId: "preview-token-scope",
        checkId: risky.id,
        summary: "plan",
        actions: ["do it"],
        requiresConfirm: true,
      }),
      applyDoctorFix: async (...args) => {
        calls.push(args);
        return { message: "done" };
      },
      runDoctor: async () => cleanReport(),
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  await controller.previewFix("project-a", risky);
  const pending = pendingPreviewForTest(controller);
  assert.equal(pending.previewId, "preview-token-scope");

  selectHealthProjectForTest(store.setState, store.getState, "project-b");
  const res = await controller.applyFix(
    "project-b",
    { id: risky.id, fixKind: "risky" },
    pending.previewId,
    true,
  );
  assert.equal(res, null, "stale preview applied under a new scope, want a refusal");
  assert.deepEqual(calls, [], "stale preview reached the bridge under a new scope");
});
