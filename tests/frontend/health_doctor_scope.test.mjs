import test from "node:test";
import assert from "node:assert/strict";

import {
  createHealthController,
  selectDoctorEntry,
  selectHealthProjectForTest,
} from "../../desktop/frontend/modules/health.js";

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

const cleanReport = () => ({
  checks: [{ id: "host.govard.home", title: "Govard home", status: "pass", message: "ok", fixKind: "safe" }],
  passed: 1,
  warnings: 0,
  failures: 0,
  opId: "doctor:9",
});

// I1: the doctor run is scoped to the selected project, so the controller
// must forward the project to the bridge. A CWD-bound run bucketed under
// the project key misattributes host state to the project.
test("runDoctor forwards the selected project to the bridge", async () => {
  const store = fakeStore();
  const seen = [];
  const controller = createHealthController({
    bridge: {
      runDoctor: async (project) => {
        seen.push(project);
        return cleanReport();
      },
    },
    getState: store.getState,
    setState: store.setState,
    onStatus: noop,
    onToast: noop,
  });

  await controller.runDoctor("project-a");
  assert.equal(seen.length, 1, "bridge.runDoctor was never called");
  assert.equal(
    seen[0],
    "project-a",
    `bridge.runDoctor received ${JSON.stringify(seen[0])}, want the selected project`,
  );
  // The report still lands in the selected project's bucket: scope moves
  // only through an explicit project-switch, so point the slices at
  // project-a first and the completion lands in view.
  selectHealthProjectForTest(store.setState, store.getState, "project-a");
  await controller.runDoctor("project-a");
  assert.ok(
    selectDoctorEntry(store.state).report,
    "scoped run stored no report in its bucket",
  );
});
