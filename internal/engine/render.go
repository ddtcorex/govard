package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"govard/internal/blueprints"
	"govard/internal/conventions"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

func renderTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"emdashRuntimeCommand": buildEmdashRuntimeCommand,
		"nextjsRuntimeCommand": buildNextJSRuntimeCommand,
	}
}

// RenderData holds all data needed for template rendering
type RenderData struct {
	Config                 Config
	RequiresPHP            bool
	NGINXPublic            string
	NGINXTemplate          string
	NginxConfigPath        string
	NginxMageRunMapPath    string
	NginxCustomConfigDir   string
	ApacheDocumentRoot     string
	ApacheConfigDir        string
	ApacheHTTPDConfigPath  string
	ApacheMageRunMapPath   string
	ApacheCustomConfigDir  string
	DatabaseName           string
	ImageRepository        string
	XdebugSessionPattern   string
	SSHAuthSock            string
	HostSSHDir             string
	SafeSSHConfig          string
	HostComposerCacheDir   string
	HostComposerConfigDir  string
	HostComposerAuthPath   string
	HostComposerConfigPath string
	VarnishVclPath         string
	RabbitMQConfPath       string
	PackageManager         string
	ComposerVersion        string
	RuntimeDomainHosts     []string
	HostGovardRootCAPath   string
	WorkDir                string
	HomeWWWData            string
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

func blueprintsFingerprint(blueprintsFS fs.FS) (string, error) {
	hasher := sha256.New()

	if err := fs.WalkDir(blueprintsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		content, err := fs.ReadFile(blueprintsFS, path)
		if err != nil {
			return err
		}

		if _, err := hasher.Write([]byte(path)); err != nil {
			return err
		}
		if _, err := hasher.Write([]byte{0}); err != nil {
			return err
		}
		if _, err := hasher.Write(content); err != nil {
			return err
		}
		if _, err := hasher.Write([]byte{0}); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func projectComposeOverrideFingerprint(root string) (string, error) {
	overridePath := filepath.Join(root, ProjectComposeOverridePath)
	data, err := os.ReadFile(overridePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func projectCustomConfigDirFingerprint(dirPath string) (string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	hasher := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dirPath, name))
		if err != nil {
			return "", err
		}
		hasher.Write([]byte(name))
		hasher.Write([]byte{0})
		hasher.Write(data)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func renderEnvironmentFingerprint() string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK")))
	sb.WriteString("|")
	sb.WriteString(strings.TrimSpace(os.Getenv("HOME")))
	sb.WriteString("|")
	sb.WriteString(strings.TrimSpace(os.Getenv("GOVARD_IMAGE_REPOSITORY")))
	sb.WriteString("|")
	sb.WriteString(strings.TrimSpace(os.Getenv("GOVARD_BLUEPRINTS_DIR")))
	return sb.String()
}

func knownProjectRuntimeHostDomains(projectRoot string, currentConfig Config) []string {
	seen := make(map[string]bool)
	hostMappings := make([]string, 0)

	addMapping := func(mapping string) {
		trimmed := strings.TrimSpace(mapping)
		if trimmed == "" || seen[trimmed] {
			return
		}
		seen[trimmed] = true
		hostMappings = append(hostMappings, trimmed)
	}

	if len(currentConfig.LinkedProjects) == 0 {
		return nil
	}

	entries, err := ReadProjectRegistryEntries()
	if err != nil {
		// Fallback: only include raw domain:ip mappings since we can't resolve project names
		for _, h := range currentConfig.LinkedProjects {
			if strings.Contains(h, ":") {
				addMapping(h)
			}
		}
		return hostMappings
	}

	entryByProjectName := make(map[string]ProjectRegistryEntry, len(entries))
	for _, entry := range entries {
		projectName := strings.TrimSpace(entry.ProjectName)
		if projectName != "" {
			entryByProjectName[projectName] = entry
		}
	}

	cleanRoot := filepath.Clean(strings.TrimSpace(projectRoot))

	for _, host := range currentConfig.LinkedProjects {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}

		// Raw mapping
		if strings.Contains(host, ":") {
			addMapping(host)
			continue
		}

		// It's a project name
		if entry, ok := entryByProjectName[host]; ok {
			entryPath := filepath.Clean(strings.TrimSpace(entry.Path))
			if cleanRoot != "" && entryPath == cleanRoot {
				continue
			}

			var projectDomains []string
			if entryPath != "" {
				if cfg, _, loadErr := LoadConfigFromDir(entryPath, false); loadErr == nil {
					projectDomains = cfg.AllDomains()
				}
			}

			if len(projectDomains) == 0 {
				if entry.Domain != "" {
					projectDomains = append(projectDomains, entry.Domain)
				}
				projectDomains = append(projectDomains, entry.ExtraDomains...)
			}

			for _, d := range projectDomains {
				addMapping(fmt.Sprintf("%s:host-gateway", d))
			}
		}
	}

	sort.Strings(hostMappings)
	return hostMappings
}

func knownProjectRuntimeHostDomainsFingerprint(projectRoot string, currentConfig Config) string {
	domains := knownProjectRuntimeHostDomains(projectRoot, currentConfig)
	if len(domains) == 0 {
		return ""
	}

	hasher := sha256.New()
	for _, domain := range domains {
		_, _ = hasher.Write([]byte(domain))
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func hostGovardRootCAPath() string {
	candidate := filepath.Join(GovardHomeDir(), "ssl", "root.crt")
	info, err := os.Stat(candidate)
	if err != nil || info.IsDir() {
		return ""
	}

	return candidate
}

// RenderBlueprint renders layered blueprints into a single docker-compose file
func RenderBlueprint(root string, config Config) error {
	return RenderBlueprintWithProfile(root, config, config.Profile)
}

// varnishTemplateFramework returns the framework whose Varnish VCL blueprint
// asset should be used for framework. mageos has no VCL of its own - it
// reuses magento2's, since Mage-OS is a drop-in fork with the same runtime
// shape and no Varnish-relevant differences.
func varnishTemplateFramework(framework string) string {
	if framework == "mageos" {
		return "magento2"
	}
	return framework
}

// BlueprintVersion should be incremented whenever architectural changes are made to the embedded blueprints
// to ensure that 'govard env up' re-renders existing environments.
const BlueprintVersion = "1.46"

func RenderBlueprintWithProfile(root string, config Config, profile string) error {
	blueprintsFS, err := resolveBlueprintsDirForConfig(root, config)
	if err != nil {
		return fmt.Errorf("resolve blueprints directory: %w", err)
	}

	NormalizeConfig(&config, root)
	config.Profile = profile

	blueprintFingerprint, err := blueprintsFingerprint(blueprintsFS)
	if err != nil {
		return fmt.Errorf("fingerprint blueprints: %w", err)
	}
	overrideFingerprint, err := projectComposeOverrideFingerprint(root)
	if err != nil {
		return fmt.Errorf("fingerprint compose override: %w", err)
	}
	nginxCustomFingerprint, err := projectCustomConfigDirFingerprint(filepath.Join(root, ProjectNginxCustomDir))
	if err != nil {
		return fmt.Errorf("fingerprint nginx custom config dir: %w", err)
	}
	apacheCustomFingerprint, err := projectCustomConfigDirFingerprint(filepath.Join(root, ProjectApacheCustomDir))
	if err != nil {
		return fmt.Errorf("fingerprint apache custom config dir: %w", err)
	}
	envFingerprint := renderEnvironmentFingerprint()
	packageManager := ResolveNodePackageManager(root)
	runtimeDomainHosts := knownProjectRuntimeHostDomains(root, config)
	runtimeDomainHostsFingerprint := knownProjectRuntimeHostDomainsFingerprint(root, config)
	govardRootCAPath := hostGovardRootCAPath()

	outputPath := ComposeFilePathWithProfile(root, config.ProjectName, profile)
	hashPath := outputPath + ".hash"

	hashData, _ := json.Marshal(config)
	hashSum := sha256.Sum256(append(hashData, []byte(profile+BlueprintVersion+blueprintFingerprint+overrideFingerprint+nginxCustomFingerprint+apacheCustomFingerprint+envFingerprint+packageManager+runtimeDomainHostsFingerprint+govardRootCAPath)...))
	currentHash := hex.EncodeToString(hashSum[:])

	if existingHash, err := os.ReadFile(hashPath); err == nil && string(existingHash) == currentHash {
		if _, statErr := os.Stat(outputPath); statErr == nil {
			pterm.Info.Println("Blueprint unchanged, skipping render")
			return nil
		}
		// Hash matches but the compose file is missing — fall through to re-render.
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
		imageRepo = conventions.DefaultImageRepoPrefix
	}

	// Prepare render data
	renderData := RenderData{
		Config:               config,
		RequiresPHP:          RequiresPHP(config),
		NGINXPublic:          fwConfig.NGINXPUBLIC,
		NGINXTemplate:        fwConfig.NGINXTemplate,
		DatabaseName:         fwConfig.DatabaseName,
		ImageRepository:      imageRepo,
		XdebugSessionPattern: buildXdebugSessionPattern(config.Stack.XdebugSession),
		VarnishVclPath:       filepath.Join(GovardHomeDir(), "varnish", config.ProjectName, "default.vcl"),
		RabbitMQConfPath:     filepath.Join(GovardHomeDir(), "rabbitmq", config.ProjectName, "rabbitmq.conf"),
		PackageManager:       packageManager,
		ComposerVersion:      config.Stack.ComposerVersion,
		RuntimeDomainHosts:   runtimeDomainHosts,
		HostGovardRootCAPath: govardRootCAPath,
		WorkDir:              conventions.DefaultWorkDir,
		HomeWWWData:          conventions.HomeWWWData,
	}
	if config.Stack.WebRoot != "" {
		renderData.NGINXPublic = config.Stack.WebRoot
	}
	renderData.ApacheDocumentRoot = buildContainerDocumentRoot(renderData.NGINXPublic)

	// Any real os.Stat error here (as opposed to "not exist") would already have
	// surfaced from projectCustomConfigDirFingerprint above, so treating every
	// non-nil error as "directory absent" is safe only in that context.
	if info, err := os.Stat(filepath.Join(root, ProjectNginxCustomDir)); err == nil && info.IsDir() {
		renderData.NginxCustomConfigDir = filepath.Join(root, ProjectNginxCustomDir)
	}
	if info, err := os.Stat(filepath.Join(root, ProjectApacheCustomDir)); err == nil && info.IsDir() {
		renderData.ApacheCustomConfigDir = filepath.Join(root, ProjectApacheCustomDir)
	}

	if nginxMapPath, apacheMapPath, err := prepareMagentoRunMappingAssets(config); err != nil {
		return fmt.Errorf("failed to prepare Magento run mapping assets: %w", err)
	} else {
		renderData.NginxMageRunMapPath = nginxMapPath
		renderData.ApacheMageRunMapPath = apacheMapPath
	}
	if nginxConfigPath, apacheHTTPDConfigPath, err := prepareWebServerConfigAssets(blueprintsFS, renderData); err != nil {
		return fmt.Errorf("failed to prepare web server config assets: %w", err)
	} else {
		renderData.NginxConfigPath = nginxConfigPath
		renderData.ApacheHTTPDConfigPath = apacheHTTPDConfigPath
		if apacheHTTPDConfigPath != "" {
			renderData.ApacheConfigDir = filepath.Dir(apacheHTTPDConfigPath)
		}
	}

	renderData.SSHAuthSock = strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		sshDir := filepath.Join(home, ".ssh")
		if info, err := os.Stat(sshDir); err == nil && info.IsDir() {
			renderData.HostSSHDir = sshDir
			renderData.SafeSSHConfig = prepareSafeSSHConfig(sshDir)
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

		// Detect Composer config directory and files
		composerConfigDir := filepath.Join(home, ".composer")
		if info, err := os.Stat(composerConfigDir); err == nil && info.IsDir() {
			renderData.HostComposerConfigDir = composerConfigDir

			authPath := filepath.Join(composerConfigDir, "auth.json")
			if _, err := os.Stat(authPath); err == nil {
				renderData.HostComposerAuthPath = authPath
			}

			configPath := filepath.Join(composerConfigDir, "config.json")
			if _, err := os.Stat(configPath); err == nil {
				renderData.HostComposerConfigPath = configPath
			}
		}
	}

	// Ensure support assets (Varnish, etc)
	if config.Stack.Features.Varnish {
		vclSrc := path.Join(varnishTemplateFramework(config.Framework), "varnish", "default.vcl")
		vclDest := renderData.VarnishVclPath
		vclDestDir := filepath.Dir(vclDest)

		rendered, err := renderTemplateFS(blueprintsFS, vclSrc, renderData)
		if err != nil {
			return fmt.Errorf("failed to render varnish vcl: %w", err)
		}
		if err := os.MkdirAll(vclDestDir, conventions.DefaultDirPerm); err != nil {
			return fmt.Errorf("failed to create varnish dir: %w", err)
		}
		if err := os.WriteFile(vclDest, []byte(rendered), conventions.DefaultFilePerm); err != nil {
			return fmt.Errorf("failed to write varnish vcl: %w", err)
		}
	}

	if config.Stack.Services.Queue == conventions.ServiceRabbitMQ {
		confData, err := fs.ReadFile(blueprintsFS, "includes/rabbitmq/rabbitmq.conf")
		if err != nil {
			return fmt.Errorf("failed to read rabbitmq conf: %w", err)
		}
		confDest := renderData.RabbitMQConfPath
		if err := os.MkdirAll(filepath.Dir(confDest), conventions.DefaultDirPerm); err != nil {
			return fmt.Errorf("failed to create rabbitmq dir: %w", err)
		}
		if err := os.WriteFile(confDest, confData, conventions.DefaultFilePerm); err != nil {
			return fmt.Errorf("failed to write rabbitmq conf: %w", err)
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
	outputPath = ComposeFilePathWithProfile(root, config.ProjectName, profile)
	if err := EnsureComposePathReady(outputPath); err != nil {
		return fmt.Errorf("failed to prepare compose output path: %w", err)
	}
	f, err := os.OpenFile(outputPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, conventions.DefaultFilePerm)
	if err != nil {
		return fmt.Errorf("create compose output %s: %w", outputPath, err)
	}
	defer f.Close()

	// Prune empty root-level maps to avoid validation errors (e.g., "volumes must be a mapping")
	if volumes, ok := merged["volumes"].(map[string]interface{}); ok && len(volumes) == 0 {
		delete(merged, "volumes")
	} else if merged["volumes"] == nil {
		delete(merged, "volumes")
	}

	out, err := yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal rendered compose: %w", err)
	}

	_, err = f.Write(out)
	if err != nil {
		return fmt.Errorf("write compose output %s: %w", outputPath, err)
	}

	_ = os.WriteFile(hashPath, []byte(currentHash), conventions.DefaultFilePerm)

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
	tmpl, err := template.New(path.Base(tmplPath)).Funcs(renderTemplateFuncMap()).ParseFS(bfs, tmplPath)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", tmplPath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, path.Base(tmplPath), data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", tmplPath, err)
	}

	return buf.String(), nil
}

func buildEmdashRuntimeCommand(packageManager string, domain string) string {
	domain = strings.TrimSpace(domain)

	if packageManager == "pnpm" {
		return strings.Join([]string{
			"corepack enable >/dev/null 2>&1 || true;",
			"if ! command -v pnpm >/dev/null 2>&1; then corepack prepare pnpm@latest --activate >/dev/null 2>&1; fi;",
			`if [ ! -d node_modules ] || [ -z "$$(ls -A node_modules 2>/dev/null)" ]; then pnpm install; fi;`,
			fmt.Sprintf("exec pnpm dev --host 0.0.0.0 --port 80 --allowed-hosts %s;", domain),
		}, " ")
	}

	return strings.Join([]string{
		`if [ ! -d node_modules ] || [ -z "$$(ls -A node_modules 2>/dev/null)" ]; then npm install; fi;`,
		fmt.Sprintf("exec npm run dev -- --host 0.0.0.0 --port 80 --allowed-hosts %s;", domain),
	}, " ")
}

// buildNextJSRuntimeCommand installs dependencies if node_modules is
// missing (e.g. wiped independently of a fresh bootstrap) before running
// the dev server, matching emdashRuntimeCommand's resilience.
func buildNextJSRuntimeCommand() string {
	return strings.Join([]string{
		`if [ ! -d node_modules ] || [ -z "$$(ls -A node_modules 2>/dev/null)" ]; then npm install; fi;`,
		`exec npm run dev -- --hostname 0.0.0.0 --port 80;`,
	}, " ")
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

func prepareSafeSSHConfig(hostSSHDir string) string {
	configPath := filepath.Join(hostSSHDir, "config")
	info, err := os.Stat(configPath)
	if err != nil {
		return ""
	}

	// If permissions are too broad (group/world writable), create a safe copy
	if info.Mode().Perm()&0o022 != 0 {
		safeDir := filepath.Join(GovardHomeDir(), "ssh")
		if err := os.MkdirAll(safeDir, conventions.SecretDirPerm); err != nil {
			return ""
		}

		safePath := filepath.Join(safeDir, "config")
		data, err := os.ReadFile(configPath)
		if err != nil {
			return ""
		}

		// Write with 600 permissions
		if err := os.WriteFile(safePath, data, conventions.SecretFilePerm); err != nil {
			return ""
		}

		return safePath
	}

	return ""
}

// renderLegacyBlueprint renders using the old single-file approach
func renderLegacyBlueprint(root string, blueprintsFS fs.FS, config Config) error {
	tmplPath := config.Framework + ".tmpl"

	tmpl, err := template.ParseFS(blueprintsFS, tmplPath)
	if err != nil {
		return fmt.Errorf("parse legacy template %s: %w", tmplPath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return fmt.Errorf("execute legacy template %s: %w", tmplPath, err)
	}

	var merged map[string]interface{}
	if err := yaml.Unmarshal(buf.Bytes(), &merged); err != nil {
		return fmt.Errorf("parse rendered legacy yaml %s: %w", tmplPath, err)
	}

	if err := mergeProjectComposeOverride(root, merged); err != nil {
		return err
	}

	outputPath := ComposeFilePathWithProfile(root, config.ProjectName, config.Profile)
	if err := EnsureComposePathReady(outputPath); err != nil {
		return fmt.Errorf("failed to prepare compose output path: %w", err)
	}

	out, err := yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal legacy rendered compose: %w", err)
	}

	if err := os.WriteFile(outputPath, out, conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write legacy legacy compose output %s: %w", outputPath, err)
	}

	return nil
}
