import test from "node:test";
import assert from "node:assert/strict";
import { createToast } from "../../desktop/frontend/ui/toast.js";

// Toast titles carry project names ("Stopping <project>..."), and a project
// name is a directory name the user does not fully control, so the title must
// reach the DOM as text.
const hostile = '<img src=x onerror="alert(1)">';

const installFakeDocument = (t) => {
  const items = [];
  const element = () => {
    const el = {
      className: "",
      innerHTML: "",
      style: {},
      classList: { add() {}, remove() {}, contains: () => false },
      querySelector: () => ({ addEventListener() {}, style: {}, textContent: "" }),
      remove() {},
    };
    items.push(el);
    return el;
  };
  const previous = { document: globalThis.document, raf: globalThis.requestAnimationFrame };
  globalThis.document = {
    createElement: element,
    getElementById: () => ({}),
    head: { appendChild() {} },
  };
  globalThis.requestAnimationFrame = () => {};
  t.mock.timers.enable({ apis: ["setTimeout"] });
  t.after(() => {
    globalThis.document = previous.document;
    globalThis.requestAnimationFrame = previous.raf;
  });
  return items;
};

test("show renders the message as text", (t) => {
  const items = installFakeDocument(t);
  createToast({ appendChild() {} }).show(hostile, "error");
  assert.equal(items.length, 1);
  assert.equal(items[0].innerHTML.includes("<img"), false, "raw markup must not reach innerHTML");
  assert.match(items[0].innerHTML, /&lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;/);
});

test("showStreaming renders the title as text", (t) => {
  const items = installFakeDocument(t);
  createToast({ appendChild() {} }).showStreaming(`Stopping ${hostile}...`, "info");
  assert.equal(items.length, 1);
  assert.equal(items[0].innerHTML.includes("<img"), false, "raw markup must not reach innerHTML");
  assert.match(items[0].innerHTML, /Stopping &lt;img src=x onerror=&quot;alert\(1\)&quot;&gt;\.\.\./);
});

test("show strips ANSI escapes and caps the message", (t) => {
  const items = installFakeDocument(t);
  const toast = createToast({ appendChild() {} });
  toast.show("[32mok[0m", "success");
  assert.equal(items[0].innerHTML.includes(""), false, "ansi escapes must not reach the toast");
  assert.match(items[0].innerHTML, /ok/);
  toast.show("x".repeat(500), "info");
  assert.match(items[1].innerHTML, /…\(truncated\)/, "long lines must be capped with a marker");
});

const installStreamingDocument = (t) => {
  const state = { streamText: "" };
  const streamLine = {};
  Object.defineProperty(streamLine, "textContent", {
    configurable: true,
    get: () => state.streamText,
    set: (value) => {
      state.streamText = value;
    },
  });
  const item = {
    className: "",
    innerHTML: "",
    style: {},
    classList: { add() {}, remove() {}, contains: () => false },
    querySelector: (selector) => {
      if (selector === ".toast-stream-line") return streamLine;
      if (selector === ".toast-content") return { scrollTop: 0, scrollHeight: 0 };
      if (selector === ".toast-close") return { addEventListener() {} };
      if (selector === ".toast-icon") return { textContent: "" };
      if (selector === ".toast-spinner") return { style: {} };
      return null;
    },
    remove() {},
  };
  const previous = { document: globalThis.document, raf: globalThis.requestAnimationFrame };
  globalThis.document = {
    createElement: () => item,
    getElementById: () => null,
    head: { appendChild() {} },
  };
  globalThis.requestAnimationFrame = () => {};
  t.after(() => {
    globalThis.document = previous.document;
    globalThis.requestAnimationFrame = previous.raf;
  });
  return state;
};

test("showStreaming update renders the structured message, not raw lines", (t) => {
  const state = installStreamingDocument(t);
  const handle = createToast({ appendChild() {} }).showStreaming("Syncing...", "info");
  handle.update("[32mImporting database dump[0m");
  assert.equal(state.streamText.includes(""), false, "ansi escapes must not reach the toast");
  assert.equal(state.streamText, "Importing database dump");
});
