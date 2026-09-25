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
      applyOverrides: false,
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
  async quit() {
    return invoke("Quit");
  },
};
