import test from "node:test";
import assert from "node:assert/strict";
import { materializeResponse } from "../../../desktop/frontend/preview/response-materializer.js";

test("a model route goes through the real generated model and fills defaults", async () => {
  const metrics = await materializeResponse("SystemService", "GetSystemMetrics", {
    cpuUsage: 12.5,
  });
  assert.equal(metrics.constructor.name, "SystemMetrics");
  assert.equal(metrics.cpuUsage, 12.5);
  assert.equal(metrics.memoryUsage, 0, "an omitted field takes the model default");
});

test("a model route with no fixture at all still returns a model instance", async () => {
  const metrics = await materializeResponse("SystemService", "GetSystemMetrics");
  assert.equal(metrics.constructor.name, "SystemMetrics");
  assert.equal(metrics.cpuUsage, 0);
});

test("a value route returns its classified default, and the raw result when there is one", async () => {
  assert.equal(await materializeResponse("LogService", "GetLogsForService"), "");
  assert.equal(await materializeResponse("LogService", "GetLogsForService", "hello"), "hello");
});

test("a void route returns undefined", async () => {
  assert.equal(await materializeResponse("SystemService", "Quit"), undefined);
  assert.equal(await materializeResponse("SystemService", "Quit", "ignored"), undefined);
});

test("a slice route returns an array, never undefined", async () => {
  assert.deepEqual(await materializeResponse("EnvironmentService", "ListFrameworks"), []);
  assert.deepEqual(
    await materializeResponse("EnvironmentService", "ListFrameworks", ["a"]),
    ["a"],
  );
});

test("an unrouted service.method throws with the regeneration hint", async () => {
  await assert.rejects(
    () => materializeResponse("NoSuchService", "NoSuchMethod"),
    /no route default for NoSuchService\.NoSuchMethod[\s\S]*regenerate route-defaults\.generated\.js/,
  );
});
