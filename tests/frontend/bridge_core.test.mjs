import test from "node:test";
import assert from "node:assert/strict";
import {
  ROUTES,
  desktopBridge,
  __setBackendForTest,
  __setBindingsLoaderForTest,
} from "../../desktop/frontend/services/bridge.js";

test("every route targets a known service", () => {
  const services = new Set([
    "SettingsService", "OnboardingService", "EnvironmentService", "RemoteService",
    "SystemService", "LogService", "GlobalServiceService", "UpdateService",
    "DeployService",
    "OperationService",
    "HealthService",
    "ToolService", "DBService", "SnapshotService",
  ]);
  for (const [name, [service, method]] of Object.entries(ROUTES)) {
    assert.ok(services.has(service), `${name} routes to unknown service ${service}`);
    assert.match(method, /^[A-Z][A-Za-z]+$/, `${name} has a bad method name`);
  }
  assert.equal(Object.keys(ROUTES).length, 101);
});

test("health routes reach the HealthService bindings", () => {
  assert.deepEqual(ROUTES.RunDoctor, ["HealthService", "RunDoctor"]);
  assert.deepEqual(ROUTES.RunningDoctorOp, ["HealthService", "RunningDoctorOp"]);
  assert.deepEqual(ROUTES.PreviewDoctorFix, ["HealthService", "PreviewDoctorFix"]);
  assert.deepEqual(ROUTES.ApplyDoctorFix, ["HealthService", "ApplyDoctorFix"]);
  assert.deepEqual(ROUTES.RunIntegrityAudit, ["HealthService", "RunIntegrityAudit"]);
  assert.deepEqual(ROUTES.LastAuditResult, ["HealthService", "LastAuditResult"]);
  assert.deepEqual(ROUTES.CancelAudit, ["HealthService", "CancelAudit"]);
  assert.deepEqual(ROUTES.RunningAuditOp, ["HealthService", "RunningAuditOp"]);
});

test("tool routes reach the ToolService bindings", () => {
  assert.deepEqual(ROUTES.ListToolPresets, ["ToolService", "ListToolPresets"]);
  assert.deepEqual(ROUTES.RunTool, ["ToolService", "RunTool"]);
  assert.deepEqual(ROUTES.CancelToolRun, ["ToolService", "CancelToolRun"]);
  assert.deepEqual(ROUTES.GetToolHistory, ["ToolService", "GetToolHistory"]);
});

test("data routes reach the DBService and SnapshotService bindings", () => {
  assert.deepEqual(ROUTES.GetDBInfo, ["DBService", "GetDBInfo"]);
  assert.deepEqual(ROUTES.RunDBQuery, ["DBService", "RunDBQuery"]);
  assert.deepEqual(ROUTES.DumpDB, ["DBService", "DumpDB"]);
  assert.deepEqual(ROUTES.ImportDB, ["DBService", "ImportDB"]);
  assert.deepEqual(ROUTES.ListSnapshots, ["SnapshotService", "ListSnapshots"]);
  assert.deepEqual(ROUTES.CreateSnapshot, ["SnapshotService", "CreateSnapshot"]);
  assert.deepEqual(ROUTES.ApplySnapshotRestore, ["SnapshotService", "ApplySnapshotRestore"]);
  assert.deepEqual(ROUTES.CancelSnapshotOp, ["SnapshotService", "CancelSnapshotOp"]);
});

test("deploy routes reach the DeployService bindings", () => {
  assert.deepEqual(ROUTES.DeployPlan, ["DeployService", "DeployPlan"]);
  assert.deepEqual(ROUTES.DeployCheck, ["DeployService", "DeployCheck"]);
  assert.deepEqual(ROUTES.DeployStatus, ["DeployService", "DeployStatus"]);
  assert.deepEqual(ROUTES.ListReleases, ["DeployService", "ListReleases"]);
  assert.deepEqual(ROUTES.PreviewRollback, ["DeployService", "PreviewRollback"]);
  assert.deepEqual(ROUTES.ApplyRollback, ["DeployService", "ApplyRollback"]);
  assert.deepEqual(ROUTES.PreviewSandboxUp, ["DeployService", "PreviewSandboxUp"]);
  assert.deepEqual(ROUTES.SandboxUp, ["DeployService", "SandboxUp"]);
  assert.deepEqual(ROUTES.PreviewSandboxDown, ["DeployService", "PreviewSandboxDown"]);
  assert.deepEqual(ROUTES.SandboxDown, ["DeployService", "SandboxDown"]);
  assert.deepEqual(ROUTES.SandboxSSHinfo, ["DeployService", "SandboxSSHinfo"]);
  assert.deepEqual(ROUTES.OpenSandboxSSHInTerminal, ["DeployService", "OpenSandboxSSHInTerminal"]);
});

test("operation routes reach the OperationService bindings", () => {
  assert.deepEqual(ROUTES.ListOps, ["OperationService", "ListOps"]);
  assert.deepEqual(ROUTES.ListHistory, ["OperationService", "ListHistory"]);
  assert.deepEqual(ROUTES.CancelOp, ["OperationService", "CancelOp"]);
  assert.deepEqual(ROUTES.RetryOp, ["OperationService", "RetryOp"]);
});

test("renamed routes keep their frontend names", () => {
  assert.deepEqual(ROUTES.OpenEnvironment, ["EnvironmentService", "OpenEnvironment"]);
  assert.deepEqual(ROUTES.RunRemoteSyncBackground, ["RemoteService", "RunRemoteSync"]);
  assert.deepEqual(ROUTES.GetSyncPresetOptions, ["RemoteService", "GetSyncOptions"]);
});

test("bridge methods call the routed service method with their arguments", async () => {
  const calls = [];
  const restore = __setBackendForTest(async (service, method, args) => {
    calls.push([service, method, args]);
    return "ok";
  });
  try {
    assert.equal(await desktopBridge.startEnvironment("sample-project"), "ok");
    assert.deepEqual(calls, [["EnvironmentService", "StartEnvironment", ["sample-project"]]]);
  } finally {
    restore();
  }
});

test("a missing backend rejects with a readable error", async () => {
  const restore = __setBackendForTest(async () => {
    throw new Error("Desktop bridge not available: EnvironmentService.GetDashboard");
  });
  try {
    await assert.rejects(desktopBridge.getDashboard(), /Desktop bridge not available/);
  } finally {
    restore();
  }
});

test("bindings backend calls the generated service function", async () => {
  const restore = __setBindingsLoaderForTest(async () => ({
    EnvironmentService: { StartEnvironment: async (p) => `started ${p}` },
  }));
  try {
    assert.equal(await desktopBridge.startEnvironment("sample-project"), "started sample-project");
  } finally {
    restore();
  }
});

test("bindings that fail to load reject with a readable error", async () => {
  const restore = __setBindingsLoaderForTest(async () => {
    throw new Error("Failed to fetch dynamically imported module");
  });
  try {
    await assert.rejects(desktopBridge.getDashboard(), /Desktop bridge not available/);
  } finally {
    restore();
  }
});
