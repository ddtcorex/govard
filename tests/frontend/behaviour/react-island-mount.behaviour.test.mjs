import test from "node:test";
import assert from "node:assert/strict";
import { withPreview } from "./support/preview-session.mjs";

test("the demo island mounts, owns its subtree, and unmounts cleanly", async (t) => {
  const session = await withPreview(t);
  if (!session) return;

  const initialText = await session.evaluate(
    `document.getElementById("react-demo-island-root").textContent`,
  );
  assert.match(initialText, /Count: 0/);

  // D5: the island owns every element in its subtree; no [data-action] may leak
  // into it for main.js's global delegate to double-handle.
  const dataActionCount = await session.evaluate(
    `document.querySelectorAll("#react-demo-island-root [data-action]").length`,
  );
  assert.equal(dataActionCount, 0);

  await session.evaluate(
    `document.querySelector('[data-testid="demo-island-increment"]').click()`,
  );
  await session.waitFor(
    `document.getElementById("react-demo-island-root").textContent.includes("Count: 1")`,
    true,
  );

  // D8: the shadcn Button must render in Govard's real primary colour, not
  // shadcn's default palette. Without the adapter's @theme mapping the class
  // would compile to nothing and this would read the transparent default.
  const buttonBg = await session.evaluate(
    `getComputedStyle(document.querySelector('[data-testid="demo-island-increment"]')).backgroundColor`,
  );
  const expectedBg = await session.evaluate(`(() => {
    const hex = getComputedStyle(document.documentElement).getPropertyValue('--primary').trim();
    const m = hex.match(/^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i);
    return m ? \`rgb(\${parseInt(m[1], 16)}, \${parseInt(m[2], 16)}, \${parseInt(m[3], 16)})\` : hex;
  })()`);
  assert.equal(
    buttonBg,
    expectedBg,
    "the generated Button must render in Govard's real --primary colour, not shadcn's default palette",
  );
  // The other half of the adapter: a token shadcn owns and Govard did not have,
  // which would be absent and therefore unreadable if the mapping were missing.
  const buttonColor = await session.evaluate(
    `getComputedStyle(document.querySelector('[data-testid="demo-island-increment"]')).color`,
  );
  assert.equal(buttonColor, "rgb(15, 23, 42)", "text-primary-foreground must resolve to the app's slate-900");

  // D6: the island reads the same store the vanilla modules write, so a setState
  // from outside React has to reach it without any manual binding.
  await session.evaluate(
    `import("/state/store.js").then((m) => m.setState({ sidebarMode: "environments" }))`,
  );
  await session.waitFor(
    `document.getElementById("react-demo-island-root").textContent.includes("sidebarMode: environments")`,
    true,
  );

  await session.evaluate(`window.__govardDemoIsland.unmount()`);
  assert.equal(
    await session.evaluate(`document.getElementById("react-demo-island-root").childElementCount`),
    0,
    "unmount() must leave the container empty",
  );

  assert.deepEqual(session.consoleErrors, []);
});
