package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"govard/internal/cmd"
)

func TestNormalizeReleaseTagForTest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "already prefixed", in: "v1.2.3", want: "v1.2.3"},
		{name: "without prefix", in: "1.2.3", want: "v1.2.3"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := cmd.NormalizeReleaseTagForTest(tt.in); got != tt.want {
				t.Fatalf("NormalizeReleaseTagForTest(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateReleaseTagForTest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "valid", in: "1.2.3", want: "v1.2.3"},
		{name: "valid prefixed", in: "v2.3.4", want: "v2.3.4"},
		{name: "empty", in: "", wantErr: true},
		{name: "invalid suffix", in: "v1.2.3-rc1", wantErr: true},
		{name: "path traversal", in: "../../1.2.3", wantErr: true},
		{name: "valid beta", in: "v1.60.0-beta.1", want: "v1.60.0-beta.1"},
		{name: "invalid beta missing number", in: "v1.2.3-beta", wantErr: true},
		{name: "invalid beta trailing dot segment", in: "v1.2.3-beta.1.2", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cmd.ValidateReleaseTagForTest(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateReleaseTagForTest(%q) expected error, got none", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateReleaseTagForTest(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("ValidateReleaseTagForTest(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildReleaseAssetNameForTest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		goos       string
		goarch     string
		wantAsset  string
		wantBinary string
		wantErr    bool
	}{
		{
			name:       "linux amd64",
			goos:       "linux",
			goarch:     "amd64",
			wantAsset:  "govard_1.0.1_Linux_amd64.tar.gz",
			wantBinary: "govard",
		},
		{
			name:       "darwin arm64",
			goos:       "darwin",
			goarch:     "arm64",
			wantAsset:  "govard_1.0.1_Darwin_arm64.tar.gz",
			wantBinary: "govard",
		},
		{
			name:       "windows amd64",
			goos:       "windows",
			goarch:     "amd64",
			wantAsset:  "govard_1.0.1_Windows_amd64.zip",
			wantBinary: "govard.exe",
		},
		{
			name:    "unsupported arch",
			goos:    "linux",
			goarch:  "386",
			wantErr: true,
		},
		{
			name:    "invalid release tag",
			goos:    "linux",
			goarch:  "amd64",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			releaseTag := "v1.0.1"
			if tt.name == "invalid release tag" {
				releaseTag = "v1.0.1-rc1"
			}
			gotAsset, gotBinary, err := cmd.BuildReleaseAssetNameForTest("govard", releaseTag, tt.goos, tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("BuildReleaseAssetNameForTest() expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildReleaseAssetNameForTest() unexpected error: %v", err)
			}
			if gotAsset != tt.wantAsset {
				t.Fatalf("asset = %q, want %q", gotAsset, tt.wantAsset)
			}
			if gotBinary != tt.wantBinary {
				t.Fatalf("binary = %q, want %q", gotBinary, tt.wantBinary)
			}
		})
	}
}

func TestChecksumForAssetForTest(t *testing.T) {
	t.Parallel()

	const checksums = `aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  govard_1.0.1_Linux_amd64.tar.gz
bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  govard_1.0.1_Darwin_arm64.tar.gz
`
	got, err := cmd.ChecksumForAssetForTest(checksums, "govard_1.0.1_Darwin_arm64.tar.gz")
	if err != nil {
		t.Fatalf("ChecksumForAssetForTest() error = %v", err)
	}
	want := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if got != want {
		t.Fatalf("checksum = %q, want %q", got, want)
	}
}

func TestChecksumForAssetForTestMissing(t *testing.T) {
	t.Parallel()

	_, err := cmd.ChecksumForAssetForTest("abc  foo.tar.gz\n", "missing.tar.gz")
	if err == nil {
		t.Fatal("ChecksumForAssetForTest() expected error for missing asset")
	}
}

func TestSelfUpdateLatestReleaseURLForTestUsesOverride(t *testing.T) {
	t.Setenv("GOVARD_SELF_UPDATE_LATEST_URL", "http://127.0.0.1:8080/latest/")

	got := cmd.SelfUpdateLatestReleaseURLForTest("ignored/repo")
	want := "http://127.0.0.1:8080/latest"
	if got != want {
		t.Fatalf("latest URL = %q, want %q", got, want)
	}
}

func TestSelfUpdateReleaseBaseURLForTestUsesOverride(t *testing.T) {
	t.Setenv("GOVARD_SELF_UPDATE_RELEASE_BASE_URL", "http://127.0.0.1:8080/releases/")

	got := cmd.SelfUpdateReleaseBaseURLForTest("ignored/repo", "v1.0.2")
	want := "http://127.0.0.1:8080/releases"
	if got != want {
		t.Fatalf("release base URL = %q, want %q", got, want)
	}
}

func TestResolveDesktopUpdateTargetsForTestPrefersSiblingTargetWhenCLIBinaryPathIsKnown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-update is not supported on windows")
	}

	tempDir := t.TempDir()
	pathDir := filepath.Join(tempDir, "path")
	cliDir := filepath.Join(tempDir, "cli")
	if err := os.MkdirAll(pathDir, 0o755); err != nil {
		t.Fatalf("mkdir path dir: %v", err)
	}
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatalf("mkdir cli dir: %v", err)
	}

	pathDesktop := filepath.Join(pathDir, "govard-desktop")
	siblingDesktop := filepath.Join(cliDir, "govard-desktop")
	cliBinary := filepath.Join(cliDir, "govard")
	for _, candidate := range []string{pathDesktop, siblingDesktop, cliBinary} {
		if err := os.WriteFile(candidate, []byte(""), 0o755); err != nil {
			t.Fatalf("write %s: %v", candidate, err)
		}
	}

	t.Setenv("PATH", pathDir)

	got := cmd.ResolveDesktopUpdateTargetsForTest(cliBinary)
	if len(got) != 1 {
		t.Fatalf("expected 1 desktop target, got %d: %v", len(got), got)
	}
	expectedSiblingDesktop := canonicalTestPath(t, siblingDesktop)
	if got[0] != expectedSiblingDesktop {
		t.Fatalf("expected sibling desktop target %q, got %v", expectedSiblingDesktop, got)
	}
}

func TestResolveDesktopUpdateTargetsForTestDeduplicatesTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-update is not supported on windows")
	}

	tempDir := t.TempDir()
	cliDir := filepath.Join(tempDir, "cli")
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatalf("mkdir cli dir: %v", err)
	}

	desktopBinary := filepath.Join(cliDir, "govard-desktop")
	cliBinary := filepath.Join(cliDir, "govard")
	for _, candidate := range []string{desktopBinary, cliBinary} {
		if err := os.WriteFile(candidate, []byte(""), 0o755); err != nil {
			t.Fatalf("write %s: %v", candidate, err)
		}
	}
	t.Setenv("PATH", cliDir)

	got := cmd.ResolveDesktopUpdateTargetsForTest(cliBinary)
	if len(got) != 1 {
		t.Fatalf("expected deduplicated desktop target, got %d: %v", len(got), got)
	}
	expectedDesktopBinary := canonicalTestPath(t, desktopBinary)
	if got[0] != expectedDesktopBinary {
		t.Fatalf("unexpected desktop target %q, want %q", got[0], expectedDesktopBinary)
	}
}

func TestResolveDesktopUpdateTargetsForTestFallsBackToPATHWhenSiblingIsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-update is not supported on windows")
	}

	tempDir := t.TempDir()
	pathDir := filepath.Join(tempDir, "path")
	cliDir := filepath.Join(tempDir, "cli")
	if err := os.MkdirAll(pathDir, 0o755); err != nil {
		t.Fatalf("mkdir path dir: %v", err)
	}
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatalf("mkdir cli dir: %v", err)
	}

	pathDesktop := filepath.Join(pathDir, "govard-desktop")
	cliBinary := filepath.Join(cliDir, "govard")
	for _, candidate := range []string{pathDesktop, cliBinary} {
		if err := os.WriteFile(candidate, []byte(""), 0o755); err != nil {
			t.Fatalf("write %s: %v", candidate, err)
		}
	}

	t.Setenv("PATH", pathDir)
	got := cmd.ResolveDesktopUpdateTargetsForTest(cliBinary)
	if len(got) != 1 {
		t.Fatalf("expected path fallback target, got %d: %v", len(got), got)
	}
	expectedPathDesktop := canonicalTestPath(t, pathDesktop)
	if got[0] != expectedPathDesktop {
		t.Fatalf("expected PATH target %q, got %v", expectedPathDesktop, got)
	}
}

func TestResolveDesktopUpdateTargetsForTestUsesExplicitDesktopTargetEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-update is not supported on windows")
	}

	tempDir := t.TempDir()
	customDesktop := filepath.Join(tempDir, "custom-govard-desktop")
	cliBinary := filepath.Join(tempDir, "govard")
	if err := os.WriteFile(customDesktop, []byte(""), 0o755); err != nil {
		t.Fatalf("write custom desktop target: %v", err)
	}
	if err := os.WriteFile(cliBinary, []byte(""), 0o755); err != nil {
		t.Fatalf("write cli binary: %v", err)
	}

	t.Setenv("GOVARD_SELF_UPDATE_DESKTOP_TARGET", customDesktop)

	got := cmd.ResolveDesktopUpdateTargetsForTest(cliBinary)
	if len(got) != 1 {
		t.Fatalf("expected explicit desktop target, got %d: %v", len(got), got)
	}
	expectedCustomDesktop := canonicalTestPath(t, customDesktop)
	if !slices.Contains(got, expectedCustomDesktop) {
		t.Fatalf("expected explicit target %q in %v", expectedCustomDesktop, got)
	}
}

func canonicalTestPath(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve test path %s: %v", path, err)
	}
	return resolved
}

func TestDetectMixedInstallChannelPairsForTestIncludesConflictingCopies(t *testing.T) {
	localDir := filepath.Join(t.TempDir(), "local-bin")
	systemDir := filepath.Join(t.TempDir(), "system-bin")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatalf("mkdir local dir: %v", err)
	}
	if err := os.MkdirAll(systemDir, 0o755); err != nil {
		t.Fatalf("mkdir system dir: %v", err)
	}

	localGovard := filepath.Join(localDir, "govard")
	systemGovard := filepath.Join(systemDir, "govard")
	if err := os.WriteFile(localGovard, []byte("local"), 0o755); err != nil {
		t.Fatalf("write local govard: %v", err)
	}
	if err := os.WriteFile(systemGovard, []byte("system"), 0o755); err != nil {
		t.Fatalf("write system govard: %v", err)
	}

	pairs := cmd.DetectMixedInstallChannelPairsForTest([]string{"govard"}, localDir, systemDir)
	if len(pairs) != 1 {
		t.Fatalf("expected 1 conflicting pair, got %d: %v", len(pairs), pairs)
	}
	if pairs[0][0] != localGovard || pairs[0][1] != systemGovard {
		t.Fatalf("unexpected pair %v", pairs[0])
	}
}

func TestFetchChannelReleaseTagForTestBetaPicksSemverMax(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0-beta.1"},{"tag_name":"v1.0.0-beta.2"}]`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_SELF_UPDATE_LIST_URL", server.URL)

	tag, err := cmd.FetchChannelReleaseTagForTest(server.Client(), "ignored/repo", "beta")
	if err != nil {
		t.Fatalf("FetchChannelReleaseTagForTest() error = %v", err)
	}
	if tag != "v1.0.0-beta.2" {
		t.Fatalf("tag = %q, want %q", tag, "v1.0.0-beta.2")
	}
}

func TestFetchChannelReleaseTagForTestStableDelegatesToLatest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0"}`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_SELF_UPDATE_LATEST_URL", server.URL)

	tag, err := cmd.FetchChannelReleaseTagForTest(server.Client(), "ignored/repo", "stable")
	if err != nil {
		t.Fatalf("FetchChannelReleaseTagForTest() error = %v", err)
	}
	if tag != "v2.0.0" {
		t.Fatalf("tag = %q, want %q", tag, "v2.0.0")
	}
}

func TestFetchChannelReleaseTagForTestBetaIgnoresDrafts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v3.0.0-beta.9","draft":true},{"tag_name":"v3.0.0-beta.1"}]`))
	}))
	defer server.Close()

	t.Setenv("GOVARD_SELF_UPDATE_LIST_URL", server.URL)

	tag, err := cmd.FetchChannelReleaseTagForTest(server.Client(), "ignored/repo", "beta")
	if err != nil {
		t.Fatalf("FetchChannelReleaseTagForTest() error = %v", err)
	}
	if tag != "v3.0.0-beta.1" {
		t.Fatalf("tag = %q, want %q (draft must be excluded)", tag, "v3.0.0-beta.1")
	}
}

func TestDetectMixedInstallChannelPairsForTestSkipsSameTargetViaSymlink(t *testing.T) {
	localDir := filepath.Join(t.TempDir(), "local-bin")
	systemDir := filepath.Join(t.TempDir(), "system-bin")
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		t.Fatalf("mkdir local dir: %v", err)
	}
	if err := os.MkdirAll(systemDir, 0o755); err != nil {
		t.Fatalf("mkdir system dir: %v", err)
	}

	localGovard := filepath.Join(localDir, "govard")
	systemGovard := filepath.Join(systemDir, "govard")
	if err := os.WriteFile(localGovard, []byte("local"), 0o755); err != nil {
		t.Fatalf("write local govard: %v", err)
	}
	if err := os.Symlink(localGovard, systemGovard); err != nil {
		t.Fatalf("symlink system govard: %v", err)
	}

	pairs := cmd.DetectMixedInstallChannelPairsForTest([]string{"govard"}, localDir, systemDir)
	if len(pairs) != 0 {
		t.Fatalf("expected no conflicting pairs for shared symlink target, got %v", pairs)
	}
}
