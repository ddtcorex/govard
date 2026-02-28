package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"govard/internal/blueprints"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

// RenderData holds all data needed for template rendering
type RenderData struct {
	Config               Config
	NGINXPublic          string
	NGINXTemplate        string
	DatabaseName         string
	ImageRepository      string
	XdebugSessionPattern string
	SSHAuthSock          string
	HostSSHDir           string
	HostComposerCacheDir string
}

func findBlueprintsDir(startDir string) (string, error) {
	if envPath := strings.TrimSpace(os.Getenv("GOVARD_BLUEPRINTS_DIR")); envPath != "" {
		if abs, err := filepath.Abs(envPath); err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				return abs, nil
			}
		}
	}

	abs, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	curr := abs
	for {
		// Check both legacy root path and new internal path
		candidates := []string{
			filepath.Join(curr, "blueprints"),
			filepath.Join(curr, "internal", "blueprints", "files"),
		}
		for _, target := range candidates {
			if _, err := os.Stat(target); err == nil {
				return target, nil
			}
		}

		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}

	// Fallback to standard install path
	standard := "/usr/local/share/govard/blueprints"
	if _, err := os.Stat(standard); err == nil {
		return standard, nil
	}

	standard = "/usr/share/govard/blueprints"
	if _, err := os.Stat(standard); err == nil {
		return standard, nil
	}

	// Fallback to binary-relative paths for local builds.
	if executablePath, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executablePath)
		candidates := []string{
			filepath.Join(executableDir, "blueprints"),
			filepath.Join(executableDir, "..", "blueprints"),
		}
		for _, candidate := range candidates {
			clean := filepath.Clean(candidate)
			if _, err := os.Stat(clean); err == nil {
				return clean, nil
			}
		}
	}

	return "", fmt.Errorf("blueprints directory not found")
}

func findBlueprintsFS(startDir string) (fs.FS, error) {
	dir, err := findBlueprintsDir(startDir)
	if err == nil {
		return os.DirFS(dir), nil
	}

	return blueprints.FS, nil
}

// RenderBlueprint renders layered blueprints into a single docker-compose file
func RenderBlueprint(root string, config Config) error {
	return RenderBlueprintWithProfile(root, config, "")
}

func RenderBlueprintWithProfile(root string, config Config, profile string) error {
	blueprintsFS, err := resolveBlueprintsDirForConfig(root, config)
	if err != nil {
		return fmt.Errorf("resolve blueprints directory: %w", err)
	}

	NormalizeConfig(&config)

	outputPath := ComposeFilePathWithProfile(root, config.ProjectName, profile)
	hashPath := outputPath + ".hash"

	hashData, _ := json.Marshal(config)
	hashSum := sha256.Sum256(append(hashData, []byte(profile)...))
	currentHash := hex.EncodeToString(hashSum[:])

	if existingHash, err := os.ReadFile(hashPath); err == nil && string(existingHash) == currentHash {
		pterm.Info.Println("Blueprint unchanged, skipping render")
		return nil
	}

	// Get framework configuration
	fwConfig, ok := GetFrameworkConfig(config.Framework)
	if !ok {
		// Fallback to old single-file approach for backward compatibility
		return renderLegacyBlueprint(root, blueprintsFS, config)
	}

	// Determine image repository
	imageRepo := strings.TrimSpace(os.Getenv("GOVARD_IMAGE_REPOSITORY"))
	if imageRepo == "" {
		imageRepo = "ddtcorex/govard-"
	}

	// Prepare render data
	renderData := RenderData{
		Config:               config,
		NGINXPublic:          fwConfig.NGINXPUBLIC,
		NGINXTemplate:        fwConfig.NGINXTemplate,
		DatabaseName:         fwConfig.DatabaseName,
		ImageRepository:      imageRepo,
		XdebugSessionPattern: buildXdebugSessionPattern(config.Stack.XdebugSession),
	}
	if config.Stack.WebRoot != "" {
		renderData.NGINXPublic = config.Stack.WebRoot
	}

	renderData.SSHAuthSock = strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		sshDir := filepath.Join(home, ".ssh")
		if info, err := os.Stat(sshDir); err == nil && info.IsDir() {
			renderData.HostSSHDir = sshDir
		}

		// Detect Composer cache directory
		composerCacheCandidates := []string{
			filepath.Join(home, ".cache", "composer"),
			filepath.Join(home, ".composer", "cache"),
		}
		for _, candidate := range composerCacheCandidates {
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				renderData.HostComposerCacheDir = candidate
				break
			}
		}
	}

	// Ensure support assets (Varnish, etc)
	if config.Stack.Features.Varnish {
		vclSrc := path.Join(config.Framework, "varnish", "default.vcl")
		vclDestDir := filepath.Join(root, "varnish")
		vclDest := filepath.Join(vclDestDir, "default.vcl")

		rendered, err := renderTemplateFS(blueprintsFS, vclSrc, renderData)
		if err != nil {
			return fmt.Errorf("failed to render varnish vcl: %w", err)
		}
		if err := os.MkdirAll(vclDestDir, 0755); err != nil {
			return fmt.Errorf("failed to create varnish dir: %w", err)
		}
		if err := os.WriteFile(vclDest, []byte(rendered), 0644); err != nil {
			return fmt.Errorf("failed to write varnish vcl: %w", err)
		}
	}

	// Render each include file and merge YAML content
	merged := map[string]interface{}{}
	for _, include := range fwConfig.Includes {
		tmplPath := include

		// Check if file exists
		if _, err := fs.Stat(blueprintsFS, tmplPath); os.IsNotExist(err) {
			continue // Skip missing includes
		}

		rendered, err := renderTemplateFS(blueprintsFS, tmplPath, renderData)
		if err != nil {
			return fmt.Errorf("failed to render %s: %w", include, err)
		}

		if strings.TrimSpace(rendered) == "" {
			continue
		}

		var part map[string]interface{}
		if err := yaml.Unmarshal([]byte(rendered), &part); err != nil {
			return fmt.Errorf("failed to parse rendered yaml %s: %w", include, err)
		}
		if part == nil {
			continue
		}

		MergeMap(merged, part)
	}

	if err := mergeProjectComposeOverride(root, merged); err != nil {
		return err
	}

	// Merge all parts into final output
	config.Profile = profile
	outputPath = ComposeFilePathWithProfile(root, config.ProjectName, profile)
	if err := EnsureComposePathReady(outputPath); err != nil {
		return fmt.Errorf("failed to prepare compose output path: %w", err)
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create compose output %s: %w", outputPath, err)
	}
	defer f.Close()

	out, err := yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal rendered compose: %w", err)
	}

	_, err = f.Write(out)
	if err != nil {
		return fmt.Errorf("write compose output %s: %w", outputPath, err)
	}

	_ = os.WriteFile(hashPath, []byte(currentHash), 0644)

	return nil
}

func mergeProjectComposeOverride(root string, merged map[string]interface{}) error {
	overridePath := filepath.Join(root, ProjectComposeOverridePath)
	data, err := os.ReadFile(overridePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read compose override %s: %w", overridePath, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}

	var override map[string]interface{}
	if err := yaml.Unmarshal(data, &override); err != nil {
		return fmt.Errorf("failed to parse compose override %s: %w", overridePath, err)
	}
	if override == nil {
		return nil
	}

	MergeMap(merged, override)
	return nil
}

// renderTemplateFS renders a single template from an fs.FS
func renderTemplateFS(bfs fs.FS, tmplPath string, data RenderData) (string, error) {
	tmpl, err := template.ParseFS(bfs, tmplPath)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", tmplPath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", tmplPath, err)
	}

	return buf.String(), nil
}

func buildXdebugSessionPattern(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "PHPSTORM"
	}

	parts := strings.Split(raw, ",")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		clean = append(clean, regexp.QuoteMeta(part))
	}
	if len(clean) == 0 {
		return "PHPSTORM"
	}
	if len(clean) == 1 {
		return clean[0]
	}
	return "(" + strings.Join(clean, "|") + ")"
}

// BuildXdebugSessionPatternForTest exposes the session pattern builder for tests.
func BuildXdebugSessionPatternForTest(raw string) string {
	return buildXdebugSessionPattern(raw)
}

// renderLegacyBlueprint renders using the old single-file approach
func renderLegacyBlueprint(root string, blueprintsFS fs.FS, config Config) error {
	tmplPath := config.Framework + ".tmpl"

	tmpl, err := template.ParseFS(blueprintsFS, tmplPath)
	if err != nil {
		return fmt.Errorf("parse legacy template %s: %w", tmplPath, err)
	}

	outputPath := ComposeFilePath(root, config.ProjectName)
	if err := EnsureComposePathReady(outputPath); err != nil {
		return fmt.Errorf("failed to prepare compose output path: %w", err)
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create legacy compose output %s: %w", outputPath, err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, config); err != nil {
		return fmt.Errorf("execute legacy template %s: %w", tmplPath, err)
	}

	return nil
}
