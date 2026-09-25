import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

import {
  applySyncOutputBatch,
  canUseSyncPreset,
  formatAuthMethodLabel,
  normalizeRemotePreset,
  normalizeRemotesPayload,
  protectedWarningCopy,
  resolveSyncPresetConfig,
} from "../../desktop/frontend/modules/remotes.js";

const readIsland = (name) =>
  readFile(
    new URL(`../../desktop/frontend/islands/${name}`, import.meta.url),
    "utf8",
  );

const readEntryPoint = (name) =>
  readFile(new URL(`../../desktop/frontend/${name}`, import.meta.url), "utf8");

/**
 * The ids the two vanilla renderers created. renderSyncModal injected the
 * dialog (the first eleven, including the confirm button remotes.js:603 looked
 * up through the document), renderRemotes injected the hover panel and its sync
 * terminal (the last nine), and a scenario asserted them by hand; each island
 * renders one half, so these are the contract the port has to keep.
 */
const SYNC_DIALOG_IDS = [
  "syncOptionsModal",
  "syncModalIcon",
  "syncModalTitle",
  "syncModalStep1",
  "syncModalRemoteName",
  "syncModalOptionsContainer",
  "syncModalStep2",
  "previewSyncPlanBtn",
  "syncPlanOutput",
  "syncPlanLoading",
  "confirmSyncBtn",
];
const VISUAL_PANEL_IDS = [
  "visual-source-box",
  "visual-source-badge",
  "visual-source-name",
  "visual-source-host",
  "visual-center-ring",
  "visual-center-icon",
  "visual-sync-progress-container",
  "visual-sync-scroll-viewport",
  "visual-sync-progress-line",
];

test("normalizeRemotePreset canonicalizes aliases", () => {
  assert.equal(normalizeRemotePreset("file"), "files");
  assert.equal(normalizeRemotePreset("database"), "db");
  assert.equal(normalizeRemotePreset("full"), "full");
  assert.equal(normalizeRemotePreset("unknown"), "");
});

test("normalizeRemotesPayload maps mixed-case payload fields", () => {
  const payload = normalizeRemotesPayload({
    Project: "demo",
    Remotes: [
      {
        Name: "staging",
        Host: "staging.example.com",
        User: "deploy",
        Path: "/var/www/staging",
        Port: 22,
        Protected: false,
        AuthMethod: "keychain",
        LastSync: "2m ago",
        Capabilities: ["files", "media"],
      },
    ],
    Warnings: ["warn"],
  });

  assert.equal(payload.project, "demo");
  assert.equal(payload.remotes.length, 1);
  assert.equal(payload.remotes[0].name, "staging");
  assert.equal(payload.remotes[0].authMethod, "keychain");
  assert.equal(payload.remotes[0].lastSync, "2m ago");
  assert.deepEqual(payload.remotes[0].capabilities, ["files", "media"]);
  assert.deepEqual(payload.warnings, ["warn"]);
});

test("formatAuthMethodLabel labels the auth methods it knows", () => {
  assert.equal(formatAuthMethodLabel("ssh-agent"), "SSH Agent");
  assert.equal(formatAuthMethodLabel("keyfile"), "Key File");
  assert.equal(formatAuthMethodLabel("keychain"), "Keychain");
  assert.equal(formatAuthMethodLabel(""), "Keychain");
  assert.equal(formatAuthMethodLabel("gssapi"), "gssapi");
});

test("canUseSyncPreset gates a preset on the capabilities the remote declares", () => {
  assert.equal(
    canUseSyncPreset({ capabilities: [] }, "db"),
    true,
    "a remote that declares nothing keeps every pull enabled",
  );
  assert.equal(canUseSyncPreset({}, "media"), true);
  assert.equal(
    canUseSyncPreset({ capabilities: ["files"] }, "db"),
    false,
    "a declared list without db disables the database pull",
  );
  assert.equal(canUseSyncPreset({ capabilities: ["files"] }, "media"), false);
  assert.equal(canUseSyncPreset({ capabilities: ["db"] }, "db"), true);
  assert.equal(canUseSyncPreset({ capabilities: ["db"] }, "media"), false);
  assert.equal(
    canUseSyncPreset({ capabilities: ["files"] }, "full"),
    true,
    "pull-everything has no capability of its own and is never gated",
  );
});

test("the protected warning is one shared string, not production wording", () => {
  assert.equal(
    protectedWarningCopy.includes("protected remote can overwrite local data"),
    true,
    "expected protected warning copy",
  );
  assert.equal(
    protectedWarningCopy.includes("from Production can overwrite"),
    false,
    "warning copy should not hardcode production wording",
  );
  assert.equal(
    protectedWarningCopy.includes("Consider creating a snapshot before syncing."),
    true,
  );
});

test("applySyncOutputBatch rewrites on carriage return and caps the buffer", () => {
  assert.deepEqual(applySyncOutputBatch([], "first\nsecond"), ["first", "second"]);
  assert.deepEqual(
    applySyncOutputBatch(["a"], "b\rc"),
    ["c"],
    "a carriage return overwrites the line already on screen",
  );
  assert.deepEqual(
    applySyncOutputBatch([], "b\rc"),
    ["c"],
    "with nothing on screen a carriage return line is simply the first line",
  );
  assert.deepEqual(
    applySyncOutputBatch([], "\u001b[32mok\u001b[0m\n\n  "),
    ["ok"],
    "ansi sequences and blank lines never reach the terminal",
  );
  assert.deepEqual(
    applySyncOutputBatch([], "l0\nl1\nl2\nl3", 3),
    ["l1", "l2", "l3"],
    "the buffer keeps only the newest lines",
  );
});

test("resolveSyncPresetConfig fills the backend's defaults and keeps prior choices", () => {
  const options = [
    { key: "noNoise", defaultValue: false },
    { key: "compress", defaultValue: true },
  ];
  assert.deepEqual(resolveSyncPresetConfig(options, {}), {
    noNoise: false,
    compress: true,
  });
  assert.deepEqual(
    resolveSyncPresetConfig(options, { noNoise: true }),
    { noNoise: true, compress: true },
    "a stored choice wins over the default",
  );
  assert.deepEqual(resolveSyncPresetConfig(options, { gone: true }), {
    gone: true,
    noNoise: false,
    compress: true,
  });
});

test("the remotes tab and its modal are islands, not delegate markup", async () => {
  for (const entry of ["index.html", "preview.html"]) {
    const html = await readEntryPoint(entry);
    assert.equal(
      html.includes('id="remotesIsland"'),
      true,
      `${entry} is missing the remotes island container`,
    );
    assert.equal(html.includes('id="remotes"'), false, `${entry} still ships the remotes panel`);
    assert.equal(
      html.includes('id="remotesList"'),
      false,
      `${entry} still ships the container the vanilla renderer wrote into`,
    );
    assert.equal(
      html.includes('data-action="refresh-remotes"'),
      false,
      `${entry} still routes the remotes refresh through the delegate`,
    );
  }

  const list = await readIsland("RemotesList.tsx");
  assert.equal(list.includes('id="remotes"'), true, "the island must keep the remotes panel id");
  assert.equal(list.includes('id="remotesList"'), true, "missing remotes list container");
  assert.equal(list.includes('id="remotesWarnings"'), true, "missing remotes warnings list");
  assert.equal(
    list.includes('data-testid="refresh-remotes"'),
    true,
    "missing remotes refresh action",
  );
  assert.equal(
    list.includes('data-testid="remote-card"'),
    true,
    "the card needs the testid the scenarios drive",
  );

  const modal = await readIsland("SyncModal.tsx");
  for (const id of SYNC_DIALOG_IDS) {
    assert.equal(modal.includes(`id="${id}"`), true, `missing ${id} in the sync modal island`);
  }
  for (const id of VISUAL_PANEL_IDS) {
    assert.equal(list.includes(`id="${id}"`), true, `missing ${id} in the remotes list island`);
  }
});

test("the remote card keeps its control order and its loading markers", async () => {
  const list = await readIsland("RemotesList.tsx");

  const openIndex = list.indexOf('data-testid="open-remote-url"');
  const testIndex = list.indexOf('data-testid="remote-test"');
  assert.equal(openIndex >= 0, true, "missing open-remote-url action button");
  assert.equal(testIndex >= 0, true, "missing remote-test action button");
  assert.equal(
    openIndex < testIndex,
    true,
    "open-remote-url button should be rendered to the left of remote-test",
  );
  assert.equal(
    list.includes("CONNECTED"),
    false,
    "connected badge should no longer be rendered",
  );
  for (const testid of ["open-remote-shell", "open-remote-db", "open-remote-sftp"]) {
    assert.equal(list.includes(`data-testid="${testid}"`), true, `missing ${testid} action button`);
  }
  for (const label of ["Opening SSH...", "Opening Database...", "Opening SFTP..."]) {
    assert.equal(
      list.includes(`data-loading-label="${label}"`),
      true,
      `open action button should define the ${label} loading label`,
    );
  }
  assert.equal(
    list.includes('data-role="label"'),
    true,
    "open action buttons should include label span for loading updates",
  );
});

test("the card renders the module's auth summary and capability gate", async () => {
  const list = await readIsland("RemotesList.tsx");
  assert.equal(
    list.includes("formatAuthMethodLabel"),
    true,
    "the auth summary must go through the module's label helper",
  );
  assert.equal(list.includes("Auth: "), true, "expected the auth summary line");
  assert.equal(
    list.includes("canUseSyncPreset"),
    true,
    "the pull buttons must be gated by the module's capability helper",
  );
  assert.equal(
    list.includes("protectedWarningCopy"),
    true,
    "the protected notice must render the shared copy",
  );
});
