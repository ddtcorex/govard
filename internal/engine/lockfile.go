package engine

import (
	"fmt"
	"govard/internal/conventions"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	lockFilePathEnvVar = conventions.EnvGovardLock
	LockFileName       = conventions.LockFileName
)

type LockDependencies struct {
	ReadDockerVersion        func() (string, error)
	ReadDockerComposeVersion func() (string, error)
	ReadServiceImages        func(composePath string) (map[string]string, error)
	ReadCurrentUser          func() (string, error)
	Now                      func() time.Time
}

type LockFile struct {
	Version     int               `yaml:"version"`
	GeneratedAt string            `yaml:"generated_at"`
	GeneratedBy string            `yaml:"generated_by,omitempty"`
	Govard      LockGovardInfo    `yaml:"govard"`
	Host        LockHostInfo      `yaml:"host"`
	Project     LockProjectInfo   `yaml:"project"`
	Stack       LockStackInfo     `yaml:"stack"`
	Services    map[string]string `yaml:"services,omitempty"`
}

type LockGovardInfo struct {
	Version string `yaml:"version"`
}

type LockHostInfo struct {
	OS                   string `yaml:"os"`
	Arch                 string `yaml:"arch"`
	DockerVersion        string `yaml:"docker_version"`
	DockerComposeVersion string `yaml:"docker_compose_version"`
}

type LockProjectInfo struct {
	Name             string            `yaml:"name"`
	Domain           string            `yaml:"domain,omitempty"`
	ExtraDomains     []string          `yaml:"extra_domains,omitempty"`
	StoreDomains     map[string]string `yaml:"store_domains,omitempty"`
	Framework        string            `yaml:"framework,omitempty"`
	FrameworkVersion string            `yaml:"framework_version,omitempty"`
	Profile          string            `yaml:"profile,omitempty"`
}

type LockStackInfo struct {
	PHPVersion    string `yaml:"php_version,omitempty"`
	NodeVersion   string `yaml:"node_version,omitempty"`
	DB            string `yaml:"db,omitempty"`
	DBVersion     string `yaml:"db_version,omitempty"`
	CacheVersion  string `yaml:"cache_version,omitempty"`
	SearchVersion string `yaml:"search_version,omitempty"`
	QueueVersion  string `yaml:"queue_version,omitempty"`
}

type LockCompliance struct {
	Compliant  bool
	Mismatches []string
}

func LockFilePath(root string) string {
	if override := strings.TrimSpace(os.Getenv(lockFilePathEnvVar)); override != "" {
		return filepath.Clean(override)
	}
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			cleanRoot = cwd
		}
	}
	if cleanRoot == "" {
		return LockFileName
	}
	return filepath.Join(filepath.Clean(cleanRoot), LockFileName)
}

func BuildLockFileFromConfig(cwd string, config Config, govardVersion string, deps LockDependencies) (LockFile, error) {
	resolvedDeps := resolveLockDependencies(deps)

	dockerVersion, err := resolvedDeps.ReadDockerVersion()
	if err != nil {
		return LockFile{}, fmt.Errorf("resolve docker version: %w", err)
	}
	dockerComposeVersion, err := resolvedDeps.ReadDockerComposeVersion()
	if err != nil {
		return LockFile{}, fmt.Errorf("resolve docker compose version: %w", err)
	}

	composePath := ComposeFilePathWithProfile(cwd, normalizeLockProjectName(config, cwd), config.Profile)
	serviceImages, err := resolvedDeps.ReadServiceImages(composePath)
	if err != nil {
		return LockFile{}, fmt.Errorf("resolve service images: %w", err)
	}

	now := resolvedDeps.Now().UTC().Format(time.RFC3339)
	lock := LockFile{
		Version:     1,
		GeneratedAt: now,
		GeneratedBy: getGeneratedBy(resolvedDeps),
		Govard: LockGovardInfo{
			Version: strings.TrimSpace(govardVersion),
		},
		Host: LockHostInfo{
			OS:                   runtime.GOOS,
			Arch:                 runtime.GOARCH,
			DockerVersion:        strings.TrimSpace(dockerVersion),
			DockerComposeVersion: strings.TrimSpace(dockerComposeVersion),
		},
		Project: LockProjectInfo{
			Name:             normalizeLockProjectName(config, cwd),
			Domain:           strings.TrimSpace(config.Domain),
			ExtraDomains:     config.ExtraDomains,
			StoreDomains:     flattenStoreDomainsForLock(config.StoreDomains),
			Framework:        strings.TrimSpace(config.Framework),
			FrameworkVersion: strings.TrimSpace(config.FrameworkVersion),
			Profile:          strings.TrimSpace(config.Profile),
		},
		Stack: LockStackInfo{
			PHPVersion:    strings.TrimSpace(config.Stack.PHPVersion),
			NodeVersion:   strings.TrimSpace(config.Stack.NodeVersion),
			DB:            strings.TrimSpace(config.Stack.Services.DB),
			DBVersion:     strings.TrimSpace(config.Stack.DBVersion),
			CacheVersion:  strings.TrimSpace(config.Stack.CacheVersion),
			SearchVersion: strings.TrimSpace(config.Stack.SearchVersion),
			QueueVersion:  strings.TrimSpace(config.Stack.QueueVersion),
		},
		Services: normalizeServiceImages(serviceImages),
	}
	return lock, nil
}

func WriteLockFile(path string, lock LockFile) error {
	cleanPath := filepath.Clean(path)
	if cleanPath == "" {
		return fmt.Errorf("lock file path is required")
	}
	if lock.Version == 0 {
		lock.Version = 1
	}
	if strings.TrimSpace(lock.GeneratedAt) == "" {
		lock.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	lock.Services = normalizeServiceImages(lock.Services)

	payload, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("marshal lock file: %w", err)
	}
	payload = append(payload, '\n')

	if err := os.MkdirAll(filepath.Dir(cleanPath), conventions.DefaultDirPerm); err != nil {
		return fmt.Errorf("create lock file directory: %w", err)
	}
	if err := os.WriteFile(cleanPath, payload, conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	return nil
}

func ReadLockFile(path string) (LockFile, error) {
	cleanPath := filepath.Clean(path)
	payload, err := os.ReadFile(cleanPath)
	if err != nil {
		return LockFile{}, fmt.Errorf("read lock file: %w", err)
	}
	lock := LockFile{}
	if err := yaml.Unmarshal(payload, &lock); err != nil {
		return LockFile{}, fmt.Errorf("parse lock file: %w", err)
	}
	if lock.Version == 0 {
		lock.Version = 1
	}
	lock.Services = normalizeServiceImages(lock.Services)
	return lock, nil
}

func CompareLockFile(expected LockFile, current LockFile, ignoreFields []string) LockCompliance {
	ignored := make(map[string]bool, len(ignoreFields))
	for _, field := range ignoreFields {
		ignored[strings.TrimSpace(strings.ToLower(field))] = true
	}

	mismatches := make([]string, 0)
	appendMismatch := func(field, expectedValue, currentValue string) {
		if ignored[strings.ToLower(field)] {
			return
		}
		if strings.TrimSpace(expectedValue) == strings.TrimSpace(currentValue) {
			return
		}
		mismatches = append(mismatches, fmt.Sprintf("%s mismatch: expected=%q current=%q", field, expectedValue, currentValue))
	}

	appendMismatch("govard.version", expected.Govard.Version, current.Govard.Version)
	appendMismatch("host.os", expected.Host.OS, current.Host.OS)
	appendMismatch("host.arch", expected.Host.Arch, current.Host.Arch)
	appendMismatch("host.docker_version", expected.Host.DockerVersion, current.Host.DockerVersion)
	appendMismatch("host.docker_compose_version", expected.Host.DockerComposeVersion, current.Host.DockerComposeVersion)
	appendMismatch("project.name", expected.Project.Name, current.Project.Name)
	appendMismatch("project.domain", expected.Project.Domain, current.Project.Domain)

	// extra_domains comparison (order independent)
	if !ignored["project.extra_domains"] {
		e := make([]string, len(expected.Project.ExtraDomains))
		copy(e, expected.Project.ExtraDomains)
		sort.Strings(e)
		c := make([]string, len(current.Project.ExtraDomains))
		copy(c, current.Project.ExtraDomains)
		sort.Strings(c)
		expectedStr := strings.Join(e, ",")
		currentStr := strings.Join(c, ",")
		if expectedStr != currentStr {
			mismatches = append(mismatches, fmt.Sprintf("project.extra_domains mismatch: expected=%q current=%q", expectedStr, currentStr))
		}
	}

	// store_domains comparison (deep equality via sorted map)
	if !ignored["project.store_domains"] {
		expectedHosts := make([]string, 0, len(expected.Project.StoreDomains))
		for h := range expected.Project.StoreDomains {
			expectedHosts = append(expectedHosts, h)
		}
		sort.Strings(expectedHosts)

		currentHosts := make([]string, 0, len(current.Project.StoreDomains))
		for h := range current.Project.StoreDomains {
			currentHosts = append(currentHosts, h)
		}
		sort.Strings(currentHosts)

		expectedStr := ""
		for _, h := range expectedHosts {
			expectedStr += fmt.Sprintf("%s:%s,", h, expected.Project.StoreDomains[h])
		}
		currentStr := ""
		for _, h := range currentHosts {
			currentStr += fmt.Sprintf("%s:%s,", h, current.Project.StoreDomains[h])
		}

		if expectedStr != currentStr {
			mismatches = append(mismatches, fmt.Sprintf("project.store_domains mismatch: expected=%q current=%q", expectedStr, currentStr))
		}
	}

	appendMismatch("project.framework", expected.Project.Framework, current.Project.Framework)
	appendMismatch("project.framework_version", expected.Project.FrameworkVersion, current.Project.FrameworkVersion)
	appendMismatch("stack.php_version", expected.Stack.PHPVersion, current.Stack.PHPVersion)
	appendMismatch("stack.node_version", expected.Stack.NodeVersion, current.Stack.NodeVersion)
	appendMismatch("stack.db", expected.Stack.DB, current.Stack.DB)
	appendMismatch("stack.db_version", expected.Stack.DBVersion, current.Stack.DBVersion)
	appendMismatch("stack.cache_version", expected.Stack.CacheVersion, current.Stack.CacheVersion)
	appendMismatch("stack.search_version", expected.Stack.SearchVersion, current.Stack.SearchVersion)
	appendMismatch("stack.queue_version", expected.Stack.QueueVersion, current.Stack.QueueVersion)

	expectedServices := normalizeServiceImages(expected.Services)
	currentServices := normalizeServiceImages(current.Services)
	serviceNames := make([]string, 0, len(expectedServices))
	for name := range expectedServices {
		if !ignored["services."+strings.ToLower(name)] {
			serviceNames = append(serviceNames, name)
		}
	}
	sort.Strings(serviceNames)
	for _, name := range serviceNames {
		appendMismatch("services."+name, expectedServices[name], currentServices[name])
	}

	return LockCompliance{Compliant: len(mismatches) == 0, Mismatches: mismatches}
}

func ReadServiceImagesFromCompose(composePath string) (map[string]string, error) {
	payload, err := os.ReadFile(filepath.Clean(composePath))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read compose file: %w", err)
	}

	doc := map[string]interface{}{}
	if err := yaml.Unmarshal(payload, &doc); err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}

	servicesRaw, ok := doc["services"].(map[string]interface{})
	if !ok || len(servicesRaw) == 0 {
		return map[string]string{}, nil
	}

	images := map[string]string{}
	for serviceName, serviceSpecRaw := range servicesRaw {
		serviceSpec, ok := serviceSpecRaw.(map[string]interface{})
		if !ok {
			continue
		}
		imageRaw, ok := serviceSpec["image"]
		if !ok {
			continue
		}
		image, ok := imageRaw.(string)
		if !ok {
			continue
		}
		image = strings.TrimSpace(image)
		if image == "" {
			continue
		}
		images[strings.TrimSpace(serviceName)] = image
	}
	return normalizeServiceImages(images), nil
}

func detectDockerVersionForLock() (string, error) {
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", fmt.Errorf("empty docker version output")
	}
	return version, nil
}

func detectDockerComposeVersionForLock() (string, error) {
	cmd := exec.Command("docker", "compose", "version", "--short")
	output, err := cmd.Output()
	if err == nil {
		version := strings.TrimSpace(string(output))
		if version != "" {
			return version, nil
		}
	}

	fallback := exec.Command("docker", "compose", "version")
	fallbackOutput, fallbackErr := fallback.Output()
	if fallbackErr != nil {
		if err != nil {
			return "", err
		}
		return "", fallbackErr
	}
	text := strings.TrimSpace(string(fallbackOutput))
	for _, token := range strings.Fields(text) {
		trimmed := strings.TrimSpace(strings.TrimPrefix(token, "v"))
		if trimmed == "" {
			continue
		}
		if strings.Count(trimmed, ".") >= 1 && strings.IndexFunc(trimmed, func(r rune) bool {
			return (r < '0' || r > '9') && r != '.'
		}) == -1 {
			return strings.TrimPrefix(token, "v"), nil
		}
	}
	return "", fmt.Errorf("unable to parse docker compose version from output: %s", text)
}

func normalizeLockProjectName(config Config, cwd string) string {
	if value := strings.TrimSpace(config.ProjectName); value != "" {
		return value
	}
	if value := strings.TrimSpace(cwd); value != "" {
		return filepath.Base(filepath.Clean(value))
	}
	if cwdValue, err := os.Getwd(); err == nil {
		return filepath.Base(filepath.Clean(cwdValue))
	}
	return ""
}

func normalizeServiceImages(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	normalized := map[string]string{}
	for key, value := range raw {
		name := strings.TrimSpace(key)
		image := strings.TrimSpace(value)
		if name == "" || image == "" {
			continue
		}
		normalized[name] = image
	}
	return normalized
}

func flattenStoreDomainsForLock(mappings StoreDomainMappings) map[string]string {
	if len(mappings) == 0 {
		return nil
	}
	result := make(map[string]string, len(mappings))
	for host, mapping := range mappings {
		h := strings.TrimSpace(host)
		if h == "" {
			continue
		}
		result[h] = strings.TrimSpace(mapping.Code)
	}
	return result
}

func resolveLockDependencies(deps LockDependencies) LockDependencies {
	if deps.ReadDockerVersion == nil {
		deps.ReadDockerVersion = detectDockerVersionForLock
	}
	if deps.ReadDockerComposeVersion == nil {
		deps.ReadDockerComposeVersion = detectDockerComposeVersionForLock
	}
	if deps.ReadServiceImages == nil {
		deps.ReadServiceImages = ReadServiceImagesFromCompose
	}
	if deps.ReadCurrentUser == nil {
		deps.ReadCurrentUser = detectCurrentUserForLock
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return deps
}

func getGeneratedBy(deps LockDependencies) string {
	if deps.ReadCurrentUser == nil {
		return ""
	}
	user, _ := deps.ReadCurrentUser()
	return user
}

func detectCurrentUserForLock() (string, error) {
	if user := os.Getenv("USER"); user != "" {
		return user, nil
	}
	return "unknown", nil
}
