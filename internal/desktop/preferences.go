package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type preferences struct {
	ShellUsers map[string]string `json:"shellUsers"`
	Settings   DesktopSettings   `json:"settings"`
}

var prefsMu sync.Mutex
var cachedPrefs *preferences

func getShellUser(project string) (string, error) {
	if err := ensureProjectName(project); err != nil {
		return "", err
	}
	prefs, err := loadPreferences()
	if err != nil {
		return "", err
	}
	if prefs.ShellUsers == nil {
		return "", nil
	}
	return prefs.ShellUsers[project], nil
}

func setShellUser(project string, user string) error {
	if err := ensureProjectName(project); err != nil {
		return err
	}
	prefs, err := loadPreferences()
	if err != nil {
		return err
	}
	if prefs.ShellUsers == nil {
		prefs.ShellUsers = map[string]string{}
	}
	if user == "" {
		delete(prefs.ShellUsers, project)
	} else {
		prefs.ShellUsers[project] = user
	}
	return savePreferences(prefs)
}

func resetShellUsers() error {
	prefs, err := loadPreferences()
	if err != nil {
		return err
	}
	prefs.ShellUsers = map[string]string{}
	return savePreferences(prefs)
}

func getSettings() (DesktopSettings, error) {
	prefs, err := loadPreferences()
	if err != nil {
		return DesktopSettings{}, err
	}
	return normalizeSettings(prefs.Settings), nil
}

func setSettings(settings DesktopSettings) error {
	prefs, err := loadPreferences()
	if err != nil {
		return err
	}
	prefs.Settings = normalizeSettings(settings)
	return savePreferences(prefs)
}

func resetSettings() error {
	prefs, err := loadPreferences()
	if err != nil {
		return err
	}
	prefs.Settings = normalizeSettings(DesktopSettings{})
	return savePreferences(prefs)
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
				ShellUsers: map[string]string{},
				Settings:   normalizeSettings(DesktopSettings{}),
			}
			return cachedPrefs, nil
		}
		return nil, err
	}

	var prefs preferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return nil, err
	}
	if prefs.ShellUsers == nil {
		prefs.ShellUsers = map[string]string{}
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

	if err := os.WriteFile(path, data, 0644); err != nil {
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
	if err := os.MkdirAll(dir, 0755); err != nil {
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
	if settings.ProxyTarget == "" {
		settings.ProxyTarget = "govard.test"
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

func ensureProjectName(project string) error {
	if project == "" {
		return fmt.Errorf("project name is required")
	}
	return nil
}
