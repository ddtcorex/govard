package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"govard/internal/engine"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v3"
)

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

	// Docker connectivity is optional — if unavailable, fall back to registry-only mode
	if cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation()); err == nil {
		if containers, err := cli.ContainerList(ctx, container.ListOptions{All: true}); err == nil {
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
		for _, entry := range entries {
			projectName := strings.TrimSpace(entry.ProjectName)
			if projectName == "" {
				continue
			}
			if knownProjects[projectName] {
				continue
			}

			// Check if we have dynamic info from registry entry
			entryServices := map[string]bool{}
			if entry.Framework != "" {
				// Stub services if we know the framework but it's stopped
				if entry.Framework == "magento2" {
					entryServices["web"] = true
					entryServices["php"] = true
					entryServices["db"] = true
					entryServices["redis"] = true
				}
			}

			env := Environment{
				Project:        projectName,
				Domain:         entry.Domain,
				Name:           projectName,
				Framework:      "Unknown",
				PHP:            "-",
				Database:       "-",
				Services:       []Service{},
				ServiceTargets: []string{"web"},
				Status:         "stopped",
			}

			// Try to load detailed info from path if available
			if info, err := loadProjectInfoFromPath(entry.Path); err == nil {
				env.Domain = info.config.Domain
				env.Framework = displayFramework(info.config.Framework)
				env.PHP = info.config.Stack.PHPVersion
				env.Database = formatDatabase(info.config.Stack.DBType, info.config.Stack.DBVersion)
				env.Services = deriveServices(info.config, info.serviceState)
				env.Technologies = buildTechnologies(env)
				if entry.Domain != "" {
					env.Name = entry.Domain
				}
			}

			if entry.Domain != "" {
				env.Name = entry.Domain
			}
			env.ExtraDomains = entry.ExtraDomains
			if env.Framework == "Unknown" && entry.Framework != "" {
				env.Framework = displayFramework(entry.Framework)
			}
			environments = append(environments, env)
			knownProjects[projectName] = true
		}
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

func (s *EnvironmentService) GetDashboard() (Dashboard, error) {
	return buildDashboardInternal()
}

func (s *EnvironmentService) StartEnvironment(project string) (string, error) {
	return startEnvironment(project)
}

func (s *EnvironmentService) StopEnvironment(project string) (string, error) {
	return stopEnvironment(project)
}

func (s *EnvironmentService) RestartEnvironment(project string) (string, error) {
	return restartEnvironment(project)
}

func (s *EnvironmentService) ToggleEnvironment(project string) (string, error) {
	return toggleEnvironment(project)
}

func (s *EnvironmentService) GetEnvironmentURL(project string) (string, error) {
	return environmentURL(project)
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
		engine.NormalizeConfig(&config)
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

	if info.configLoaded {
		env.Domain = info.config.Domain
		env.ExtraDomains = info.config.ExtraDomains
		env.Framework = displayFramework(info.config.Framework)
		env.PHP = info.config.Stack.PHPVersion
		env.Database = formatDatabase(info.config.Stack.DBType, info.config.Stack.DBVersion)
		env.Services = deriveServices(info.config, info.serviceState)
		env.ServiceTargets = collectServiceTargets(info)
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

	env.Technologies = buildTechnologies(env)

	return env
}

func isProxyRunning(containers []container.Summary) bool {
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		if nameMatches(c.Names, "proxy-caddy-1") {
			return true
		}
	}
	return false
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

func toggleEnvironment(project string) (string, error) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", err
	}

	args := filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+project))
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", fmt.Errorf("no containers found")
	}

	running := false
	for _, c := range containers {
		if c.State == "running" {
			running = true
			break
		}
	}

	if running {
		timeout := 10
		for _, c := range containers {
			if c.State != "running" {
				continue
			}
			if err := cli.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout}); err != nil {
				return "", err
			}
		}
		return "Stopped environment " + project, nil
	}

	for _, c := range containers {
		if err := cli.ContainerStart(ctx, c.ID, container.StartOptions{}); err != nil {
			return "", err
		}
	}
	return "Started environment " + project, nil
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
