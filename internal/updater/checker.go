package updater

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pterm/pterm"
)

const updateCheckLatestURLEnvVar = "GOVARD_UPDATE_CHECK_URL"

var (
	updateCheckHTTPClient = &http.Client{Timeout: 2 * time.Second}
	updateCheckNotifier   = func(latestTag, currentVersion string) {
		pterm.Warning.Printf("A new version of Govard is available: %s (current: %s)\n", latestTag, currentVersion)
		pterm.Info.Println("Run 'govard self-update' to upgrade.")
	}
)

func CheckForUpdates(current string) {
	release, err := FetchLatestRelease(updateCheckHTTPClient)
	if err != nil {
		return
	}

	if shouldNotifyUpdate(current, release.Tag) {
		updateCheckNotifier(release.Tag, current)
	}
}

// LatestRelease is the subset of the GitHub releases API response callers need.
type LatestRelease struct {
	Tag  string
	Body string
}

// FetchLatestRelease fetches the latest GitHub release (tag + changelog body),
// honoring the GOVARD_UPDATE_CHECK_URL override. Shared by the CLI update check
// and the desktop app's update check so both stay in sync.
func FetchLatestRelease(client *http.Client) (LatestRelease, error) {
	if client == nil {
		client = updateCheckHTTPClient
	}

	req, err := http.NewRequest(http.MethodGet, LatestReleaseURL(), nil)
	if err != nil {
		return LatestRelease{}, fmt.Errorf("prepare update check request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "govard")

	resp, err := client.Do(req)
	if err != nil {
		return LatestRelease{}, fmt.Errorf("request latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LatestRelease{}, fmt.Errorf("request latest release failed with status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return LatestRelease{}, fmt.Errorf("decode latest release payload: %w", err)
	}

	return LatestRelease{Tag: release.TagName, Body: release.Body}, nil
}

// LatestReleaseURL returns the GitHub releases API URL to check, honoring
// the GOVARD_UPDATE_CHECK_URL override used by tests and self-hosted mirrors.
func LatestReleaseURL() string {
	if override := strings.TrimSpace(os.Getenv(updateCheckLatestURLEnvVar)); override != "" {
		return override
	}
	return "https://api.github.com/repos/ddtcorex/govard/releases/latest"
}

// ShouldNotifyUpdate reports whether latestTag represents a version newer
// than currentVersion. Exported for reuse outside this package (e.g. the
// desktop app); see ShouldNotifyUpdateForTest for the test-only alias.
func ShouldNotifyUpdate(currentVersion, latestTag string) bool {
	return shouldNotifyUpdate(currentVersion, latestTag)
}

func shouldNotifyUpdate(currentVersion, latestTag string) bool {
	latest := strings.TrimSpace(latestTag)
	if latest == "" {
		return false
	}
	current := strings.TrimSpace(currentVersion)
	if current == "" {
		return true
	}

	// If current version is a development build (e.g. 1.31.0-2-gf2a0be7),
	// check if the base version matches the latest tag to avoid redundant warnings.
	if strings.Contains(current, "-") {
		base := strings.SplitN(current, "-", 2)[0]
		if latest == "v"+base {
			return false
		}
	}

	return latest != "v"+current
}

// SetUpdateCheckHTTPClientForTest overrides the HTTP client used by update checks.
func SetUpdateCheckHTTPClientForTest(client *http.Client) func() {
	previous := updateCheckHTTPClient
	if client != nil {
		updateCheckHTTPClient = client
	}
	return func() {
		updateCheckHTTPClient = previous
	}
}

// SetUpdateCheckNotifierForTest overrides update notification side effects.
func SetUpdateCheckNotifierForTest(fn func(latestTag, currentVersion string)) func() {
	previous := updateCheckNotifier
	if fn != nil {
		updateCheckNotifier = fn
	}
	return func() {
		updateCheckNotifier = previous
	}
}

// ShouldNotifyUpdateForTest exposes update comparison logic for tests in /tests.
func ShouldNotifyUpdateForTest(currentVersion, latestTag string) bool {
	return shouldNotifyUpdate(currentVersion, latestTag)
}
