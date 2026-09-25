import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";

const FRONTEND = new URL("../../desktop/frontend/", import.meta.url);
const read = (rel) => readFileSync(new URL(rel, FRONTEND), "utf8");

const moduleFiles = () =>
  readdirSync(new URL("modules/", FRONTEND))
    .filter((f) => f.endsWith(".js"))
    .map((f) => `modules/${f}`);

const islandFiles = () =>
  readdirSync(new URL("islands/", FRONTEND))
    .filter((f) => f.endsWith(".tsx"))
    .map((f) => `islands/${f}`);

const EMITTER_SOURCES = () => [
  "index.html",
  "preview.html",
  ...moduleFiles(),
  ...islandFiles(),
];

/**
 * action -> the file that handles it, for both dispatchers.
 *
 * `main.js` is the DOCUMENT DELEGATE: its branches exist only because an element
 * carries the attribute e action, so every one of them must have an emitter.
 * `modules/actions.js` is a dispatch TARGET instead - island callbacks call
 * `actionsController.handle("env-start", project)` with the name as a value, so its
 * names legitimately have no attribute (the reverse direction still applies: an
 * attribute naming one of them is handled).
 */
function delegateActions() {
  const delegate = new Map();
  for (const match of read("main.js").matchAll(/action === "([^"]+)"/g)) {
    delegate.set(match[1], "main.js");
  }
  return delegate;
}

function handledActions() {
  const handled = new Map(delegateActions());
  const actionsSrc = read("modules/actions.js");
  for (const match of actionsSrc.matchAll(/action === "([^"]+)"/g)) {
    handled.set(match[1], "modules/actions.js");
  }
  // actions.js also dispatches a group by membership, e.g.
  // `if (["open-ide", "open-folder"].includes(action))`.
  for (const group of actionsSrc.matchAll(/\[([^\]]*)\]\.includes\(\s*action\s*\)/g)) {
    for (const name of group[1].matchAll(/"([^"]+)"/g)) {
      handled.set(name[1], "modules/actions.js");
    }
  }
  return handled;
}

/**
 * action -> the file that puts it on an element, from BOTH spellings: the literal
 * attribute and `setAttribute("data-action", "...")`.
 *
 * The second spelling is the one that matters. `modules/onboarding.js` built the
 * bootstrap prompt's option inputs with setAttribute at RUNTIME, so a grep for the
 * attribute form - and the source-level island guard, which only scans
 * islands/*.tsx - both reported the control as nonexistent, and the delegate branch
 * that served it was deleted as dead. The control was live, and its toggles
 * silently stopped working. This scan is what makes that class visible.
 */
function emittedActions() {
  const emitted = new Map();
  for (const rel of EMITTER_SOURCES()) {
    const src = read(rel);
    for (const match of src.matchAll(/data-action="([^"$]+)"/g)) {
      emitted.set(match[1], rel);
    }
    for (const match of src.matchAll(
      /setAttribute\(\s*["']data-action["']\s*,\s*["']([^"']+)["']\s*\)/g,
    )) {
      emitted.set(match[1], rel);
    }
  }
  return emitted;
}

// A dynamic attribute cannot be enumerated, and an unenumerable emitter is exactly
// how a live control loses its handler without anyone noticing. Build the value
// from a lookup the scanner can read (a literal, or a map in the same file).
test("no file builds a data-action the scanner cannot read", () => {
  const dynamicPatterns = [
    { re: /data-action="\$\{/, what: 'data-action="${...}" (template)' },
    { re: /data-action="'\s*\+/, what: "data-action=\"' + …\" (concatenation)" },
    { re: /setAttribute\(\s*["']data-action["']\s*,\s*[^"'`]/, what: "setAttribute with a dynamic value" },
  ];
  for (const rel of EMITTER_SOURCES()) {
    const src = read(rel);
    for (const { re, what } of dynamicPatterns) {
      assert.equal(
        re.test(src),
        false,
        `${rel} builds ${what}; the delegate contract test cannot enumerate it`,
      );
    }
  }
});

test("every delegate action still has an element that emits it", () => {
  const emitted = emittedActions();
  const dead = [...delegateActions().keys()].filter((action) => !emitted.has(action));
  assert.deepEqual(
    dead,
    [],
    "delegate branches whose control no longer exists (delete the branch)",
  );
});

test("every emitted data-action still has a handler", () => {
  const handled = handledActions();
  const dead = [...emittedActions().keys()].filter((action) => !handled.has(action));
  assert.deepEqual(
    dead,
    [],
    "data-action attributes nothing handles (delete the attribute, or add the handler)",
  );
});
