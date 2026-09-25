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
