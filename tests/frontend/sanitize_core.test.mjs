import test from "node:test";
import assert from "node:assert/strict";
import { sanitizeUIText } from "../../desktop/frontend/utils/sanitize.js";

test("strips ANSI escapes", () => {
  assert.equal(sanitizeUIText("\x1b[32mOK\x1b[0m done", 200), "OK done");
});

test("strips orphan ANSI fragments", () => {
  assert.equal(sanitizeUIText("[1mTitle[0m", 200), "Title");
});

test("strips carriage returns and control chars", () => {
  assert.equal(sanitizeUIText("10%\r50%\x07bell\x00nul", 200), "10%50%bellnul");
});

test("keeps newline and tab", () => {
  assert.equal(sanitizeUIText("a\tb\nc", 200), "a\tb\nc");
});

test("drops lone surrogates (the JS form of invalid UTF-8)", () => {
  assert.equal(sanitizeUIText("A\uD800B\uDC00C", 200), "ABC");
});

test("truncates over-max input with marker", () => {
  const got = sanitizeUIText("x".repeat(100), 20);
  assert.ok(got.endsWith("…(truncated)"), `expected marker suffix, got ${JSON.stringify(got)}`);
  assert.equal([...got].length, 20);
});

test("leaves short text untouched", () => {
  assert.equal(sanitizeUIText("short", 200), "short");
});
