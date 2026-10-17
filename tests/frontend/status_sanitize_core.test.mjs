import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { sanitizeUIText } from "../../desktop/frontend/utils/sanitize.js";

const loadMainSource = () =>
  readFile(new URL("../../desktop/frontend/main.js", import.meta.url), "utf8");

// Bridge errors carry ANSI color codes and control bytes. setText writes
// textContent verbatim, so setStatus must route the message through the shared
// sanitizeUIText first; raw escapes must never reach the status line.
// (Built only from String.fromCharCode so no raw control byte ever sits in
// this file's source.)
const ESC = String.fromCharCode(27);
const BEL = String.fromCharCode(7);
const NUL = String.fromCharCode(0);
const CR = String.fromCharCode(13);

test("setStatus routes the message through sanitizeUIText", async () => {
  const src = await loadMainSource();
  assert.match(
    src,
    /import \{[^}]*sanitizeUIText[^}]*\} from "\.\/utils\/sanitize\.js"/,
    "main.js must import the shared sanitizer",
  );
  const match = src.match(/const setStatus = \([^)]*\) => \{([\s\S]*?)\};/);
  assert.ok(match, "expected a setStatus definition in main.js");
  assert.match(match[1], /sanitizeUIText\(/, "setStatus must sanitize before setText");
});

test("an ANSI-laden bridge error reaches the status line clean", () => {
  const bridgeError =
    "Status: " + ESC + "[31mError" + ESC + "[0m: dial " + BEL + "tcp" + NUL + " 10.0.0.1:80" + CR + "failed[1m";
  const clean = sanitizeUIText(bridgeError, 280);
  const hasControlByte = [...clean].some((ch) => {
    const code = ch.codePointAt(0);
    return (code < 32 && code !== 9 && code !== 10) || code === 127;
  });
  assert.equal(
    clean.includes(ESC),
    false,
    "ANSI escapes must not reach the status line",
  );
  assert.equal(
    hasControlByte,
    false,
    "control bytes must not reach the status line",
  );
  assert.match(clean, /Error/, "the human message must survive sanitizing");
  assert.match(clean, /dial tcp/, "orphan style fragments must be stripped");
});
