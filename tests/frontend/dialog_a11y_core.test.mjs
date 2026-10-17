import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  FOCUSABLE_SELECTOR,
  getFocusableElements,
  trapTabKey,
  focusFirstElement,
  rememberFocus,
} from "../../desktop/frontend/utils/focus-trap.js";

// ---------------------------------------------------------------------------
// Shared focus-trap helper (pure, node-importable, fake-element driven).
// ---------------------------------------------------------------------------

const fakeButton = (name) => ({
  name,
  disabled: false,
  tabIndex: 0,
  focused: false,
  focus() {
    this.focused = true;
  },
});

const fakeContainer = (elements) => ({
  querySelectorAll: () => elements,
});

test("getFocusableElements skips disabled and tabindex=-1 elements", () => {
  const ok = fakeButton("ok");
  const disabled = { ...fakeButton("off"), disabled: true };
  const skipped = { ...fakeButton("skip"), tabIndex: -1 };
  const found = getFocusableElements(
    fakeContainer([ok, disabled, skipped]),
  );
  assert.deepEqual(
    found.map((el) => el.name),
    ["ok"],
  );
});

test("trapTabKey wraps Tab on last element to first", () => {
  const first = fakeButton("first");
  const last = fakeButton("last");
  const container = fakeContainer([first, last]);
  let prevented = false;
  const handled = trapTabKey(container, {
    key: "Tab",
    shiftKey: false,
    target: last,
    preventDefault() {
      prevented = true;
    },
  });
  assert.equal(handled, true);
  assert.equal(prevented, true);
  assert.equal(first.focused, true);
});

test("trapTabKey wraps Shift+Tab on first element to last", () => {
  const first = fakeButton("first");
  const last = fakeButton("last");
  const container = fakeContainer([first, last]);
  const handled = trapTabKey(container, {
    key: "Tab",
    shiftKey: true,
    target: first,
    preventDefault() {},
  });
  assert.equal(handled, true);
  assert.equal(last.focused, true);
});

test("trapTabKey ignores non-Tab keys", () => {
  const first = fakeButton("first");
  const handled = trapTabKey(fakeContainer([first]), {
    key: "Escape",
    shiftKey: false,
    target: first,
    preventDefault() {},
  });
  assert.equal(handled, false);
  assert.equal(first.focused, false);
});

test("focusFirstElement focuses the first focusable element", () => {
  const first = fakeButton("first");
  const second = fakeButton("second");
  const container = fakeContainer([first, second]);
  container.focus = () => {
    container.selfFocused = true;
  };
  focusFirstElement(container);
  assert.equal(first.focused, true);
  assert.equal(container.selfFocused, undefined);
});

test("rememberFocus restores the previously focused element", () => {
  const previous = fakeButton("previous");
  const fakeDoc = { activeElement: previous };
  const restore = rememberFocus(fakeDoc);
  restore();
  assert.equal(previous.focused, true);
});

test("FOCUSABLE_SELECTOR covers native controls", () => {
  for (const part of ["button", "input", "select", "textarea", 'a[href]']) {
    assert.ok(
      FOCUSABLE_SELECTOR.includes(part),
      `selector must include ${part}`,
    );
  }
});

// ---------------------------------------------------------------------------
// Dialog roots: role, aria-modal, labelledby (source assertions, per repo
// convention for .tsx islands that node cannot import).
// ---------------------------------------------------------------------------

test("confirm dialog root has dialog role and modal semantics", async () => {
  const html = await readFile(
    new URL("../../desktop/frontend/index.html", import.meta.url),
    "utf8",
  );
  const root = html.match(/<div id="confirmModal"[^>]*>/);
  assert.ok(root, "missing #confirmModal root");
  for (const attr of [
    'role="dialog"',
    'aria-modal="true"',
    'aria-labelledby="confirmTitle"',
  ]) {
    assert.ok(root[0].includes(attr), `#confirmModal must carry ${attr}`);
  }
});

test("onboarding dialog root has dialog role and modal semantics", async () => {
  const markup = await readFile(
    new URL(
      "../../desktop/frontend/islands/OnboardingModal.tsx",
      import.meta.url,
    ),
    "utf8",
  );
  assert.ok(
    markup.includes('id="onboardingModal"'),
    "missing onboardingModal root",
  );
  for (const attr of [
    'role="dialog"',
    'aria-modal="true"',
    "focus-trap",
  ]) {
    assert.ok(
      markup.includes(attr),
      `OnboardingModal must carry ${attr}`,
    );
  }
});

test("sync dialog root has dialog role and modal semantics", async () => {
  const markup = await readFile(
    new URL("../../desktop/frontend/islands/SyncModal.tsx", import.meta.url),
    "utf8",
  );
  assert.ok(markup.includes('id="syncOptionsModal"'), "missing sync root");
  for (const attr of ['role="dialog"', 'aria-modal="true"', "focus-trap"]) {
    assert.ok(markup.includes(attr), `SyncModal must carry ${attr}`);
  }
});

test("settings drawer root has dialog role and modal semantics", async () => {
  const markup = await readFile(
    new URL(
      "../../desktop/frontend/islands/SettingsDrawer.tsx",
      import.meta.url,
    ),
    "utf8",
  );
  assert.ok(markup.includes('id="settingsDrawer"'), "missing settings root");
  for (const attr of ['role="dialog"', 'aria-modal="true"', "focus-trap"]) {
    assert.ok(markup.includes(attr), `SettingsDrawer must carry ${attr}`);
  }
});

// ---------------------------------------------------------------------------
// Focus move/restore + Escape everywhere (source assertions).
// ---------------------------------------------------------------------------

test("confirm modal moves focus in and restores it, closes on Escape", async () => {
  const source = await readFile(
    new URL("../../desktop/frontend/ui/modal.js", import.meta.url),
    "utf8",
  );
  assert.ok(source.includes("focus-trap"), "modal.js must use focus-trap");
  assert.ok(source.includes("Escape"), "confirm modal must close on Escape");
  assert.ok(
    source.includes("rememberFocus"),
    "confirm modal must remember prior focus",
  );
  // main.js showConfirm drives the same node through its own open path.
  const shell = await readFile(
    new URL("../../desktop/frontend/main.js", import.meta.url),
    "utf8",
  );
  assert.ok(
    shell.includes("restoreConfirmFocus"),
    "showConfirm must restore prior focus",
  );
});

test("onboarding dialog closes on Escape and restores focus", async () => {
  const island = await readFile(
    new URL(
      "../../desktop/frontend/islands/OnboardingModal.tsx",
      import.meta.url,
    ),
    "utf8",
  );
  const controller = await readFile(
    new URL("../../desktop/frontend/modules/onboarding.js", import.meta.url),
    "utf8",
  );
  assert.ok(island.includes("Escape"), "onboarding must close on Escape");
  assert.ok(
    controller.includes("rememberFocus"),
    "onboarding must remember prior focus",
  );
});

test("sync dialog restores focus on close", async () => {
  const markup = await readFile(
    new URL("../../desktop/frontend/islands/SyncModal.tsx", import.meta.url),
    "utf8",
  );
  assert.ok(
    markup.includes("rememberFocus"),
    "SyncModal must remember prior focus",
  );
});

test("settings drawer restores focus on close", async () => {
  const source = await readFile(
    new URL("../../desktop/frontend/modules/settings.js", import.meta.url),
    "utf8",
  );
  assert.ok(
    source.includes("rememberFocus"),
    "settings drawer must remember prior focus",
  );
});

// ---------------------------------------------------------------------------
// Glyphs: every material-symbols span in the touched files is aria-hidden.
// ---------------------------------------------------------------------------

const assertGlyphsHidden = async (relativePath) => {
  const source = await readFile(new URL(relativePath, import.meta.url), "utf8");
  const spans = source.match(/<span[^>]*material-symbols[^>]*>/g) || [];
  assert.ok(spans.length > 0, `${relativePath} should contain glyphs`);
  for (const span of spans) {
    assert.ok(
      span.includes('aria-hidden="true"'),
      `${relativePath}: glyph missing aria-hidden: ${span.slice(0, 90)}`,
    );
  }
};

test("dialog glyphs are hidden from assistive tech", async () => {
  await assertGlyphsHidden("../../desktop/frontend/islands/SyncModal.tsx");
  await assertGlyphsHidden("../../desktop/frontend/islands/SettingsDrawer.tsx");
  await assertGlyphsHidden(
    "../../desktop/frontend/islands/EnvironmentList.tsx",
  );
  await assertGlyphsHidden("../../desktop/frontend/islands/OnboardingModal.tsx");
  await assertGlyphsHidden("../../desktop/frontend/index.html");
  await assertGlyphsHidden("../../desktop/frontend/preview.html");
});

// ---------------------------------------------------------------------------
// Keyboard-operable controls.
// ---------------------------------------------------------------------------

test("environment row has sibling native controls, never nested interactives", async () => {
  const markup = await readFile(
    new URL(
      "../../desktop/frontend/islands/EnvironmentList.tsx",
      import.meta.url,
    ),
    "utf8",
  );
  // The row container must be a plain layout element: a button row with a
  // control inside is a double tab stop and invalid HTML.
  const card = markup.match(/<(div|button)[^>]*data-testid="env-card"[^>]*>/);
  assert.ok(card, "missing env-card row");
  assert.equal(card[1], "div", "env-card must be a div, not a button");
  // Both controls are native buttons: keyboard support comes from the
  // platform, with exactly one tab stop each, so no tabIndex is needed.
  const toggle = markup.match(
    /<(button|span)[^>]*data-testid="toggle-env"[^>]*>/,
  );
  assert.ok(toggle, "missing toggle-env control");
  assert.equal(toggle[1], "button", "toggle-env must be a native button");
  assert.ok(toggle[0].includes("aria-label"), "toggle needs an aria-label");
  assert.ok(
    !toggle[0].includes("tabIndex"),
    "native toggle must not add a second tab stop",
  );
  const select = markup.match(
    /<button[^>]*data-testid="env-select"[^>]*>/,
  );
  assert.ok(select, "missing env-select control");
  assert.ok(
    !select[0].includes("tabIndex"),
    "native select must not add a second tab stop",
  );
  assert.ok(
    !markup.includes('role="button"'),
    "no roled button may remain in the row",
  );
});

test("environment variable rows are keyboard-operable", async () => {
  const markup = await readFile(
    new URL("../../desktop/frontend/islands/EnvVars.tsx", import.meta.url),
    "utf8",
  );
  const row = markup.match(/<div[^>]*data-testid="env-var-row"[^>]*>/);
  assert.ok(row, "missing env-var-row control");
  assert.ok(row[0].includes('role="button"'), "row needs role=button");
  assert.ok(row[0].includes("tabIndex"), "row must be focusable");
  assert.ok(
    markup.includes("onKeyDown"),
    "env-var rows must handle keyboard activation",
  );
});

test("sync option toggles stay native checkboxes (keyboard by default)", async () => {
  const markup = await readFile(
    new URL("../../desktop/frontend/islands/SyncModal.tsx", import.meta.url),
    "utf8",
  );
  assert.ok(
    markup.includes('type="checkbox"'),
    "sync options must remain native checkboxes",
  );
});
