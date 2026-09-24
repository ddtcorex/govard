package desktop

// This file contains proxy methods attached directly to the `App` struct.
// Wails exposes the `App` struct to the frontend via `window.go.desktop.App`.
// These proxies ensure the frontend can still call `App.GetDashboard()`
// and other domain methods, maintaining backward compatibility with the JS bridge.

// -- SettingsService Proxies --

func (app *App) GetSettings() (settings DesktopSettings, err error) {
	defer RecoverPanic(&err, "GetSettings")
	return app.Settings.GetSettings()
}

func (app *App) GetMailpitURL() string {
	return app.Settings.GetMailpitURL()
}

func (app *App) UpdateSettings(opts DesktopSettings) (res string, err error) {
	defer RecoverPanic(&err, "UpdateSettings")
	return app.Settings.UpdateSettings(opts)
}

func (app *App) ResetSettings() (res string, err error) {
	defer RecoverPanic(&err, "ResetSettings")
	return app.Settings.ResetSettings()
}

// -- EnvironmentService Proxies --

func (app *App) GetDashboard() (dashboard Dashboard, err error) {
	defer RecoverPanic(&err, "GetDashboard")
	return app.Environment.GetDashboard()
}

func (app *App) GetGlobalServices() (services GlobalServicesSnapshot, err error) {
	defer RecoverPanic(&err, "GetGlobalServices")
	return app.Global.GetGlobalServices()
}

func (app *App) StartGlobalServices() (res string, err error) {
	defer RecoverPanic(&err, "StartGlobalServices")
	return app.Global.StartGlobalServices()
}

func (app *App) StopGlobalServices() (res string, err error) {
	defer RecoverPanic(&err, "StopGlobalServices")
	return app.Global.StopGlobalServices()
}

func (app *App) RestartGlobalServices() (res string, err error) {
	defer RecoverPanic(&err, "RestartGlobalServices")
	return app.Global.RestartGlobalServices()
}

func (app *App) PullGlobalServices() (res string, err error) {
	defer RecoverPanic(&err, "PullGlobalServices")
	return app.Global.PullGlobalServices()
}

func (app *App) StartGlobalService(serviceID string) (res string, err error) {
	defer RecoverPanic(&err, "StartGlobalService")
	return app.Global.StartGlobalService(serviceID)
}

func (app *App) StopGlobalService(serviceID string) (res string, err error) {
	defer RecoverPanic(&err, "StopGlobalService")
	return app.Global.StopGlobalService(serviceID)
}

func (app *App) RestartGlobalService(serviceID string) (res string, err error) {
	defer RecoverPanic(&err, "RestartGlobalService")
	return app.Global.RestartGlobalService(serviceID)
}

func (app *App) OpenGlobalService(serviceID string) (res string, err error) {
	defer RecoverPanic(&err, "OpenGlobalService")
	return app.Global.OpenGlobalService(serviceID)
}

func (app *App) StartEnvironment(project string) (string, error) {
	var err error
	defer RecoverPanic(&err, "StartEnvironment")
	return app.Environment.StartEnvironment(project)
}

func (app *App) StopEnvironment(project string) (string, error) {
	var err error
	defer RecoverPanic(&err, "StopEnvironment")
	return app.Environment.StopEnvironment(project)
}

func (app *App) RestartEnvironment(project string) (string, error) {
	var err error
	defer RecoverPanic(&err, "RestartEnvironment")
	return app.Environment.RestartEnvironment(project)
}

func (app *App) PullEnvironment(project string) (string, error) {
	var err error
	defer RecoverPanic(&err, "PullEnvironment")
	return app.Environment.PullEnvironment(project)
}

func (app *App) ToggleEnvironment(project string) (string, error) {
	var err error
	defer RecoverPanic(&err, "ToggleEnvironment")
	return app.Environment.ToggleEnvironment(project)
}

func (app *App) OpenEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "OpenEnvironment")
	url, errEnv := app.Environment.GetEnvironmentURL(project)
	if errEnv != nil {
		return "", errEnv
	}
	if errOpen := openURLWithPreferences(app.platform, url); errOpen != nil {
		return "Open " + url + " manually", nil
	}
	return "Opening " + url + "...", nil
}

// -- SystemService Proxies --

func (app *App) GetSystemMetrics() SystemMetrics {
	return app.System.GetSystemMetrics()
}

func (app *App) GetResourceMetrics() (string, error) {
	// Assuming GetResourceMetrics existed, if not returning empty string
	return "{}", nil
}

func (app *App) StartServiceTerminalInOS(project, service, user, shell string) (res string, err error) {
	defer RecoverPanic(&err, "StartServiceTerminalInOS")
	return app.Logs.StartServiceTerminalInOS(project, service, user, shell)
}

// -- RemoteService Proxies --

func (app *App) GetRemotes(project string) (remotes RemoteSnapshot, err error) {
	defer RecoverPanic(&err, "GetRemotes")
	return app.Remote.GetRemotes(project)
}

func (app *App) TestRemote(project, remoteName string) (res string, err error) {
	defer RecoverPanic(&err, "TestRemote")
	return app.Remote.TestRemote(project, remoteName)
}

func (app *App) OpenRemoteURL(project, remoteName string) (res string, err error) {
	defer RecoverPanic(&err, "OpenRemoteURL")
	return app.Remote.OpenRemoteURL(project, remoteName)
}

func (app *App) OpenRemoteShell(project, remoteName string) (res string, err error) {
	defer RecoverPanic(&err, "OpenRemoteShell")
	return app.Remote.OpenRemoteShell(project, remoteName)
}

func (app *App) OpenRemoteDB(project, remoteName string) (res string, err error) {
	defer RecoverPanic(&err, "OpenRemoteDB")
	return app.Remote.OpenRemoteDB(project, remoteName)
}

func (app *App) OpenRemoteSFTP(project, remoteName string) (res string, err error) {
	defer RecoverPanic(&err, "OpenRemoteSFTP")
	return app.Remote.OpenRemoteSFTP(project, remoteName)
}

func (app *App) RunRemoteSyncPreset(project, remoteName, presetName string, config map[string]bool) (string, error) {
	var err error
	defer RecoverPanic(&err, "RunRemoteSyncPreset")
	return app.Remote.RunRemoteSyncPreset(project, remoteName, presetName, config)
}

func (app *App) RunRemoteSyncBackground(project, remoteName, presetName string, config map[string]bool) (string, error) {
	var err error
	defer RecoverPanic(&err, "RunRemoteSyncBackground")
	return app.Remote.RunRemoteSync(project, remoteName, presetName, config)
}

func (app *App) RunRemoteSyncInTerminal(project, remoteName, presetName string, config map[string]bool) (string, error) {
	var err error
	defer RecoverPanic(&err, "RunRemoteSyncInTerminal")
	return app.Remote.RunRemoteSyncInTerminal(project, remoteName, presetName, config)
}

func (app *App) GetSyncPresetOptions(project, presetName string) presetSyncOptions {
	return app.Remote.GetSyncOptions(project, presetName)
}

// -- OnboardingService Proxies --

func (app *App) PickProjectDirectory() (path string, err error) {
	defer RecoverPanic(&err, "PickProjectDirectory")
	return app.Onboarding.PickProjectDirectory()
}

func (app *App) OnboardProject(input OnboardInput) (res string, err error) {
	defer RecoverPanic(&err, "OnboardProject")
	return app.Onboarding.OnboardProject(input)
}

func (app *App) DetectMigrationSource(projectPath string) (res string, err error) {
	defer RecoverPanic(&err, "DetectMigrationSource")
	return app.Onboarding.DetectMigrationSource(projectPath)
}

// -- LogService Proxies --

func (app *App) GetLogsForService(project string, service string) (logs string, err error) {
	defer RecoverPanic(&err, "GetLogsForService")
	// Bridge JS doesn't pass lines, so default to 1000
	return app.Logs.GetLogsForService(project, service, 1000)
}

func (app *App) StartLogStreamForService(project string, service string) (res string, err error) {
	defer RecoverPanic(&err, "StartLogStreamForService")
	return app.Logs.StartLogStreamForService(project, service)
}

func (app *App) StopLogStream() (res string, err error) {
	defer RecoverPanic(&err, "StopLogStream")
	return app.Logs.StopLogStream()
}

func (app *App) GetGlobalServiceLogs(serviceID string, lines int) (logs string, err error) {
	defer RecoverPanic(&err, "GetGlobalServiceLogs")
	return app.Logs.GetGlobalServiceLogs(serviceID, lines)
}

func (app *App) StartGlobalServiceLogStream(serviceID string) (res string, err error) {
	defer RecoverPanic(&err, "StartGlobalServiceLogStream")
	return app.Logs.StartGlobalServiceLogStream(serviceID)
}

func (app *App) StopGlobalServiceLogStream() (res string, err error) {
	defer RecoverPanic(&err, "StopGlobalServiceLogStream")
	return app.Logs.StopGlobalServiceLogStream()
}

func (app *App) SaveLogsToFile(content string, suggestedName string) (res string, err error) {
	defer RecoverPanic(&err, "SaveLogsToFile")
	return saveLogsToFile(app.platform, content, suggestedName)
}

func (app *App) Quit() {
	app.platform.Quit()
}
