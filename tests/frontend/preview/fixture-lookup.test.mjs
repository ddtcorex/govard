import test from "node:test";
import assert from "node:assert/strict";
import { resolveFixtureResponse } from "../../../desktop/frontend/preview/fixture-lookup.js";

const fixtures = [
  {
    service: "RemoteService",
    method: "RunRemoteSync",
    args: ["sample-project", "staging", "quick", {}],
    error: "ssh: connection refused",
  },
  {
    service: "EnvironmentService",
    method: "GetDashboard",
    args: [],
    result: { activeEnvironments: 2 },
  },
];

test("exact args match wins first", () => {
  const found = resolveFixtureResponse(fixtures, "RemoteService", "RunRemoteSync", [
    "sample-project",
    "staging",
    "quick",
    {},
  ]);
  assert.deepEqual(found, { found: true, error: "ssh: connection refused" });
});

test("service+method matches when args differ (a fixture recorded once, called with a different project)", () => {
  const found = resolveFixtureResponse(fixtures, "RemoteService", "RunRemoteSync", [
    "other-project",
    "staging",
    "quick",
    {},
  ]);
  assert.deepEqual(found, { found: true, error: "ssh: connection refused" });
});

test("no match returns found:false", () => {
  const found = resolveFixtureResponse(fixtures, "SystemService", "GetSystemMetrics", []);
  assert.deepEqual(found, { found: false });
});

test("result and error entries both resolve", () => {
  const found = resolveFixtureResponse(fixtures, "EnvironmentService", "GetDashboard", []);
  assert.deepEqual(found, { found: true, result: { activeEnvironments: 2 } });
});
