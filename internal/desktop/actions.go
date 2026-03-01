package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"govard/internal/engine"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/pkg/browser"
	"gopkg.in/yaml.v3"
)

func quickAction(ctx context.Context, action string, project string) (string, error) {
	switch action {
	case "open-mail", "open-mail-client":
		return openDestination(ctx, buildProxyURL("mail"), "Opening Mailpit...")
	case "open-pma":
		return openDestination(ctx, buildProxyURL("pma"), "Opening PHPMyAdmin...")
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
	settings, err := getSettingsInternal()
	if err == nil && settings.DBClientPreference == "pma" {
		if err := browser.OpenURL("https://pma.govard.test/?db=" + project); err != nil {
			return "", fmt.Errorf("failed to open PHPMyAdmin URL: %w", err)
		}
		return "PMA Opened", nil
	}

	info, _ := loadProjectInfo(project)
	if info == nil {
		info = &projectInfo{}
	}

	containerName := project + "-db-1"

	user := "magento"
	pass := "magento"
	db := "magento"

	envCmd := exec.Command("docker", "inspect", "-f", "{{range .Config.Env}}{{println .}}{{end}}", containerName)
	if envOut, err := envCmd.Output(); err == nil {
		lines := strings.Split(string(envOut), "\n")
		for _, line := range lines {
			parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
			if len(parts) == 2 {
				switch parts[0] {
				case "MYSQL_USER":
					if parts[1] != "" {
						user = parts[1]
					}
				case "POSTGRES_USER":
					if parts[1] != "" {
						user = parts[1]
					}
				case "MYSQL_PASSWORD":
					if parts[1] != "" {
						pass = parts[1]
					}
				case "POSTGRES_PASSWORD":
					if parts[1] != "" {
						pass = parts[1]
					}
				case "MYSQL_DATABASE":
					if parts[1] != "" {
						db = parts[1]
					}
				case "POSTGRES_DB":
					if parts[1] != "" {
						db = parts[1]
					}
				}
			}
		}
	}

	scheme := "mysql"
	internalPort := "3306"
	if info.configLoaded {
		dbType := strings.ToLower(info.config.Stack.DBType)
		if strings.Contains(dbType, "postgres") {
			scheme = "postgresql"
			internalPort = "5432"
		}
	}

	port := internalPort
	portCmd := exec.Command("docker", "port", containerName, internalPort)
	if portOut, err := portCmd.Output(); err == nil {
		res := strings.TrimSpace(string(portOut))
		parts := strings.Split(res, "\n")
		if len(parts) > 0 {
			idx := strings.LastIndex(parts[0], ":")
			if idx != -1 {
				port = parts[0][idx+1:]
			}
		}
	}

	host := "127.0.0.1"
	ipCmd := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", containerName)
	if ipOut, err := ipCmd.Output(); err == nil {
		ip := strings.TrimSpace(string(ipOut))
		if ip != "" {
			host = ip
		}
	}

	urlStr := fmt.Sprintf("%s://%s:%s@%s:%s/%s", scheme, user, pass, host, port, db)
	_ = browser.OpenURL(urlStr)

	return "Opening DB Client...", nil
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
	config := info.config
	if config.ProjectName == "" {
		config.ProjectName = info.name
	}
	config.Stack.Features.Xdebug = !config.Stack.Features.Xdebug

	if info.configPath == "" {
		return "", fmt.Errorf(".govard.yml path unavailable")
	}
	writableConfig := engine.PrepareConfigForWrite(config)

	data, err := yaml.Marshal(&writableConfig)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(info.configPath, data, 0644); err != nil {
		return "", err
	}

	if err := engine.RenderBlueprint(info.workingDir, config); err != nil {
		return "", err
	}

	composePath := engine.ComposeFilePath(info.workingDir, config.ProjectName)
	if err := runCompose(info.workingDir, config.ProjectName, composePath); err != nil {
		return "", err
	}

	state := "enabled"
	if !config.Stack.Features.Xdebug {
		state = "disabled"
	}
	return "Xdebug " + state + " for " + info.name, nil
}

func startEnvironment(project string) (string, error) {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", err
	}
	args := filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+info.name))
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", fmt.Errorf("no containers found")
	}
	for _, c := range containers {
		if c.State == "running" {
			continue
		}
		if err := cli.ContainerStart(ctx, c.ID, container.StartOptions{}); err != nil {
			return "", err
		}
	}
	return "Started environment " + info.name, nil
}

func stopEnvironment(project string) (string, error) {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", err
	}
	args := filters.NewArgs(filters.Arg("label", "com.docker.compose.project="+info.name))
	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return "", err
	}
	if len(containers) == 0 {
		return "", fmt.Errorf("no containers found")
	}
	timeout := 10
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		if err := cli.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout}); err != nil {
			return "", err
		}
	}
	return "Stopped environment " + info.name, nil
}

func restartEnvironment(project string) (string, error) {
	if _, err := stopEnvironment(project); err != nil {
		return "", fmt.Errorf("stop phase: %w", err)
	}
	if _, err := startEnvironment(project); err != nil {
		return "", fmt.Errorf("start phase: %w", err)
	}
	return "Restarted environment " + project, nil
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

func runCompose(dir, project, composeFile string) error {
	args := []string{"compose", "--project-directory", filepath.Clean(dir)}
	if project != "" {
		args = append(args, "-p", project)
	}
	if composeFile != "" {
		args = append(args, "-f", composeFile)
	}
	args = append(args, "up", "-d")

	cmd := exec.Command("docker", args...)
	cmd.Dir = filepath.Clean(dir)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
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
	if shell == "sh" {
		return "sh"
	}
	return "bash"
}

func normalizeShellUser(info *projectInfo, service string, user string) string {
	if user != "" {
		return user
	}
	if info != nil {
		if saved, err := getShellUser(info.name); err == nil && saved != "" {
			return saved
		}
	}
	if service == "php" || service == "" {
		return "www-data"
	}
	if service == "apache" {
		return "www-data"
	}
	if info != nil && info.configLoaded && info.config.Stack.Services.WebServer == "apache" && service == "web" {
		return "www-data"
	}
	return ""
}

// End of actions.go

func resolveShellContainer(info *projectInfo, service string) string {
	target := resolveServiceName(info, service, "php")
	return fmt.Sprintf("%s-%s-1", info.name, target)
}

func resolveServiceName(info *projectInfo, service string, fallback string) string {
	candidate := strings.TrimSpace(service)
	if candidate != "" {
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
