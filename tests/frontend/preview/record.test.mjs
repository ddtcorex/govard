import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRecordingLoader } from "../../../desktop/frontend/preview/record.js";
import { fixtureNameFor } from "../../../desktop/frontend/preview/vite-plugin-preview.js";

test("createRecordingLoader calls through to the real loader and posts the captured entry", async () => {
  const posted = [];
  const fakeReal = {
    EnvironmentService: {
      GetDashboard: async () => ({ activeEnvironments: 2 }),
    },
  };
  const loader = createRecordingLoader({
    loadReal: async () => fakeReal,
    post: async (entry) => { posted.push(entry); },
  });
  const bindings = await loader();
  const result = await bindings.EnvironmentService.GetDashboard();
  assert.deepEqual(result, { activeEnvironments: 2 });
  assert.deepEqual(posted, [
    {
      service: "EnvironmentService",
      method: "GetDashboard",
      args: [],
      result: { activeEnvironments: 2 },
    },
  ]);
});

test("a rejected real call is recorded as an error entry and still rejects", async () => {
  const posted = [];
  const loader = createRecordingLoader({
    loadReal: async () => ({
      RemoteService: {
        RunRemoteSync: async () => { throw new Error("ssh: connection refused"); },
      },
    }),
    post: async (entry) => { posted.push(entry); },
  });
  const bindings = await loader();
  await assert.rejects(
    bindings.RemoteService.RunRemoteSync("sample-project", "staging", "quick", {}),
    /ssh: connection refused/,
  );
  assert.equal(posted[0].error, "ssh: connection refused");
});

// The file a recording lands in must be the name the harness will later install,
// not whatever the app happened to be showing. GOVARD_PREVIEW_RECORD_NAME is that
// name (matching installFixtures("metrics")); without it, one file per service is
// the honest default, because a single mixed dump is unusable as a fixture.
test("fixtureNameFor prefers the requested recording name", () => {
  assert.equal(fixtureNameFor({ service: "SystemService" }, "metrics"), "metrics");
});

test("fixtureNameFor falls back to the service, never to the sidebar", () => {
  assert.equal(fixtureNameFor({ service: "SystemService" }, ""), "SystemService");
  assert.equal(fixtureNameFor({ service: "RemoteService" }, undefined), "RemoteService");
});

test("fixtureNameFor has no name for an entry without a service", () => {
  assert.equal(fixtureNameFor({}, "metrics"), "metrics");
  assert.equal(fixtureNameFor({}, ""), "");
});

test("record-bootstrap does not name fixtures after the sidebar", () => {
  const src = readFileSync(
    new URL("../../../desktop/frontend/preview/record-bootstrap.js", import.meta.url),
    "utf8",
  );
  assert.ok(!src.includes("sidebarMode"), "record-bootstrap must not read the sidebar mode");
  assert.ok(!src.includes("getModule"), "record-bootstrap must not name the file; the plugin owns that");
});
