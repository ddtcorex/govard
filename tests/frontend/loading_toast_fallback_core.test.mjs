import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { escapeHTML } from "../../desktop/frontend/utils/dom.js";
import { sanitizeUIText } from "../../desktop/frontend/utils/sanitize.js";

const loadMainSource = () =>
  readFile(new URL("../../desktop/frontend/main.js", import.meta.url), "utf8");

// The showLoadingToast fallback runs when the toast module captured no
// container at startup (headless), so it is reachable and must keep the same
// contract as ui/toast.js: kinds through the allowlist, strings through the
// shared sanitizer. (Control bytes built from char codes only, so no raw
// control byte ever sits in this file's source.)
const loadFallbackSource = async () => {
  const src = await loadMainSource();
  const start = src.indexOf("const showLoadingToast");
  assert.ok(start >= 0, "expected a showLoadingToast definition in main.js");
  const end = src.indexOf("const resolveSyncConfigForPreset", start);
  assert.ok(end > start, "expected the fallback to end before resolveSyncConfigForPreset");
  return src.slice(start, end);
};

test("loading-toast fallback allowlists the toast kind", async () => {
  const fallback = await loadFallbackSource();
  assert.equal(
    fallback.includes("toast--${type}"),
    false,
    "caller-controlled type must not reach className raw",
  );
  assert.equal(
    fallback.includes("toast--${finalType}"),
    false,
    "caller-controlled finalType must not reach className raw",
  );
  assert.match(
    fallback,
    /toastKind\(/,
    "fallback kinds must go through the toast allowlist",
  );
});

test("loading-toast fallback sanitizes its strings", async () => {
  const fallback = await loadFallbackSource();
  assert.match(
    fallback,
    /sanitizeUIText\(/,
    "fallback strings must go through the shared sanitizer",
  );
  assert.equal(
    fallback.includes("String(line)"),
    false,
    "stream updates must not reach textContent raw",
  );
  assert.equal(
    fallback.includes("String(finalLabel)"),
    false,
    "close labels must not reach textContent raw",
  );
});

test("a hostile type and message render clean through the fallback pipeline", async () => {
  const { toastKind } = await import("../../desktop/frontend/ui/toast.js");
  assert.equal(
    typeof toastKind,
    "function",
    "the toast module must export its kind allowlist",
  );
  const hostileType = 'info" onmouseover="alert(1)';
  const kind = toastKind(hostileType, "info");
  assert.equal(kind, "info", "a hostile kind must collapse to the default");
  assert.equal(
    ("toast toast--" + kind + " group").includes('"'),
    false,
    "a hostile type must not reach className",
  );

  const ESC = String.fromCharCode(27);
  const BEL = String.fromCharCode(7);
  const NUL = String.fromCharCode(0);
  const hostileMessage =
    ESC + "[31mFailed " + '<img src=x onerror="alert(1)">' + BEL + NUL + "done";
  const rendered = escapeHTML(sanitizeUIText(hostileMessage, 280));
  assert.equal(
    rendered.includes(ESC),
    false,
    "ANSI escapes must not reach the fallback toast",
  );
  assert.equal(
    rendered.includes("<img"),
    false,
    "raw markup must not reach the fallback toast",
  );
  assert.match(rendered, /&lt;img/, "the message must still identify the failure");
  assert.match(rendered, /Failed/, "the human message must survive sanitizing");
});
