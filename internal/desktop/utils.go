package desktop

import (
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// titleCase returns the string with the first letter capitalized.
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// RecoverPanic is a helper to catch panics in bridge methods and return a clean error.
func RecoverPanic(err *error, actionName string) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("internal error during %s: %v", actionName, r)
	}
}

// uniqueStrings returns a slice with unique strings from the input.
func uniqueStrings(values []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range values {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// containsService checks if a specific service is in the list.
func containsService(services []Service, target string) bool {
	for _, s := range services {
		if strings.Contains(strings.ToLower(s.Name), strings.ToLower(target)) {
			return true
		}
	}
	return false
}

// ... existing code ...

func buildTechnologies(env Environment) []string {
	var techs []string
	if env.PHP != "" && env.PHP != "-" {
		techs = append(techs, "PHP "+env.PHP)
	}
	if env.Database != "" && env.Database != "-" && env.Database != "No database" && env.Database != "None" {
		techs = append(techs, env.Database)
	}
	for _, svc := range env.Services {
		svcLower := strings.ToLower(svc.Name)
		if svcLower != "mysql" && svcLower != "mariadb" && svcLower != "postgres" && svcLower != "postgresql" {
			techs = append(techs, svc.Name)
		}
	}
	return uniqueStrings(techs)
}

// nameMatches checks if the target string matches any in the names list.
func nameMatches(names []string, target string) bool {
	for _, n := range names {
		if strings.EqualFold(n, target) {
			return true
		}
	}
	return false
}

// normalizeProjectPath ensures a project path is absolute and exists.
func normalizeProjectPath(projectPath string) (string, error) {
	path := strings.TrimSpace(projectPath)
	if path == "" {
		return "", fmt.Errorf("project path is required")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve project path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("project path does not exist: %s", absPath)
		}
		return "", fmt.Errorf("inspect project path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path is not a directory: %s", absPath)
	}

	return filepath.Clean(absPath), nil
}

// normalizeOnboardingDomain adds .test suffix if missing.
func normalizeOnboardingDomain(domain string) string {
	normalized := strings.ToLower(strings.TrimSpace(domain))
	if normalized == "" {
		return ""
	}
	if !strings.Contains(normalized, ".") && !strings.Contains(normalized, ":") {
		normalized += ".test"
	}
	return normalized
}

// normalizeOnboardingFramework normalizes framework aliases.
func normalizeOnboardingFramework(framework string) string {
	switch strings.ToLower(strings.TrimSpace(framework)) {
	case "", "auto", "detect":
		return ""
	default:
		return frameworks.Normalize(framework)
	}
}

// ... (existing functions)

func displayFramework(framework string) string {
	if def, ok := frameworks.Get(framework); ok && def.DisplayName != "" {
		return def.DisplayName
	}
	return titleCase(framework)
}

func formatDatabase(dbType, dbVersion string) string {
	if dbType == "" || dbType == "none" {
		return "No database"
	}
	label := titleCase(dbType)
	lower := strings.ToLower(label)
	switch lower {
	case "mariadb":
		label = "MariaDB"
	case "mysql":
		label = "MySQL"
	case "postgres", "postgresql":
		label = "PostgreSQL"
	}

	if dbVersion == "" {
		return label
	}
	return fmt.Sprintf("%s %s", label, dbVersion)
}

func defaultDatabasePortForType(dbType string) string {
	lowerDBType := strings.ToLower(strings.TrimSpace(dbType))
	if lowerDBType == conventions.ServicePostgreSQL || lowerDBType == "postgresql" {
		return strconv.Itoa(conventions.PostgresPort)
	}
	return strconv.Itoa(conventions.MySQLPort)
}

func mergeServiceState(current, candidate string) string {
	next := strings.ToLower(strings.TrimSpace(candidate))
	if next == "" {
		return strings.ToLower(strings.TrimSpace(current))
	}
	cur := strings.ToLower(strings.TrimSpace(current))
	if serviceStatePriority(next) > serviceStatePriority(cur) {
		return next
	}
	return cur
}

func serviceStatePriority(state string) int {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running":
		return 100
	case "restarting":
		return 90
	case "paused":
		return 80
	case "created":
		return 70
	case "exited":
		return 60
	case "dead", "removing":
		return 50
	default:
		return 0
	}
}

func serviceStatus(states map[string]string, target string, fallback string) string {
	if states != nil {
		if status := strings.ToLower(strings.TrimSpace(states[target])); status != "" {
			return status
		}
	}
	return fallback
}

func deriveServices(config engine.Config, states map[string]string) []Service {
	var services []Service
	if config.Stack.Services.WebServer != "" {
		services = append(services, Service{
			Name:   titleCase(config.Stack.Services.WebServer),
			Status: serviceStatus(states, conventions.TargetWeb, "stopped"),
			Port:   strconv.Itoa(conventions.HTTPPort),
			Target: conventions.TargetWeb,
		})
	}
	if config.Stack.Services.DB != "" && config.Stack.Services.DB != "none" {
		label := titleCase(config.Stack.Services.DB)
		lower := strings.ToLower(label)
		switch lower {
		case "mariadb":
			label = "MariaDB"
		case "mysql":
			label = "MySQL"
		case "postgres", "postgresql":
			label = "PostgreSQL"
		}
		services = append(services, Service{
			Name:   label,
			Status: serviceStatus(states, conventions.TargetDB, "stopped"),
			Port:   defaultDatabasePortForType(config.Stack.Services.DB),
			Target: conventions.TargetDB,
		})
	}
	if config.Stack.PHPVersion != "" {
		services = append(services, Service{
			Name:   "PHP",
			Status: serviceStatus(states, conventions.TargetPHP, "stopped"),
			Port:   strconv.Itoa(conventions.PHPFPMPort),
			Target: conventions.TargetPHP,
		})
	}
	switch config.Stack.Services.Cache {
	case conventions.ServiceRedis:
		services = append(services, Service{
			Name:   "Redis",
			Status: serviceStatus(states, conventions.TargetRedis, "stopped"),
			Port:   strconv.Itoa(conventions.RedisPort),
			Target: conventions.TargetRedis,
		})
	case conventions.ServiceValkey:
		services = append(services, Service{
			Name:   "Valkey",
			Status: serviceStatus(states, conventions.TargetValkey, "stopped"),
			Port:   strconv.Itoa(conventions.RedisPort),
			Target: conventions.TargetValkey,
		})
	}
	switch config.Stack.Services.Search {
	case conventions.ServiceOpenSearch:
		services = append(services, Service{
			Name:   "OpenSearch",
			Status: serviceStatus(states, conventions.TargetOpenSearch, "stopped"),
			Port:   strconv.Itoa(conventions.SearchPort),
			Target: conventions.TargetOpenSearch,
		})
	case conventions.ServiceElasticsearch:
		services = append(services, Service{
			Name:   "Elasticsearch",
			Status: serviceStatus(states, conventions.TargetElasticsearch, "stopped"),
			Port:   strconv.Itoa(conventions.SearchPort),
			Target: conventions.TargetElasticsearch,
		})
	}
	if config.Stack.Services.Queue == conventions.ServiceRabbitMQ {
		services = append(services, Service{
			Name:   "RabbitMQ",
			Status: serviceStatus(states, conventions.TargetRabbitMQ, "stopped"),
			Port:   strconv.Itoa(conventions.RabbitMQPort),
			Target: conventions.TargetRabbitMQ,
		})
	}
	if config.Stack.Features.Varnish {
		services = append(services, Service{
			Name:   "Varnish",
			Status: serviceStatus(states, conventions.TargetVarnish, "stopped"),
			Port:   strconv.Itoa(conventions.HTTPPort),
			Target: conventions.TargetVarnish,
		})
	}
	return services
}

func fallbackServices(services map[string]bool, states map[string]string) []Service {
	var out []Service
	var keys []string
	for name := range services {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	for _, name := range keys {
		switch name {
		case conventions.TargetRedis:
			out = append(out, Service{
				Name:   "Redis",
				Status: serviceStatus(states, conventions.TargetRedis, "running"),
				Port:   strconv.Itoa(conventions.RedisPort),
				Target: conventions.TargetRedis,
			})
		case conventions.TargetElasticsearch:
			out = append(out, Service{
				Name:   "Elasticsearch",
				Status: serviceStatus(states, conventions.TargetElasticsearch, "running"),
				Port:   strconv.Itoa(conventions.SearchPort),
				Target: conventions.TargetElasticsearch,
			})
		case conventions.TargetOpenSearch:
			out = append(out, Service{
				Name:   "OpenSearch",
				Status: serviceStatus(states, conventions.TargetOpenSearch, "running"),
				Port:   strconv.Itoa(conventions.SearchPort),
				Target: conventions.TargetOpenSearch,
			})
		case conventions.TargetVarnish:
			out = append(out, Service{
				Name:   "Varnish",
				Status: serviceStatus(states, conventions.TargetVarnish, "running"),
				Port:   strconv.Itoa(conventions.HTTPPort),
				Target: conventions.TargetVarnish,
			})
		case conventions.TargetRabbitMQ:
			out = append(out, Service{
				Name:   "RabbitMQ",
				Status: serviceStatus(states, conventions.TargetRabbitMQ, "running"),
				Port:   strconv.Itoa(conventions.RabbitMQPort),
				Target: conventions.TargetRabbitMQ,
			})
		case conventions.TargetWeb:
			out = append(out, Service{
				Name:   "Web",
				Status: serviceStatus(states, conventions.TargetWeb, "running"),
				Port:   strconv.Itoa(conventions.HTTPPort),
				Target: conventions.TargetWeb,
			})
		case conventions.TargetPHP:
			out = append(out, Service{
				Name:   "PHP",
				Status: serviceStatus(states, conventions.TargetPHP, "running"),
				Port:   strconv.Itoa(conventions.PHPFPMPort),
				Target: conventions.TargetPHP,
			})
		case conventions.TargetDB:
			out = append(out, Service{
				Name:   "Database",
				Status: serviceStatus(states, conventions.TargetDB, "running"),
				Port:   strconv.Itoa(conventions.MySQLPort),
				Target: conventions.TargetDB,
			})
		}
	}
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

var orderedServiceTargets = []string{
	conventions.TargetWeb,
	conventions.TargetPHP,
	conventions.TargetDB,
	conventions.TargetRedis,
	conventions.TargetValkey,
	conventions.TargetElasticsearch,
	conventions.TargetOpenSearch,
	conventions.TargetVarnish,
	conventions.TargetRabbitMQ,
	conventions.TargetMail,
	conventions.TargetPMA,
}

func normalizeServiceTargets(discovered map[string]bool) []string {
	if len(discovered) == 0 {
		return []string{conventions.TargetWeb}
	}

	var targets []string
	for _, name := range orderedServiceTargets {
		if discovered[name] {
			targets = append(targets, name)
		}
	}

	var extras []string
	for name := range discovered {
		found := false
		for _, existing := range targets {
			if existing == name {
				found = true
				break
			}
		}
		if !found {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	targets = append(targets, extras...)

	if len(targets) == 0 {
		return []string{conventions.TargetWeb}
	}
	return targets
}

func collectServiceTargets(info *projectInfo) []string {
	if info == nil {
		return nil
	}
	return normalizeServiceTargets(info.services)
}

func collectServiceTargetsFromServices(info *projectInfo, services []Service) []string {
	discovered := map[string]bool{}
	for _, service := range services {
		target := strings.ToLower(strings.TrimSpace(service.Target))
		if target == "" {
			continue
		}
		discovered[target] = true
	}

	if info != nil {
		for target, state := range info.serviceState {
			normalizedTarget := strings.ToLower(strings.TrimSpace(target))
			if normalizedTarget == "" || discovered[normalizedTarget] {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(state), "running") {
				discovered[normalizedTarget] = true
			}
		}
	}

	return normalizeServiceTargets(discovered)
}

func loadProjectInfoFromPath(path string) (*projectInfo, error) {
	info := &projectInfo{
		name:         filepath.Base(path),
		services:     map[string]bool{},
		serviceState: map[string]string{},
	}
	info.workingDir = path
	err := loadProjectConfig(info)
	if err != nil {
		return nil, err
	}
	return info, nil
}
