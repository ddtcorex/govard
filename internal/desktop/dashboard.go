package desktop

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/projectstores"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"gopkg.in/yaml.v3"
)

var (
	syncingProjects   = make(map[string]string) // projectName -> remoteName
	syncingProjectsMu sync.Mutex
)

// RegisterSyncingProject records an in-flight sync outside the atomic
// claim. Production starters must use TryRegisterSyncingProject (check and
// register under one mutex hold); this export stays for test setup only,
// where a fresh map makes the claim trivially succeed.
func RegisterSyncingProject(project, remote string) {
	syncingProjectsMu.Lock()
	defer syncingProjectsMu.Unlock()
	if id := canonicalProjectID(project); id != "" {
		syncingProjects[id] = remote
	}
}

func UnregisterSyncingProject(project string) {
	syncingProjectsMu.Lock()
	defer syncingProjectsMu.Unlock()
	delete(syncingProjects, canonicalProjectID(project))
}

// GetSyncingRemote is the exported face of the sync guard lookup: one
// shared implementation, so badge, guard and tests cannot drift apart.
func GetSyncingRemote(project string) string {
	return syncingRemoteForProject(project)
}

// syncingRemoteForProject reports the remote an in-flight sync is pulling
// from for a project. Both sides key on the canonical project ID, so names,
// domains and paths that resolve to the same project share one map entry
// and a single lookup suffices.
func syncingRemoteForProject(project string) string {
	id := canonicalProjectID(strings.TrimSpace(project))
	if id == "" {
		return ""
	}
	syncingProjectsMu.Lock()
	defer syncingProjectsMu.Unlock()
	return syncingProjects[id]
}

// refuseDuringSync rejects a mutating environment action while a sync is
// pulling into the same project: the sync owns the containers (bootstrap
// runs `up` itself unless told otherwise), so a concurrent start/stop would
// strand one of the two operations halfway.
func refuseDuringSync(verb string, project string) error {
	if remote := syncingRemoteForProject(project); remote != "" {
		return fmt.Errorf("cannot %s %s while sync from %q is in progress", verb, strings.TrimSpace(project), remote)
	}
	return nil
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

// isActiveEnvironmentStatus reports whether an environment counts as active
// for the header counter. Every entry is anchored: running and degraded come
// from the rollup, syncing from the sync override, warning from the tray
// contract (kept even though no producer emits it today, so a future warning
// never reads as stopped), healthy/up from frontend ACTIVE parity. Only a
// stopped (or unknown) environment does not count.
func isActiveEnvironmentStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "running", "healthy", "up", "degraded", "warning", "syncing":
		return true
	default:
		return false
	}
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

		// One goroutine per registry entry, but semaphore-bounded: every
		// entry forks git for its branch label, so an unbounded fan-out on
		// a large registry becomes a fork storm per dashboard refresh.
		sem := make(chan struct{}, registryEnvFanoutLimit())
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
				sem <- struct{}{}
				defer func() { <-sem }()

				env := buildRegistryEnvironmentEntry(ent, name)

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
		if isActiveEnvironmentStatus(env.Status) {
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

// registryEnvFanoutDefault caps the per-entry goroutines building
// registry-only environments: every entry forks git for its branch label.
const registryEnvFanoutDefault = 8

var registryEnvFanoutLimitValue = registryEnvFanoutDefault

func registryEnvFanoutLimit() int {
	if registryEnvFanoutLimitValue < 1 {
		return 1
	}
	return registryEnvFanoutLimitValue
}

// buildRegistryEnvironmentEntry renders one registry entry into an
// Environment without any Docker dependency: path config first, entry
// metadata as fallback, stopped unless a sync owns the project.
func buildRegistryEnvironmentEntry(ent engine.ProjectRegistryEntry, name string) Environment {
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

	if remote := syncingRemoteForProject(name); remote != "" {
		env.Status = "syncing"
		env.SyncingRemote = remote
	}
	return env
}

// EnvironmentService methods

func (s *EnvironmentService) GetDashboard() (dashboard Dashboard, err error) {
	defer RecoverPanic(&err, "GetDashboard")
	return buildDashboardInternal()
}

func (s *EnvironmentService) StartEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "StartEnvironment")
	if err := refuseDuringSync("start", project); err != nil {
		return "", err
	}
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}
	// A start always recreates: the desktop owns no per-service start, so a
	// plain `up` could leave config drift (a switched-off service, a changed
	// image) half applied. Volumes hold the data, so recreation is safe.
	output, err := runGovardCommandForDesktop(root, []string{"up", "--force-recreate", "--remove-orphans"})
	if err != nil {
		return "", err
	}
	return withCommandOutput("Environment started.", output), nil
}

func (s *EnvironmentService) StopEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "StopEnvironment")
	if err := refuseDuringSync("stop", project); err != nil {
		return "", err
	}
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

// restartFallbackErrorMax caps the original restart failure quoted in a
// fallback message: the text reaches toasts, so it stays short and
// sanitized while the diagnosis survives.
const restartFallbackErrorMax = 300

func (s *EnvironmentService) RestartEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "RestartEnvironment")
	if err := refuseDuringSync("restart", project); err != nil {
		return "", err
	}
	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}
	output, err := runGovardCommandForDesktop(root, []string{"env", "restart"})
	if err != nil {
		restartErr := err
		// Fallback if restart fails or is not implemented as expected
		_, _ = runGovardCommandForDesktop(root, []string{"env", "stop"})
		output, err = runGovardCommandForDesktop(root, []string{"up", "--force-recreate", "--remove-orphans"})
		if err != nil {
			return "", fmt.Errorf(
				"environment restart failed (%s) and fallback recreate failed: %w",
				sanitizeUIText(restartErr.Error(), restartFallbackErrorMax),
				err,
			)
		}
		return withCommandOutput(fmt.Sprintf(
			"Environment restarted (fallback: stop + recreate; restart failed: %s).",
			sanitizeUIText(restartErr.Error(), restartFallbackErrorMax),
		), output), nil
	}
	return withCommandOutput("Environment restarted.", output), nil
}

func (s *EnvironmentService) PullEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "PullEnvironment")
	if err := refuseDuringSync("pull images for", project); err != nil {
		return "", err
	}
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

// composeProjectNameForToggle maps whatever identifier the UI hands over
// (project name, domain or path) to the compose project name Docker labels
// carry, falling back to the input when the registry knows nothing about
// it. Toggle decides stop-vs-start off container labels, so resolving
// through the canonical project ID first keeps a renamed or
// domain-addressed project from taking the wrong branch.
func composeProjectNameForToggle(project string) string {
	if id := canonicalProjectID(project); id != "" {
		return id
	}
	return strings.TrimSpace(project)
}

func (s *EnvironmentService) ToggleEnvironment(project string) (res string, err error) {
	defer RecoverPanic(&err, "ToggleEnvironment")
	if err := refuseDuringSync("toggle", project); err != nil {
		return "", err
	}
	info, err := loadProjectInfo(composeProjectNameForToggle(project))
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

func init() {
	// The desktop binary does not link the CLI package, so it registers the
	// verify, audit and lint-cache store resolver itself; otherwise DeleteProject
	// would keep the stores that `project delete` removes.
	projectstores.Register()
}

// deleteWarningLinesMax caps the warnings quoted in a delete result: the
// engine prints progress chatter to the same stream, and the message
// reaches toasts.
const deleteWarningLinesMax = 5
const deleteWarningLineMax = 300

// appendDeleteWarnings folds engine warnings printed during deletion into
// the result message. Desktop output streams go nowhere visible (no
// terminal), so without this hook and docker warnings are lost entirely;
// plain progress lines stay out.
func appendDeleteWarnings(base, output string) string {
	var kept []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.Contains(trimmed, "Warning:") {
			continue
		}
		kept = append(kept, sanitizeUIText(trimmed, deleteWarningLineMax))
		if len(kept) >= deleteWarningLinesMax {
			break
		}
	}
	if len(kept) == 0 {
		return base
	}
	return base + "\n" + strings.Join(kept, "\n")
}

func (s *EnvironmentService) DeleteProject(projectQuery string) (res string, err error) {
	defer RecoverPanic(&err, "DeleteProject")
	if projectQuery == "" {
		return "", fmt.Errorf("project name or path is required")
	}
	if err := refuseDuringSync("delete", projectQuery); err != nil {
		return "", err
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
		var deleteOut bytes.Buffer
		if err := engine.DeleteOrphanProject(s.lifecycleContext(), root, &deleteOut, &deleteOut); err != nil {
			return "", err
		}
		return appendDeleteWarnings("Orphaned project resources removed", deleteOut.String()), nil
	}

	// We use the application context for the deletion process. Output is
	// captured, not pointed at the desktop's own stdout: warnings surface
	// in the result instead of vanishing into the app log.
	var deleteOut bytes.Buffer
	if err := engine.DeleteProject(s.lifecycleContext(), root, &deleteOut, &deleteOut); err != nil {
		return "", err
	}
	return appendDeleteWarnings("Project deleted successfully", deleteOut.String()), nil
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
		if project, service, ok := splitContainerNameFallback(name); ok {
			return project, service
		}
	}

	return "", ""
}

// fallbackServiceNames are the compose service names the label-less parser
// recognizes, longest first. The framework core comes from the conventions
// targets so a new target cannot rot the list silently (the pin test fails
// until it is added); the alias tail mirrors the service vocabulary the
// frontend's inferServiceTarget knows (mailpit, nginx images and the like).
var fallbackServiceNames = buildFallbackServiceNames()

func buildFallbackServiceNames() []string {
	names := []string{
		conventions.TargetElasticsearch,
		conventions.TargetOpenSearch,
		conventions.TargetRabbitMQ,
		conventions.TargetVarnish,
		conventions.TargetRedis,
		conventions.TargetValkey,
		conventions.TargetWeb,
		conventions.TargetPHP,
		conventions.TargetDB,
		conventions.TargetPMA,
		conventions.TargetApp,
		conventions.TargetMail,
		"phpmyadmin",
		"mailpit",
		"mailhog",
		"apache",
		"nginx",
		"mysql",
		"mariadb",
		"postgres",
	}
	sort.SliceStable(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	return names
}

// splitContainerNameFallback parses "<project>-<service>[-<replica>]"
// without compose labels: a trailing -<digits> replica is stripped, then
// the service splits off the end (longest known-service suffix first, else
// the last segment). Containers with a custom name carry no replica suffix,
// which the old 3-parts-or-nothing split dropped entirely.
func splitContainerNameFallback(raw string) (string, string, bool) {
	clean := strings.TrimPrefix(strings.TrimSpace(raw), "/")
	if clean == "" {
		return "", "", false
	}
	if i := strings.LastIndex(clean, "-"); i > 0 {
		if _, err := strconv.Atoi(clean[i+1:]); err == nil {
			clean = clean[:i]
		}
	}
	lower := strings.ToLower(clean)
	for _, svc := range fallbackServiceNames {
		if strings.HasSuffix(lower, "-"+svc) {
			project := strings.TrimSpace(clean[:len(clean)-len(svc)-1])
			if project != "" {
				return project, clean[len(clean)-len(svc):], true
			}
		}
	}
	if i := strings.LastIndex(clean, "-"); i > 0 && i < len(clean)-1 {
		return clean[:i], clean[i+1:], true
	}
	return "", "", false
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

// rollupEnvironmentStatus reduces container counts to one environment
// status: anything less than fully up but not fully down is degraded rather
// than green, so a half-started stack never reads as healthy.
func rollupEnvironmentStatus(runningCount int, totalCount int) string {
	if runningCount <= 0 {
		return "stopped"
	}
	if runningCount < totalCount {
		return "degraded"
	}
	return "running"
}

func buildEnvironment(info *projectInfo) Environment {
	env := Environment{
		Project: info.name,
		Name:    info.name,
		Status:  rollupEnvironmentStatus(info.runningCount, info.totalCount),
	}

	if remote := syncingRemoteForProject(info.name); remote != "" {
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
