// @ts-check

/**
 * Frontend name -> [bound Go service, method]. The only map from UI calls to
 * Go. Plan B Task 3 points `backend` at the generated bindings; nothing else
 * in this file changes then.
 * @type {Record<string, [string, string]>}
 */
export const ROUTES = {
  GetSettings: ["SettingsService", "GetSettings"],
  GetMailpitURL: ["SettingsService", "GetMailpitURL"],
  UpdateSettings: ["SettingsService", "UpdateSettings"],
  ResetSettings: ["SettingsService", "ResetSettings"],
  GetTrayStatus: ["SettingsService", "GetTrayStatus"],
  GetDashboard: ["EnvironmentService", "GetDashboard"],
  StartEnvironment: ["EnvironmentService", "StartEnvironment"],
  StopEnvironment: ["EnvironmentService", "StopEnvironment"],
  RestartEnvironment: ["EnvironmentService", "RestartEnvironment"],
  PullEnvironment: ["EnvironmentService", "PullEnvironment"],
  ToggleEnvironment: ["EnvironmentService", "ToggleEnvironment"],
  OpenEnvironment: ["EnvironmentService", "OpenEnvironment"],
  QuickActionForProject: ["EnvironmentService", "QuickActionForProject"],
  DeleteProject: ["EnvironmentService", "DeleteProject"],
  ListFrameworks: ["EnvironmentService", "ListFrameworks"],
  GetGlobalServices: ["GlobalServiceService", "GetGlobalServices"],
  StartGlobalServices: ["GlobalServiceService", "StartGlobalServices"],
  StopGlobalServices: ["GlobalServiceService", "StopGlobalServices"],
  RestartGlobalServices: ["GlobalServiceService", "RestartGlobalServices"],
  PullGlobalServices: ["GlobalServiceService", "PullGlobalServices"],
  StartGlobalService: ["GlobalServiceService", "StartGlobalService"],
  StopGlobalService: ["GlobalServiceService", "StopGlobalService"],
  RestartGlobalService: ["GlobalServiceService", "RestartGlobalService"],
  OpenGlobalService: ["GlobalServiceService", "OpenGlobalService"],
  GetSystemMetrics: ["SystemService", "GetSystemMetrics"],
  GetResourceMetrics: ["SystemService", "GetResourceMetrics"],
  GetUserInfo: ["SystemService", "GetUserInfo"],
  GetVersion: ["SystemService", "GetVersion"],
  Quit: ["SystemService", "Quit"],
  StartServiceTerminalInOS: ["LogService", "StartServiceTerminalInOS"],
  GetLogsForService: ["LogService", "GetLogsForService"],
  StartLogStreamForService: ["LogService", "StartLogStreamForService"],
  StopLogStream: ["LogService", "StopLogStream"],
  GetGlobalServiceLogs: ["LogService", "GetGlobalServiceLogs"],
  StartGlobalServiceLogStream: ["LogService", "StartGlobalServiceLogStream"],
  StopGlobalServiceLogStream: ["LogService", "StopGlobalServiceLogStream"],
  SaveLogsToFile: ["LogService", "SaveLogsToFile"],
  GetRemotes: ["RemoteService", "GetRemotes"],
  TestRemote: ["RemoteService", "TestRemote"],
  OpenRemoteURL: ["RemoteService", "OpenRemoteURL"],
  OpenRemoteShell: ["RemoteService", "OpenRemoteShell"],
  OpenRemoteDB: ["RemoteService", "OpenRemoteDB"],
  OpenRemoteSFTP: ["RemoteService", "OpenRemoteSFTP"],
  RunRemoteSyncPreset: ["RemoteService", "RunRemoteSyncPreset"],
  RunRemoteSyncBackground: ["RemoteService", "RunRemoteSync"],
  RunRemoteSyncInTerminal: ["RemoteService", "RunRemoteSyncInTerminal"],
  GetSyncPresetOptions: ["RemoteService", "GetSyncOptions"],
  PickProjectDirectory: ["OnboardingService", "PickProjectDirectory"],
  OnboardProject: ["OnboardingService", "OnboardProject"],
  DetectMigrationSource: ["OnboardingService", "DetectMigrationSource"],
  CheckForUpdates: ["UpdateService", "CheckForUpdates"],
  GetUpdateChannel: ["UpdateService", "GetUpdateChannel"],
  SetUpdateChannel: ["UpdateService", "SetUpdateChannel"],
  InstallLatestUpdate: ["UpdateService", "InstallLatestUpdate"],
  RestartDesktopApp: ["UpdateService", "RestartDesktopApp"],
  ListOps: ["OperationService", "ListOps"],
  ListHistory: ["OperationService", "ListHistory"],
  CancelOp: ["OperationService", "CancelOp"],
  RetryOp: ["OperationService", "RetryOp"],
  RunDoctor: ["HealthService", "RunDoctor"],
  PreviewDoctorFix: ["HealthService", "PreviewDoctorFix"],
  ApplyDoctorFix: ["HealthService", "ApplyDoctorFix"],
  CancelDoctorRun: ["HealthService", "CancelDoctorRun"],
  RunningDoctorOp: ["HealthService", "RunningDoctorOp"],
  RunIntegrityAudit: ["HealthService", "RunIntegrityAudit"],
  GetAuditResult: ["HealthService", "GetAuditResult"],
  GetAuditDiff: ["HealthService", "GetAuditDiff"],
  LastAuditResult: ["HealthService", "LastAuditResult"],
  CancelAudit: ["HealthService", "CancelAudit"],
  RunningAuditOp: ["HealthService", "RunningAuditOp"],
  ListToolPresets: ["ToolService", "ListToolPresets"],
  RunTool: ["ToolService", "RunTool"],
  CancelToolRun: ["ToolService", "CancelToolRun"],
  GetToolHistory: ["ToolService", "GetToolHistory"],
  GetDBInfo: ["DBService", "GetDBInfo"],
  RunDBQuery: ["DBService", "RunDBQuery"],
  DumpDB: ["DBService", "DumpDB"],
  ImportDB: ["DBService", "ImportDB"],
  CancelDBOp: ["DBService", "CancelDBOp"],
  PickDumpDestination: ["DBService", "PickDumpDestination"],
  PickImportSource: ["DBService", "PickImportSource"],
  GetDBWriteConfirmToken: ["DBService", "GetDBWriteConfirmToken"],
  ListSnapshots: ["SnapshotService", "ListSnapshots"],
  CreateSnapshot: ["SnapshotService", "CreateSnapshot"],
  PreviewSnapshotRestore: ["SnapshotService", "PreviewSnapshotRestore"],
  ApplySnapshotRestore: ["SnapshotService", "ApplySnapshotRestore"],
  ExportSnapshot: ["SnapshotService", "ExportSnapshot"],
  CancelSnapshotOp: ["SnapshotService", "CancelSnapshotOp"],
  PickExportDestination: ["SnapshotService", "PickExportDestination"],
};

/**
 * Wails v3 backend: the generated bindings, loaded lazily so node tests that
 * import this module never load the browser runtime. The module also exports
 * the generated model classes, so only the lookup below is typed.
 * @type {() => Promise<any>}
 */
const realLoadBindings = () =>
  import("../bindings/govard/internal/desktop/index.js");

/** @type {() => Promise<any>} */
let loadBindings = realLoadBindings;

/**
 * The real generated bindings module, never the swapped-in test/preview loader.
 * The preview's response materializer needs it so a fixture replays through the
 * same model constructors production uses (spec D4): under the loader seam the
 * swappable `loadBindings` above would hand it the fake module, which has no
 * model classes at all.
 * @type {() => Promise<any>}
 */
export const loadGeneratedModules = realLoadBindings;

/**
 * The generated model classes, as a namespace.
 *
 * The bindings' index re-exports a model per EXPORTED Go type only, while
 * models.js carries a class for every type the services use - including an
 * unexported one such as RemoteService.GetSyncOptions's return type. The
 * preview's materializer falls back to this namespace for exactly those, so a
 * fixture still replays through the real constructor (spec D4) instead of
 * failing the route. Lazy for the same reason as the loader above.
 * @type {() => Promise<Record<string, any>>}
 */
export const loadGeneratedModels = () =>
  import("../bindings/govard/internal/desktop/models.js");

/**
 * Test and preview/record seam: replace the bindings loader, returns a restore function.
 * @param {() => Promise<any>} fn
 */
export function __setBindingsLoaderForTest(fn) {
  const previous = loadBindings;
  loadBindings = fn;
  return () => {
    loadBindings = previous;
  };
}

/**
 * `objectNames.Call` in @wailsio/runtime: the object id every generated binding
 * call arrives on.
 * @type {number}
 */
export const CALL_OBJECT_ID = 0;

/**
 * `objectNames.CancelCall` in @wailsio/runtime. A transport that answers binding
 * calls must answer this too, at minimum as a no-op success, because
 * CancellablePromise's oncancelled path sends the cancellation through that same
 * transport.
 * @type {number}
 */
export const CANCEL_CALL_OBJECT_ID = 10;

/**
 * Test and preview/record seam: replace the Wails runtime transport, returns a
 * restore function. It lives here rather than in a preview module because the
 * frontend runtime guard (tests/desktop_frontend_bridge_guard_test.go) allows
 * exactly two modules to touch the runtime or the generated bindings, and
 * bridge.js is one of them. The runtime import is dynamic so node tests that
 * import this module still never load the browser runtime.
 * @param {{call: (objectID: number, method: number, windowName: string, args: any) => Promise<any>}} transport
 * @returns {Promise<() => void>}
 */
export async function __setTransportForTest(transport) {
  const rt = await import("@wailsio/runtime");
  const previous = rt.getTransport();
  rt.setTransport(transport);
  return () => rt.setTransport(previous);
}

/**
 * @param {string} service
 * @param {string} method
 * @param {any[]} args
 */
const bindingsBackend = async (service, method, args) => {
  /** @type {Record<string, Record<string, (...args: any[]) => Promise<any>>> | undefined} */
  let bindings;
  /** @type {((...args: any[]) => Promise<any>) | undefined} */
  let fn;
  try {
    bindings = await loadBindings();
    fn = bindings?.[service]?.[method];
  } catch (err) {
    throw new Error(
      `Desktop bridge not available: ${service}.${method} (${err instanceof Error ? err.message : String(err)})`,
    );
  }
  if (typeof fn !== "function") {
    throw new Error(`Desktop bridge not available: ${service}.${method}`);
  }
  return fn(...args);
};

/** @type {(service: string, method: string, args: any[]) => Promise<any>} */
let backend = bindingsBackend;

/**
 * Test and preview/record seam: replace the backend, returns a restore function.
 * @param {(service: string, method: string, args: any[]) => Promise<any>} fn
 */
export function __setBackendForTest(fn) {
  const previous = backend;
  backend = fn;
  return () => {
    backend = previous;
  };
}

/**
 * @param {keyof typeof ROUTES} name
 * @param {...any} args
 */
const invoke = (name, ...args) => {
  const [service, method] = ROUTES[name];
  return backend(service, method, args);
};

export const desktopBridge = {
  /**
   * @param {any[]} args
   */
  async getDashboard(...args) {
    if (args && args.length > 0) {
      console.warn("ROGUE ARGS SENT TO GETDASHBOARD:", args);
    }
    return invoke("GetDashboard"); // explicitly drop args
  },
  async getGlobalServices() {
    return invoke("GetGlobalServices");
  },
  async startGlobalServices() {
    return invoke("StartGlobalServices");
  },
  async stopGlobalServices() {
    return invoke("StopGlobalServices");
  },
  async restartGlobalServices() {
    return invoke("RestartGlobalServices");
  },
  async pullGlobalServices() {
    return invoke("PullGlobalServices");
  },
  /**
   * @param {string} serviceID
   */
  async startGlobalService(serviceID) {
    return invoke("StartGlobalService", serviceID);
  },
  /**
   * @param {string} serviceID
   */
  async stopGlobalService(serviceID) {
    return invoke("StopGlobalService", serviceID);
  },
  /**
   * @param {string} serviceID
   */
  async restartGlobalService(serviceID) {
    return invoke("RestartGlobalService", serviceID);
  },
  /**
   * @param {string} serviceID
   */
  async openGlobalService(serviceID) {
    return invoke("OpenGlobalService", serviceID);
  },
  /**
   * @param {string} serviceID
   * @param {number} [lines=200]
   */
  async getGlobalServiceLogs(serviceID, lines = 200) {
    return invoke("GetGlobalServiceLogs", serviceID, Number(lines) || 200);
  },
  /**
   * @param {string} serviceID
   */
  async startGlobalServiceLogStream(serviceID) {
    return invoke("StartGlobalServiceLogStream", serviceID);
  },
  async stopGlobalServiceLogStream() {
    return invoke("StopGlobalServiceLogStream");
  },
  async getCurrentUser() {
    return invoke("GetUserInfo");
  },
  async getVersion() {
    return invoke("GetVersion");
  },
  async getSystemMetrics() {
    return invoke("GetSystemMetrics");
  },
  async getResourceMetrics() {
    return invoke("GetResourceMetrics");
  },
  async pickProjectDirectory() {
    return invoke("PickProjectDirectory");
  },
  async listFrameworks() {
    return invoke("ListFrameworks");
  },
  /**
   * @param {any} inputOrPath
   * @param {string} framework
   * @param {string} [domain=""]
   * @param {Record<string, any>} [serviceOptions={}]
   */
  async onboardProject(
    inputOrPath,
    framework,
    domain = "",
    serviceOptions = {},
  ) {
    // Support both object payload (current onboarding flow) and legacy positional args.
    if (
      inputOrPath &&
      typeof inputOrPath === "object" &&
      !Array.isArray(inputOrPath)
    ) {
      const input = inputOrPath;
      return invoke("OnboardProject", {
        projectPath: String(input.projectPath || "").trim(),
        framework: String(input.framework || "").trim(),
        frameworkVersion: String(input.frameworkVersion || "").trim(),
        domain: String(input.domain || "").trim(),
        cloneFromGit: Boolean(input.cloneFromGit),
        gitProtocol: String(input.gitProtocol || "").trim(),
        gitURL: String(input.gitURL || "").trim(),
        confirmFolderOverride: Boolean(input.confirmFolderOverride),
        varnishEnabled: Boolean(input.varnishEnabled),
        redisEnabled: Boolean(input.redisEnabled),
        rabbitMQEnabled: Boolean(input.rabbitMQEnabled),
        elasticsearchEnabled: Boolean(input.elasticsearchEnabled),
        applyOverrides:
          input.applyOverrides === undefined
            ? true
            : Boolean(input.applyOverrides),
        skipIDE: Boolean(input.skipIDE),
      });
    }

    const opts = serviceOptions || {};
    return invoke("OnboardProject", {
      projectPath: String(inputOrPath || "").trim(),
      framework: String(framework || "").trim(),
      frameworkVersion: "",
      domain: String(domain || "").trim(),
      cloneFromGit: false,
      gitProtocol: "",
      gitURL: "",
      confirmFolderOverride: false,
      varnishEnabled: Boolean(opts.varnish),
      redisEnabled: Boolean(opts.redis),
      rabbitMQEnabled: Boolean(opts.rabbitmq),
      elasticsearchEnabled: Boolean(opts.elasticsearch),
      applyOverrides: true,
      skipIDE: false,
    });
  },
  /**
   * @param {string} projectPath
   */
  async detectMigrationSource(projectPath) {
    return invoke("DetectMigrationSource", projectPath);
  },
  /**
   * @param {string} project
   */
  async getRemotes(project) {
    return invoke("GetRemotes", project);
  },
  /**
   * @param {string} project
   * @param {string} remoteName
   */
  async testRemote(project, remoteName) {
    return invoke("TestRemote", project, remoteName);
  },
  /**
   * @param {string} project
   * @param {string} remoteName
   */
  async openRemoteURL(project, remoteName) {
    return invoke("OpenRemoteURL", project, remoteName);
  },
  /**
   * @param {string} project
   * @param {string} remoteName
   */
  async openRemoteShell(project, remoteName) {
    return invoke("OpenRemoteShell", project, remoteName);
  },
  /**
   * @param {string} project
   * @param {string} remoteName
   */
  async openRemoteDB(project, remoteName) {
    return invoke("OpenRemoteDB", project, remoteName);
  },
  /**
   * @param {string} project
   * @param {string} remoteName
   */
  async openRemoteSFTP(project, remoteName) {
    return invoke("OpenRemoteSFTP", project, remoteName);
  },
  /**
   * @param {string} project
   * @param {string} remoteName
   * @param {string} preset
   * @param {Record<string, any>} [syncConfig={}]
   */
  async runRemoteSyncPreset(project, remoteName, preset, syncConfig = {}) {
    return invoke(
      "RunRemoteSyncPreset",
      project,
      remoteName,
      preset,
      syncConfig || {},
    );
  },
  /**
   * @param {string} project
   * @param {string} remoteName
   * @param {string} preset
   * @param {Record<string, any>} [syncConfig={}]
   */
  async runRemoteSyncBackground(project, remoteName, preset, syncConfig = {}) {
    return invoke(
      "RunRemoteSyncBackground",
      project,
      remoteName,
      preset,
      syncConfig || {},
    );
  },
  /**
   * @param {string} project
   * @param {string} remoteName
   * @param {string} preset
   * @param {Record<string, any>} [syncConfig={}]
   */
  async runRemoteSyncInTerminal(project, remoteName, preset, syncConfig = {}) {
    return invoke(
      "RunRemoteSyncInTerminal",
      project,
      remoteName,
      preset,
      syncConfig || {},
    );
  },
  /**
   * @param {string} project
   * @param {string} preset
   */
  async getSyncPresetOptions(project, preset) {
    return invoke("GetSyncPresetOptions", project, preset);
  },
  /**
   * @param {string} project
   */
  async startEnvironment(project) {
    return invoke("StartEnvironment", project);
  },
  /**
   * @param {string} project
   */
  async stopEnvironment(project) {
    return invoke("StopEnvironment", project);
  },
  /**
   * @param {string} project
   */
  async restartEnvironment(project) {
    return invoke("RestartEnvironment", project);
  },
  /**
   * @param {string} project
   */
  async pullEnvironment(project) {
    return invoke("PullEnvironment", project);
  },
  /**
   * @param {string} project
   */
  async toggleEnvironment(project) {
    return invoke("ToggleEnvironment", project);
  },
  /**
   * @param {string} project
   */
  async openEnvironment(project) {
    return invoke("OpenEnvironment", project);
  },
  /**
   * @param {string} project
   */
  async deleteProject(project) {
    return invoke("DeleteProject", project);
  },
  /**
   * @param {string} action
   * @param {string} project
   */
  async quickActionForProject(action, project) {
    return invoke("QuickActionForProject", action, project);
  },
  /**
   * The Go method takes a line count; the UI never passes one, so the bridge
   * keeps the 1000-line default the old App proxy applied.
   * @param {string} project
   * @param {string} service
   * @param {number} [lines=1000]
   */
  async getLogsForService(project, service, lines = 1000) {
    return invoke("GetLogsForService", project, service, Number(lines) || 1000);
  },
  /**
   * @param {string} project
   * @param {string} service
   */
  async startLogStreamForService(project, service) {
    return invoke("StartLogStreamForService", project, service);
  },
  async stopLogStream() {
    return invoke("StopLogStream");
  },
  /**
   * @param {string} content
   * @param {string} suggestedName
   */
  async saveLogsToFile(content, suggestedName) {
    return invoke(
      "SaveLogsToFile",
      String(content || ""),
      String(suggestedName || ""),
    );
  },
  /**
   * @param {string} project
   * @param {string} service
   * @param {string} user
   * @param {string} shell
   */
  async startServiceTerminalInOS(project, service, user, shell) {
    return invoke("StartServiceTerminalInOS", project, service, user, shell);
  },
  async getSettings() {
    return invoke("GetSettings");
  },
  async getMailpitURL() {
    return invoke("GetMailpitURL");
  },
  /**
   * @param {Record<string, any>} [settings={}]
   */
  async updateSettings(settings = {}) {
    const payload = {
      theme: String(settings.theme || "system"),
      proxyTarget: String(settings.proxyTarget || ""),
      preferredBrowser: String(settings.preferredBrowser || ""),
      codeEditor: String(settings.codeEditor || ""),
      dbClientPreference: String(settings.dbClientPreference || "pma"),
      runInBackground: Boolean(settings.runInBackground),
    };
    return invoke("UpdateSettings", payload);
  },
  async resetSettings() {
    return invoke("ResetSettings");
  },
  async getTrayStatus() {
    return invoke("GetTrayStatus");
  },
  async checkForUpdates() {
    return invoke("CheckForUpdates");
  },
  async installLatestUpdate() {
    return invoke("InstallLatestUpdate");
  },
  async getUpdateChannel() {
    return invoke("GetUpdateChannel");
  },
  /**
   * @param {string} channel
   */
  async setUpdateChannel(channel) {
    return invoke("SetUpdateChannel", channel);
  },
  async restartDesktopApp() {
    return invoke("RestartDesktopApp");
  },
  async listOps() {
    return invoke("ListOps");
  },
  /**
   * @param {number} [limit=0] at most this many records, newest first; 0 means all.
   */
  async listHistory(limit = 0) {
    return invoke("ListHistory", Number(limit) || 0);
  },
  /**
   * @param {string} id
   */
  async cancelOp(id) {
    return invoke("CancelOp", String(id || ""));
  },
  /**
   * @param {string} id
   */
  async retryOp(id) {
    return invoke("RetryOp", String(id || ""));
  },
  /**
   * @param {string} project
   */
  async runDoctor(project) {
    return invoke("RunDoctor", project);
  },
  /**
   * @param {string} project
   * @param {string} checkID
   */
  async previewDoctorFix(project, checkID) {
    return invoke("PreviewDoctorFix", project, checkID);
  },
  /**
   * @param {string} project
   * @param {string} checkID
   * @param {string} previewID
   * @param {boolean} confirm
   */
  async applyDoctorFix(project, checkID, previewID, confirm) {
    return invoke("ApplyDoctorFix", project, checkID, previewID, Boolean(confirm));
  },
  /**
   * @param {string} opID
   */
  async cancelDoctorRun(opID) {
    return invoke("CancelDoctorRun", opID);
  },
  /**
   * The in-flight doctor run op id for a project scope, or "" when none is
   * running. The doctor panel queries it on mount so Cancel is available
   * even when the run started before the panel did.
   * @param {string} project
   */
  async runningDoctorOp(project) {
    return invoke("RunningDoctorOp", project);
  },
  /**
   * @param {string} project
   */
  async listToolPresets(project) {
    return invoke("ListToolPresets", project);
  },
  /**
   * Runs a preset by ID with an args map. The palette never sends argv or a
   * command line: the exec argv is backend-owned.
   * @param {string} project
   * @param {string} presetId
   * @param {Record<string, string>} [args={}]
   */
  async runTool(project, presetId, args = {}) {
    return invoke("RunTool", project, presetId, args || {});
  },
  /**
   * Masked connection fact sheet for the project database. The backend
   * carries no password field by construction, and the island renders it
   * through the display allowlist, so secrets never reach the DOM.
   * @param {string} project
   */
  async getDBInfo(project) {
    return invoke("GetDBInfo", project);
  },
  /**
   * Runs the user's own query text. Only the project id, the text and two
   * explicit flags travel: the island never assembles SQL fragments, argv
   * or command lines.
   * @param {string} project
   * @param {string} sql
   * @param {boolean} [allowWrite=false]
   * @param {string} [confirmToken=""]
   */
  async runDBQuery(project, sql, allowWrite = false, confirmToken = "") {
    return invoke(
      "RunDBQuery",
      project,
      String(sql ?? ""),
      Boolean(allowWrite),
      String(confirmToken ?? ""),
    );
  },
  /**
   * Starts an async dump to the dialog-picked destination, returns the op id.
   * Progress arrives on db:progress keyed by (op, generation).
   * @param {string} project
   * @param {string} destPath dialog-picked destination
   */
  async dumpDB(project, destPath) {
    return invoke("DumpDB", project, destPath);
  },
  /**
   * Starts an async import of the dialog-picked source, returns the op id.
   * @param {string} project
   * @param {string} srcPath dialog-picked source
   */
  async importDB(project, srcPath) {
    return invoke("ImportDB", project, srcPath);
  },
  /**
   * @param {string} opId
   */
  async cancelToolRun(opId) {
    return invoke("CancelToolRun", opId);
  },
  /**
   * @param {string} opId
   */
  async cancelDBOp(opId) {
    return invoke("CancelDBOp", opId);
  },
  /**
   * Native save dialog for a dump destination. The island calls dumpDB with
   * exactly the path this returns, never an invented one.
   * @param {string} project
   */
  async pickDumpDestination(project) {
    return invoke("PickDumpDestination", project);
  },
  /**
   * Native open-file dialog for a dump to import. Empty means cancelled.
   */
  async pickImportSource() {
    return invoke("PickImportSource");
  },
  /**
   * The per-project write token the confirm dialog passes back to runDBQuery.
   * The format stays backend-owned; the island only carries the token.
   * @param {string} project
   */
  async getDBWriteConfirmToken(project) {
    return invoke("GetDBWriteConfirmToken", project);
  },
  /**
   * @param {string} project
   */
  async runIntegrityAudit(project) {
    return invoke("RunIntegrityAudit", project);
  },
  /**
   * @param {string} projectID
   * @param {string} sessionID
   * @param {string} runID
   */
  async getAuditResult(projectID, sessionID, runID) {
    return invoke("GetAuditResult", projectID, sessionID, runID);
  },
  /**
   * @param {string} projectID
   * @param {string} sessionID
   * @param {string} fromRunID
   * @param {string} toRunID
   */
  async getAuditDiff(projectID, sessionID, fromRunID, toRunID) {
    return invoke("GetAuditDiff", projectID, sessionID, fromRunID, toRunID);
  },
  /**
   * @param {string} project
   */
  async lastAuditResult(project) {
    return invoke("LastAuditResult", project);
  },
  /**
   * @param {string} opID
   */
  async cancelAudit(opID) {
    return invoke("CancelAudit", opID);
  },
  /**
   * The in-flight integrity-audit op id for a project scope, or "" when
   * none is running. The health panel queries it on mount so Cancel is
   * available even when the run started before the panel did.
   * @param {string} project
   */
  async runningAuditOp(project) {
    return invoke("RunningAuditOp", project);
  },
  /**
   * @param {string} project
   */
  async getToolHistory(project) {
    return invoke("GetToolHistory", project);
  },
  /**
   * @param {string} project
   */
  async listSnapshots(project) {
    return invoke("ListSnapshots", project);
  },
  /**
   * Starts an async snapshot create, returns the op id.
   * @param {string} project
   * @param {string} name
   */
  async createSnapshot(project, name) {
    return invoke("CreateSnapshot", project, String(name ?? ""));
  },
  /**
   * Describes what restoring name would replace and returns a single-use
   * preview token. Writes nothing.
   * @param {string} project
   * @param {string} name snapshot id
   * @param {{ dbOnly?: boolean, mediaOnly?: boolean }} [options={}]
   */
  async previewSnapshotRestore(project, name, options = {}) {
    return invoke(
      "PreviewSnapshotRestore",
      project,
      name,
      Boolean(options.dbOnly),
      Boolean(options.mediaOnly),
    );
  },
  /**
   * Runs the previewed restore. Requires the preview token plus explicit
   * confirm; either missing, the restore never starts.
   * @param {string} project
   * @param {string} name snapshot id
   * @param {string} previewToken
   * @param {boolean} confirm
   * @param {{ dbOnly?: boolean, mediaOnly?: boolean }} [options={}]
   */
  async applySnapshotRestore(project, name, previewToken, confirm, options = {}) {
    return invoke(
      "ApplySnapshotRestore",
      project,
      name,
      previewToken,
      Boolean(confirm),
      Boolean(options.dbOnly),
      Boolean(options.mediaOnly),
    );
  },
  /**
   * Starts an async export of name to the dialog-picked destination.
   * @param {string} project
   * @param {string} name snapshot id
   * @param {string} destPath dialog-picked destination
   */
  async exportSnapshot(project, name, destPath) {
    return invoke("ExportSnapshot", project, name, destPath);
  },
  /**
   * @param {string} opId
   */
  async cancelSnapshotOp(opId) {
    return invoke("CancelSnapshotOp", opId);
  },
  /**
   * Native save dialog for a snapshot export destination. The result carries
   * whether the path already exists, so the island can show its
   * overwrite-confirm before calling exportSnapshot.
   * @param {string} name snapshot id
   */
  async pickExportDestination(name) {
    return invoke("PickExportDestination", name);
  },
  async quit() {
    return invoke("Quit");
  },
};
