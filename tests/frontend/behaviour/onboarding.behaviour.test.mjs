import test from "node:test";
import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { withPreview } from "./support/preview-session.mjs";

const VIEWPORT = { width: 1440, height: 900 };
const MODAL = "#onboardingModal";

const text = (session, selector) =>
  session.evaluate(`document.querySelector(${JSON.stringify(selector)}).textContent.trim()`);

/**
 * Opens the wizard through the shell's own button.
 *
 * The modal's markup is mounted at boot (hidden), so the click is what makes it
 * visible - and it is also what proves the island handed the controller its refs,
 * because toggleModal is the controller's own method.
 */
/**
 * Types into an input the island renders.
 *
 * React installs its own value setter on the inputs it renders and uses it as a
 * change tracker, so `input.value = x` followed by an `input` event looks like no
 * change at all and onChange never fires; the prototype's setter is what makes
 * the event reach React. This is the one place a scenario has to know that.
 */
const typeInto = (session, selector, value) =>
  session.evaluate(`(() => {
    const el = document.querySelector(${JSON.stringify(selector)});
    const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value").set;
    setter.call(el, ${JSON.stringify(value)});
    el.dispatchEvent(new Event("input", { bubbles: true }));
  })()`);

async function openWizard(session) {
  await session.evaluate(`window.__govardPreview.reset()`);
  await session.evaluate(`window.__govardPreview.installFixtures("onboarding")`);
  await session.evaluate(`document.querySelector('[data-action="open-onboarding"]').click()`);
  await session.waitFor(
    `document.getElementById("onboardingModal") !== null && !document.getElementById("onboardingModal").classList.contains("hidden")`,
    true,
  );
}

test("the wizard renders through the island and its controls are the island's", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openWizard(session);

  // The island owns the whole modal, so nothing in it may carry the attribute
  // main.js's document-wide delegate resolves (D5).
  assert.equal(
    await session.evaluate(`document.querySelectorAll('${MODAL} [data-action]').length`),
    0,
    "the migrated modal must carry no data-action for main.js's delegate",
  );

  // The picker is populated from the fixture rather than left on its two static
  // options, which is the island's own loadFrameworkOptions call: main.js's
  // bootstrap ran that before React had committed the markup.
  await session.waitFor(
    `document.querySelectorAll("#projectFramework option").length >= 5`,
    true,
    { timeoutMs: 5000 },
  );
  assert.equal(
    await session.evaluate(
      `[...document.querySelectorAll("#projectFramework option")].some((o) => o.value === "magento2")`,
    ),
    true,
  );

  const shot = await session.screenshot({ x: 0, y: 0, ...VIEWPORT });
  assert.ok(shot.length > 0, "the scenario captures the rendered wizard");
  writeFileSync(join(tmpdir(), "onboarding-island.png"), shot);

  assert.deepEqual(session.consoleErrors, []);
});

test("browsing, typing and the git sub-flow all reach the controller", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openWizard(session);

  // The path card was a delegate target; the island owns its click now.
  await session.evaluate(`document.querySelector("[data-testid='browse-project']").click()`);
  await session.waitFor(`document.getElementById("projectPath").value`, "/tmp/sample-project");
  await session.waitFor(`document.getElementById("projectPathHint").textContent.length > 0`, true);
  assert.equal(
    await session.evaluate(`document.getElementById("displayProjectPath").textContent.includes("sample-project")`),
    true,
    "the card shows the folder the picker returned",
  );
  // The summary is the controller's own write.
  await session.waitFor(`document.getElementById("onboardingSummaryProject").textContent`, "sample-project");

  // Typing reaches the controller through the island's onChange - the listeners
  // main.js bound by hand - and the value survives the controller's own write
  // back into the field, which is the case Review Focus 1 is about.
  await typeInto(session, "#projectDomain", "typed-domain");
  await session.waitFor(
    `document.getElementById("projectDomainHint").textContent.includes("typed-domain.test")`,
    true,
  );
  assert.equal(await session.evaluate(`document.getElementById("projectDomain").value`), "typed-domain");

  // The git sub-flow toggles a whole field group, and the hint follows the URL.
  assert.equal(
    await session.evaluate(`document.getElementById("gitCloneFields").offsetParent !== null`),
    false,
    "the git fields start hidden",
  );
  await session.evaluate(`document.getElementById("onboardFromGit").click()`);
  await session.waitFor(`document.getElementById("gitCloneFields").offsetParent !== null`, true);
  await typeInto(session, "#gitUrl", "git@github.com:org/repo.git");
  await session.waitFor(`document.getElementById("gitUrlHint").textContent.length > 0`, true);

  assert.deepEqual(session.consoleErrors, []);
});

test("initializing reaches the bridge exactly once", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openWizard(session);

  await session.evaluate(`document.querySelector("[data-testid='browse-project']").click()`);
  await session.waitFor(`document.getElementById("projectPath").value`, "/tmp/sample-project");
  // The submit button is the controller's own enable rule.
  await session.waitFor(`document.getElementById("onboardingSubmit").disabled`, false);

  await session.evaluate(`document.querySelector("[data-testid='add-project']").click()`);
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "OnboardProject")`,
    true,
    { timeoutMs: 8000 },
  );
  const calls = JSON.parse(
    await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls().filter(
      (c) => c.method === "OnboardProject",
    ))`),
  );
  assert.equal(calls.length, 1, "one click must initialize exactly once");
  assert.equal(calls[0].args[0].projectPath, "/tmp/sample-project");
  assert.equal(calls[0].args[0].domain, "sample-project.test");

  assert.deepEqual(session.consoleErrors, []);
});

test("the bootstrap prompt's options toggle, and the choice travels with the sync", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  await session.send("Emulation.setDeviceMetricsOverride", {
    ...VIEWPORT,
    deviceScaleFactor: 1,
    mobile: false,
  });
  await openWizard(session);

  // The fixture's project has one remote, so initializing opens the wizard's own
  // bootstrap prompt.
  await session.evaluate(`document.querySelector("[data-testid='browse-project']").click()`);
  await session.waitFor(`document.getElementById("projectPath").value`, "/tmp/sample-project");
  await session.waitFor(`document.getElementById("onboardingSubmit").disabled`, false);
  await session.evaluate(`document.querySelector("[data-testid='add-project']").click()`);
  await session.waitFor(
    `document.getElementById("onboardingBootstrapPrompt") !== null && !document.getElementById("onboardingBootstrapPrompt").classList.contains("hidden")`,
    true,
    { timeoutMs: 8000 },
  );
  await session.waitFor(
    `document.querySelectorAll("#onboardingBootstrapOptions input[type='checkbox']").length > 0`,
    true,
  );

  // The option list is built by the surviving controller inside a container this
  // island owns, so the click path belongs to the island: nothing in a migrated
  // subtree may carry a data-action the document delegate resolves (D5), and that
  // delegate's preventDefault would otherwise cancel the checkbox outright.
  assert.equal(
    await session.evaluate(`document.querySelectorAll('#onboardingModal [data-action]').length`),
    0,
    "an open bootstrap prompt must not reintroduce a data-action into the migrated modal",
  );

  const box = "#onboardingBootstrapOptions input[type='checkbox']";
  assert.equal(await session.evaluate(`document.querySelector("${box}").checked`), false);
  await session.evaluate(`document.querySelector("${box}").click()`);
  assert.equal(
    await session.evaluate(`document.querySelector("${box}").checked`),
    true,
    "clicking a bootstrap option must toggle it",
  );

  // And the toggle is what the sync is asked to run with.
  await session.evaluate(`document.querySelector("[data-testid='confirm-onboarding-bootstrap']").click()`);
  await session.waitFor(
    `window.__govardPreview.getCalls().some((c) => c.method === "RunRemoteSync")`,
    true,
    { timeoutMs: 8000 },
  );
  const syncCalls = JSON.parse(
    await session.evaluate(`JSON.stringify(window.__govardPreview.getCalls().filter(
      (c) => c.method === "RunRemoteSync",
    ))`),
  );
  assert.equal(
    syncCalls.at(-1).args[3].noNoise,
    true,
    "the toggled bootstrap option must reach the backend",
  );

  assert.deepEqual(session.consoleErrors, []);
});
