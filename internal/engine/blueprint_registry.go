package engine

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	blueprintRegistryProviderGit  = "git"
	blueprintRegistryProviderHTTP = "http"
)

var blueprintRegistryChecksumLength = 64

var blueprintRegistryHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

func resolveBlueprintsDirForConfig(root string, config Config) (fs.FS, error) {
	if strings.TrimSpace(config.BlueprintRegistry.URL) == "" {
		return findBlueprintsFS(root)
	}

	path, err := resolveBlueprintRegistryBlueprintsDir(config.BlueprintRegistry)
	if err != nil {
		return nil, fmt.Errorf("resolve blueprint registry: %w", err)
	}
	return os.DirFS(path), nil
}

func resolveBlueprintRegistryBlueprintsDir(cfg BlueprintRegistryConfig) (string, error) {
	normalizeBlueprintRegistryConfig(&cfg)
	if err := validateBlueprintRegistryConfig(cfg); err != nil {
		return "", err
	}

	cacheRoot, err := blueprintRegistryCacheRoot()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return "", fmt.Errorf("create blueprint registry cache dir %s: %w", cacheRoot, err)
	}

	cacheKey := blueprintRegistryCacheKey(cfg)
	cacheDir := filepath.Join(cacheRoot, cacheKey)
	blueprintsDir := filepath.Join(cacheDir, "blueprints")
	if info, err := os.Stat(blueprintsDir); err == nil && info.IsDir() {
		return blueprintsDir, nil
	}

	stagingDir, err := os.MkdirTemp(cacheRoot, "registry-*")
	if err != nil {
		return "", fmt.Errorf("create blueprint registry staging dir: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var fetchedBlueprintDir string
	switch cfg.Provider {
	case blueprintRegistryProviderHTTP:
		fetchedBlueprintDir, err = fetchHTTPBlueprints(ctx, cfg, stagingDir)
	case blueprintRegistryProviderGit:
		fetchedBlueprintDir, err = fetchGitBlueprints(ctx, cfg, stagingDir)
	default:
		err = fmt.Errorf("unsupported blueprint registry provider %q", cfg.Provider)
	}
	if err != nil {
		return "", err
	}

	stagedCacheDir := filepath.Join(stagingDir, "cache")
	if err := os.MkdirAll(stagedCacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create staged blueprint cache dir: %w", err)
	}

	stagedBlueprintDir := filepath.Join(stagedCacheDir, "blueprints")
	if err := os.Rename(fetchedBlueprintDir, stagedBlueprintDir); err != nil {
		if err := copyDirectory(fetchedBlueprintDir, stagedBlueprintDir); err != nil {
			return "", fmt.Errorf("stage blueprint cache: %w", err)
		}
	}

	if err := os.Rename(stagedCacheDir, cacheDir); err != nil {
		if info, statErr := os.Stat(blueprintsDir); statErr == nil && info.IsDir() {
			return blueprintsDir, nil
		}
		return "", fmt.Errorf("store blueprint cache %s: %w", cacheDir, err)
	}

	return blueprintsDir, nil
}

func fetchHTTPBlueprints(ctx context.Context, cfg BlueprintRegistryConfig, workspace string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		return "", fmt.Errorf("create blueprint registry request: %w", err)
	}

	resp, err := blueprintRegistryHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download blueprint registry archive: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download blueprint registry archive: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 250<<20))
	if err != nil {
		return "", fmt.Errorf("read blueprint registry archive: %w", err)
	}

	actualChecksum := sha256HexBytes(body)
	if !strings.EqualFold(actualChecksum, cfg.Checksum) {
		return "", fmt.Errorf(
			"blueprint registry checksum mismatch: expected %s, got %s",
			cfg.Checksum,
			actualChecksum,
		)
	}

	extractRoot, err := extractBlueprintArchive(body, filepath.Join(workspace, "http"))
	if err != nil {
		return "", err
	}

	blueprintsDir, err := locateBlueprintsDir(extractRoot)
	if err != nil {
		return "", err
	}

	return blueprintsDir, nil
}

func fetchGitBlueprints(ctx context.Context, cfg BlueprintRegistryConfig, workspace string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is required for blueprint registry provider %q", blueprintRegistryProviderGit)
	}

	repoDir := filepath.Join(workspace, "repo")
	cloneArgs := []string{"clone", "--quiet", "--depth", "1"}
	if cfg.Ref != "" {
		cloneArgs = append(cloneArgs, "--branch", cfg.Ref)
	}
	cloneArgs = append(cloneArgs, cfg.URL, repoDir)

	if err := runGitCommand(ctx, cloneArgs...); err != nil {
		return "", fmt.Errorf("clone blueprint registry: %w", err)
	}

	if cfg.Ref != "" {
		if err := runGitCommand(ctx, "-C", repoDir, "checkout", "--quiet", cfg.Ref); err != nil {
			return "", fmt.Errorf("checkout blueprint registry ref %q: %w", cfg.Ref, err)
		}
	}

	blueprintsDir, err := locateBlueprintsDir(repoDir)
	if err != nil {
		return "", err
	}

	actualChecksum, err := checksumDirectory(blueprintsDir)
	if err != nil {
		return "", fmt.Errorf("compute blueprint registry checksum: %w", err)
	}
	if !strings.EqualFold(actualChecksum, cfg.Checksum) {
		return "", fmt.Errorf(
			"blueprint registry checksum mismatch: expected %s, got %s",
			cfg.Checksum,
			actualChecksum,
		)
	}

	return blueprintsDir, nil
}

func runGitCommand(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message != "" {
		return fmt.Errorf("%w: %s", err, message)
	}
	return err
}

func extractBlueprintArchive(payload []byte, workspace string) (string, error) {
	tarRoot := filepath.Join(workspace, "tar")
	if err := os.MkdirAll(tarRoot, 0o755); err != nil {
		return "", fmt.Errorf("create tar extraction dir: %w", err)
	}
	if err := extractTarGzArchive(payload, tarRoot); err == nil {
		return tarRoot, nil
	}

	zipRoot := filepath.Join(workspace, "zip")
	if err := os.MkdirAll(zipRoot, 0o755); err != nil {
		return "", fmt.Errorf("create zip extraction dir: %w", err)
	}
	if err := extractZipArchive(payload, zipRoot); err == nil {
		return zipRoot, nil
	}

	return "", fmt.Errorf("unsupported blueprint registry archive format (expected .tar.gz or .zip)")
}

func extractTarGzArchive(payload []byte, dest string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer gzReader.Close()

	reader := tar.NewReader(gzReader)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		targetPath, err := safeArchivePath(dest, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, reader); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		}
	}

	return nil
}

func extractZipArchive(payload []byte, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return err
	}

	for _, zipped := range reader.File {
		targetPath, err := safeArchivePath(dest, zipped.Name)
		if err != nil {
			return err
		}

		if zipped.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}

		source, err := zipped.Open()
		if err != nil {
			return err
		}

		target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = source.Close()
			return err
		}
		if _, err := io.Copy(target, source); err != nil {
			_ = source.Close()
			_ = target.Close()
			return err
		}
		if err := source.Close(); err != nil {
			_ = target.Close()
			return err
		}
		if err := target.Close(); err != nil {
			return err
		}
	}

	return nil
}

func safeArchivePath(root string, name string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(name))
	if clean == "." || clean == "" {
		return root, nil
	}
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || clean == ".." {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}

	path := filepath.Join(root, clean)
	if path != root && !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return path, nil
}

func locateBlueprintsDir(root string) (string, error) {
	direct := filepath.Join(root, "blueprints")
	if info, err := os.Stat(direct); err == nil && info.IsDir() {
		return direct, nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("scan blueprint archive root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name(), "blueprints")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("blueprints directory not found in registry payload")
}

func checksumDirectory(root string) (string, error) {
	paths := make([]string, 0, 64)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(paths)
	hasher := sha256.New()
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		fileChecksum := sha256.Sum256(content)
		_, _ = hasher.Write([]byte(filepath.ToSlash(rel)))
		_, _ = hasher.Write([]byte{'\n'})
		_, _ = hasher.Write([]byte(hex.EncodeToString(fileChecksum[:])))
		_, _ = hasher.Write([]byte{'\n'})
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func blueprintRegistryCacheRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory for blueprint cache: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("resolve home directory for blueprint cache: empty home path")
	}
	return filepath.Join(home, ".govard", "blueprint-registry"), nil
}

func blueprintRegistryCacheKey(cfg BlueprintRegistryConfig) string {
	seed := strings.Join([]string{cfg.Provider, cfg.URL, cfg.Ref, cfg.Checksum}, "|")
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func validateBlueprintRegistryConfig(cfg BlueprintRegistryConfig) error {
	hasSettings := strings.TrimSpace(cfg.URL) != "" ||
		strings.TrimSpace(cfg.Provider) != "" ||
		strings.TrimSpace(cfg.Ref) != "" ||
		strings.TrimSpace(cfg.Checksum) != "" ||
		cfg.Trusted
	if !hasSettings {
		return nil
	}

	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("blueprint_registry.url is required when blueprint registry settings are configured")
	}
	if !cfg.Trusted {
		return fmt.Errorf("blueprint_registry.trusted must be true to opt in to remote blueprint registry")
	}
	if strings.TrimSpace(cfg.Checksum) == "" {
		return fmt.Errorf("blueprint_registry.checksum is required for remote blueprint registry")
	}
	if len(cfg.Checksum) != blueprintRegistryChecksumLength || !isHexString(cfg.Checksum) {
		return fmt.Errorf("blueprint_registry.checksum must be a 64-character SHA-256 hex string")
	}

	if cfg.Provider == "" {
		return fmt.Errorf("blueprint_registry.provider is required")
	}
	ref := ProviderRef{
		Kind: ProviderKindBlueprintRegistry,
		Name: cfg.Provider,
	}
	if err := ValidateProviderRef(ref); err != nil {
		return fmt.Errorf("blueprint_registry.provider: %w", err)
	}

	switch cfg.Provider {
	case blueprintRegistryProviderGit:
		return nil
	case blueprintRegistryProviderHTTP:
		lowerURL := strings.ToLower(cfg.URL)
		if !strings.HasPrefix(lowerURL, "http://") && !strings.HasPrefix(lowerURL, "https://") {
			return fmt.Errorf("blueprint_registry.url must use http:// or https:// for provider %q", cfg.Provider)
		}
		return nil
	default:
		return fmt.Errorf("unsupported blueprint registry provider %q (allowed: git, http)", cfg.Provider)
	}
}

func normalizeBlueprintRegistryConfig(cfg *BlueprintRegistryConfig) {
	if cfg == nil {
		return
	}

	cfg.Provider = NormalizeProviderName(cfg.Provider)
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.Ref = strings.TrimSpace(cfg.Ref)
	cfg.Checksum = strings.ToLower(strings.TrimSpace(cfg.Checksum))

	if cfg.Provider == "" && cfg.URL != "" {
		lowerURL := strings.ToLower(cfg.URL)
		if strings.HasPrefix(lowerURL, "http://") || strings.HasPrefix(lowerURL, "https://") {
			cfg.Provider = blueprintRegistryProviderHTTP
		} else {
			cfg.Provider = blueprintRegistryProviderGit
		}
	}
}

func isHexString(value string) bool {
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func copyDirectory(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func sha256HexBytes(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// ResolveBlueprintRegistryForTest exposes registry cache resolution for tests.
func ResolveBlueprintRegistryForTest(cfg BlueprintRegistryConfig) (string, error) {
	return resolveBlueprintRegistryBlueprintsDir(cfg)
}
