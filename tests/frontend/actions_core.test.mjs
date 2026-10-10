import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

test("environment actions wire loading toast lifecycle", async () => {
  const actionsJS = await readFile(
    new URL("../../desktop/frontend/modules/actions.js", import.meta.url),
    "utf8",
  );

  assert.equal(
    actionsJS.includes("MIN_LOADING_TOAST_MS"),
    true,
    "actions controller should keep loading toast visible briefly",
  );
  assert.equal(
    actionsJS.includes("onToastLoading?.("),
    true,
    "actions controller should invoke loading toast callback",
  );
  assert.equal(
    actionsJS.includes("loadingToast.close(message || fallbackMessage, \"success\")"),
    true,
    "actions controller should close loading toast on success",
  );
  assert.equal(
    actionsJS.includes("loadingToast.close(message, \"error\")"),
    true,
    "actions controller should close loading toast on failure",
  );
  assert.equal(
    actionsJS.includes('if (action === "env-pull") {'),
    true,
    "actions controller should handle env-pull action",
  );
  assert.equal(
    actionsJS.includes("bridge.pullEnvironment"),
    true,
    "actions controller should call desktop pullEnvironment bridge",
  );
});

// actions.js imports ui/modal.js, which looks up its DOM nodes at module load
// (null-safe), so the module is imported with a minimal document stub.
const loadActionsModule = async () => {
  const previousDocument = globalThis.document;
  globalThis.document = { getElementById: () => null };
  try {
    return await import("../../desktop/frontend/modules/actions.js");
  } finally {
    globalThis.document = previousDocument;
  }
};

test("delete confirm escapes the project name", async () => {
  const { buildDeleteConfirmMessage } = await loadActionsModule();
  const html = buildDeleteConfirmMessage('<img src=x onerror="alert(1)">');
  assert.equal(
    html.includes("<img"),
    false,
    "raw markup from a project name must not reach the dialog",
  );
  assert.match(html, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
  assert.match(html, /PERMANENTLY delete project/);
});

// An environment action raises the sidebar's loading frame before it runs, and
// only the dashboard refresh closes it. The success path refreshed; the failure
// path did not, so a failed start/stop/restart left the skeleton up until some
// unrelated refresh happened. The frame is modelled here the way main.js wires
// it: renderSkeletons raises it and refreshDashboard publishes and lowers it.
test("a failed environment action still closes the loading frame it raised", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { createActionsController } = await loadActionsModule();
  let loading = false;
  const refreshes = [];
  const toasts = [];
  const controller = createActionsController({
    bridge: {
      startEnvironment: async () => {
        throw new Error("compose up failed");
      },
    },
    getProject: () => "sample-project",
    refreshDashboard: async (options) => {
      refreshes.push(options);
      loading = false;
    },
    renderSkeletons: () => {
      loading = true;
    },
    onStatus: () => {},
    onToast: (message, tone) => toasts.push({ message, tone }),
  });

  await controller.handle("env-start");

  assert.equal(loading, false, "the loading frame must not outlive a failed action");
  assert.deepEqual(refreshes, [{ silent: true }], "the failure path refreshes once, silently");
  assert.equal(toasts.length, 1);
  assert.equal(toasts[0].tone, "error");
  assert.match(toasts[0].message, /compose up failed/);
});

test("action backstop defers to the backend timeout", async () => {
  const { ACTION_BACKSTOP_MS } = await import(
    "../../desktop/frontend/modules/actions.js"
  );
  assert.equal(
    ACTION_BACKSTOP_MS,
    16 * 60 * 1000,
    "backstop must sit above the 15-minute backend maximum",
  );

  const actionsJS = await readFile(
    new URL("../../desktop/frontend/modules/actions.js", import.meta.url),
    "utf8",
  );
  assert.equal(
    actionsJS.includes("timed out on frontend"),
    false,
    "frontend must not contradict the backend with its own timeout story",
  );
  assert.equal(
    actionsJS.includes("clearTimeout("),
    true,
    "the backstop timer must not outlive a settled action",
  );
});

test("a second mutating action for the same project is refused while one runs", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const { createActionsController } = await loadActionsModule();
  let releaseStart;
  const statuses = [];
  let starts = 0;
  let stops = 0;
  const controller = createActionsController({
    bridge: {
      startEnvironment: () => {
        starts += 1;
        return new Promise((resolve) => {
          releaseStart = resolve;
        });
      },
      stopEnvironment: async () => {
        stops += 1;
        return "stopped";
      },
    },
    getProject: () => "sample-project",
    refreshDashboard: async () => {},
    renderSkeletons: () => {},
    onStatus: (message) => statuses.push(message),
    onToast: () => {},
  });

  const first = controller.handle("env-start");
  // Let the first action claim the project's lock before the second fires.
  await Promise.resolve();
  await controller.handle("env-stop");
  assert.equal(starts, 1);
  assert.equal(stops, 0, "Stop must not run concurrently with Start");
  assert.ok(
    statuses.some((s) => s.includes("env-start is already running")),
    `expected a refusal notice, got: ${JSON.stringify(statuses)}`,
  );
  releaseStart("started");
  await first;
});
