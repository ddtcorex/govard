package tests

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"govard/internal/updater"
)

func TestShouldNotifyUpdateForTest(t *testing.T) {
	testCases := []struct {
		name           string
		currentVersion string
		latestTag      string
		want           bool
	}{
		{
			name:           "no latest tag",
			currentVersion: "1.0.0",
			latestTag:      "",
			want:           false,
		},
		{
			name:           "empty current version",
			currentVersion: "",
			latestTag:      "v1.0.1",
			want:           true,
		},
		{
			name:           "same semantic version",
			currentVersion: "1.0.1",
			latestTag:      "v1.0.1",
			want:           false,
		},
		{
			name:           "different semantic version",
			currentVersion: "1.0.1",
			latestTag:      "v1.0.2",
			want:           true,
		},
		{
			name:           "trimmed values",
			currentVersion: " 1.0.1 ",
			latestTag:      " v1.0.1 ",
			want:           false,
		},
		{
			name:           "dev build of same version",
			currentVersion: "1.1.0-2-gf2a0be7",
			latestTag:      "v1.1.0",
			want:           false,
		},
		{
			name:           "dev build of newer version available",
			currentVersion: "1.1.0-dev",
			latestTag:      "v1.2.0",
			want:           true,
		},
		{
			name:           "pre-release build of same version",
			currentVersion: "1.1.0-beta1",
			latestTag:      "v1.1.0",
			want:           false,
		},
		{
			name:           "official beta tag superseded by stable release of same base version",
			currentVersion: "1.60.0-beta.1",
			latestTag:      "v1.60.0",
			want:           true,
		},
		{
			name:           "official beta tag same version no notify",
			currentVersion: "1.60.0-beta.1",
			latestTag:      "v1.60.0-beta.1",
			want:           false,
		},
		{
			name:           "official beta tag newer beta available",
			currentVersion: "1.60.0-beta.1",
			latestTag:      "v1.60.0-beta.2",
			want:           true,
		},
		{
			name:           "official beta tag latest is stale older beta",
			currentVersion: "1.60.0-beta.2",
			latestTag:      "v1.60.0-beta.1",
			want:           false,
		},
		{
			name:           "latest is older than current (rollback/local build scenario)",
			currentVersion: "1.0.2",
			latestTag:      "v1.0.1",
			want:           false,
		},
		{
			name:           "latest equals current with v prefix already on current",
			currentVersion: "v1.0.1",
			latestTag:      "v1.0.1",
			want:           false,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			got := updater.ShouldNotifyUpdateForTest(testCase.currentVersion, testCase.latestTag)
			if got != testCase.want {
				t.Fatalf("ShouldNotifyUpdateForTest(%q, %q) = %v, want %v", testCase.currentVersion, testCase.latestTag, got, testCase.want)
			}
		})
	}
}

func TestCheckForUpdatesNotifiesWhenNewVersionAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_UPDATE_CHECK_URL", server.URL)
	defer updater.SetUpdateCheckHTTPClientForTest(server.Client())()

	notifyCalls := 0
	var gotLatestTag, gotCurrentVersion string
	defer updater.SetUpdateCheckNotifierForTest(func(latestTag, currentVersion string) {
		notifyCalls++
		gotLatestTag = latestTag
		gotCurrentVersion = currentVersion
	})()

	updater.CheckForUpdates("1.0.0")

	if notifyCalls != 1 {
		t.Fatalf("expected notifier to be called once, got %d", notifyCalls)
	}
	if gotLatestTag != "v9.9.9" {
		t.Fatalf("latest tag = %q, want %q", gotLatestTag, "v9.9.9")
	}
	if gotCurrentVersion != "1.0.0" {
		t.Fatalf("current version = %q, want %q", gotCurrentVersion, "1.0.0")
	}
}

func TestCheckForUpdatesSkipsNotifierWhenVersionMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.0.0"}`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_UPDATE_CHECK_URL", server.URL)
	defer updater.SetUpdateCheckHTTPClientForTest(server.Client())()

	notifyCalls := 0
	defer updater.SetUpdateCheckNotifierForTest(func(latestTag, currentVersion string) {
		notifyCalls++
	})()

	updater.CheckForUpdates("1.0.0")

	if notifyCalls != 0 {
		t.Fatalf("expected notifier to be skipped, got %d call(s)", notifyCalls)
	}
}

func TestCheckForUpdatesSkipsNotifierOnInvalidPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_UPDATE_CHECK_URL", server.URL)
	defer updater.SetUpdateCheckHTTPClientForTest(server.Client())()

	notifyCalls := 0
	defer updater.SetUpdateCheckNotifierForTest(func(latestTag, currentVersion string) {
		notifyCalls++
	})()

	updater.CheckForUpdates("1.0.0")

	if notifyCalls != 0 {
		t.Fatalf("expected notifier to be skipped for invalid payload, got %d call(s)", notifyCalls)
	}
}

func TestNewestReleaseTagPicksSemverMax(t *testing.T) {
	tag, ok := updater.NewestReleaseTag([]string{"v1.60.0-beta.1", "v1.59.5", "v1.60.0-beta.2"})
	if !ok {
		t.Fatal("expected a newest tag")
	}
	if tag != "v1.60.0-beta.2" {
		t.Fatalf("newest tag = %q, want %q", tag, "v1.60.0-beta.2")
	}
}

func TestNewestReleaseTagIgnoresInvalidSemver(t *testing.T) {
	tag, ok := updater.NewestReleaseTag([]string{"not-a-version", "v1.0.0"})
	if !ok || tag != "v1.0.0" {
		t.Fatalf("NewestReleaseTag = (%q, %v), want (\"v1.0.0\", true)", tag, ok)
	}
}

func TestNewestReleaseTagNoneQualify(t *testing.T) {
	_, ok := updater.NewestReleaseTag([]string{"garbage"})
	if ok {
		t.Fatal("expected ok=false when no tag is valid semver")
	}
}

func TestFetchChannelReleaseBetaPicksSemverMaxAcrossOutOfOrderList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Chronologically the stable patch is listed first (published most
		// recently) but is semver-older than the second beta - this is the
		// list-order trap NewestReleaseTag must not fall into.
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.59.5","body":"stable patch"},
			{"tag_name":"v1.60.0-beta.2","body":"newer beta"},
			{"tag_name":"v1.60.0-beta.1","body":"older beta"}
		]`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_UPDATE_CHECK_LIST_URL", server.URL)

	release, err := updater.FetchChannelRelease(server.Client(), updater.ChannelBeta)
	if err != nil {
		t.Fatalf("FetchChannelRelease() error = %v", err)
	}
	if release.Tag != "v1.60.0-beta.2" {
		t.Fatalf("release.Tag = %q, want %q", release.Tag, "v1.60.0-beta.2")
	}
	if release.Body != "newer beta" {
		t.Fatalf("release.Body = %q, want %q", release.Body, "newer beta")
	}
}

func TestFetchChannelReleaseStableUsesLatestEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.59.0"}`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_UPDATE_CHECK_URL", server.URL)

	release, err := updater.FetchChannelRelease(server.Client(), updater.ChannelStable)
	if err != nil {
		t.Fatalf("FetchChannelRelease() error = %v", err)
	}
	if release.Tag != "v1.59.0" {
		t.Fatalf("release.Tag = %q, want %q", release.Tag, "v1.59.0")
	}
}

func TestCheckForUpdatesUsesPersistedBetaChannel(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	if err := updater.SetChannel(updater.ChannelBeta); err != nil {
		t.Fatalf("SetChannel(beta) error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v9.9.9-beta.1"}]`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_UPDATE_CHECK_LIST_URL", server.URL)
	defer updater.SetUpdateCheckHTTPClientForTest(server.Client())()

	notifyCalls := 0
	var gotLatestTag string
	defer updater.SetUpdateCheckNotifierForTest(func(latestTag, currentVersion string) {
		notifyCalls++
		gotLatestTag = latestTag
	})()

	updater.CheckForUpdates("1.0.0")

	if notifyCalls != 1 {
		t.Fatalf("expected notifier to be called once, got %d", notifyCalls)
	}
	if gotLatestTag != "v9.9.9-beta.1" {
		t.Fatalf("latest tag = %q, want %q", gotLatestTag, "v9.9.9-beta.1")
	}
}
