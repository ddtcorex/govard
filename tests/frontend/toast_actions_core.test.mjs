import test from "node:test";
import assert from "node:assert/strict";
import { createToast } from "../../desktop/frontend/ui/toast.js";

// Toast action buttons (Retry / Open logs / Copy) must render as real
// buttons with text labels, never raw markup: titles carry CLI-controlled
// text, so every string reaches the DOM escaped.
const hostile = '<img src=x onerror="alert(1)">';

const installActionDocument = (t, clipboard) => {
  const buttons = [];
  const makeElement = (tag) => {
    if (tag === "button") {
      const handlers = {};
      return {
        tag,
        isToastAction: false,
        textContent: "",
        dataset: {},
        setAttribute() {},
        addEventListener: (event, fn) => {
          handlers[event] = fn;
        },
        click: () => handlers.click?.({ stopPropagation() {} }),
      };
    }
    const el = {
      tag,
      className: "",
      innerHTML: "",
      style: {},
      dataset: {},
      textContent: "",
      classList: { add() {}, remove() {}, contains: () => false },
      querySelector: (selector) => {
        if (selector === ".toast-actions") return makeElement("div");
        return { addEventListener() {}, style: {}, textContent: "" };
      },
      querySelectorAll: (selector) => (selector === ".toast-action" ? buttons : []),
      appendChild(child) {
        if (child?.tag === "button") {
          child.isToastAction = true;
          buttons.push(child);
        }
      },
      addEventListener() {},
      remove() {},
    };
    return el;
  };
  const previous = {
    document: globalThis.document,
    raf: globalThis.requestAnimationFrame,
    navigatorDescriptor: Object.getOwnPropertyDescriptor(globalThis, "navigator"),
  };
  globalThis.document = {
    createElement: makeElement,
    getElementById: () => ({}),
    head: { appendChild() {} },
  };
  globalThis.requestAnimationFrame = () => {};
  Object.defineProperty(globalThis, "navigator", {
    configurable: true,
    writable: true,
    value: { clipboard },
  });
  t.mock.timers.enable({ apis: ["setTimeout"] });
  t.after(() => {
    globalThis.document = previous.document;
    globalThis.requestAnimationFrame = previous.raf;
    if (previous.navigatorDescriptor) {
      Object.defineProperty(globalThis, "navigator", previous.navigatorDescriptor);
    } else {
      delete globalThis.navigator;
    }
  });
  return buttons;
};

test("showOpMessage renders title and hint as text with action buttons", (t) => {
  const clipboard = { written: "", writeText(text) { this.written = text; return Promise.resolve(); } };
  const buttons = installActionDocument(t, clipboard);
  const seen = [];
  createToast({ appendChild() {} }).showOpMessage(
    {
      title: `Sync failed ${hostile}`,
      hint: "Check the connection and retry.",
      actions: [
        { id: "retry", label: "Retry" },
        { id: "open-logs", label: "Open logs" },
      ],
    },
    "error",
    (id) => seen.push(id),
  );
  assert.equal(buttons.length, 2);
  assert.deepEqual(
    buttons.map((b) => b.textContent),
    ["Retry", "Open logs"],
  );
  buttons[0].click();
  buttons[1].click();
  assert.deepEqual(seen, ["retry", "open-logs"]);
});

test("showOpMessage escapes hostile title text", (t) => {
  const clipboard = { writeText() { return Promise.resolve(); } };
  installActionDocument(t, clipboard);
  let rendered = "";
  const container = {
    appendChild: (el) => {
      rendered = el.innerHTML;
    },
  };
  createToast(container).showOpMessage(
    { title: hostile, hint: "hint", actions: [] },
    "error",
    () => {},
  );
  assert.equal(rendered.includes("<img"), false, "raw markup must not reach innerHTML");
  assert.match(rendered, /&lt;img/);
});

test("showOpMessage copy action writes the hint to the clipboard", async (t) => {
  const clipboard = { written: "", writeText(text) { this.written = text; return Promise.resolve(); } };
  const buttons = installActionDocument(t, clipboard);
  createToast({ appendChild() {} }).showOpMessage(
    { title: "Up failed", hint: "weird frobnicate failure", actions: [{ id: "copy", label: "Copy" }] },
    "error",
    () => {},
  );
  assert.equal(buttons.length, 1);
  buttons[0].click();
  await Promise.resolve();
  assert.equal(clipboard.written, "weird frobnicate failure");
});

test("showOpMessage strips ANSI escapes from title and hint", (t) => {
  const clipboard = { writeText() { return Promise.resolve(); } };
  installActionDocument(t, clipboard);
  let rendered = "";
  const container = {
    appendChild: (el) => {
      rendered = el.innerHTML;
    },
  };
  createToast(container).showOpMessage(
    { title: "[31mSync failed[0m", hint: "[32mok[0m", actions: [] },
    "error",
    () => {},
  );
  assert.equal(rendered.includes(""), false, "ansi escapes must not reach the toast");
  assert.match(rendered, /Sync failed/);
});

test("showOpMessage allowlists a hostile toast type", (t) => {
  const clipboard = { writeText() { return Promise.resolve(); } };
  installActionDocument(t, clipboard);
  let rendered = "";
  const container = {
    appendChild: (el) => {
      rendered = `${el.className} ${el.innerHTML}`;
    },
  };
  createToast(container).showOpMessage(
    { title: "Sync failed", hint: "hint", actions: [] },
    'error"><img src=x onerror="alert(1)',
    () => {},
  );
  assert.equal(rendered.includes("<img"), false, "hostile type must not reach the DOM");
  assert.match(rendered, /toast--error/, "unknown types fall back to the error style");
});
