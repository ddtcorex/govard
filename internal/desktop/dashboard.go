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
	runningCount int
	totalCount   int
	containers   []string
	workingDir   string
	configFiles  []string
	configPath   string
	config       engine.Config
	configLoaded bool
}

func buildDashboard() (Dashboard, error) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return Dashboard{}, err
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return Dashboard{}, err
	}

	projects := map[string]*projectInfo{}
	warnings := []string{}
	proxyRunning := isProxyRunning(containers)
	for _, c := range containers {
		projectName, serviceName := extractProjectAndService(c)
		if projectName == "" {
			continue
		}

		info := projects[projectName]
		if info == nil {
			info = &projectInfo{
				name:     projectName,
				services: map[string]bool{},
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
			serviceSummary[svc] = true
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

			env := Environment{
				Project:        projectName,
				Domain:         entry.Domain,
				Name:           projectName,
				Framework:      "Unknown",
				PHP:            "-",
				Database:       "-",
				Services:       []string{},
				ServiceTargets: []string{"web"},
				Status:         "stopped",
			}
			if entry.Domain != "" {
				env.Name = entry.Domain
			}
			if recipe := strings.TrimSpace(entry.Recipe); recipe != "" {
				env.Framework = displayFramework(recipe)
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
		env.Framework = displayFramework(info.config.Recipe)
		env.PHP = info.config.Stack.PHPVersion
		env.Database = formatDatabase(info.config.Stack.DBType, info.config.Stack.DBVersion)
		env.Services = deriveServices(info.config)
		env.ServiceTargets = collectServiceTargets(info)
		if env.Domain != "" {
			env.Name = env.Domain
		}
	} else {
		env.Framework = "Unknown"
		env.PHP = "-"
		env.Database = "-"
		env.Services = fallbackServices(info.services)
		env.ServiceTargets = collectServiceTargets(info)
	}

	return env
}

func displayFramework(recipe string) string {
	switch recipe {
	case "magento1":
		return "Magento 1"
	case "magento2":
		return "Magento 2"
	case "nextjs":
		return "Next.js"
	case "cakephp":
		return "CakePHP"
	default:
		return strings.Title(recipe)
	}
}

func formatDatabase(dbType, dbVersion string) string {
	if dbType == "" || dbType == "none" {
		return "No database"
	}
	label := strings.Title(dbType)
	if dbVersion == "" {
		return label
	}
	return fmt.Sprintf("%s %s", label, dbVersion)
}

func deriveServices(config engine.Config) []string {
	var services []string
	if config.Stack.Services.WebServer != "" {
		services = append(services, strings.Title(config.Stack.Services.WebServer))
	}
	if config.Stack.DBType != "" && config.Stack.DBType != "none" {
		services = append(services, strings.Title(config.Stack.DBType))
	}
	switch config.Stack.Services.Cache {
	case "redis":
		services = append(services, "Redis")
	case "valkey":
		services = append(services, "Valkey")
	}
	switch config.Stack.Services.Search {
	case "opensearch":
		services = append(services, "OpenSearch")
	case "elasticsearch":
		services = append(services, "Elasticsearch")
	}
	if config.Stack.Services.Queue == "rabbitmq" {
		services = append(services, "RabbitMQ")
	}
	if config.Stack.Features.Varnish {
		services = append(services, "Varnish")
	}
	return services
}

func fallbackServices(services map[string]bool) []string {
	var out []string
	for name := range services {
		switch name {
		case "redis":
			out = append(out, "Redis")
		case "elasticsearch":
			out = append(out, "Search")
		case "varnish":
			out = append(out, "Varnish")
		case "rabbitmq":
			out = append(out, "RabbitMQ")
		}
	}
	sort.Strings(out)
	return out
}

func summarizeNames(environments []Environment) string {
	var names []string
	for _, env := range environments {
		label := env.Domain
		if label == "" {
			label = env.Project
		}
		names = append(names, label)
		if len(names) == 3 {
			break
		}
	}
	if len(names) == 0 {
		return "No environments detected"
	}
	if len(environments) > len(names) {
		return strings.Join(names, ", ") + "..."
	}
	return strings.Join(names, ", ")
}

func summarizeServices(services map[string]bool) string {
	if len(services) == 0 {
		return "No services detected"
	}
	ordered := []string{"PHP", "Nginx", "Apache", "MariaDB", "MySQL", "Redis", "Valkey", "OpenSearch", "Elasticsearch", "Varnish", "RabbitMQ"}
	var present []string
	for _, svc := range ordered {
		if services[svc] {
			present = append(present, svc)
		}
	}
	for svc := range services {
		found := false
		for _, existing := range present {
			if existing == svc {
				found = true
				break
			}
		}
		if !found {
			present = append(present, svc)
		}
	}
	return strings.Join(present, ", ")
}

func collectServiceTargets(info *projectInfo) []string {
	if info == nil {
		return nil
	}

	ordered := []string{"web", "php", "db", "redis", "valkey", "elasticsearch", "opensearch", "varnish", "rabbitmq", "mail", "pma"}
	var targets []string
	for _, name := range ordered {
		if info.services[name] {
			targets = append(targets, name)
		}
	}
	for name := range info.services {
		found := false
		for _, existing := range targets {
			if existing == name {
				found = true
				break
			}
		}
		if !found {
			targets = append(targets, name)
		}
	}
	if len(targets) == 0 {
		targets = append(targets, "web")
	}
	return targets
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func containsService(services []string, target string) bool {
	for _, svc := range services {
		if svc == target {
			return true
		}
	}
	return false
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

func nameMatches(names []string, target string) bool {
	for _, name := range names {
		if strings.TrimPrefix(name, "/") == target {
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

	args := filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+project))
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, err
	}
	if len(containers) == 0 {
		return nil, fmt.Errorf("no containers found")
	}

	info := &projectInfo{
		name:     project,
		services: map[string]bool{},
	}

	for _, c := range containers {
		_, service := extractProjectAndService(c)
		if service != "" {
			info.services[service] = true
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
