import test from "node:test";
import assert from "node:assert/strict";
import { mountIsland } from "../../desktop/frontend/islands/mount.js";

test("mountIsland returns null when the container is missing", () => {
  globalThis.document = { getElementById: () => null };
  try {
    assert.equal(mountIsland("missing-container", null), null);
  } finally {
    delete globalThis.document;
  }
});
