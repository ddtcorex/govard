import test from "node:test";
import assert from "node:assert/strict";
import { createRecordingLoader } from "../../../desktop/frontend/preview/record.js";

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
    getModule: () => "dashboard",
  });
  const bindings = await loader();
  const result = await bindings.EnvironmentService.GetDashboard();
  assert.deepEqual(result, { activeEnvironments: 2 });
  assert.deepEqual(posted, [
    {
      module: "dashboard",
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
    getModule: () => "remotes",
  });
  const bindings = await loader();
  await assert.rejects(
    bindings.RemoteService.RunRemoteSync("sample-project", "staging", "quick", {}),
    /ssh: connection refused/,
  );
  assert.equal(posted[0].error, "ssh: connection refused");
});
