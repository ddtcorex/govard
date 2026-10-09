import test from "node:test";
import assert from "node:assert/strict";

import { extractMaterialIconNames } from "../../desktop/frontend/scripts/subset-icons.mjs";

test("extractMaterialIconNames finds ligature usages", () => {
  const names = extractMaterialIconNames([
    '<span class="material-symbols-outlined">settings</span>',
    '<span className="material-symbols-outlined toast-icon">cloud_sync</span>',
  ]);
  assert.deepEqual(names, ["cloud_sync", "settings"]);
});

test("extractMaterialIconNames finds code-referenced icons and skips noise", () => {
  const names = extractMaterialIconNames([
    'return "report";',
    "icon: 'restart_alt',",
    "const x = 1;",
    'return "not-an-icon name";',
  ]);
  assert.ok(names.includes("report"), "getIcon return values must be kept");
  assert.ok(names.includes("restart_alt"), "snake_case literals must be kept");
  assert.equal(names.includes("x"), false, "noise must not become glyphs");
});

test("extractMaterialIconNames dedupes and sorts", () => {
  const names = extractMaterialIconNames([
    '"settings"',
    "'settings'",
    '<span class="material-symbols-outlined">settings</span>',
    "bare words without quotes are noise",
  ]);
  assert.deepEqual(names, ["settings"]);
});

test("single-word allowlist pins the code-only icons", async () => {
  const { SINGLE_WORD_ICONS } = await import(
    "../../desktop/frontend/scripts/subset-icons.mjs"
  );
  for (const name of ["report", "warning", "info", "error"]) {
    assert.ok(
      SINGLE_WORD_ICONS.includes(name),
      `${name} must stay allowlisted`,
    );
  }
});

test("extractor keeps code-only icon map values and fallbacks", async () => {
  const { readFile } = await import("node:fs/promises");
  // The real trap file: globalServiceIcon() renders map values as ligatures,
  // but no ligature pattern matches them in source.
  const source = await readFile(
    new URL(
      "../../desktop/frontend/modules/global-services.js",
      import.meta.url,
    ),
    "utf8",
  );
  const { extractMaterialIconNames } = await import(
    "../../desktop/frontend/scripts/subset-icons.mjs"
  );
  const names = extractMaterialIconNames([source]);
  for (const name of ["shield", "mail", "database", "deployed_code", "dns", "widgets"]) {
    assert.ok(names.includes(name), `${name} must reach the subset`);
  }
});
