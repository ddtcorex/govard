import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

// Same document stub as actions_core.test.mjs: modal.js resolves its
// elements at module scope, so the import needs a document present.
const loadActionsModule = async () => {
  const previousDocument = globalThis.document;
  globalThis.document = { getElementById: () => null };
  try {
    return await import("../../desktop/frontend/modules/actions.js");
  } finally {
    globalThis.document = previousDocument;
  }
};

test("delete confirm renders a hostile project name as inert text", async () => {
  const { buildDeleteConfirmMessage } = await loadActionsModule();
  const hostile = `<img src=x onerror=alert(1)>`;
  const message = buildDeleteConfirmMessage(hostile);
  assert.equal(
    message.includes("<img"),
    false,
    "the project name must be escaped where the dialog uses innerHTML",
  );
  assert.ok(
    message.includes("&lt;img"),
    "the escaped name must still identify the project",
  );
});

test("main.js has no unescaped interpolation in innerHTML templates", async () => {
  const src = await readFile(
    new URL("../../desktop/frontend/main.js", import.meta.url),
    "utf8",
  );
  // showConfirm keeps innerHTML deliberately for rich text, so it is not
  // scanned here; its callers are pinned by the test above. Every other
  // innerHTML template in main.js must escape its interpolations.
  const templates = [...src.matchAll(/\.innerHTML\s*=\s*`([\s\S]*?)`;/g)];
  assert.ok(templates.length > 0, "expected at least one innerHTML template");
  for (const [, body] of templates) {
    for (const interp of body.matchAll(/\$\{([^}]+)\}/g)) {
      assert.ok(
        /escapeHTML\(/.test(interp[1]),
        `unescaped interpolation in main.js innerHTML: \${${interp[1]}}`,
      );
    }
  }
});
