package desktop

import (
	"fmt"
	"govard/internal/engine"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// titleCase returns the string with the first letter capitalized.
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
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
	case "m2":
		return "magento2"
	case "m1":
		return "magento1"
	case "wp":
		return "wordpress"
	default:
		return strings.ToLower(strings.TrimSpace(framework))
	}
}

// ... (existing functions)

func displayFramework(framework string) string {
	switch framework {
	case "magento1":
		return "Magento 1"
	case "magento2":
		return "Magento 2"
	case "nextjs":
		return "Next.js"
	case "cakephp":
		return "CakePHP"
	default:
		return titleCase(framework)
	}
}

func formatDatabase(dbType, dbVersion string) string {
	if dbType == "" || dbType == "none" {
		return "No database"
	}
	label := titleCase(dbType)
	lower := strings.ToLower(label)
	if lower == "mariadb" {
		label = "MariaDB"
	} else if lower == "mysql" {
		label = "MySQL"
	} else if lower == "postgres" || lower == "postgresql" {
		label = "PostgreSQL"
	}

	if dbVersion == "" {
		return label
	}
	return fmt.Sprintf("%s %s", label, dbVersion)
}

func deriveServices(config engine.Config) []Service {
	var services []Service
	if config.Stack.Services.WebServer != "" {
		services = append(services, Service{
			Name:   titleCase(config.Stack.Services.WebServer),
			Status: "stopped",
			Port:   "80",
		})
	}
	if config.Stack.DBType != "" && config.Stack.DBType != "none" {
		label := titleCase(config.Stack.DBType)
		lower := strings.ToLower(label)
		if lower == "mariadb" {
			label = "MariaDB"
		} else if lower == "mysql" {
			label = "MySQL"
		} else if lower == "postgres" || lower == "postgresql" {
			label = "PostgreSQL"
		}
		services = append(services, Service{
			Name:   label,
			Status: "stopped",
			Port:   "3306",
		})
	}
	if config.Stack.PHPVersion != "" {
		services = append(services, Service{Name: "PHP", Status: "stopped", Port: "9000"})
	}
	switch config.Stack.Services.Cache {
	case "redis":
		services = append(services, Service{Name: "Redis", Status: "stopped", Port: "6379"})
	case "valkey":
		services = append(services, Service{Name: "Valkey", Status: "stopped", Port: "6379"})
	}
	switch config.Stack.Services.Search {
	case "opensearch":
		services = append(services, Service{Name: "OpenSearch", Status: "stopped", Port: "9200"})
	case "elasticsearch":
		services = append(services, Service{Name: "Elasticsearch", Status: "stopped", Port: "9200"})
	}
	if config.Stack.Services.Queue == "rabbitmq" {
		services = append(services, Service{Name: "RabbitMQ", Status: "stopped", Port: "5672"})
	}
	if config.Stack.Features.Varnish {
		services = append(services, Service{Name: "Varnish", Status: "stopped", Port: "80"})
	}
	return services
}

func fallbackServices(services map[string]bool) []Service {
	var out []Service
	var keys []string
	for name := range services {
		keys = append(keys, name)
	}
	sort.Strings(keys)

	for _, name := range keys {
		switch name {
		case "redis":
			out = append(out, Service{Name: "Redis", Status: "running", Port: "6379"})
		case "elasticsearch":
			out = append(out, Service{Name: "Search", Status: "running", Port: "9200"})
		case "varnish":
			out = append(out, Service{Name: "Varnish", Status: "running", Port: "80"})
		case "rabbitmq":
			out = append(out, Service{Name: "RabbitMQ", Status: "running", Port: "5672"})
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

func loadProjectInfoFromPath(path string) (*projectInfo, error) {
	info := &projectInfo{
		name:     filepath.Base(path),
		services: map[string]bool{},
	}
	info.workingDir = path
	err := loadProjectConfig(info)
	if err != nil {
		return nil, err
	}
	return info, nil
}
