import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, unlinkSync, writeFileSync } from "node:fs";
import { withPreview } from "./support/preview-session.mjs";

const DRAWER = "#settingsDrawer";
const GEAR = "#openSettings";
const FIXTURE = "settings-quit-error";

/**
 * A failing Quit under the standard settings fixture. Written and removed
 * by the scenario; the dev server serves the repo tree, so no restart is
 * needed.
 */
function installQuitErrorFixture() {
  const base = JSON.parse(
    readFileSync(
      new URL(
        "../../../desktop/frontend/preview/fixtures/settings.json",
        import.meta.url,
      ),
    ),
  );
  base.push({
    service: "SystemService",
    method: "Quit",
    args: [],
    error: "quit refused in the preview",
  });
  writeFileSync(
    new URL(
      `../../../desktop/frontend/preview/fixtures/${FIXTURE}.json`,
      import.meta.url,
    ),
    JSON.stringify(base, null, 2),
  );
}

function removeQuitErrorFixture() {
  try {
    unlinkSync(
      new URL(
        `../../../desktop/frontend/preview/fixtures/${FIXTURE}.json`,
        import.meta.url,
      ),
    );
  } catch {
    // Already gone; the assertions below are what matter.
  }
}

test("a failed quit surfaces an error toast instead of silence", async (t) => {
  const session = await withPreview(t);
  if (!session) return;
  installQuitErrorFixture();
  try {
    await session.evaluate(`window.__govardUpdatePromptModel.clearTimers()`);
    await session.evaluate(`window.__govardPreview.reset()`);
    await session.evaluate(
      `window.__govardPreview.installFixtures("${FIXTURE}")`,
    );
    await session.evaluate(`document.querySelector('${GEAR}').click()`);
    await session.waitFor(
      `document.querySelector('${DRAWER}').classList.contains("hidden")`,
      false,
    );
    await session.evaluate(
      `document.querySelector('[data-testid="quit-app"]').click()`,
    );
    await session.waitFor(
      `document.querySelector("#toastContainer").textContent.includes("Failed to quit")`,
      true,
      { timeoutMs: 10000 },
    );
    const toastClass = await session.evaluate(
      `document.querySelector("#toastContainer .toast").className`,
    );
    assert.match(toastClass, /toast--error/);
  } finally {
    removeQuitErrorFixture();
  }
});
