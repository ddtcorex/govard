package tests

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"govard/internal/desktop"

	"gopkg.in/yaml.v3"
)

// linuxApplicationIDSetting matches the LinuxOptions.ApplicationID assignment
// inside internal/desktop. The value is captured so the test can insist on the
// shared constant instead of a second literal that can drift from the installed
// launcher.
var linuxApplicationIDSetting = regexp.MustCompile(`ApplicationID:\s*([A-Za-z_][A-Za-z0-9_.]*)`)

// TestLinuxDesktopConfiguresApplicationID guards the one thing that makes
// GNOME group the window with its launcher on Wayland.
//
// GNOME Shell resolves a Wayland window by taking the GTK application id from
// the surface and looking up the desktop file id "<application id>.desktop"
// (gnome-shell src/shell-window-tracker.c, get_app_from_id); the
// WM_CLASS/StartupWMClass heuristics run before that but only ever see X11
// values. Wails derives "org.wails.<sanitised Name>" when LinuxOptions has no
// ApplicationID, which matches no installed desktop file: the window becomes a
// window-backed app and the dock shows a second, differently named icon next to
// the pinned launcher.
func TestLinuxDesktopConfiguresApplicationID(t *testing.T) {
	matches, err := filepath.Glob("../internal/desktop/*.go")
	if err != nil {
		t.Fatalf("glob internal/desktop: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("internal/desktop has no Go files")
	}

	var settings []string
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, match := range linuxApplicationIDSetting.FindAllStringSubmatch(string(data), -1) {
			settings = append(settings, match[1])
		}
	}

	if len(settings) == 0 {
		t.Fatalf("internal/desktop never sets LinuxOptions.ApplicationID: "+
			"Wails then advertises the derived %q id, which matches no desktop file id "+
			"and leaves the Wayland window ungrouped in the dock", "org.wails.<name>")
	}
	if len(settings) > 1 {
		t.Errorf("LinuxOptions.ApplicationID is set %d times (%v), want a single shared constant", len(settings), settings)
	}
	for _, value := range settings {
		if value != "DesktopApplicationID" {
			t.Errorf("LinuxOptions.ApplicationID = %q, want the DesktopApplicationID constant", value)
		}
	}
}

// TestLinuxDesktopLauncherMatchesApplicationID keeps the launcher on disk in
// lockstep with the id the app advertises, and rejects an id GTK would refuse:
// an invalid id is not an error the user sees, it is a silent fallback to the
// derived "org.wails.<name>" id, which is the ungrouped state this contract
// exists to prevent.
func TestLinuxDesktopLauncherMatchesApplicationID(t *testing.T) {
	appID := desktop.DesktopApplicationID
	if err := validateApplicationIDShape(appID); err != nil {
		t.Fatalf("DesktopApplicationID %q is not a valid GTK application id: %v", appID, err)
	}

	entries, err := filepath.Glob("../packaging/linux/*.desktop")
	if err != nil {
		t.Fatalf("glob packaging/linux: %v", err)
	}
	launcher := appID + ".desktop"
	if len(entries) != 1 || filepath.Base(entries[0]) != launcher {
		t.Fatalf("packaging/linux holds %v, want exactly %q: the desktop file id must be "+
			"\"<application id>.desktop\" or GNOME cannot group the window with its launcher", entries, launcher)
	}

	entry, err := parseDesktopEntry(entries[0])
	if err != nil {
		t.Fatalf("parse %s: %v", entries[0], err)
	}
	for key, want := range map[string]string{
		"Type":           "Application",
		"Name":           "Govard",
		"Exec":           "govard-desktop",
		"Icon":           "govard",
		"StartupWMClass": appID,
	} {
		if got := entry[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// TestLinuxDesktopLauncherIsPackaged proves the deb carries the launcher under
// the same id, since a launcher that only exists in the working tree cannot
// group anything on a user's machine.
func TestLinuxDesktopLauncherIsPackaged(t *testing.T) {
	data, err := os.ReadFile("../.goreleaser.yml")
	if err != nil {
		t.Fatalf("read GoReleaser config: %v", err)
	}
	var config releaseNFPMConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse GoReleaser config: %v", err)
	}

	desktopPkg := findReleaseNFPM(t, config.NFPMS, "govard-desktop")
	launcher := desktop.DesktopApplicationID + ".desktop"
	if !releaseNFPMHasContent(desktopPkg, "./packaging/linux/"+launcher, "/usr/share/applications/"+launcher) {
		t.Errorf("desktop package does not install packaging/linux/%s to /usr/share/applications/%s", launcher, launcher)
	}

	var packaged []string
	for _, content := range desktopPkg.Contents {
		if strings.HasPrefix(content.Source, "./packaging/linux/") {
			packaged = append(packaged, content.Source)
		}
	}
	if len(packaged) != 1 {
		t.Errorf("desktop package ships %v, want exactly one launcher", packaged)
	}
}

// validateApplicationIDShape mirrors g_application_id_is_valid(), the rule GTK
// asserts on and Wails pre-checks.
func validateApplicationIDShape(id string) error {
	if id == "" {
		return errors.New("id is empty")
	}
	if len(id) > 255 {
		return fmt.Errorf("id is %d characters, the maximum is 255", len(id))
	}
	elements := strings.Split(id, ".")
	if len(elements) < 2 {
		return fmt.Errorf("id has %d element(s), want at least 2 separated by '.'", len(elements))
	}
	for _, element := range elements {
		if element == "" {
			return fmt.Errorf("id has an empty element")
		}
		if element[0] >= '0' && element[0] <= '9' {
			return fmt.Errorf("element %q starts with a digit", element)
		}
		if strings.Trim(element, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "" {
			return fmt.Errorf("element %q has a character outside [A-Za-z0-9_-]", element)
		}
	}
	return nil
}

// parseDesktopEntry reads the [Desktop Entry] group into a key to value map.
func parseDesktopEntry(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entry := map[string]string{}
	inGroup := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(line, "["):
			inGroup = line == "[Desktop Entry]"
			continue
		}
		if !inGroup {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("line %q is not a key=value pair", line)
		}
		if _, duplicate := entry[key]; duplicate {
			return nil, fmt.Errorf("key %q appears twice", key)
		}
		entry[key] = value
	}
	return entry, nil
}
