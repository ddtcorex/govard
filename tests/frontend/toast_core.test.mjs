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
