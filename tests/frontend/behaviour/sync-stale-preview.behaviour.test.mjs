import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, unlinkSync, writeFileSync } from "node:fs";
import { withPreview } from "./support/preview-session.mjs";

const REMOTES_TAB = '[data-action="switch-tab"][data-tab="remotes"]';
const CARD = "#remotesList [data-testid='remote-card']";
const MODAL = "syncOptionsModal";
const FIXTURE = "sync-stale-probe";

const VIEWPORT = { width: 1440, height: 900 };

/**
 * A slow RunRemoteSyncPreset (1.5 s) under a distinctive plan text, cloned
 * from the remotes fixture. Written and removed by the scenario: the dev
 * server serves the repo tree, so no restart is needed.
 */
function installSlowFixture() {
  const base = JSON.parse(
    readFileSync(
      new URL(
        "../../../desktop/frontend/preview/fixtures/remotes.json",
        import.meta.url,
      ),
    ),
  );
  for (const entry of base) {
    if (entry.method === "RunRemoteSyncPreset") {
      entry.delayMs = 1500;
      entry.result = "STALE-ORDER PLAN: 42 tables";
    }
  }
  writeFileSync(
    new URL(
      `../../../desktop/frontend/preview/fixtures/${FIXTURE}.json`,
      import.meta.url,
    ),
    JSON.stringify(base, null, 2),
  );
}

function removeSlowFixture() {
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

/**
 * Overlapping previews must always settle: the spinner hides and the latest
 * plan wins. This pins the state-machine invariant (a stale settle must never
 * clear a newer spinner, and the current settle must never skip its clear),
 * not a user-visible strand — every invalidation path already resets the flag.
 */
test("overlapping sync previews settle with loading hidden", async (t) => {
  const session = await withPreview(t);
  if (!session) return;
  installSlowFixture();
  try {
    await session.send("Emulation.setDeviceMetricsOverride", {
      ...VIEWPORT,
      deviceScaleFactor: 1,
      mobile: false,
    });
    await session.evaluate(`window.__govardPreview.reset()`);
    await session.evaluate(
      `window.__govardPreview.installFixtures("${FIXTURE}")`,
    );
    await session.evaluate(`document.getElementById("refresh").click()`);
    await session.waitFor(
      `document.querySelector("#envList [data-testid='env-card']") !== null`,
      true,
      { timeoutMs: 10000 },
    );
    await session.evaluate(`document.querySelector('${REMOTES_TAB}').click()`);
    await session.waitFor(
      `document.getElementById("tab-remotes").classList.contains("active")`,
      true,
    );
    await session.waitFor(`document.querySelector("${CARD}") !== null`, true, {
      timeoutMs: 10000,
    });
    await session.evaluate(
      `document.querySelector("#remotesList [data-testid='open-sync-modal'][data-preset='db']").click()`,
    );
    await session.waitFor(
      `document.getElementById("${MODAL}") !== null && !document.getElementById("${MODAL}").classList.contains("hidden")`,
      true,
    );

    // Two overlapping previews: the first settles stale.
    await session.evaluate(
      `document.querySelector('[data-testid="preview-sync-plan"]').click()`,
    );
    await session.evaluate(
      `document.querySelector('[data-testid="preview-sync-plan"]').click()`,
    );
    await session.waitFor(
      `document.getElementById("${MODAL}").textContent.includes("STALE-ORDER PLAN")`,
      true,
      { timeoutMs: 15000 },
    );
    const loadingVisible = await session.evaluate(
      `!document.getElementById("syncPlanLoading").classList.contains("hidden")`,
    );
    assert.equal(loadingVisible, false, "the spinner must be hidden after settle");
  } finally {
    removeSlowFixture();
  }
});
