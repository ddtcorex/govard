import test from "node:test";
import assert from "node:assert/strict";
import { withPreview } from "./support/preview-session.mjs";

test("a throwing island renders the error boundary, and retry recovers", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.evaluate(`(() => {
    const host = document.createElement("div");
    host.id = "boundary-probe-root";
    document.body.appendChild(host);
    window.__boundaryThrows = true;
    return import("/preview/boundary-probe.js").then((m) => {
      window.__boundaryProbe = m.mountBoundaryProbe("boundary-probe-root");
    });
  })()`);
  await session.waitFor(
    `document.querySelector('#boundary-probe-root [role="alert"]') !== null`,
    true,
    { timeoutMs: 10000 },
  );
  const alertText = await session.evaluate(
    `document.querySelector('#boundary-probe-root [role="alert"]').textContent`,
  );
  assert.match(alertText, /Something went wrong in this panel/);

  // Retry with the throw disabled boots the island fresh.
  await session.evaluate(`window.__boundaryThrows = false`);
  await session.evaluate(
    `document.querySelector('[data-testid="island-error-retry"]').click()`,
  );
  await session.waitFor(
    `document.querySelector('[data-testid="boundary-recovered"]') !== null`,
    true,
    { timeoutMs: 10000 },
  );
});
