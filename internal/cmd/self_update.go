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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const (
	selfUpdateDefaultRepo   = "ddtcorex/govard"
	selfUpdateBinaryName    = "govard"
	selfUpdateChecksumsFile = "checksums.txt"
)

var selfUpdateVersion string

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Upgrade the Govard binary",
	RunE: func(cmd *cobra.Command, args []string) error {
		pterm.DefaultHeader.Println("Govard Self-Update")

		if runtime.GOOS == "windows" {
			return errors.New("self-update is not supported on Windows yet; use a fresh release install")
		}

		if !shouldProceedWithSelfUpdate() {
			pterm.Info.Println("Update cancelled.")
			return nil
		}

		client := &http.Client{Timeout: 30 * time.Second}
		releaseTag := normalizeReleaseTag(selfUpdateVersion)
		repo := selfUpdateRepo()

		var err error
		if releaseTag == "" {
			pterm.Info.Println("Resolving latest release...")
			releaseTag, err = fetchLatestReleaseTag(client, repo)
			if err != nil {
				return err
			}
		}

		pterm.Info.Printf("Target version: %s\n", releaseTag)

		archiveName, binaryNameInArchive, err := buildReleaseAssetName(selfUpdateBinaryName, releaseTag, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return err
		}

		tmpDir, err := os.MkdirTemp("", "govard-self-update-*")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, releaseTag)
		archiveURL := fmt.Sprintf("%s/%s", baseURL, archiveName)
		checksumsURL := fmt.Sprintf("%s/%s", baseURL, selfUpdateChecksumsFile)
		archivePath := filepath.Join(tmpDir, archiveName)

		pterm.Info.Printf("Downloading %s...\n", archiveName)
		if err := downloadFile(client, archiveURL, archivePath); err != nil {
			return err
		}

		checksumsBody, err := downloadText(client, checksumsURL)
		if err != nil {
			return err
		}
		expectedChecksum, err := checksumForAsset(checksumsBody, archiveName)
		if err != nil {
			return err
		}
		if err := verifySHA256(archivePath, expectedChecksum); err != nil {
			return err
		}
		pterm.Success.Println("Checksum verified.")

		extractedBinary, err := extractBinaryFromArchive(archivePath, tmpDir, binaryNameInArchive)
		if err != nil {
			return err
		}

		execPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}
		if resolved, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil {
			execPath = resolved
		}

		if err := replaceBinary(extractedBinary, execPath); err != nil {
			if errors.Is(err, os.ErrPermission) {
				return fmt.Errorf("permission denied replacing %s; re-run with elevated privileges: %w", execPath, err)
			}
			return err
		}

		pterm.Success.Printf("Successfully updated Govard to %s\n", releaseTag)
		pterm.Info.Println("Run 'govard version' to verify.")
		return nil
	},
}

func init() {
	selfUpdateCmd.Flags().StringVar(&selfUpdateVersion, "version", "", "Install a specific version (e.g. v1.0.1)")
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

func selfUpdateRepo() string {
	repo := strings.TrimSpace(os.Getenv("GOVARD_REPO"))
	if repo == "" {
		return selfUpdateDefaultRepo
	}
	return repo
}

func fetchLatestReleaseTag(client *http.Client, repo string) (string, error) {
	cacheFile := filepath.Join(os.TempDir(), "govard-latest-release.json")
	var cacheData struct {
		Tag       string    `json:"tag"`
		FetchedAt time.Time `json:"fetched_at"`
	}
	if b, err := os.ReadFile(cacheFile); err == nil {
		if json.Unmarshal(b, &cacheData) == nil && time.Since(cacheData.FetchedAt) < time.Hour {
			return cacheData.Tag, nil
		}
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
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

	tag := normalizeReleaseTag(release.TagName)
	if tag == "" {
		return "", errors.New("latest release response did not include tag_name")
	}

	cacheData.Tag = tag
	cacheData.FetchedAt = time.Now()
	if b, err := json.Marshal(cacheData); err == nil {
		_ = os.WriteFile(cacheFile, b, 0644)
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

	versionNoPrefix := strings.TrimPrefix(normalizeReleaseTag(releaseTag), "v")
	if versionNoPrefix == "" {
		return "", "", errors.New("release tag is empty")
	}

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
	defer gzReader.Close()

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

		if err := os.Chmod(outPath, 0o755); err != nil {
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
	defer reader.Close()

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

		if err := os.Chmod(outPath, 0o755); err != nil {
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
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
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

	if err := os.Chmod(tempPath, 0o755); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("set executable bit on temp file: %w", err)
	}

	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace binary at %s: %w", targetPath, err)
	}

	return nil
}

func shouldProceedWithSelfUpdate() bool {
	override := strings.ToLower(strings.TrimSpace(os.Getenv("GOVARD_SELF_UPDATE_CONFIRM")))
	switch override {
	case "1", "true", "yes", "y":
		pterm.Info.Println("Auto-confirmed via GOVARD_SELF_UPDATE_CONFIRM.")
		return true
	case "0", "false", "no", "n":
		pterm.Info.Println("Auto-cancelled via GOVARD_SELF_UPDATE_CONFIRM.")
		return false
	}

	if !stdinIsTerminal() {
		pterm.Info.Println("Non-interactive session detected; skipping update. Set GOVARD_SELF_UPDATE_CONFIRM=yes to force.")
		return false
	}

	msg, _ := pterm.DefaultInteractiveConfirm.Show("Do you want to proceed with the update?")
	return msg
}

// NormalizeReleaseTagForTest exposes normalizeReleaseTag for tests in /tests.
func NormalizeReleaseTagForTest(tag string) string {
	return normalizeReleaseTag(tag)
}

// BuildReleaseAssetNameForTest exposes buildReleaseAssetName for tests in /tests.
func BuildReleaseAssetNameForTest(binaryName, releaseTag, goos, goarch string) (string, string, error) {
	return buildReleaseAssetName(binaryName, releaseTag, goos, goarch)
}

// ChecksumForAssetForTest exposes checksumForAsset for tests in /tests.
func ChecksumForAssetForTest(checksumsBody, assetName string) (string, error) {
	return checksumForAsset(checksumsBody, assetName)
}
