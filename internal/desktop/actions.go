package desktop

import (
	"context"
	"fmt"
	"net"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"govard/internal/conventions"

	"github.com/pkg/browser"
)

var defaultOpenExternalURLForDesktop = browser.OpenURL
var openExternalURLForDesktop = defaultOpenExternalURLForDesktop

func quickAction(ctx context.Context, action string, project string) (string, error) {
	switch action {
	case "open-mail", "open-mail-client":
		return openDestination(ctx, buildProxyURL(conventions.TargetMail), "Opening Mailpit...")
	case "open-pma":
		return openDestination(ctx, buildProxyURL(conventions.TargetPMA), "Opening PHPMyAdmin...")
	case "open-db-client":
		return openDBClient(ctx, project)
	case "toggle-xdebug":
		return toggleXdebug(project)
	case "check-health":
		return checkHealth()
	case "open-folder":
		return openFolder(project)
	case "open-ide":
		return openIDE(project)
	default:
		return "", fmt.Errorf("unknown action")
	}
}

func openDBClient(ctx context.Context, project string) (string, error) {
	normalizedProject := strings.TrimSpace(project)
	if normalizedProject == "" {
		info, err := selectProject("")
		if err != nil {
			return "", err
		}
		normalizedProject = strings.TrimSpace(info.name)
	}

	settings, err := getSettingsInternal()
	preferPMA := err == nil && settings.DBClientPreference == "pma"

	info, err := loadProjectInfo(normalizedProject)
	if err != nil {
		info = &projectInfo{name: normalizedProject}
	}

	containerProjectName := strings.TrimSpace(info.name)
	if containerProjectName == "" {
		containerProjectName = normalizedProject
	}
	containerName := containerProjectName + conventions.DBSuffix

	user := conventions.DefaultMagentoDBUser
	pass := conventions.DefaultMagentoDBPass
	db := conventions.DefaultMagentoDBName

	envCmd := exec.Command("docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", containerName)
	if envOut, err := envCmd.Output(); err == nil {
		lines := strings.Split(string(envOut), "\n")
		for _, line := range lines {
			parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
			if len(parts) == 2 {
				switch parts[0] {
				case conventions.EnvMySQLUser, conventions.EnvPostgresUser:
					if parts[1] != "" {
						user = parts[1]
					}
				case conventions.EnvMySQLPassword, conventions.EnvPostgresPassword:
					if parts[1] != "" {
						pass = parts[1]
					}
				case conventions.EnvMySQLDatabase, conventions.EnvPostgresDatabase:
					if parts[1] != "" {
						db = parts[1]
					}
				}
			}
		}
	}

	if preferPMA {
		target := buildPMAOpenURL(containerProjectName, db)
		return openDestination(ctx, target, "Opening PHPMyAdmin...")
	}

	scheme := "mysql"
	internalPort := strconv.Itoa(conventions.MySQLPort)
	if info.configLoaded {
		dbType := strings.ToLower(info.config.Stack.Services.DB)
		if strings.Contains(dbType, conventions.ServicePostgreSQL) {
			scheme = "postgresql"
			internalPort = defaultDatabasePortForType(info.config.Stack.Services.DB)
		}
	}

	host := "127.0.0.1"
	port := internalPort
	portCmd := exec.Command("docker", "port", containerName, internalPort)
	if portOut, err := portCmd.Output(); err == nil {
		if mappedHost, mappedPort, ok := parseDockerPublishedPort(strings.TrimSpace(string(portOut))); ok {
			host = mappedHost
			port = mappedPort
		}
	}

	if host == "127.0.0.1" && port == internalPort {
		ipCmd := exec.Command(
			"docker",
			"inspect",
			"-f",
			"{{range .NetworkSettings.Networks}}{{println .IPAddress}}{{end}}",
			containerName,
		)
		if ipOut, err := ipCmd.Output(); err == nil {
			candidates := parseContainerIPAddresses(string(ipOut))
			if resolvedHost := chooseReachableContainerHost(candidates, port); resolvedHost != "" {
				host = resolvedHost
			}
		}
	}

	urlStr := buildDesktopDBClientURL(scheme, user, pass, host, port, db)
	if err := openExternalURLForDesktop(urlStr); err != nil {
		fallbackTarget := buildPMAOpenURL(containerProjectName, db)
		if _, fallbackErr := openDestination(
			ctx,
			fallbackTarget,
			"Desktop DB client is unavailable. Opening PHPMyAdmin...",
		); fallbackErr != nil {
			return "", fmt.Errorf("failed to open desktop DB client (%v) and fallback PMA (%w)", err, fallbackErr)
		}
		return fmt.Sprintf("Desktop DB client unavailable, opened PHPMyAdmin for %s.", db), nil
	}

	return "Opening DB Client...", nil
}

func buildPMAOpenURL(project string, database string) string {
	target := buildProxyURL(conventions.TargetPMA)
	params := neturl.Values{}

	if normalizedProject := strings.TrimSpace(project); normalizedProject != "" {
		params.Set("project", normalizedProject)
	}

	if normalizedDatabase := strings.TrimSpace(database); normalizedDatabase != "" {
		params.Set("db", normalizedDatabase)
	}

	encoded := params.Encode()
	if encoded == "" {
		return target
	}
	return target + "/?" + encoded
}

func parseDockerPublishedPort(raw string) (string, string, bool) {
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		candidate := strings.TrimSpace(line)
		if candidate == "" {
			continue
		}
		if strings.Contains(candidate, "->") {
			parts := strings.Split(candidate, "->")
			candidate = strings.TrimSpace(parts[len(parts)-1])
		}
		candidate = strings.TrimPrefix(candidate, "tcp://")

		host, port, err := net.SplitHostPort(candidate)
		if err != nil {
			idx := strings.LastIndex(candidate, ":")
			if idx <= 0 || idx >= len(candidate)-1 {
				continue
			}
			host = strings.TrimSpace(candidate[:idx])
			port = strings.TrimSpace(candidate[idx+1:])
		}

		host = normalizeDockerPublishedHost(host)
		if port == "" {
			continue
		}
		if _, err := strconv.Atoi(port); err != nil {
			continue
		}

		return host, port, true
	}

	return "", "", false
}

func normalizeDockerPublishedHost(rawHost string) string {
	host := strings.Trim(strings.TrimSpace(rawHost), "[]")
	if host == "" || host == "0.0.0.0" || host == "::" || host == "*" {
		return "127.0.0.1"
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsUnspecified() {
			return "127.0.0.1"
		}
		return host
	}

	// Some runtimes emit host values like "192.168.65.2:172.18.0.4";
	// pick the first concrete address token to avoid malformed URLs.
	if strings.Contains(host, ":") {
		for _, token := range strings.Split(host, ":") {
			candidate := strings.Trim(strings.TrimSpace(token), "[]")
			if candidate == "" {
				continue
			}
			if ip := net.ParseIP(candidate); ip != nil {
				if ip.IsUnspecified() {
					continue
				}
				return candidate
			}
		}
		return "127.0.0.1"
	}

	return host
}

func buildDesktopDBClientURL(scheme string, user string, pass string, host string, port string, db string) string {
	normalizedScheme := strings.TrimSpace(scheme)
	if normalizedScheme == "" {
		normalizedScheme = "mysql"
	}

	normalizedHost := strings.TrimSpace(host)
	if normalizedHost == "" {
		normalizedHost = "127.0.0.1"
	}

	normalizedPort := strings.TrimSpace(port)
	if normalizedPort == "" {
		if normalizedScheme == "postgresql" {
			normalizedPort = strconv.Itoa(conventions.PostgresPort)
		} else {
			normalizedPort = strconv.Itoa(conventions.MySQLPort)
		}
	}

	dbPath := "/" + strings.TrimPrefix(strings.TrimSpace(db), "/")
	if dbPath == "/" {
		dbPath = ""
	}

	connectionURL := &neturl.URL{
		Scheme: normalizedScheme,
		User:   neturl.UserPassword(user, pass),
		Host:   net.JoinHostPort(normalizedHost, normalizedPort),
		Path:   dbPath,
	}
	return connectionURL.String()
}

func parseContainerIPAddresses(raw string) []string {
	lines := strings.Split(raw, "\n")
	seen := map[string]bool{}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		candidate := strings.Trim(strings.TrimSpace(line), "[]")
		if candidate == "" {
			continue
		}
		ip := net.ParseIP(candidate)
		if ip == nil || ip.IsUnspecified() {
			continue
		}
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		result = append(result, candidate)
	}
	return result
}

func chooseReachableContainerHost(candidates []string, port string) string {
	for _, candidate := range candidates {
		target := net.JoinHostPort(candidate, port)
		conn, err := net.DialTimeout("tcp", target, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return candidate
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0]
}

func openFolder(project string) (string, error) {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}
	dir := info.workingDir
	if dir == "" {
		return "", fmt.Errorf("project directory not found")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default: // linux
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to open folder: %w", err)
	}
	return "Opening folder: " + dir, nil
}

func openIDE(project string) (string, error) {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}
	dir := info.workingDir
	if dir == "" {
		return "", fmt.Errorf("project directory not found")
	}

	settings, _ := getSettingsInternal()
	editor := strings.TrimSpace(settings.CodeEditor)
	if editor == "" {
		editor = "code" // Default to VS Code
	}

	// For some editors like VS Code, we might need to handle it specifically if they are not in PATH,
	// but usually they are.
	cmd := exec.Command(editor, dir)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to launch IDE (%s): %w", editor, err)
	}
	return fmt.Sprintf("Opening %s in %s", filepath.Base(dir), editor), nil
}

func toggleXdebug(project string) (string, error) {
	info, err := selectProject(project)
	if err != nil {
		return "", err
	}
	if !info.configLoaded {
		return "", fmt.Errorf(".govard.yml not found for %s", info.name)
	}

	root, _, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", err
	}

	// Determine toggle action
	action := "on"
	if info.config.Stack.Features.Xdebug {
		action = "off"
	}

	output, err := runGovardCommandForDesktop(root, []string{"debug", action})
	if err != nil {
		return "", err
	}

	state := "enabled"
	if action == "off" {
		state = "disabled"
	}
	return withCommandOutput("Xdebug "+state+" for "+info.name, output), nil
}

func checkHealth() (string, error) {
	dashboard, err := buildDashboardInternal()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Docker OK. %d environments, %d services running.", len(dashboard.Environments), dashboard.RunningServices), nil
}

func selectProject(project string) (*projectInfo, error) {
	if project != "" {
		return loadProjectInfo(project)
	}

	dashboard, err := buildDashboardInternal()
	if err != nil {
		return nil, err
	}
	if len(dashboard.Environments) == 0 {
		return nil, fmt.Errorf("no environments found")
	}

	selected := dashboard.Environments[0].Project
	for _, env := range dashboard.Environments {
		if env.Status == "running" {
			selected = env.Project
			break
		}
	}

	return loadProjectInfo(selected)
}

func openDestination(ctx context.Context, url string, message string) (string, error) {
	if err := openURLWithPreferences(ctx, url); err != nil {
		return message + " Open manually: " + url, nil
	}
	return message, nil
}

func openURLWithPreferences(ctx context.Context, url string) error {
	settings, err := getSettingsInternal()
	if err == nil && settings.PreferredBrowser != "" {
		cmd := exec.Command(settings.PreferredBrowser, url)
		if err := cmd.Start(); err == nil {
			return nil
		}
	}
	return openURL(ctx, url)
}

func buildProxyURL(host string) string {
	domain := resolveProxyDomain()
	return "https://" + host + "." + domain
}

func resolveProxyDomain() string {
	settings, err := getSettingsInternal()
	if err != nil {
		return "govard.test"
	}
	target := strings.TrimSpace(settings.ProxyTarget)
	if target == "" {
		return "govard.test"
	}
	if strings.Contains(target, ".") || strings.Contains(target, ":") {
		return target
	}
	return target + ".test"
}

func openDocs(ctx context.Context, docPath string) error {
	root, err := FindRepoRoot()
	if err != nil {
		return err
	}
	fullPath := filepath.Join(root, docPath)
	_, err = os.Stat(fullPath)
	if err != nil {
		return err
	}
	return openURLWithPreferences(ctx, "file://"+fullPath)
}

// Log helpers moved to stream.go

// Log helpers moved to stream.go

// Shell functions moved to LogService/SystemService

func normalizeShell(shell string) string {
	normalized := strings.ToLower(strings.TrimSpace(shell))
	switch normalized {
	case "bash", "sh":
		return normalized
	default:
		return "bash"
	}
}

func normalizeShellUser(info *projectInfo, service string, user string) string {
	if user != "" {
		return user
	}
	if info != nil {
		if info.configLoaded {
			// Match CLI behavior: use ResolveProjectExecUser for consistency (UID:GID mapping)
			return info.config.ResolveProjectExecUser(conventions.UserWWWData)
		}
	}
	if service == conventions.TargetPHP || service == "" {
		return conventions.UserWWWData
	}
	if service == "apache" {
		return conventions.UserWWWData
	}
	if info != nil && info.configLoaded && info.config.Stack.Services.WebServer == "apache" && service == conventions.TargetWeb {
		return conventions.UserWWWData
	}
	return ""
}

// End of actions.go

func resolveShellContainer(info *projectInfo, service string) string {
	target := resolveShellServiceName(info, service)
	return fmt.Sprintf("%s-%s%s", info.name, target, conventions.ReplicaSuffix)
}

func resolveShellServiceName(info *projectInfo, service string) string {
	return resolveServiceName(info, service, defaultShellService(info))
}

func defaultShellService(info *projectInfo) string {
	if info == nil || len(info.services) == 0 {
		return conventions.TargetPHP
	}

	for _, preferred := range []string{conventions.TargetWeb, conventions.TargetPHP, conventions.TargetApp} {
		if info.services[preferred] {
			return preferred
		}
	}

	candidates := make([]string, 0, len(info.services))
	for name := range info.services {
		candidates = append(candidates, name)
	}
	sort.Strings(candidates)
	if len(candidates) > 0 {
		return candidates[0]
	}

	return conventions.TargetPHP
}

func resolveServiceName(info *projectInfo, service string, fallback string) string {
	candidate := strings.TrimSpace(service)
	if candidate != "" && candidate != "all" {
		if info == nil || info.services[candidate] {
			return candidate
		}
	}
	if info != nil {
		if info.services[fallback] {
			return fallback
		}
		for name := range info.services {
			return name
		}
	}
	return fallback
}
