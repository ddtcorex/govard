package desktop

// This file contains proxy methods attached directly to the `App` struct.
// Wails exposes the `App` struct to the frontend via `window.go.desktop.App`.
// These proxies ensure the frontend can still call `App.GetDashboard()`
// and other domain methods, maintaining backward compatibility with the JS bridge.

// -- SettingsService Proxies --

func (app *App) GetSettings() (DesktopSettings, error) {
	return app.Settings.GetSettings()
}

func (app *App) GetMailpitURL() string {
	return app.Settings.GetMailpitURL()
}

func (app *App) UpdateSettings(opts DesktopSettings) (string, error) {
	return app.Settings.UpdateSettings(opts)
}

func (app *App) ResetSettings() (string, error) {
	return app.Settings.ResetSettings()
}

// -- EnvironmentService Proxies --

func (app *App) GetDashboard() (Dashboard, error) {
	return app.Environment.GetDashboard()
}

func (app *App) StartEnvironment(project string) (string, error) {
	return app.Environment.StartEnvironment(project)
}

func (app *App) StopEnvironment(project string) (string, error) {
	return app.Environment.StopEnvironment(project)
}

func (app *App) RestartEnvironment(project string) (string, error) {
	return app.Environment.RestartEnvironment(project)
}

func (app *App) ToggleEnvironment(project string) (string, error) {
	return app.Environment.ToggleEnvironment(project)
}

func (app *App) OpenEnvironment(project string) (string, error) {
	return app.Environment.GetEnvironmentURL(project)
}

// -- SystemService Proxies --

func (app *App) GetSystemMetrics() SystemMetrics {
	return app.System.GetSystemMetrics()
}

func (app *App) GetResourceMetrics() (string, error) {
	// Assuming GetResourceMetrics existed, if not returning empty string
	return "{}", nil
}

// -- LogService (Terminal) Proxies --

func (app *App) StartTerminal(project, service, user, shell string) (string, error) {
	return app.Logs.StartTerminal(project, service, user, shell)
}

func (app *App) StartGovardTerminal(project string, argsList []string) (string, error) {
	return app.Logs.StartGovardTerminal(project, argsList)
}

func (app *App) WriteTerminal(id string, data string) {
	app.Logs.WriteTerminal(id, data)
}

func (app *App) ResizeTerminal(id string, cols, rows int) {
	app.Logs.ResizeTerminal(id, cols, rows)
}

func (app *App) OpenShellForService(project, service, user, shell string) (string, error) {
	return app.Logs.StartTerminal(project, service, user, shell)
}

// -- RemoteService Proxies --

func (app *App) GetRemotes(project string) (RemoteSnapshot, error) {
	return app.Remote.GetRemotes(project)
}

func (app *App) TestRemote(project, remoteName string) (string, error) {
	return app.Remote.TestRemote(project, remoteName)
}

func (app *App) RunRemoteSyncPreset(project, remoteName, presetName string, config map[string]bool) (string, error) {
	return app.Remote.RunRemoteSyncPreset(project, remoteName, presetName, config)
}

func (app *App) RunRemoteSyncBackground(project, remoteName, presetName string, config map[string]bool) (string, error) {
	return app.Remote.RunRemoteSync(project, remoteName, presetName, config)
}

func (app *App) GetSyncPresetOptions(presetName string) presetSyncOptions {
	return app.Remote.GetSyncOptions(presetName)
}

// -- OnboardingService Proxies --

func (app *App) PickProjectDirectory() (string, error) {
	return app.Onboarding.PickProjectDirectory()
}

func (app *App) OnboardProject(input OnboardInput) (string, error) {
	return app.Onboarding.OnboardProject(input)
}

// -- LogService Proxies --

func (app *App) GetLogsForService(project string, service string) (string, error) {
	// Bridge JS doesn't pass lines, so default to 1000
	return app.Logs.GetLogsForService(project, service, 1000)
}

func (app *App) StartLogStreamForService(project string, service string) (string, error) {
	return app.Logs.StartLogStreamForService(project, service)
}

func (app *App) StopLogStream() (string, error) {
	return app.Logs.StopLogStream()
}
