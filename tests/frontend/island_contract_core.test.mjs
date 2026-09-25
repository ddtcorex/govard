import test from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";

const ISLANDS_DIR = new URL("../../desktop/frontend/islands/", import.meta.url);
const islandFiles = readdirSync(ISLANDS_DIR).filter((f) => f.endsWith(".tsx"));

test("there is at least one island to check", () => {
  assert.ok(islandFiles.length > 0);
});

// D5: an island owns its subtree, so main.js's global [data-action] delegate must
// never see a click inside one: it would run the same action a second time.
//
// The pattern is the attribute form, not the bare word: MetricsFooter.tsx's own
// comment explains the data-action it replaced, and a substring scan would fail
// on that comment (the same trap the Go bridge guard has, where a comment counts).
test("no island renders a data-action attribute", () => {
  for (const file of islandFiles) {
    const src = readFileSync(new URL(file, ISLANDS_DIR), "utf8");
    assert.ok(
      !/data-action\s*=/.test(src),
      `${file} renders data-action; use data-testid and an onClick`,
    );
  }
});

// A ported template must render server text as text. The update prompt pinned
// this once already; making it a rule for every island is the point of this test.
test("no island reaches for dangerouslySetInnerHTML", () => {
  for (const file of islandFiles) {
    const src = readFileSync(new URL(file, ISLANDS_DIR), "utf8");
    assert.ok(!src.includes("dangerouslySetInnerHTML"), `${file} uses dangerouslySetInnerHTML`);
  }
});

// Every id main.js still holds a refs entry for must be an element some file
// creates. This is a ratchet rather than a plain index.html check, because four
// modules still build their markup at runtime: their ids live in modules/*.js
// today and move into islands/*.tsx as they migrate, and the scan covers both.
// Five refs named an id NOTHING ever created (sidebarGlobalStatus,
// globalServicesSummary, envSelector, logSelector, warningList), which is the
// class this catches.
test("every refs entry in main.js names an element some file creates", () => {
  const mainSource = readFileSync(new URL("../../desktop/frontend/main.js", import.meta.url), "utf8");
  const start = mainSource.indexOf("const getLiveRefs");
  assert.ok(start > 0, "main.js no longer has a getLiveRefs table; update this test");
  const end = mainSource.indexOf("\n});", start);
  assert.ok(end > start, "could not find the end of the getLiveRefs table");
  const ids = [...mainSource.slice(start, end).matchAll(/byId\("([^"]+)"\)/g)].map((m) => m[1]);
  assert.ok(ids.length > 50, `expected the refs table to be readable, found ${ids.length} ids`);

  const frontend = new URL("../../desktop/frontend/", import.meta.url);
  const sources = ["index.html", "preview.html", "main.js"];
  for (const dir of ["modules", "islands", "ui"]) {
    for (const file of readdirSync(new URL(dir + "/", frontend))) sources.push(`${dir}/${file}`);
  }
  const created = new Set();
  for (const rel of sources) {
    let src;
    try {
      src = readFileSync(new URL(rel, frontend), "utf8");
    } catch {
      continue;
    }
    for (const m of src.matchAll(/id="([^"]+)"/g)) created.add(m[1]);
  }
  const missing = ids.filter((id) => !created.has(id));
  assert.deepEqual(missing, [], "refs entries whose element nothing creates");
});
