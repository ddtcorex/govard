package cmd

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"govard/internal/conventions"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const (
	selfUpdateDefaultRepo           = "ddtcorex/govard"
	selfUpdateBinaryName            = "govard"
	selfUpdateDesktopBinaryName     = "govard-desktop"
	selfUpdateChecksumsFile         = "checksums.txt"
	selfUpdateRepoEnvVar            = "GOVARD_REPO"
	selfUpdateLatestURLEnvVar       = "GOVARD_SELF_UPDATE_LATEST_URL"
	selfUpdateReleaseBaseURLEnvVar  = "GOVARD_SELF_UPDATE_RELEASE_BASE_URL"
	selfUpdateConfirmOverrideEnvVar = "GOVARD_SELF_UPDATE_CONFIRM"
	selfUpdateDesktopTargetEnvVar   = "GOVARD_SELF_UPDATE_DESKTOP_TARGET"
	selfUpdateLocalBinDir           = "/usr/local/bin"
	selfUpdateSystemBinDir          = "/usr/bin"
)

var selfUpdateVersion string
var selfUpdateAssumeYes bool

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Upgrade installed Govard binaries",
	RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Println()
		pterm.NewStyle(pterm.BgLightBlue, pterm.FgBlack, pterm.Bold).Println(" Govard Self-Update ")
		fmt.Println()

		if runtime.GOOS == "windows" {
			return errors.New("self-update is not supported on Windows yet; use a fresh release install")
		}

		if !shouldProceedWithSelfUpdate(selfUpdateAssumeYes) {
			pterm.Info.Println("Update cancelled.")
			return nil
		}

		effectiveAssumeYes := selfUpdateAssumeYes
		if os.Getenv(selfUpdateConfirmOverrideEnvVar) == "true" || os.Getenv(selfUpdateConfirmOverrideEnvVar) == "yes" {
			effectiveAssumeYes = true
		}

		if runtime.GOOS == "linux" && os.Getenv("GOVARD_SKIP_DEP_CHECK") != "true" {
			checkAndFixSystemDependencies(effectiveAssumeYes)
		}

		client := &http.Client{Timeout: 300 * time.Second}
		releaseTag := ""
		repo := selfUpdateRepo()

		var err error
		if strings.TrimSpace(selfUpdateVersion) != "" {
			releaseTag, err = validateReleaseTag(selfUpdateVersion)
			if err != nil {
				return err
			}
		} else {
			pterm.Info.Println("Resolving latest release...")
			releaseTag, err = fetchLatestReleaseTag(client, repo)
			if err != nil {
				return err
			}
		}

		pterm.Info.Printf("Target version: %s\n", releaseTag)

		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}
		if resolved, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil {
			execPath = resolved
		}

		targetsByBinary := map[string][]string{
			selfUpdateBinaryName: {execPath},
		}
		desktopTargets := resolveDesktopUpdateTargets(execPath)
		if len(desktopTargets) > 0 {
			targetsByBinary[selfUpdateDesktopBinaryName] = desktopTargets
			pterm.Info.Printf("Detected %d Govard Desktop target(s) to update.\n", len(desktopTargets))
		} else {
			pterm.Info.Println("No installed Govard Desktop binary detected; skipping desktop update.")
		}

		tmpDir, err := os.MkdirTemp("", "govard-self-update-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		baseURL := selfUpdateReleaseBaseURL(repo, releaseTag)
		checksumsURL := fmt.Sprintf("%s/%s", baseURL, selfUpdateChecksumsFile)
		checksumsBody, err := downloadText(client, checksumsURL)
		if err != nil {
			return err
		}

		// On Linux with dpkg available, prefer .deb package installation
		if runtime.GOOS == "linux" {
			if _, err := exec.LookPath("dpkg"); err == nil {
				pterm.Info.Println("Debian-based system detected — using .deb package for update.")
				if err := installViaDeb(client, checksumsBody, baseURL, releaseTag, tmpDir); err != nil {
					pterm.Warning.Printf("Debian package update failed: %v. Falling back to archive update.\n", err)
				} else {
					pterm.Success.Println("Update complete via Debian package.")
					runPostUpdateHooks(execPath, selfUpdateAssumeYes)
					reportMixedInstallChannels([]string{selfUpdateBinaryName, selfUpdateDesktopBinaryName})
					pterm.Info.Println("Run 'govard version' to verify.")
					return nil
				}
			}
		}

		type updatedTarget struct {
			binaryName string
			path       string
		}
		updatedTargets := []updatedTarget{}
		updateOrder := []string{selfUpdateBinaryName, selfUpdateDesktopBinaryName}
		for _, binaryName := range updateOrder {
			targets := targetsByBinary[binaryName]
			if len(targets) == 0 {
				continue
			}

			archiveName, binaryNameInArchive, err := buildReleaseAssetName(binaryName, releaseTag, runtime.GOOS, runtime.GOARCH)
			if err != nil {
				return err
			}
			archivePath := filepath.Join(tmpDir, archiveName)
			archiveURL := fmt.Sprintf("%s/%s", baseURL, archiveName)

			pterm.Info.Printf("Downloading %s...\n", archiveName)
			var extractedBinary string
			if err := downloadFile(client, archiveURL, archivePath); err != nil {
				if shouldTryLinuxDesktopDebFallback(binaryName, err) {
					pterm.Warning.Printf("Desktop archive %s not found; falling back to Linux package asset.\n", archiveName)
					extractedBinary, err = downloadDesktopBinaryFromLinuxDeb(client, checksumsBody, baseURL, releaseTag, tmpDir)
					if err != nil {
						return err
					}
				} else {
					return err
				}
			} else {
				expectedChecksum, err := checksumForAsset(checksumsBody, archiveName)
				if err != nil {
					return err
				}
				if err := verifySHA256(archivePath, expectedChecksum); err != nil {
					return err
				}
				pterm.Success.Printf("Checksum verified for %s.\n", archiveName)

				extractedBinary, err = extractBinaryFromArchive(archivePath, tmpDir, binaryNameInArchive)
				if err != nil {
					return err
				}
			}

			for _, targetPath := range targets {
				if err := replaceBinary(extractedBinary, targetPath); err != nil {
					if errors.Is(err, os.ErrPermission) {
						return fmt.Errorf("permission denied replacing %s at %s; re-run with elevated privileges: %w", binaryName, targetPath, err)
					}
					return err
				}
				updatedTargets = append(updatedTargets, updatedTarget{binaryName: binaryName, path: targetPath})
			}
		}

		pterm.Success.Printf("Successfully updated Govard to %s\n", releaseTag)
		for _, updated := range updatedTargets {
			pterm.Info.Printf("Updated %s at %s\n", updated.binaryName, updated.path)
		}

		runPostUpdateHooks(execPath, selfUpdateAssumeYes)

		reportMixedInstallChannels([]string{selfUpdateBinaryName, selfUpdateDesktopBinaryName})
		pterm.Info.Println("Run 'govard version' to verify.")
		return nil
	},
}

func init() {
	selfUpdateCmd.Flags().StringVar(&selfUpdateVersion, "version", "", "Install a specific version (e.g. v1.0.1)")
	selfUpdateCmd.Flags().BoolVar(&selfUpdateAssumeYes, "yes", false, "Skip confirmation prompt")
}

func normalizeReleaseTag(tag string) string {
	trimmed := strings.TrimSpace(tag)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "v") {
		return trimmed
	}
	return "v" + trimmed
}

func isValidReleaseTag(tag string) bool {
	// Simple regex to ensure tag follows vX.Y.Z format, avoiding path traversal or command injection
	matched, _ := regexp.MatchString(`^v\d+\.\d+\.\d+$`, tag)
	return matched
}

func validateReleaseTag(tag string) (string, error) {
	normalized := normalizeReleaseTag(tag)
	if normalized == "" {
		return "", errors.New("release tag is empty")
	}
	if !isValidReleaseTag(normalized) {
		return "", fmt.Errorf("invalid release tag: %q", tag)
	}
	return normalized, nil
}

func selfUpdateRepo() string {
	repo := strings.TrimSpace(os.Getenv(selfUpdateRepoEnvVar))
	if repo == "" {
		return selfUpdateDefaultRepo
	}
	return repo
}

func selfUpdateLatestReleaseURL(repo string) string {
	if override := strings.TrimSpace(os.Getenv(selfUpdateLatestURLEnvVar)); override != "" {
		return strings.TrimRight(override, "/")
	}
	return fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
}

func selfUpdateReleaseBaseURL(repo, releaseTag string) string {
	if override := strings.TrimSpace(os.Getenv(selfUpdateReleaseBaseURLEnvVar)); override != "" {
		return strings.TrimRight(override, "/")
	}
	return fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, releaseTag)
}

func fetchLatestReleaseTag(client *http.Client, repo string) (string, error) {
	url := selfUpdateLatestReleaseURL(repo)
	body, err := downloadText(client, url)
	if err != nil {
		return "", err
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal([]byte(body), &release); err != nil {
		return "", fmt.Errorf("decode latest release payload: %w", err)
	}

	tag, err := validateReleaseTag(release.TagName)
	if err != nil {
		return "", fmt.Errorf("invalid release tag received from upstream: %w", err)
	}

	return tag, nil
}

func buildReleaseAssetName(binaryName, releaseTag, goos, goarch string) (string, string, error) {
	if goarch != "amd64" && goarch != "arm64" {
		return "", "", fmt.Errorf("unsupported architecture: %s", goarch)
	}

	var osLabel string
	switch goos {
	case "linux":
		osLabel = "Linux"
	case "darwin":
		osLabel = "Darwin"
	case "windows":
		osLabel = "Windows"
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", goos)
	}

	validReleaseTag, err := validateReleaseTag(releaseTag)
	if err != nil {
		return "", "", err
	}
	versionNoPrefix := strings.TrimPrefix(validReleaseTag, "v")

	if goos == "windows" {
		return fmt.Sprintf("%s_%s_%s_%s.zip", binaryName, versionNoPrefix, osLabel, goarch), binaryName + ".exe", nil
	}
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", binaryName, versionNoPrefix, osLabel, goarch), binaryName, nil
}

func downloadFile(client *http.Client, url, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("prepare download request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "govard-self-update")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download %s failed with status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("write file %s: %w", path, err)
	}
	return nil
}

func downloadText(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("prepare request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "govard-self-update")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request %s failed with status %d: %s", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return string(body), nil
}

func checksumForAsset(checksumsBody, assetName string) (string, error) {
	for _, line := range strings.Split(checksumsBody, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}

		fileField := strings.TrimPrefix(fields[len(fields)-1], "*")
		if fileField == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum not found for %s", assetName)
}

func verifySHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("hash %s: %w", path, err)
	}

	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", path, expected, actual)
	}
	return nil
}

func extractBinaryFromArchive(archivePath, workDir, binaryName string) (string, error) {
	switch {
	case strings.HasSuffix(archivePath, ".tar.gz"):
		return extractBinaryFromTarGz(archivePath, workDir, binaryName)
	case strings.HasSuffix(archivePath, ".zip"):
		return extractBinaryFromZip(archivePath, workDir, binaryName)
	default:
		return "", fmt.Errorf("unsupported archive format: %s", archivePath)
	}
}

func extractBinaryFromTarGz(archivePath, workDir, binaryName string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar stream: %w", err)
		}
		if header.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(header.Name) != binaryName {
			continue
		}

		outPath := filepath.Join(workDir, binaryName+".new")
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
		if err != nil {
			return "", fmt.Errorf("create extracted binary: %w", err)
		}

		_, copyErr := io.Copy(out, tarReader)
		closeErr := out.Close()
		if copyErr != nil {
			return "", fmt.Errorf("extract binary: %w", copyErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close extracted binary: %w", closeErr)
		}

		if err := os.Chmod(outPath, conventions.ExecutablePerm); err != nil {
			return "", fmt.Errorf("set executable bit: %w", err)
		}
		return outPath, nil
	}

	return "", fmt.Errorf("binary %s not found in archive", binaryName)
}

func extractBinaryFromZip(archivePath, workDir, binaryName string) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", fmt.Errorf("open zip archive: %w", err)
	}
	defer func() { _ = reader.Close() }()

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(file.Name) != binaryName {
			continue
		}

		in, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("open zip entry: %w", err)
		}

		outPath := filepath.Join(workDir, binaryName+".new")
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			in.Close()
			return "", fmt.Errorf("create extracted binary: %w", err)
		}

		_, copyErr := io.Copy(out, in)
		closeInErr := in.Close()
		closeOutErr := out.Close()
		if copyErr != nil {
			return "", fmt.Errorf("extract binary: %w", copyErr)
		}
		if closeInErr != nil {
			return "", fmt.Errorf("close zip entry: %w", closeInErr)
		}
		if closeOutErr != nil {
			return "", fmt.Errorf("close extracted binary: %w", closeOutErr)
		}

		if err := os.Chmod(outPath, conventions.ExecutablePerm); err != nil {
			return "", fmt.Errorf("set executable bit: %w", err)
		}
		return outPath, nil
	}

	return "", fmt.Errorf("binary %s not found in archive", binaryName)
}

func replaceBinary(sourcePath, targetPath string) error {
	in, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source binary: %w", err)
	}
	defer in.Close()

	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("ensure target directory: %w", err)
	}

	tempFile, err := os.CreateTemp(targetDir, ".govard-update-*")
	if err != nil {
		return fmt.Errorf("create temp target file: %w", err)
	}
	tempPath := tempFile.Name()

	if _, err := io.Copy(tempFile, in); err != nil {
		tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("copy binary to temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tempPath, conventions.ExecutablePerm); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("set executable bit on temp file: %w", err)
	}

	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace binary at %s: %w", targetPath, err)
	}

	return nil
}

func shouldTryLinuxDesktopDebFallback(binaryName string, err error) bool {
	if binaryName != selfUpdateDesktopBinaryName {
		return false
	}
	if runtime.GOOS != "linux" {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "status 404")
}

func downloadDesktopBinaryFromLinuxDeb(client *http.Client, checksumsBody, baseURL, releaseTag, workDir string) (string, error) {
	versionNoPrefix := strings.TrimPrefix(normalizeReleaseTag(releaseTag), "v")
	if versionNoPrefix == "" {
		return "", errors.New("release tag is empty")
	}

	debAssetName := fmt.Sprintf("%s_%s_linux_%s.deb", selfUpdateBinaryName, versionNoPrefix, runtime.GOARCH)
	debPath := filepath.Join(workDir, debAssetName)
	debURL := fmt.Sprintf("%s/%s", baseURL, debAssetName)
	pterm.Info.Printf("Downloading %s...\n", debAssetName)
	if err := downloadFile(client, debURL, debPath); err != nil {
		return "", err
	}

	expectedChecksum, err := checksumForAsset(checksumsBody, debAssetName)
	if err != nil {
		return "", err
	}
	if err := verifySHA256(debPath, expectedChecksum); err != nil {
		return "", err
	}
	pterm.Success.Printf("Checksum verified for %s.\n", debAssetName)

	return extractBinaryFromDebPackage(debPath, workDir, selfUpdateDesktopBinaryName)
}

func extractBinaryFromDebPackage(debPath, workDir, binaryName string) (string, error) {
	if _, err := exec.LookPath("dpkg-deb"); err != nil {
		return "", fmt.Errorf("desktop fallback requires dpkg-deb to extract %s: %w", debPath, err)
	}

	extractDir := filepath.Join(workDir, "deb-extract-"+binaryName)
	if err := os.RemoveAll(extractDir); err != nil {
		return "", fmt.Errorf("reset deb extract directory: %w", err)
	}
	if err := os.MkdirAll(extractDir, conventions.DefaultDirPerm); err != nil {
		return "", fmt.Errorf("create deb extract directory: %w", err)
	}

	cmd := exec.Command("dpkg-deb", "-x", debPath, extractDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("extract deb package %s: %w: %s", debPath, err, strings.TrimSpace(string(output)))
	}

	candidates := []string{
		filepath.Join(extractDir, "usr", "local", "bin", binaryName),
		filepath.Join(extractDir, "usr", "bin", binaryName),
	}
	for _, candidate := range candidates {
		if selfUpdateFileExists(candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("binary %s not found in package %s", binaryName, debPath)
}

func resolveDesktopUpdateTargets(cliExecutablePath string) []string {
	candidates := resolveDesktopUpdateTargetCandidates(cliExecutablePath)
	targets := normalizeDesktopUpdateTargets(candidates)
	if len(targets) > 0 {
		return targets
	}

	if path, err := exec.LookPath(selfUpdateDesktopBinaryName); err == nil {
		return normalizeDesktopUpdateTargets([]string{path})
	}
	return []string{}
}

func resolveDesktopUpdateTargetCandidates(cliExecutablePath string) []string {
	candidates := []string{}
	if override := strings.TrimSpace(os.Getenv(selfUpdateDesktopTargetEnvVar)); override != "" {
		candidates = append(candidates, override)
	}
	if cliExecutablePath != "" {
		sibling := filepath.Join(filepath.Dir(cliExecutablePath), selfUpdateDesktopBinaryName)
		candidates = append(candidates, sibling)
	}
	return candidates
}

func normalizeDesktopUpdateTargets(candidates []string) []string {
	seen := map[string]bool{}
	targets := []string{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		resolved := candidate
		if path, err := filepath.EvalSymlinks(candidate); err == nil {
			resolved = path
		}
		if !selfUpdateFileExists(resolved) {
			continue
		}
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		targets = append(targets, resolved)
	}
	return targets
}

func selfUpdateFileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func detectMixedInstallChannelPairs(binaryNames []string, localBinDir, systemBinDir string) [][2]string {
	pairs := [][2]string{}
	for _, binaryName := range binaryNames {
		if strings.TrimSpace(binaryName) == "" {
			continue
		}

		localPath := filepath.Join(localBinDir, binaryName)
		systemPath := filepath.Join(systemBinDir, binaryName)
		if !selfUpdateFileExists(localPath) || !selfUpdateFileExists(systemPath) {
			continue
		}

		localResolved := localPath
		if resolved, err := filepath.EvalSymlinks(localPath); err == nil {
			localResolved = resolved
		}

		systemResolved := systemPath
		if resolved, err := filepath.EvalSymlinks(systemPath); err == nil {
			systemResolved = resolved
		}

		if localResolved == systemResolved {
			continue
		}

		pairs = append(pairs, [2]string{localPath, systemPath})
	}
	return pairs
}

func reportMixedInstallChannels(binaryNames []string) {
	if runtime.GOOS != "linux" {
		return
	}

	pairs := detectMixedInstallChannelPairs(binaryNames, selfUpdateLocalBinDir, selfUpdateSystemBinDir)
	if len(pairs) == 0 {
		return
	}

	for _, pair := range pairs {
		pterm.Warning.Printf("Detected conflicting binaries: %s and %s\n", pair[0], pair[1])
	}
	pterm.Warning.Println("Mixed install channels detected (.deb + local install). Use one channel only.")
	pterm.Info.Println("Keep /usr/local/bin as source of truth: `sudo apt remove govard` (or `sudo dpkg -r govard`).")
}

func shouldProceedWithSelfUpdate(assumeYes bool) bool {
	if assumeYes {
		pterm.Info.Println("Auto-confirmed via --yes.")
		return true
	}

	override := strings.ToLower(strings.TrimSpace(os.Getenv(selfUpdateConfirmOverrideEnvVar)))
	switch override {
	case "1", "true", "yes", "y":
		pterm.Info.Printf("Auto-confirmed via %s.\n", selfUpdateConfirmOverrideEnvVar)
		return true
	case "0", "false", "no", "n":
		pterm.Info.Printf("Auto-cancelled via %s.\n", selfUpdateConfirmOverrideEnvVar)
		return false
	}

	if !stdinIsTerminal() {
		pterm.Info.Printf("Non-interactive session detected; skipping update. Set %s=yes or pass --yes to force.\n", selfUpdateConfirmOverrideEnvVar)
		return false
	}

	msg, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).Show("Do you want to proceed with the update?")
	return msg
}

func checkAndFixSystemDependencies(assumeYes bool) {
	if runtime.GOOS != "linux" {
		return
	}

	pterm.Info.Println("Checking system dependencies...")
	var missingDeps []string

	// Check certutil
	if _, err := exec.LookPath("certutil"); err != nil {
		pterm.Warning.Println("  certutil: Not found (Required for automatic browser SSL trust)")
		missingDeps = append(missingDeps, "libnss3-tools")
	} else {
		pterm.Success.Println("  certutil: Found")
	}

	// Check WebKitGTK
	hasWebKit := false
	if out, err := exec.Command("ldconfig", "-p").Output(); err == nil {
		if strings.Contains(string(out), "libwebkit2gtk-4.1") {
			hasWebKit = true
		}
	}
	if !hasWebKit {
		for _, path := range []string{"/usr/lib/x86_64-linux-gnu/libwebkit2gtk-4.1.so.0", "/usr/lib/libwebkit2gtk-4.1.so.0"} {
			if _, err := os.Stat(path); err == nil {
				hasWebKit = true
				break
			}
		}
	}

	if hasWebKit {
		pterm.Success.Println("  WebKitGTK: Found")
	} else {
		pterm.Warning.Println("  WebKitGTK: Not found (Required for Desktop App)")
		missingDeps = append(missingDeps, "libwebkit2gtk-4.1-0")
	}

	if len(missingDeps) > 0 {
		confirm := assumeYes
		if !confirm {
			msg, _ := pterm.DefaultInteractiveConfirm.WithDefaultValue(true).Show(fmt.Sprintf("Do you want to install missing dependencies (%s) automatically?", strings.Join(missingDeps, ", ")))
			confirm = msg
		}

		if confirm {
			pterm.Info.Printf("Installing missing dependencies (%s)...\n", strings.Join(missingDeps, ", "))

			if _, err := exec.LookPath("bash"); err != nil {
				pterm.Error.Printf("Failed to install dependencies: bash not found in PATH\n")
				return
			}

			// We use sudo explicitly for apt-get
			fullCmd := fmt.Sprintf("sudo apt-get update && sudo apt-get install -y %s", strings.Join(missingDeps, " "))
			cmd := exec.Command("bash", "-c", fullCmd)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			if err := cmd.Run(); err != nil {
				pterm.Error.Printf("Failed to install dependencies: %v\n", err)
			} else {
				pterm.Success.Println("Dependencies installed successfully.")
			}
		}
	}
}

func runPostUpdateHooks(govardBin string, assumeYes bool) {
	pterm.Info.Println("Running post-update automation...")

	// 1. Refresh global services
	pterm.Info.Println("Refreshing global services...")
	cmdSvc := exec.Command(govardBin, "svc", "up", "-d", "--remove-orphans")
	cmdSvc.Stdout = io.Discard // Keep it clean
	cmdSvc.Stderr = os.Stderr
	if err := cmdSvc.Run(); err != nil {
		pterm.Warning.Printf("Failed to refresh global services: %v\n", err)
	} else {
		pterm.Success.Println("Global services refreshed.")
	}

	// 2. Configure SSL trust
	pterm.Info.Println("Verifying SSL trust configuration...")
	cmdTrust := exec.Command(govardBin, "doctor", "trust")
	cmdTrust.Stdout = io.Discard
	cmdTrust.Stderr = os.Stderr
	if err := cmdTrust.Run(); err != nil {
		pterm.Warning.Printf("Failed to verify SSL trust: %v\n", err)
	} else {
		pterm.Success.Println("SSL trust verified.")
	}
}

func installViaDeb(client *http.Client, checksumsBody, baseURL, releaseTag, workDir string) error {
	versionNoPrefix := strings.TrimPrefix(normalizeReleaseTag(releaseTag), "v")
	if versionNoPrefix == "" {
		return errors.New("release tag is empty")
	}

	debAssetName := fmt.Sprintf("%s_%s_linux_%s.deb", selfUpdateBinaryName, versionNoPrefix, runtime.GOARCH)
	debPath := filepath.Join(workDir, debAssetName)
	debURL := fmt.Sprintf("%s/%s", baseURL, debAssetName)

	pterm.Info.Printf("Downloading %s...\n", debAssetName)
	if err := downloadFile(client, debURL, debPath); err != nil {
		return fmt.Errorf("download deb asset: %w", err)
	}

	expectedChecksum, err := checksumForAsset(checksumsBody, debAssetName)
	if err != nil {
		return err
	}
	if err := verifySHA256(debPath, expectedChecksum); err != nil {
		return err
	}
	pterm.Success.Printf("Checksum verified for %s.\n", debAssetName)

	pterm.Info.Println("Installing Debian package (requires sudo)...")

	// Run dpkg -i
	cmd := exec.Command("sudo", "dpkg", "-i", debPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		pterm.Warning.Printf("dpkg -i failed: %v. Attempting to fix missing dependencies...\n", err)

		// Run apt-get install -f
		cmdFix := exec.Command("sudo", "apt-get", "install", "-f", "-y")
		cmdFix.Stdout = os.Stdout
		cmdFix.Stderr = os.Stderr
		cmdFix.Stdin = os.Stdin
		if fixErr := cmdFix.Run(); fixErr != nil {
			return fmt.Errorf("package installation failed even after fixing dependencies: %w", fixErr)
		}
	}

	return nil
}

// NormalizeReleaseTagForTest exposes normalizeReleaseTag for tests in /tests.
func NormalizeReleaseTagForTest(tag string) string {
	return normalizeReleaseTag(tag)
}

// ValidateReleaseTagForTest exposes validateReleaseTag for tests in /tests.
func ValidateReleaseTagForTest(tag string) (string, error) {
	return validateReleaseTag(tag)
}

// BuildReleaseAssetNameForTest exposes buildReleaseAssetName for tests in /tests.
func BuildReleaseAssetNameForTest(binaryName, releaseTag, goos, goarch string) (string, string, error) {
	return buildReleaseAssetName(binaryName, releaseTag, goos, goarch)
}

// ChecksumForAssetForTest exposes checksumForAsset for tests in /tests.
func ChecksumForAssetForTest(checksumsBody, assetName string) (string, error) {
	return checksumForAsset(checksumsBody, assetName)
}

// SelfUpdateLatestReleaseURLForTest exposes selfUpdateLatestReleaseURL for tests in /tests.
func SelfUpdateLatestReleaseURLForTest(repo string) string {
	return selfUpdateLatestReleaseURL(repo)
}

// SelfUpdateReleaseBaseURLForTest exposes selfUpdateReleaseBaseURL for tests in /tests.
func SelfUpdateReleaseBaseURLForTest(repo, releaseTag string) string {
	return selfUpdateReleaseBaseURL(repo, releaseTag)
}

// ResolveDesktopUpdateTargetsForTest exposes desktop update target discovery for tests in /tests.
func ResolveDesktopUpdateTargetsForTest(cliExecutablePath string) []string {
	return resolveDesktopUpdateTargets(cliExecutablePath)
}

// DetectMixedInstallChannelPairsForTest exposes mixed channel detection for tests in /tests.
func DetectMixedInstallChannelPairsForTest(binaryNames []string, localBinDir, systemBinDir string) [][2]string {
	return detectMixedInstallChannelPairs(binaryNames, localBinDir, systemBinDir)
}
