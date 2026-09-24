package desktop

import (
	"encoding/json"
	"govard/internal/conventions"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type preferences struct {
	Settings DesktopSettings `json:"settings"`
}

var prefsMu sync.Mutex
var cachedPrefs *preferences

func getSettingsInternal() (DesktopSettings, error) {
	prefs, err := loadPreferences()
	if err != nil {
		return DesktopSettings{}, err
	}
	return normalizeSettings(prefs.Settings), nil
}

func setSettingsInternal(settings DesktopSettings) error {
	prefs, err := loadPreferences()
	if err != nil {
		return err
	}
	prefs.Settings = normalizeSettings(settings)
	return savePreferences(prefs)
}

func resetSettingsInternal() error {
	prefs, err := loadPreferences()
	if err != nil {
		return err
	}
	prefs.Settings = normalizeSettings(DesktopSettings{RunInBackground: true})
	return savePreferences(prefs)
}

// SettingsService methods

// TrayStatus tells the Settings page whether "run in background" can actually
// hide the window, which needs a tray to bring it back.
type TrayStatus struct {
	Available bool `json:"available"`
}

func (s *SettingsService) GetTrayStatus() (status TrayStatus, err error) {
	defer RecoverPanic(&err, "GetTrayStatus")
	return TrayStatus{Available: TrayHostAvailable()}, nil
}

func (s *SettingsService) GetSettings() (settings DesktopSettings, err error) {
	defer RecoverPanic(&err, "GetSettings")
	return getSettingsInternal()
}

func (s *SettingsService) GetMailpitURL() string {
	return buildProxyURL("mail")
}

func (s *SettingsService) UpdateSettings(settings DesktopSettings) (res string, err error) {
	defer RecoverPanic(&err, "UpdateSettings")
	if err := setSettingsInternal(settings); err != nil {
		return "", err
	}
	return "Settings updated", nil
}

func (s *SettingsService) ResetSettings() (res string, err error) {
	defer RecoverPanic(&err, "ResetSettings")
	if err := resetSettingsInternal(); err != nil {
		return "", err
	}
	return "Settings reset", nil
}

func loadPreferences() (*preferences, error) {
	prefsMu.Lock()
	defer prefsMu.Unlock()

	if cachedPrefs != nil {
		return cachedPrefs, nil
	}

	path, err := preferencesPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cachedPrefs = &preferences{
				Settings: normalizeSettings(DesktopSettings{RunInBackground: true}),
			}
			return cachedPrefs, nil
		}
		return nil, err
	}

	var prefs preferences
	prefs.Settings.RunInBackground = true
	if err := json.Unmarshal(data, &prefs); err != nil {
		return nil, err
	}
	prefs.Settings = normalizeSettings(prefs.Settings)
	cachedPrefs = &prefs
	return cachedPrefs, nil
}

func savePreferences(prefs *preferences) error {
	prefsMu.Lock()
	defer prefsMu.Unlock()

	path, err := preferencesPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, conventions.DefaultFilePerm); err != nil {
		return err
	}
	cachedPrefs = prefs
	return nil
}

func preferencesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".govard")
	if err := os.MkdirAll(dir, conventions.DefaultDirPerm); err != nil {
		return "", err
	}
	return filepath.Join(dir, "desktop-preferences.json"), nil
}

func normalizeSettings(settings DesktopSettings) DesktopSettings {
	switch settings.Theme {
	case "light", "dark", "system":
	default:
		settings.Theme = "system"
	}
	if settings.Theme == "" {
		settings.Theme = "system"
	}
	settings.ProxyTarget = sanitizeProxyTarget(settings.ProxyTarget)
	settings.PreferredBrowser = strings.TrimSpace(settings.PreferredBrowser)
	settings.CodeEditor = strings.TrimSpace(settings.CodeEditor)
	settings.DBClientPreference = strings.TrimSpace(strings.ToLower(settings.DBClientPreference))
	if settings.ProxyTarget == "" {
		settings.ProxyTarget = "govard.test"
	}
	if settings.DBClientPreference != "pma" && settings.DBClientPreference != "desktop" {
		settings.DBClientPreference = "pma"
	}
	return settings
}

func sanitizeProxyTarget(raw string) string {
	target := strings.TrimSpace(raw)
	target = strings.TrimPrefix(target, "https://")
	target = strings.TrimPrefix(target, "http://")
	target = strings.TrimSuffix(target, "/")
	target = strings.Trim(target, ".")
	return target
}
