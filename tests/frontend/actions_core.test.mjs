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
});
