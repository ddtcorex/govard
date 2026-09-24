package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"govard/internal/engine"
	"govard/internal/frameworks"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v3"
)

var (
	syncingProjects   = make(map[string]string) // projectName -> remoteName
	syncingProjectsMu sync.Mutex
)

func RegisterSyncingProject(project, remote string) {
	syncingProjectsMu.Lock()
	defer syncingProjectsMu.Unlock()
	syncingProjects[project] = remote
}

func UnregisterSyncingProject(project string) {
	syncingProjectsMu.Lock()
	defer syncingProjectsMu.Unlock()
	delete(syncingProjects, project)
}

func GetSyncingRemote(project string) string {
	syncingProjectsMu.Lock()
	defer syncingProjectsMu.Unlock()
	return syncingProjects[project]
}

type projectInfo struct {
	name         string
	services     map[string]bool
	serviceState map[string]string
	runningCount int
	totalCount   int
	containers   []string
	workingDir   string
	configFiles  []string
	configPath   string
	config       engine.Config
	configLoaded bool
}

func buildDashboardInternal() (Dashboard, error) {
	ctx := context.Background()

	projects := map[string]*projectInfo{}
	warnings := []string{}
	proxyRunning := false
	allContainers := []container.Summary{}
	containersByName := map[string]container.Summary{}

	// Docker connectivity is optional — if unavailable, fall back to registry-only mode
	if cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation()); err == nil {
		if containers, err := cli.ContainerList(ctx, container.ListOptions{All: true}); err == nil {
			allContainers = containers
			for _, c := range containers {
				for _, rawName := range c.Names {
					name := strings.TrimSpace(strings.TrimPrefix(rawName, "/"))
					if name == "" {
						continue
					}
					containersByName[name] = c
				}
			}
			proxyRunning = isProxyRunning(containers)
			for _, c := range containers {
				projectName, serviceName := extractProjectAndService(c)
				if projectName == "" {
					continue
				}

				info := projects[projectName]
				if info == nil {
					info = &projectInfo{
						name:         projectName,
						services:     map[string]bool{},
						serviceState: map[string]string{},
					}
					projects[projectName] = info
				}

				info.totalCount++
				info.containers = append(info.containers, c.ID)
				if c.State == "running" {
					info.runningCount++
				}
				if serviceName != "" {
					info.services[serviceName] = true
					info.serviceState[serviceName] = mergeServiceState(info.serviceState[serviceName], c.State)
				}

				if info.workingDir == "" {
					if wd := c.Labels["com.docker.compose.project.working_dir"]; wd != "" {
						info.workingDir = wd
					}
				}
				if len(info.configFiles) == 0 {
					if files := c.Labels["com.docker.compose.project.config_files"]; files != "" {
						info.configFiles = parseConfigFiles(files)
					}
				}
			}
		} else {
			warnings = append(warnings, "Docker unavailable: "+err.Error())
		}
	} else {
		warnings = append(warnings, "Docker client error: "+err.Error())
	}

	var environments []Environment
	runningServices := 0
	queueCount := 0
	serviceSummary := map[string]bool{}

	for _, info := range projects {
		configErr := loadProjectConfig(info)
		if !looksLikeGovard(info) {
			continue
		}
		if configErr != nil && hasGovardComposeConfigFile(info.configFiles) {
			warnings = append(warnings, "Missing .govard.yml for "+info.name+".")
		}

		env := buildEnvironment(info)
		environments = append(environments, env)
		runningServices += info.runningCount

		for _, svc := range env.Services {
			serviceSummary[svc.Name] = true
		}

		if containsService(env.Services, "RabbitMQ") && env.Status == "running" {
			queueCount++
		}

		if info.configLoaded && info.config.Domain == "" {
			warnings = append(warnings, "Domain not set for "+info.name+".")
		}
	}

	if entries, err := engine.ReadProjectRegistryEntries(); err == nil {
		knownProjects := map[string]bool{}
		for _, env := range environments {
			knownProjects[env.Project] = true
		}

		var (
			mu           sync.Mutex
			wg           sync.WaitGroup
			registryEnvs []Environment
		)

		for _, entry := range entries {
			projectName := strings.TrimSpace(entry.ProjectName)
			if projectName == "" {
				continue
			}
			if knownProjects[projectName] {
				continue
			}

			wg.Add(1)
			go func(ent engine.ProjectRegistryEntry, name string) {
				defer wg.Done()

				env := Environment{
					Project:        name,
					Domain:         ent.Domain,
					Name:           name,
					Framework:      "Unknown",
					PHP:            "-",
					Database:       "-",
					Services:       []Service{},
					ServiceTargets: []string{"web"},
					Status:         "stopped",
				}

				// Try to load detailed info from path if available
				if info, err := loadProjectInfoFromPath(ent.Path); err == nil {
					env.Domain = info.config.Domain
					env.Framework = displayFramework(info.config.Framework)
					env.PHP = info.config.Stack.PHPVersion
					env.Database = formatDatabase(info.config.Stack.Services.DB, info.config.Stack.DBVersion)
					env.Services = deriveServices(info.config, info.serviceState)
					env.ServiceTargets = collectServiceTargetsFromServices(info, env.Services)
					env.GitBranch = getGitBranch(ent.Path)
					env.Technologies = buildTechnologies(env)
					if ent.Domain != "" {
						env.Name = ent.Domain
					}
				}

				if ent.Domain != "" {
					env.Name = ent.Domain
				}
				env.ExtraDomains = ent.ExtraDomains
				if env.Framework == "Unknown" && ent.Framework != "" {
					env.Framework = displayFramework(ent.Framework)
				}

				if remote := GetSyncingRemote(name); remote != "" {
					env.Status = "syncing"
					env.SyncingRemote = remote
				}

				mu.Lock()
				registryEnvs = append(registryEnvs, env)
				mu.Unlock()
			}(entry, projectName)

			knownProjects[projectName] = true
		}
		wg.Wait()
		environments = append(environments, registryEnvs...)
	} else {
		warnings = append(warnings, "Project registry unavailable.")
	}

	sort.Slice(environments, func(i, j int) bool {
		return environments[i].Project < environments[j].Project
	})

	activeEnvs := 0
	for _, env := range environments {
		if env.Status == "running" {
			activeEnvs++
		}
	}

	activeSummary := summarizeNames(environments)
	servicesSummary := summarizeServices(serviceSummary)
	queueSummary := "Queue idle"
	if queueCount > 0 {
		queueSummary = "RabbitMQ online"
	}

	if !proxyRunning {
		warnings = append(warnings, "Govard proxy is not running. HTTPS routes may fail.")
	}
	warnings = append(warnings, buildEnvironmentRoutingWarnings(allContainers, containersByName)...)

	return Dashboard{
		ActiveEnvironments: activeEnvs,
		RunningServices:    runningServices,
		QueuedTasks:        queueCount,
		ActiveSummary:      activeSummary,
		ServicesSummary:    servicesSummary,
		QueueSummary:       queueSummary,
		Environments:       environments,
		Warnings:           uniqueStrings(warnings),
	}, nil
}

// EnvironmentService methods

func (s *EnvironmentService) GetDashboard() (dashboard Dashboard, err error) {
	defer RecoverPanic(&err, "GetDashboard")
	return buildDashboardInternal()
}

func (s *EnvironmentService) StartEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "StartEnvironment")
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}
	// 'up' can take a while, but we want to show output if possible.
	// However, Wails calls usually expect a relatively quick response or use events.
	// For now, let's use the CLI runner which captures output.
	output, err := runGovardCommandForDesktop(root, []string{"up", "--force-recreate", "--remove-orphans"})
	if err != nil {
		return "", err
	}
	return withCommandOutput("Environment started.", output), nil
}

func (s *EnvironmentService) StopEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "StopEnvironment")
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}
	output, err := runGovardCommandForDesktop(root, []string{"env", "stop"})
	if err != nil {
		return "", err
	}
	return withCommandOutput("Environment stopped.", output), nil
}

func (s *EnvironmentService) RestartEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "RestartEnvironment")
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}
	output, err := runGovardCommandForDesktop(root, []string{"env", "restart"})
	if err != nil {
		// Fallback if restart fails or is not implemented as expected
		_, _ = runGovardCommandForDesktop(root, []string{"env", "stop"})
		output, err = runGovardCommandForDesktop(root, []string{"up", "--force-recreate", "--remove-orphans"})
		if err != nil {
			return "", err
		}
	}
	return withCommandOutput("Environment restarted.", output), nil
}

func (s *EnvironmentService) PullEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "PullEnvironment")
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}
	output, err := runGovardCommandForDesktop(root, []string{"env", "pull"})
	if err != nil {
		return "", err
	}
	return withCommandOutput("Environment images pulled.", output), nil
}

func (s *EnvironmentService) ToggleEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "ToggleEnvironment")
	info, err := loadProjectInfo(project)
	if err == nil && info.runningCount > 0 {
		return s.StopEnvironment(project)
	}
	return s.StartEnvironment(project)
}

func (s *EnvironmentService) GetEnvironmentURL(project string) (res string, err error) {
	defer RecoverPanic(&err, "GetEnvironmentURL")
	return environmentURL(project)
}

// OpenEnvironment resolves the project URL and hands it to the OS browser.
// The frontend shows its own success toast, so the returned string is only a
// fallback message.
func (s *EnvironmentService) OpenEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "OpenEnvironment")
	url, errEnv := s.GetEnvironmentURL(project)
	if errEnv != nil {
		return "", errEnv
	}
	if errOpen := openURLWithPreferences(s.platform, url); errOpen != nil {
		return "Open " + url + " manually", nil
	}
	return "Opening " + url + "...", nil
}

func (s *EnvironmentService) OpenDocs(path string) (res string, err error) {
	defer RecoverPanic(&err, "OpenDocs")
	if path == "" {
		return "", fmt.Errorf("no docs path provided")
	}
	if errOpen := openDocs(s.platform, path); errOpen != nil {
		return "", fmt.Errorf("failed to open docs: %w", errOpen)
	}
	return "Opening docs...", nil
}

func (s *EnvironmentService) QuickAction(action string) (res string, err error) {
	defer RecoverPanic(&err, "QuickAction")
	return quickAction(s.platform, action, "")
}

func (s *EnvironmentService) QuickActionForProject(action string, project string) (res string, err error) {
	defer RecoverPanic(&err, "QuickActionForProject")
	return quickAction(s.platform, action, project)
}

func (s *EnvironmentService) DeleteProject(projectQuery string) (res string, err error) {
	defer RecoverPanic(&err, "DeleteProject")
	if projectQuery == "" {
		return "", fmt.Errorf("project name or path is required")
	}

	root, score, err := resolveProjectRootForRemotes(projectQuery)
	if err != nil {
		return "", err
	}

	// Safety check for weak matches in the desktop app
	if score >= engine.ScoreAmbiguousThreshold {
		return "", fmt.Errorf("match for %q is too weak for deletion (confidence score: %d): use the full name or path", projectQuery, score)
	}

	// Check if it's an orphan (root is the name, not an absolute path)
	if !filepath.IsAbs(root) && !strings.Contains(root, string(filepath.Separator)) {
		if err := engine.DeleteOrphanProject(s.lifecycleContext(), root, os.Stdout, os.Stderr); err != nil {
			return "", err
		}
		return "Orphaned project resources removed", nil
	}

	// We use the application context for the deletion process
	if err := engine.DeleteProject(s.lifecycleContext(), root, os.Stdout, os.Stderr); err != nil {
		return "", err
	}
	return "Project deleted successfully", nil
}

// ListFrameworks returns every framework registered in internal/frameworks,
// for the onboarding UI's framework picker. This keeps the frontend's
// dropdown, alias resolution, and display-name formatting in sync with the
// Go-side registry instead of duplicating framework metadata in JS.
func (s *EnvironmentService) ListFrameworks() (res []FrameworkOption, err error) {
	defer RecoverPanic(&err, "ListFrameworks")
	defs := frameworks.All()
	res = make([]FrameworkOption, 0, len(defs))
	for _, def := range defs {
		res = append(res, FrameworkOption{
			Name:        def.Name,
			DisplayName: def.DisplayName,
			Aliases:     append([]string(nil), def.Aliases...), // defensive copy - registry.go's All()/Get() warn callers not to mutate shared slice fields
		})
	}
	return res, nil
}

func extractProjectAndService(c container.Summary) (string, string) {
	project := c.Labels["com.docker.compose.project"]
	service := c.Labels["com.docker.compose.service"]
	if project != "" {
		return project, service
	}

	for _, name := range c.Names {
		clean := strings.TrimPrefix(name, "/")
		parts := strings.Split(clean, "-")
		if len(parts) >= 3 {
			project = strings.Join(parts[:len(parts)-2], "-")
			service = parts[len(parts)-2]
			return project, service
		}
	}

	return "", ""
}

func looksLikeGovard(info *projectInfo) bool {
	if info == nil {
		return false
	}
	if info.name == "proxy" || info.name == "warden" {
		return false
	}
	if info.configLoaded {
		return true
	}
	return hasGovardComposeConfigFile(info.configFiles)
}

func loadProjectConfig(info *projectInfo) error {
	if info.configLoaded {
		return nil
	}
	paths := candidateConfigPaths(info)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var config engine.Config
		if err := yaml.Unmarshal(data, &config); err != nil {
			continue
		}
		if config.ProjectName == "" {
			config.ProjectName = info.name
		}
		engine.NormalizeConfig(&config, filepath.Dir(path))
		info.config = config
		info.configPath = path
		info.configLoaded = true
		info.workingDir = filepath.Dir(path)
		return nil
	}
	return fmt.Errorf(".govard.yml not found")
}

func candidateConfigPaths(info *projectInfo) []string {
	var paths []string
	if info.workingDir != "" {
		paths = append(paths, filepath.Join(info.workingDir, ".govard.yml"))
	}
	for _, file := range info.configFiles {
		dir := filepath.Dir(file)
		paths = append(paths, filepath.Join(dir, ".govard.yml"))
	}
	return uniqueStrings(paths)
}

func parseConfigFiles(raw string) []string {
	parts := strings.Split(raw, ",")
	var cleaned []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return cleaned
}

func hasGovardComposeConfigFile(paths []string) bool {
	for _, path := range paths {
		if isGovardComposeConfigFile(path) {
			return true
		}
	}
	return false
}

func isGovardComposeConfigFile(path string) bool {
	normalized := strings.ToLower(strings.TrimSpace(path))
	if normalized == "" {
		return false
	}
	normalized = strings.ReplaceAll(normalized, "\\", "/")
	return strings.Contains(normalized, "/.govard/compose/") || strings.HasPrefix(normalized, ".govard/compose/")
}

func buildEnvironment(info *projectInfo) Environment {
	env := Environment{
		Project: info.name,
		Name:    info.name,
		Status:  "stopped",
	}
	if info.runningCount > 0 {
		env.Status = "running"
	}

	if remote := GetSyncingRemote(info.name); remote != "" {
		env.Status = "syncing"
		env.SyncingRemote = remote
	}

	if info.configLoaded {
		env.Domain = info.config.Domain
		env.ExtraDomains = info.config.ExtraDomains
		env.Framework = displayFramework(info.config.Framework)
		env.PHP = info.config.Stack.PHPVersion
		env.Database = formatDatabase(info.config.Stack.Services.DB, info.config.Stack.DBVersion)
		env.Services = deriveServices(info.config, info.serviceState)
		env.ServiceTargets = collectServiceTargetsFromServices(info, env.Services)
		if info.workingDir != "" {
			env.EnvVars = engine.ParseDotEnv(filepath.Join(info.workingDir, ".env"))
		}
		if env.Domain != "" {
			env.Name = env.Domain
		}
	} else {
		env.Framework = "Unknown"
		env.PHP = "-"
		env.Database = "-"
		env.Services = fallbackServices(info.services, info.serviceState)
		env.ServiceTargets = collectServiceTargets(info)
	}

	if info.workingDir != "" {
		env.GitBranch = getGitBranch(info.workingDir)
	} else if info.configPath != "" {
		env.GitBranch = getGitBranch(filepath.Dir(info.configPath))
	}

	env.Technologies = buildTechnologies(env)

	return env
}

func isProxyRunning(containers []container.Summary) bool {
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		if nameMatches(c.Names, "proxy-caddy-1") || nameMatches(c.Names, "govard-proxy-caddy") {
			return true
		}
		project := strings.TrimSpace(c.Labels["com.docker.compose.project"])
		service := strings.TrimSpace(c.Labels["com.docker.compose.service"])
		if strings.EqualFold(project, globalServicesComposeProjectName) && strings.EqualFold(service, "caddy") {
			return true
		}
	}
	return false
}

func buildEnvironmentRoutingWarnings(
	allContainers []container.Summary,
	containersByName map[string]container.Summary,
) []string {
	if len(allContainers) == 0 {
		return nil
	}

	routingServices := []GlobalService{}
	for _, spec := range globalServiceSpecs {
		if spec.ID != "caddy" && spec.ID != "dnsmasq" {
			continue
		}

		service := GlobalService{
			ID:            spec.ID,
			Name:          spec.Name,
			ContainerName: spec.ContainerName,
			Status:        "missing",
			State:         "not-created",
			Running:       false,
		}
		if c, ok := containersByName[spec.ContainerName]; ok {
			status, _, running := deriveGlobalContainerStatus(c.State, c.Status)
			service.Status = status
			service.State = strings.TrimSpace(c.State)
			if service.State == "" {
				service.State = "unknown"
			}
			service.Running = running
		}

		routingServices = append(routingServices, service)
	}

	bindingWarnings := detectRoutingPublishedPortBindingWarnings(routingServices, containersByName)
	if len(bindingWarnings) == 0 && !hasRoutingConflictSignal(routingServices) {
		return nil
	}

	return []string{
		"Routing layer is degraded. Environment services may appear running while domains are unreachable. Check Global Services warnings.",
	}
}

func environmentURL(project string) (string, error) {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}
	if info.configLoaded && info.config.Domain != "" {
		return "https://" + info.config.Domain, nil
	}
	return "", fmt.Errorf("domain not found")
}

func loadProjectInfo(project string) (*projectInfo, error) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	projectName := project
	args := filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+projectName))
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, err
	}

	// If no containers found, try stripping .test suffix if present
	if len(containers) == 0 && strings.HasSuffix(projectName, ".test") {
		projectName = strings.TrimSuffix(projectName, ".test")
		args = filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+projectName))
		containers, err = cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
		if err != nil {
			return nil, err
		}
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("no containers found for project '%s'", project)
	}

	info := &projectInfo{
		name:         projectName,
		services:     map[string]bool{},
		serviceState: map[string]string{},
	}

	for _, c := range containers {
		_, service := extractProjectAndService(c)
		if service != "" {
			info.services[service] = true
			info.serviceState[service] = mergeServiceState(info.serviceState[service], c.State)
		}
		if c.State == "running" {
			info.runningCount++
		}
		info.totalCount++
		if info.workingDir == "" {
			if wd := c.Labels["com.docker.compose.project.working_dir"]; wd != "" {
				info.workingDir = wd
			}
		}
		if len(info.configFiles) == 0 {
			if files := c.Labels["com.docker.compose.project.config_files"]; files != "" {
				info.configFiles = parseConfigFiles(files)
			}
		}
	}

	_ = loadProjectConfig(info)
	return info, nil
}

func getGitBranch(workingDir string) string {
	if workingDir == "" {
		return ""
	}
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}
