import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

import {
  ROUTES,
  desktopBridge,
  __setBackendForTest,
} from "../../desktop/frontend/services/bridge.js";

const ISLAND_URL = new URL(
  "../../desktop/frontend/islands/DataTab.tsx",
  import.meta.url,
);
const HELPERS_URL = "../../desktop/frontend/modules/data-tab.js";

// Pre-implementation the island does not exist: content assertions run
// against "" so they FAIL as assertions (not as thrown ENOENT), which is
// what the RED run records.
const readIslandSource = () => {
  try {
    return readFileSync(ISLAND_URL, "utf8");
  } catch {
    return "";
  }
};

const loadHelpers = async () => {
  try {
    return await import(HELPERS_URL);
  } catch (err) {
    assert.fail(`data-tab helpers missing: ${err.message}`);
  }
};

// Controller binding decision 1: the island sends ONLY ids/args/flags. Every
// route below takes a project id, a snapshot name, an op id, explicit
// booleans, or a dialog-picked path. The user's own query text is the only
// freeform string, and it is never assembled frontend-side.

test("DB routes target DBService with exact method names", () => {
  assert.deepEqual(ROUTES.GetDBInfo, ["DBService", "GetDBInfo"]);
  assert.deepEqual(ROUTES.RunDBQuery, ["DBService", "RunDBQuery"]);
  assert.deepEqual(ROUTES.DumpDB, ["DBService", "DumpDB"]);
  assert.deepEqual(ROUTES.ImportDB, ["DBService", "ImportDB"]);
  assert.deepEqual(ROUTES.CancelDBOp, ["DBService", "CancelDBOp"]);
  assert.deepEqual(ROUTES.PickDumpDestination, [
    "DBService",
    "PickDumpDestination",
  ]);
  assert.deepEqual(ROUTES.PickImportSource, ["DBService", "PickImportSource"]);
  assert.deepEqual(ROUTES.GetDBWriteConfirmToken, [
    "DBService",
    "GetDBWriteConfirmToken",
  ]);
});

test("snapshot routes target SnapshotService with exact method names", () => {
  assert.deepEqual(ROUTES.ListSnapshots, [
    "SnapshotService",
    "ListSnapshots",
  ]);
  assert.deepEqual(ROUTES.CreateSnapshot, [
    "SnapshotService",
    "CreateSnapshot",
  ]);
  assert.deepEqual(ROUTES.PreviewSnapshotRestore, [
    "SnapshotService",
    "PreviewSnapshotRestore",
  ]);
  assert.deepEqual(ROUTES.ApplySnapshotRestore, [
    "SnapshotService",
    "ApplySnapshotRestore",
  ]);
  assert.deepEqual(ROUTES.ExportSnapshot, ["SnapshotService", "ExportSnapshot"]);
  assert.deepEqual(ROUTES.CancelSnapshotOp, [
    "SnapshotService",
    "CancelSnapshotOp",
  ]);
  assert.deepEqual(ROUTES.PickExportDestination, [
    "SnapshotService",
    "PickExportDestination",
  ]);
});

test("bridge exposes the data-tab methods", () => {
  for (const name of [
    "getDBInfo",
    "runDBQuery",
    "dumpDB",
    "importDB",
    "cancelDBOp",
    "pickDumpDestination",
    "pickImportSource",
    "getDBWriteConfirmToken",
    "listSnapshots",
    "createSnapshot",
    "previewSnapshotRestore",
    "applySnapshotRestore",
    "exportSnapshot",
    "cancelSnapshotOp",
    "pickExportDestination",
  ]) {
    assert.equal(
      typeof desktopBridge[name],
      "function",
      `desktopBridge.${name} is missing`,
    );
  }
});

test("query call sends only project, sql and explicit flags", async () => {
  assert.equal(typeof desktopBridge.runDBQuery, "function");
  const calls = [];
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return { output: "ok" };
  });
  try {
    await desktopBridge.runDBQuery(
      "sample-project",
      "SELECT 1",
      false,
      "",
    );
    assert.deepEqual(calls, [
      ["DBService", "RunDBQuery", ["sample-project", "SELECT 1", false, ""]],
    ]);
  } finally {
    restore();
  }
});

test("restore apply sends ids, preview token and explicit confirm only", async () => {
  assert.equal(typeof desktopBridge.applySnapshotRestore, "function");
  const calls = [];
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return "snapshot-restore-1";
  });
  try {
    await desktopBridge.applySnapshotRestore(
      "sample-project",
      "nightly",
      "snap-restore-token",
      true,
      { dbOnly: false, mediaOnly: false },
    );
    assert.deepEqual(calls, [
      [
        "SnapshotService",
        "ApplySnapshotRestore",
        ["sample-project", "nightly", "snap-restore-token", true, false, false],
      ],
    ]);
  } finally {
    restore();
  }
});

test("export sends the dialog-picked destination only", async () => {
  assert.equal(typeof desktopBridge.exportSnapshot, "function");
  const calls = [];
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return "snapshot-export-1";
  });
  try {
    await desktopBridge.exportSnapshot(
      "sample-project",
      "nightly",
      "/home/user/Downloads/nightly.tar.gz",
    );
    assert.deepEqual(calls, [
      [
        "SnapshotService",
        "ExportSnapshot",
        ["sample-project", "nightly", "/home/user/Downloads/nightly.tar.gz"],
      ],
    ]);
  } finally {
    restore();
  }
});

test("query mode badge is a display hint: read, write or empty", async () => {
  const helpers = await loadHelpers();
  assert.equal(helpers.classifyQueryMode("SELECT * FROM users"), "read");
  assert.equal(helpers.classifyQueryMode("  -- note\nSHOW TABLES"), "read");
  assert.equal(helpers.classifyQueryMode("UPDATE users SET x = 1"), "write");
  assert.equal(helpers.classifyQueryMode("DROP TABLE users"), "write");
  assert.equal(helpers.classifyQueryMode("SELECT 1; DROP TABLE users"), "write");
  assert.equal(helpers.classifyQueryMode("   "), "empty");
  assert.equal(helpers.classifyQueryMode(""), "empty");
});

test("display info allowlist drops every secret, even an unexpected one", async () => {
  const helpers = await loadHelpers();
  const shown = helpers.pickDisplayDBInfo({
    project: "sample-project",
    engine: "mysql",
    host: "127.0.0.1",
    port: "3306",
    database: "forge",
    user: "forge",
    hasPassword: true,
    displayUrl: "mysql://forge:****@127.0.0.1:3306/forge",
    password: "s3cret",
    passwd: "s3cret",
    dsn: "mysql://forge:s3cret@127.0.0.1/forge",
  });
  assert.deepEqual(Object.keys(shown).sort(), [
    "database",
    "displayUrl",
    "engine",
    "hasPassword",
    "host",
    "port",
    "project",
    "user",
  ]);
  assert.ok(
    !JSON.stringify(shown).includes("s3cret"),
    "secret must never reach the DOM",
  );
});

test("restore confirm stays disabled until the preview renders", async () => {
  const helpers = await loadHelpers();
  assert.equal(helpers.isSnapshotRestoreConfirmEnabled(null), false);
  assert.equal(helpers.isSnapshotRestoreConfirmEnabled({}), false);
  assert.equal(
    helpers.isSnapshotRestoreConfirmEnabled({ previewToken: "" }),
    false,
  );
  assert.equal(
    helpers.isSnapshotRestoreConfirmEnabled({
      previewToken: "snap-restore-abc",
    }),
    true,
  );
});

test("snapshot scope maps to explicit db/media flags", async () => {
  const helpers = await loadHelpers();
  assert.deepEqual(helpers.snapshotScopeFlags("full"), {
    dbOnly: false,
    mediaOnly: false,
  });
  assert.deepEqual(helpers.snapshotScopeFlags("db"), {
    dbOnly: true,
    mediaOnly: false,
  });
  assert.deepEqual(helpers.snapshotScopeFlags("media"), {
    dbOnly: false,
    mediaOnly: true,
  });
  assert.deepEqual(helpers.snapshotScopeFlags("bogus"), {
    dbOnly: false,
    mediaOnly: false,
  });
});

test("transfer terminal attribution is idempotent on (op, generation)", async () => {
  const helpers = await loadHelpers();
  let run = helpers.createTransferRun("db-dump-1", 7);
  assert.equal(run.finished, false);
  run = helpers.applyTransferTerminalEvent(run, {
    opId: "db-dump-1",
    generation: 7,
    event: "db:completed",
  });
  assert.equal(run.finished, true);
  assert.equal(run.outcome, "success");
  const settled = helpers.applyTransferTerminalEvent(run, {
    opId: "db-dump-1",
    generation: 7,
    event: "db:failed",
  });
  assert.deepEqual(settled, run, "a finished run never flips back");

  const stale = helpers.applyTransferTerminalEvent(
    helpers.createTransferRun("db-dump-1", 7),
    { opId: "db-dump-1", generation: 6, event: "db:failed" },
  );
  assert.equal(stale.finished, false, "a stale generation is ignored");

  const foreign = helpers.applyTransferTerminalEvent(
    helpers.createTransferRun("db-dump-1", 7),
    { opId: "db-dump-2", generation: 7, event: "db:completed" },
  );
  assert.equal(foreign.finished, false, "another op is ignored");

  let unknown = helpers.createTransferRun("snapshot-create-3", 0);
  unknown = helpers.applyTransferTerminalEvent(unknown, {
    opId: "snapshot-create-3",
    event: "snapshot:completed",
  });
  assert.equal(unknown.finished, true);
  assert.equal(unknown.outcome, "success");
});

test("data actions stay disabled without a project", async () => {
  const helpers = await loadHelpers();
  assert.equal(helpers.isDataActionEnabled(""), false);
  assert.equal(helpers.isDataActionEnabled("   "), false);
  assert.equal(helpers.isDataActionEnabled("sample-project"), true);
  assert.equal(typeof helpers.NO_PROJECT_GUIDANCE, "string");
  assert.match(helpers.NO_PROJECT_GUIDANCE, /project/i);
});

test("island renders masked info, never a password field", () => {
  const src = readIslandSource();
  assert.ok(src.includes('data-testid="db-info"'), "info section missing");
  assert.ok(
    src.includes("displayUrl"),
    "masked display URL missing from info cards",
  );
  const scrubbed = src.replace(/hasPassword/g, "").replace(/HasPassword/g, "");
  assert.ok(
    !/passw/i.test(scrubbed),
    "password must never appear in the island outside hasPassword",
  );
  assert.ok(!/secret/i.test(scrubbed), "secrets must never appear in the island");
  assert.ok(!src.includes("argv"), "the island must never send argv");
});

test("island gates destructive confirms behind rendered previews", () => {
  const src = readIslandSource();
  assert.ok(
    src.includes('data-testid="confirm-snapshot-restore"'),
    "restore confirm missing",
  );
  assert.ok(
    /disabled=\{[^}]*preview/i.test(src),
    "restore confirm must stay disabled until the preview renders",
  );
  assert.ok(
    src.includes('data-testid="snapshot-restore-preview"'),
    "restore preview section missing",
  );
  assert.ok(
    src.includes('data-testid="confirm-overwrite"'),
    "overwrite confirm missing for existing export/dump destinations",
  );
});

test("island shows progress and cancel while a transfer runs", () => {
  const src = readIslandSource();
  assert.ok(
    src.includes('data-testid="db-transfer-progress"'),
    "transfer progress missing",
  );
  assert.ok(
    src.includes('data-testid="db-transfer-cancel"'),
    "transfer cancel missing",
  );
  assert.ok(src.includes("cancelDBOp"), "cancel must reach CancelDBOp");
  assert.ok(src.includes("db:progress"), "progress subscription missing");
});

// Fix round: no silent supersede. While a transfer is active every start
// button stays disabled and a notice names the running op, so progress and
// cancel always belong to the run the UI tracks. No queue, refusal with
// explanation.

test("second start while a transfer runs is refused naming the running op", async () => {
  const helpers = await loadHelpers();
  assert.equal(
    typeof helpers.isStartBlocked,
    "function",
    "isStartBlocked is missing",
  );
  assert.equal(
    typeof helpers.describeRunningOp,
    "function",
    "describeRunningOp is missing",
  );
  assert.equal(helpers.isStartBlocked(null), false);
  assert.equal(helpers.isStartBlocked({ finished: true }), false);
  const run = {
    ...helpers.createTransferRun("db-dump-1", 7),
    kind: "dump",
    label: "Dump",
    detail: "/tmp/sample-project-dump.sql",
    startedAt: 1760700000000,
    percent: 12,
    message: "Dumping database...",
    finished: false,
    outcome: "",
  };
  assert.equal(helpers.isStartBlocked(run), true);
  const notice = helpers.describeRunningOp(run);
  assert.match(notice, /Dump/);
  assert.ok(
    notice.includes("/tmp/sample-project-dump.sql"),
    "refusal must name the running op",
  );
  assert.equal(helpers.describeRunningOp(null), "");
  assert.equal(helpers.describeRunningOp({ finished: true }), "");
});

test("island disables start buttons and names the running op", () => {
  const src = readIslandSource();
  assert.ok(
    src.includes('data-testid="transfer-active-notice"'),
    "running-op notice missing",
  );
  assert.ok(
    src.includes("describeRunningOp"),
    "notice must render the running-op description",
  );
  assert.ok(
    src.includes('data-testid="db-transfer-owner"'),
    "progress panel must label the owning transfer",
  );
  assert.ok(
    src.includes('data-testid="snapshot-transfer-owner"'),
    "snapshot panel must label the owning op",
  );
  const gated = src.match(/\|\| transferRunning/g) || [];
  assert.ok(
    gated.length >= 6,
    `start buttons must gate on the active transfer, found ${gated.length} gates`,
  );
});
