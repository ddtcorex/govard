import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

// Icon-only buttons carry no text for a screen reader to announce: each one
// needs an explicit accessible name. A source scan fits the island tests'
// strategy (tsx is not node-importable); the regexes below scope to the
// button element carrying each id.
const HERO = new URL("../../desktop/frontend/islands/ProjectHero.tsx", import.meta.url);

const buttonBlock = (source, id) => {
  const start = source.indexOf(`id="${id}"`);
  assert.ok(start > 0, `ProjectHero must render ${id}`);
  const open = source.lastIndexOf("<button", start);
  const end = source.indexOf(">", source.indexOf("onClick", start));
  return source.slice(open, end);
};

for (const id of ["heroStopBtn", "heroPullBtn", "heroDeleteBtn"]) {
  test(`${id} has an accessible name`, async () => {
    const source = await readFile(HERO, "utf8");
    assert.ok(
      /aria-label=(?:"[^"]+"|\{[^}]*\})/.test(buttonBlock(source, id)),
      `${id} is icon-only and needs aria-label`,
    );
  });
}
