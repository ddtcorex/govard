import test from "node:test";
import assert from "node:assert/strict";
import { ROUTE_DEFAULTS } from "../../desktop/frontend/preview/route-defaults.generated.js";

// The preview boot reads the footer version first thing: an empty default
// makes loadFooterVersion burn all 15 retries × 300ms before the app reports
// ready, once per behaviour scenario. A real default answers on attempt one.
test("preview GetVersion default answers instead of retrying", () => {
  assert.deepEqual(ROUTE_DEFAULTS["SystemService.GetVersion"], {
    kind: "value",
    value: "0.0.0-dev",
  });
});
