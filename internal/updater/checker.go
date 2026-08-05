package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"golang.org/x/mod/semver"
)

const updateCheckLatestURLEnvVar = "GOVARD_UPDATE_CHECK_URL"

var officialBetaTagPattern = regexp.MustCompile(`^\d+\.\d+\.\d+-beta\.\d+$`)

var (
	updateCheckHTTPClient = &http.Client{Timeout: 2 * time.Second}
	updateCheckNotifier   = func(latestTag, currentVersion string) {
		pterm.Warning.Printf("A new version of Govard is available: %s (current: %s)\n", latestTag, currentVersion)
		pterm.Info.Println("Run 'govard self-update' to upgrade.")
	}
)

func CheckForUpdates(current string) {
	release, err := FetchChannelRelease(updateCheckHTTPClient, GetChannel())
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

	// If current version is a local development build (e.g. 1.31.0-2-gf2a0be7,
	// or a "-dev" suffix), check if the base version matches the latest tag to
	// avoid redundant warnings. This does NOT apply to an official "-beta.N"
	// release tag, where the suffix is semantically meaningful (a prerelease
	// of that base version is strictly older than the plain release).
	if strings.Contains(current, "-") && !officialBetaTagPattern.MatchString(current) {
		base := strings.SplitN(current, "-", 2)[0]
		if latest == "v"+base {
			return false
		}
	}

	currentSemver := "v" + strings.TrimPrefix(current, "v")
	if semver.IsValid(latest) && semver.IsValid(currentSemver) {
		return semver.Compare(latest, currentSemver) > 0
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

const updateCheckListURLEnvVar = "GOVARD_UPDATE_CHECK_LIST_URL"

// ReleasesListURL returns the GitHub releases-list API URL used to discover
// beta releases, honoring the GOVARD_UPDATE_CHECK_LIST_URL override.
func ReleasesListURL() string {
	if override := strings.TrimSpace(os.Getenv(updateCheckListURLEnvVar)); override != "" {
		return override
	}
	return "https://api.github.com/repos/ddtcorex/govard/releases"
}

// NewestReleaseTag returns the semver-greatest tag among tags, ignoring any
// that aren't valid semver (e.g. malformed or missing the "v" prefix). ok is
// false when no tag qualifies.
func NewestReleaseTag(tags []string) (tag string, ok bool) {
	for _, candidate := range tags {
		candidate = strings.TrimSpace(candidate)
		if !semver.IsValid(candidate) {
			continue
		}
		if !ok || semver.Compare(candidate, tag) > 0 {
			tag = candidate
			ok = true
		}
	}
	return tag, ok
}

// FetchChannelRelease returns the release Govard should treat as "latest" for
// the given channel. ChannelStable behaves exactly like FetchLatestRelease
// (GitHub already excludes prereleases from /releases/latest, so stable users
// see no behavior change). ChannelBeta fetches the full release list and
// picks the semver-greatest tag, so a stable patch published after a beta tag
// can't win just because it's chronologically newer.
func FetchChannelRelease(client *http.Client, channel string) (LatestRelease, error) {
	if channel != ChannelBeta {
		return FetchLatestRelease(client)
	}
	return fetchNewestRelease(client)
}

func fetchNewestRelease(client *http.Client) (LatestRelease, error) {
	if client == nil {
		client = updateCheckHTTPClient
	}

	req, err := http.NewRequest(http.MethodGet, ReleasesListURL(), nil)
	if err != nil {
		return LatestRelease{}, fmt.Errorf("prepare release list request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "govard")

	resp, err := client.Do(req)
	if err != nil {
		return LatestRelease{}, fmt.Errorf("request release list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LatestRelease{}, fmt.Errorf("request release list failed with status %d", resp.StatusCode)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		Draft   bool   `json:"draft"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return LatestRelease{}, fmt.Errorf("decode release list payload: %w", err)
	}

	bodyByTag := map[string]string{}
	tags := make([]string, 0, len(releases))
	for _, release := range releases {
		if release.Draft {
			continue
		}
		tag := strings.TrimSpace(release.TagName)
		tags = append(tags, tag)
		bodyByTag[tag] = release.Body
	}

	newestTag, ok := NewestReleaseTag(tags)
	if !ok {
		return LatestRelease{}, errors.New("no valid releases found in release list")
	}
	return LatestRelease{Tag: newestTag, Body: bodyByTag[newestTag]}, nil
}
