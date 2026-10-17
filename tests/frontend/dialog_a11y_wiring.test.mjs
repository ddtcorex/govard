import test from "node:test";
import assert from "node:assert/strict";

// Behavioral wiring tests for the Task 9 dialog a11y baseline.
//
// Where the node harness CAN reach (vanilla JS modules + the real shared
// focus-trap helper driven against fake DOM nodes), these tests drive open,
// Tab, Escape and close and assert on observable behavior: which element got
// focused, what the dialog promise resolved to, and the order focus was
// remembered in. Anything that needs a TSX island render or the main.js
// shell (React mount, full DOM) stays a source pin in
// dialog_a11y_core.test.mjs and is listed in the fix report with why.

const makeClassList = (initial = []) => {
  const set = new Set(initial);
  return {
    add: (...names) => names.forEach((name) => set.add(name)),
    remove: (...names) => names.forEach((name) => set.delete(name)),
    toggle: (name, force) => {
      const next = force === undefined ? !set.has(name) : Boolean(force);
      if (next) set.add(name);
      else set.delete(name);
      return next;
    },
    contains: (name) => set.has(name),
  };
};

const makeFocusable = (name, log) => ({
  name,
  disabled: false,
  tabIndex: 0,
  focus() {
    log.push(`focus:${name}`);
  },
});

// ---------------------------------------------------------------------------
// Confirm dialog (ui/modal.js) against fake DOM + the real helper.
// ---------------------------------------------------------------------------

const confirmLog = [];
const confirmKeyHandlers = [];
let confirmPrev = null;

const confirmTitleEl = {};
const confirmMessageEl = {};
const confirmIconEl = {};
const confirmCancelBtn = {
  listeners: {},
  addEventListener(type, fn) {
    (this.listeners[type] = this.listeners[type] || []).push(fn);
  },
  ...makeFocusable("cancel", confirmLog),
};
const confirmConfirmBtn = {
  listeners: {},
  addEventListener(type, fn) {
    (this.listeners[type] = this.listeners[type] || []).push(fn);
  },
  ...makeFocusable("confirm", confirmLog),
};
const confirmInner = { classList: makeClassList() };
const confirmModalEl = {
  classList: makeClassList(["opacity-0", "pointer-events-none"]),
  querySelector: (selector) => (selector === "div" ? confirmInner : null),
  querySelectorAll: () => [confirmCancelBtn, confirmConfirmBtn],
};

const confirmIds = {
  confirmModal: confirmModalEl,
  confirmTitle: confirmTitleEl,
  confirmMessage: confirmMessageEl,
  confirmIcon: confirmIconEl,
  confirmCancelBtn,
  confirmConfirmBtn,
};

const loadConfirmModule = async () => {
  confirmPrev = makeFocusable("opener", confirmLog);
  globalThis.document = {
    getElementById: (id) => confirmIds[id] || null,
    addEventListener: (type, fn) => {
      if (type === "keydown") confirmKeyHandlers.push(fn);
    },
    activeElement: confirmPrev,
  };
  return import("../../desktop/frontend/ui/modal.js");
};

const fireConfirmKey = (event) => {
  for (const handler of confirmKeyHandlers) handler(event);
};
const noopPrevent = () => {};

test("confirm() moves focus to the safe default (cancel)", async (t) => {
  t.after(() => {
    delete globalThis.document;
  });
  const modal = await loadConfirmModule();
  confirmLog.length = 0;
  const pending = modal.confirm({ title: "T", message: "M" });
  assert.ok(
    confirmLog.includes("focus:cancel"),
    `cancel must take focus on open, saw: ${confirmLog}`,
  );
  confirmCancelBtn.listeners.click.forEach((fn) => fn());
  assert.equal(await pending, false);
});

test("Tab on the last confirm control wraps to the first (trap engaged)", async (t) => {
  t.after(() => {
    delete globalThis.document;
  });
  const modal = await loadConfirmModule();
  confirmLog.length = 0;
  const pending = modal.confirm({ title: "T", message: "M" });
  confirmLog.length = 0;
  let prevented = false;
  fireConfirmKey({
    key: "Tab",
    shiftKey: false,
    target: confirmConfirmBtn,
    preventDefault: () => {
      prevented = true;
    },
  });
  assert.equal(prevented, true, "wrapping Tab must preventDefault");
  assert.ok(
    confirmLog.includes("focus:cancel"),
    `Tab on last must wrap to first, saw: ${confirmLog}`,
  );
  confirmCancelBtn.listeners.click.forEach((fn) => fn());
  assert.equal(await pending, false);
});

test("Escape cancels the confirm and restores prior focus", async (t) => {
  t.after(() => {
    delete globalThis.document;
  });
  const modal = await loadConfirmModule();
  confirmLog.length = 0;
  const pending = modal.confirm({ title: "T", message: "M" });
  confirmLog.length = 0;
  fireConfirmKey({ key: "Escape", preventDefault: noopPrevent });
  assert.equal(await pending, false, "Escape must resolve the dialog as false");
  assert.ok(
    confirmLog.includes("focus:opener"),
    `Escape must restore prior focus, saw: ${confirmLog}`,
  );
});

test("confirming resolves true", async (t) => {
  t.after(() => {
    delete globalThis.document;
  });
  const modal = await loadConfirmModule();
  const pending = modal.confirm({ title: "T", message: "M" });
  confirmConfirmBtn.listeners.click.forEach((fn) => fn());
  assert.equal(await pending, true);
});

// ---------------------------------------------------------------------------
// Onboarding toggleModal: focus engagement + remember-before-reset order.
// ---------------------------------------------------------------------------

test("onboarding open remembers focus first, then resets, then moves focus", async (t) => {
  const order = [];
  const prev = makeFocusable("opener", order);
  const firstBtn = makeFocusable("first", order);
  const secondBtn = makeFocusable("second", order);
  let pathValue = "";
  const previousWindow = globalThis.window;
  const previousDocument = globalThis.document;
  globalThis.window = { dispatchEvent() {} };
  globalThis.document = {
    get activeElement() {
      order.push("remember");
      return prev;
    },
  };
  t.after(() => {
    if (previousWindow === undefined) delete globalThis.window;
    else globalThis.window = previousWindow;
    if (previousDocument === undefined) delete globalThis.document;
    else globalThis.document = previousDocument;
  });

  const { createOnboardingController } = await import(
    "../../desktop/frontend/modules/onboarding.js"
  );
  const controller = createOnboardingController({
    bridge: {},
    refs: {
      projectPath: {
        get value() {
          return pathValue;
        },
        set value(next) {
          order.push("reset");
          pathValue = next;
        },
      },
      onboardingModal: {
        classList: makeClassList(["hidden"]),
        querySelectorAll: () => [firstBtn, secondBtn],
      },
    },
    onStatus: () => {},
    onToast: () => {},
    onProjectAdded: async () => {},
  });

  controller.toggleModal(true);
  assert.equal(order[0], "remember", `remember must come first, saw: ${order}`);
  assert.ok(order.includes("reset"), `resetForm must run, saw: ${order}`);
  assert.ok(
    order.indexOf("remember") < order.indexOf("reset"),
    `remember must precede reset, saw: ${order}`,
  );
  assert.ok(
    order.includes("focus:first"),
    `focus must move into the dialog, saw: ${order}`,
  );

  order.length = 0;
  controller.toggleModal(false);
  assert.ok(
    order.includes("focus:opener"),
    `close must restore prior focus, saw: ${order}`,
  );
});

// ---------------------------------------------------------------------------
// Settings toggleDrawer: focus engagement + restore.
// ---------------------------------------------------------------------------

test("settings drawer open moves focus in, close restores it", async (t) => {
  const order = [];
  const prev = makeFocusable("opener", order);
  const firstBtn = makeFocusable("first", order);
  const previousWindow = globalThis.window;
  const previousDocument = globalThis.document;
  globalThis.window = { dispatchEvent() {} };
  globalThis.document = { activeElement: prev };
  t.after(() => {
    if (previousWindow === undefined) delete globalThis.window;
    else globalThis.window = previousWindow;
    if (previousDocument === undefined) delete globalThis.document;
    else globalThis.document = previousDocument;
  });

  const { createSettingsController } = await import(
    "../../desktop/frontend/modules/settings.js"
  );
  const controller = createSettingsController({
    bridge: {},
    refs: {
      settingsDrawer: {
        classList: makeClassList(["hidden"]),
        setAttribute() {},
        querySelectorAll: () => [firstBtn],
      },
    },
    onStatus: () => {},
    onToast: () => {},
  });

  controller.toggleDrawer(true);
  assert.ok(
    order.includes("focus:first"),
    `drawer open must move focus in, saw: ${order}`,
  );
  controller.toggleDrawer(false);
  assert.ok(
    order.includes("focus:opener"),
    `drawer close must restore prior focus, saw: ${order}`,
  );
});
