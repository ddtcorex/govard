import test, { before } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  ROUTES,
  desktopBridge,
} from "../../desktop/frontend/services/bridge.js";

// M1 is strictly read-only: the four routes below are the whole deploy
// surface, and none of the M2 mutating names may appear as a route.
const READONLY_DEPLOY_ROUTES = {
  DeployPlan: ["DeployService", "DeployPlan"],
  DeployCheck: ["DeployService", "DeployCheck"],
  DeployStatus: ["DeployService", "DeployStatus"],
  ListReleases: ["DeployService", "ListReleases"],
};

// M2 mutating surface: the eight routes below are the whole write surface,
// every apply behind a preview token plus an explicit confirm flag.
const MUTATING_DEPLOY_ROUTES = {
  PreviewRollback: ["DeployService", "PreviewRollback"],
  ApplyRollback: ["DeployService", "ApplyRollback"],
  PreviewSandboxUp: ["DeployService", "PreviewSandboxUp"],
  SandboxUp: ["DeployService", "SandboxUp"],
  PreviewSandboxDown: ["DeployService", "PreviewSandboxDown"],
  SandboxDown: ["DeployService", "SandboxDown"],
  SandboxSSHinfo: ["DeployService", "SandboxSSHinfo"],
  OpenSandboxSSHInTerminal: ["DeployService", "OpenSandboxSSHInTerminal"],
};

// M2 vocabulary that must now be present (routed, not stray).
const MUTATING_FRAGMENTS = [
  "rollback",
  "sandbox",
  "previewrollback",
  "applyrollback",
];

const readSource = async (rel) => {
  try {
    return await readFile(new URL(rel, import.meta.url), "utf8");
  } catch {
    return "";
  }
};

test("deploy read-only routes target DeployService", () => {
  for (const [name, target] of Object.entries(READONLY_DEPLOY_ROUTES)) {
    assert.deepEqual(
      ROUTES[name],
      target,
      `${name} should route to ${target.join(".")}`,
    );
  }
});

test("desktopBridge exposes the four read-only deploy calls", () => {
  for (const method of [
    "deployPlan",
    "deployCheck",
    "deployStatus",
    "listReleases",
  ]) {
    assert.equal(
      typeof desktopBridge[method],
      "function",
      `desktopBridge.${method} should be a function`,
    );
  }
});

test("M2 routes the eight mutating deploy calls to DeployService", () => {
  for (const [name, target] of Object.entries(MUTATING_DEPLOY_ROUTES)) {
    assert.deepEqual(
      ROUTES[name],
      target,
      `${name} should route to ${target.join(".")}`,
    );
  }
});

test("desktopBridge exposes the eight mutating deploy calls", () => {
  for (const method of [
    "previewRollback",
    "applyRollback",
    "previewSandboxUp",
    "sandboxUp",
    "previewSandboxDown",
    "sandboxDown",
    "sandboxSSHinfo",
    "openSandboxSSHInTerminal",
  ]) {
    assert.equal(
      typeof desktopBridge[method],
      "function",
      `desktopBridge.${method} should be a function`,
    );
  }
});

test("mutating bridge calls forward preview token plus confirm args", async () => {
  const calls = [];
  const { __setBackendForTest } = await import(
    "../../desktop/frontend/services/bridge.js"
  );
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return "ok";
  });
  try {
    assert.equal(
      await desktopBridge.previewRollback("sample-project", "staging", ""),
      "ok",
    );
    assert.deepEqual(calls, [
      ["DeployService", "PreviewRollback", ["sample-project", "staging", ""]],
    ]);
    calls.length = 0;
    assert.equal(
      await desktopBridge.applyRollback("sample-project", "token-1", true),
      "ok",
    );
    assert.deepEqual(calls, [
      ["DeployService", "ApplyRollback", ["sample-project", "token-1", true]],
    ]);
    calls.length = 0;
    assert.equal(
      await desktopBridge.previewSandboxUp("sample-project", "", false),
      "ok",
    );
    assert.deepEqual(calls, [
      ["DeployService", "PreviewSandboxUp", ["sample-project", "", false]],
    ]);
    calls.length = 0;
    assert.equal(
      await desktopBridge.sandboxUp("sample-project", "token-2", true),
      "ok",
    );
    assert.deepEqual(calls, [
      ["DeployService", "SandboxUp", ["sample-project", "token-2", true]],
    ]);
    calls.length = 0;
    assert.equal(
      await desktopBridge.previewSandboxDown("sample-project", false, false),
      "ok",
    );
    assert.deepEqual(calls, [
      ["DeployService", "PreviewSandboxDown", ["sample-project", false, false]],
    ]);
    calls.length = 0;
    assert.equal(
      await desktopBridge.sandboxDown("sample-project", "token-3", true),
      "ok",
    );
    assert.deepEqual(calls, [
      ["DeployService", "SandboxDown", ["sample-project", "token-3", true]],
    ]);
    calls.length = 0;
    assert.equal(
      await desktopBridge.sandboxSSHinfo("sample-project"),
      "ok",
    );
    assert.deepEqual(calls, [
      ["DeployService", "SandboxSSHinfo", ["sample-project"]],
    ]);
    calls.length = 0;
    assert.equal(
      await desktopBridge.openSandboxSSHInTerminal("sample-project"),
      "ok",
    );
    assert.deepEqual(calls, [
      ["DeployService", "OpenSandboxSSHInTerminal", ["sample-project"]],
    ]);
  } finally {
    restore();
  }
});

test("deploy bridge calls reach DeployService with project and remote", async () => {
  const calls = [];
  const { __setBackendForTest } = await import(
    "../../desktop/frontend/services/bridge.js"
  );
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return "ok";
  });
  try {
    assert.equal(
      await desktopBridge.deployPlan("sample-project", "staging"),
      "ok",
    );
    assert.deepEqual(calls, [
      ["DeployService", "DeployPlan", ["sample-project", "staging"]],
    ]);
  } finally {
    restore();
  }
});

// The pure deploy helpers live in modules/deploy.js. Loaded once in before()
// through a dynamic import so a missing module fails every test below as a
// real assertion (assert.ok on the load) instead of a file-level load error.
let deploy = null;

before(async () => {
  try {
    deploy = await import("../../desktop/frontend/modules/deploy.js");
  } catch {
    deploy = null;
  }
});

test("deploy helpers module loads", () => {
  assert.ok(deploy, "desktop/frontend/modules/deploy.js should load");
});

test("normalizeDeployReleases renders revision and branch", () => {
  assert.ok(deploy, "desktop/frontend/modules/deploy.js should load");
  const rows = deploy.normalizeDeployReleases([
    { Release: "20261017-120000", Revision: "abc123", Branch: "master" },
  ]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].release, "20261017-120000");
  assert.equal(rows[0].revision, "abc123");
  assert.equal(rows[0].branch, "master");
});

test("empty releases yield guidance, not an error", () => {
  assert.ok(deploy, "desktop/frontend/modules/deploy.js should load");
  assert.deepEqual(deploy.normalizeDeployReleases([]), []);
  const guidance = deploy.emptyReleasesGuidance();
  assert.match(guidance, /no releases/i);
});

test("deploy output is sanitized", () => {
  assert.ok(deploy, "desktop/frontend/modules/deploy.js should load");
  const dirty = "\x1b[31m/var/www/app\x1b[0m\x07 leaked";
  const clean = deploy.sanitizeDeployText(dirty);
  assert.ok(!clean.includes("\x1b"), "ANSI escapes must be stripped");
  assert.ok(!clean.includes("\x07"), "control bytes must be stripped");
  assert.ok(clean.includes("/var/www/app"), "readable text must survive");
});

test("symlink check discloses its transient probe write", () => {
  assert.ok(deploy, "desktop/frontend/modules/deploy.js should load");
  const note = deploy.probeDisclosureNote({ probePerformed: true });
  assert.match(note, /transient probe/i);
  assert.equal(deploy.probeDisclosureNote({ probePerformed: false }), "");
});

test("remote picker is required before any fetch", () => {
  assert.ok(deploy, "desktop/frontend/modules/deploy.js should load");
  assert.match(
    String(deploy.deployFetchBlocked("sample-project", "")),
    /select a remote/i,
  );
  assert.match(
    String(deploy.deployFetchBlocked("", "staging")),
    /select/i,
  );
  assert.equal(deploy.deployFetchBlocked("sample-project", "staging"), "");
});

test("DeployPanel renders releases, status, plan/check panes and a picker", async () => {
  const src = await readSource(
    "../../desktop/frontend/islands/DeployPanel.tsx",
  );
  assert.ok(src.includes("<table"), "panel should render a releases table");
  assert.ok(src.includes("Revision"), "releases should show the revision");
  assert.ok(src.includes("<select"), "panel should render a remote picker");
  assert.ok(src.includes("Status"), "panel should render a status view");
  assert.ok(src.includes("Plan"), "panel should render a plan pane");
  assert.ok(src.includes("Check"), "panel should render a check pane");
});

test("DeployPanel discloses the probe and sanitizes its strings", async () => {
  const src = await readSource(
    "../../desktop/frontend/islands/DeployPanel.tsx",
  );
  assert.ok(
    src.toLowerCase().includes("probe"),
    "panel should disclose the transient probe write",
  );
  assert.ok(
    src.includes("sanitizeDeployText") || src.includes("sanitizeUIText"),
    "panel should sanitize its strings",
  );
});

test("DeployPanel requires preview plus checkbox confirm before rollback apply", async () => {
  const src = await readSource(
    "../../desktop/frontend/islands/DeployPanel.tsx",
  );
  assert.ok(src.length > 0, "DeployPanel.tsx should exist");
  // A preview step must be shown first: no token, no apply.
  assert.ok(
    src.includes("rollback-preview"),
    "panel should render a rollback preview control",
  );
  assert.ok(
    src.includes("rollback-apply"),
    "panel should render a rollback apply control",
  );
  // The confirm must be deliberate: a checkbox the operator ticks after
  // reading the preview, never a bare single click.
  assert.ok(
    src.includes("rollback-confirm"),
    "panel should render an explicit rollback confirm control",
  );
  assert.ok(
    src.includes('type="checkbox"'),
    "rollback confirm must be a checkbox, not a single click",
  );
  // The apply stays disabled until both halves of the gate hold.
  assert.match(
    src,
    /rollback-apply[\s\S]{0,2000}disabled/,
    "rollback apply should carry a disabled guard",
  );
});

test("DeployPanel renders sandbox up/down sections with preview plus confirm", async () => {
  const src = await readSource(
    "../../desktop/frontend/islands/DeployPanel.tsx",
  );
  assert.ok(src.length > 0, "DeployPanel.tsx should exist");
  for (const marker of [
    "sandbox-up-preview",
    "sandbox-up-apply",
    "sandbox-up-confirm",
    "sandbox-down-preview",
    "sandbox-down-apply",
    "sandbox-down-confirm",
    "sandbox-ssh",
  ]) {
    assert.ok(
      src.includes(marker),
      `panel should render ${marker}`,
    );
  }
});

test("DeployPanel reports cancelled mutations honestly, not as failures", async () => {
  const src = await readSource(
    "../../desktop/frontend/islands/DeployPanel.tsx",
  );
  assert.ok(src.length > 0, "DeployPanel.tsx should exist");
  assert.ok(
    src.toLowerCase().includes("cancelled"),
    "panel should distinguish a cancelled mutation from a failure",
  );
});

test("deploy docs list the exact M1/M2 command mapping and contracts", async () => {
  const docs = await readSource("../../docs/workflows/desktop-app.md");
  const vi = await readSource("../../docs/vi/workflows/desktop-app.md");
  const readme = await readSource("../../desktop/README.md");
  assert.ok(docs.length > 0, "docs/workflows/desktop-app.md should exist");
  assert.ok(vi.length > 0, "docs/vi/workflows/desktop-app.md should exist");
  assert.ok(readme.length > 0, "desktop/README.md should exist");
  for (const [name, src] of [
    ["docs/workflows/desktop-app.md", docs],
    ["docs/vi/workflows/desktop-app.md", vi],
    ["desktop/README.md", readme],
  ]) {
    // Exact M1/M2 command mapping.
    for (const cli of [
      "govard deploy plan",
      "govard deploy rollback",
      "govard sandbox up",
      "govard sandbox down",
    ]) {
      assert.ok(
        src.includes(cli),
        `${name} should map ${cli}`,
      );
    }
    // Preview-plus-confirm contract.
    assert.match(
      src,
      /preview/i,
      `${name} should describe the preview step`,
    );
    assert.match(
      src,
      /confirm/i,
      `${name} should describe the explicit confirm`,
    );
    // Probe disclosure and cancel truthfulness.
    assert.match(
      src,
      /probe/i,
      `${name} should disclose the transient check probe`,
    );
    assert.match(
      src,
      /cancel/i,
      `${name} should state the honest cancel behavior`,
    );
  }
});

test("deploy tab markup exists and preview mirrors it", async () => {
  const index = await readSource("../../desktop/frontend/index.html");
  const preview = await readSource("../../desktop/frontend/preview.html");
  for (const marker of ["tab-deploy", "deployIsland"]) {
    assert.ok(
      index.includes(marker),
      `index.html should contain ${marker}`,
    );
    assert.ok(
      preview.includes(marker),
      `preview.html should mirror ${marker}`,
    );
  }
});

test("rollback preview renders every supplied field, not just target", async () => {
  const src = await readSource(
    "../../desktop/frontend/islands/DeployPanel.tsx",
  );
  assert.ok(src.length > 0, "DeployPanel.tsx should exist");
  // The backend supplies target revision plus the replaced release; the
  // panel must show both instead of target-plus-strategy only.
  for (const marker of ["Revision:", "Previous:"]) {
    assert.ok(
      src.includes(marker),
      `rollback preview should render ${marker}`,
    );
  }
});

test("sandbox up preview renders profile, exists and recreate", async () => {
  const src = await readSource(
    "../../desktop/frontend/islands/DeployPanel.tsx",
  );
  assert.ok(src.length > 0, "DeployPanel.tsx should exist");
  // The backend supplies profile/exists/recreate; the panel must show all
  // three instead of the container name only.
  for (const marker of ["Profile:", "Recreate:", "Existing:"]) {
    assert.ok(
      src.includes(marker),
      `sandbox up preview should render ${marker}`,
    );
  }
});

test("sandbox down preview renders profile, purge and volumes", async () => {
  const src = await readSource(
    "../../desktop/frontend/islands/DeployPanel.tsx",
  );
  assert.ok(src.length > 0, "DeployPanel.tsx should exist");
  // The backend supplies profile/purge/volumes; the panel must show all
  // three instead of the container name only.
  for (const marker of ["Purge:", "Volumes:"]) {
    assert.ok(
      src.includes(marker),
      `sandbox down preview should render ${marker}`,
    );
  }
});

test("panel refresh isolates pane failures instead of failing fast", async () => {
  const src = await readSource(
    "../../desktop/frontend/islands/DeployPanel.tsx",
  );
  assert.ok(src.length > 0, "DeployPanel.tsx should exist");
  // One failing pane must not discard the four successes: settle every
  // call and render each pane from its own outcome.
  assert.ok(
    src.includes("allSettled"),
    "refresh should settle every pane instead of failing fast",
  );
  assert.ok(
    !src.includes("Promise.all("),
    "refresh must not use Promise.all, which discards successes on one failure",
  );
  assert.ok(
    src.includes("fulfilled"),
    "refresh should render each pane from its own settled outcome",
  );
});
