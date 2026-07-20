package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"govard/internal/conventions"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"govard/internal/proxy"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/pterm/pterm"
)

func EnsureGlobalProxy() error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	// 1. Ensure network exists
	netName := "govard-proxy"
	nets, _ := cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.Arg("name", netName)),
	})

	if len(nets) == 0 {
		pterm.Info.Println("Creating global govard-proxy network...")
		_, err := cli.NetworkCreate(ctx, netName, network.CreateOptions{
			Driver: "bridge",
		})
		if err != nil {
			return err
		}
	}

	// 2. Check if caddy container exists (running or stopped)
	caddyFound := false
	containers, _ := cli.ContainerList(ctx, container.ListOptions{All: true})
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.Contains(name, "govard-proxy-caddy") || strings.Contains(name, "proxy-caddy-1") {
				caddyFound = true
				break
			}
		}
	}

	tempDir := filepath.Join(os.Getenv("HOME"), ".govard", "proxy")
	_ = os.MkdirAll(tempDir, conventions.DefaultDirPerm)

	blueprintsFS, err := findBlueprintsFS(".")
	if err != nil {
		return err
	}
	content, err := fs.ReadFile(blueprintsFS, "proxy.yml")
	if err != nil {
		return fmt.Errorf("could not find proxy blueprint")
	}

	proxyFile := filepath.Join(tempDir, "docker-compose.yml")
	// Always write the file to ensure we're using the latest proxy configuration
	if err := os.WriteFile(proxyFile, content, conventions.DefaultFilePerm); err != nil {
		return err
	}

	if err := RefreshPMAActiveProjects(); err != nil {
		return err
	}

	pmaConfigContent := buildPMAConfigContent()
	if err := os.WriteFile(filepath.Join(tempDir, "config.user.inc.php"), []byte(pmaConfigContent), conventions.DefaultFilePerm); err != nil {
		return err
	}

	if !caddyFound {
		pterm.Info.Println("Starting global Govard proxy...")
		cmd := exec.Command("docker", "compose", "-p", "proxy", "up", "-d")
		cmd.Dir = tempDir
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("docker compose up: %w\n%s", err, string(output))
		}
	} else {
		// If found but stopped, start it
		pterm.Debug.Println("Global proxy already exists, ensuring it is started...")
		tempDir := filepath.Join(os.Getenv("HOME"), ".govard", "proxy")
		cmd := exec.Command("docker", "compose", "-p", "proxy", "up", "-d")
		cmd.Dir = tempDir
		if output, err := cmd.CombinedOutput(); err != nil {
			pterm.Warning.Printf("Could not start existing global proxy: %v\n%s\n", err, string(output))
		}
	}

	if !waitForAnyContainerRunning(
		ctx,
		[]string{"govard-proxy-caddy", "proxy-caddy-1"},
		8*time.Second,
	) {
		return fmt.Errorf("global proxy caddy is not running (check conflicts on ports 80/443)")
	}

	if err := proxy.EnsureTLS(); err != nil {
		pterm.Warning.Printf("Could not enable HTTPS for Govard Proxy: %v\n", err)
	}

	// 3. Register global domains (ensure they are always set)
	pterm.Debug.Println("Registering global service domains...")
	_ = proxy.RegisterDomain("mail.govard.test", "govard-proxy-mail:8025")
	_ = proxy.RegisterDomain("pma.govard.test", "govard-proxy-pma:80")
	_ = proxy.RegisterDomain("portainer.govard.test", "govard-proxy-portainer:9000")

	return nil
}

func waitForAnyContainerRunning(ctx context.Context, names []string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		for _, name := range names {
			if IsContainerRunning(ctx, name) {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(250 * time.Millisecond):
		}
	}
}

type pmaActiveProjectsDocument struct {
	Projects []string `json:"projects"`
}

func RefreshPMAActiveProjects() error {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	tempDir := filepath.Join(os.Getenv("HOME"), ".govard", "proxy")
	activeProjectsPath := filepath.Join(tempDir, "..", "active-projects.json")
	activeProjects := activeProjectNamesFromContainers(containers)

	return writePMAActiveProjectsFile(activeProjectsPath, activeProjects)
}

func writePMAActiveProjectsFile(path string, projects []string) error {
	payload := pmaActiveProjectsDocument{Projects: projects}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal PMA active projects: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write PMA active projects file: %w", err)
	}
	return nil
}

func activeProjectNamesFromContainers(containers []container.Summary) []string {
	seen := map[string]bool{}
	for _, c := range containers {
		projectName, serviceName := extractComposeProjectAndServiceFromContainer(c)
		if projectName == "" || serviceName != "db" {
			continue
		}
		if c.State != "running" {
			continue
		}
		if projectName == "proxy" || projectName == "warden" {
			continue
		}
		seen[projectName] = true
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func extractComposeProjectAndServiceFromContainer(c container.Summary) (string, string) {
	project := strings.TrimSpace(c.Labels["com.docker.compose.project"])
	service := strings.TrimSpace(c.Labels["com.docker.compose.service"])
	if project != "" {
		return project, service
	}

	for _, name := range c.Names {
		clean := strings.TrimPrefix(strings.TrimSpace(name), "/")
		// Try hyphenated naming (Compose V2 default)
		if parts := strings.Split(clean, "-"); len(parts) >= 3 {
			return strings.Join(parts[:len(parts)-2], "-"), parts[len(parts)-2]
		}
		// Try underscored naming (Compose V1 / legacy default)
		if parts := strings.Split(clean, "_"); len(parts) >= 3 {
			return strings.Join(parts[:len(parts)-2], "_"), parts[len(parts)-2]
		}
	}

	return "", ""
}

func buildPMAConfigContent() string {
	return `<?php
$projectsJson = @file_get_contents('/govard-registry/projects.json');
$activeProjectsJson = @file_get_contents('/govard-registry/active-projects.json');

$dbMap = [
    'magento1' => 'magento',
    'magento2' => 'magento',
    'laravel' => 'laravel',
    'symfony' => 'symfony',
    'shopware' => 'shopware',
    'wordpress' => 'wordpress',
    'drupal' => 'drupal',
    'cakephp' => 'cakephp',
    'openmage' => 'openmage',
    'prestashop' => 'prestashop'
];

$activeProjects = [];
if ($activeProjectsJson) {
    $activePayload = json_decode($activeProjectsJson, true);
    if (isset($activePayload['projects']) && is_array($activePayload['projects'])) {
        foreach ($activePayload['projects'] as $projectName) {
            if (is_string($projectName)) {
                $trimmed = trim($projectName);
                if ($trimmed !== '') {
                    $activeProjects[$trimmed] = true;
                }
            }
        }
    }
}

$projectToServer = [];
$projectToDatabase = [];

if ($projectsJson) {
    $projects = json_decode($projectsJson, true);
    if (isset($projects['projects']) && is_array($projects['projects'])) {
        $i = 1;
        foreach ($projects['projects'] as $project) {
            $name = isset($project['project_name']) ? trim((string)$project['project_name']) : '';
            if ($name === '' || !isset($activeProjects[$name])) {
                continue;
            }

            $framework = isset($project['framework']) ? strtolower(trim((string)$project['framework'])) : '';
            $databaseName = isset($dbMap[$framework]) ? $dbMap[$framework] : 'app';
            $dbHost = $name . '-db-1';
            // Robust hostname resolution: try both common Docker Compose naming patterns.
            // @gethostbyname returns the hostname if it fails to resolve; if it fails, fallback to legacy underscore.
            if (@gethostbyname($dbHost) === $dbHost) {
                $dbHost = $name . '_db_1';
            }

            $cfg['Servers'][$i]['host'] = $dbHost;
            $cfg['Servers'][$i]['verbose'] = $name;
            $cfg['Servers'][$i]['auth_type'] = 'config';
            $cfg['Servers'][$i]['user'] = $databaseName;
            $cfg['Servers'][$i]['password'] = $databaseName;

            $projectToServer[$name] = $i;
            $projectToDatabase[$name] = $databaseName;
            $i++;
        }
    }
}

$selectedProject = '';
if (isset($_GET['project']) && is_string($_GET['project'])) {
    $selectedProject = trim($_GET['project']);
}

// Backward compatibility for legacy links that passed ?db=<project_name>.
if ($selectedProject === '' && isset($_GET['db']) && is_string($_GET['db'])) {
    $legacy = trim($_GET['db']);
    if (isset($projectToServer[$legacy])) {
        $selectedProject = $legacy;
    }
}

if ($selectedProject !== '' && isset($projectToServer[$selectedProject])) {
    $serverIndex = (string)$projectToServer[$selectedProject];
    $_GET['server'] = $serverIndex;
    $_REQUEST['server'] = $serverIndex;

    $shouldSetDatabase = !isset($_GET['db']) || !is_string($_GET['db']) || trim($_GET['db']) === '';
    if (!$shouldSetDatabase && isset($projectToServer[trim($_GET['db'])])) {
        $shouldSetDatabase = true;
    }

    if ($shouldSetDatabase && isset($projectToDatabase[$selectedProject])) {
        $database = (string)$projectToDatabase[$selectedProject];
        if ($database !== '') {
            $_GET['db'] = $database;
            $_REQUEST['db'] = $database;
        }
    }
}
`
}

func ActiveProjectNamesFromContainersForTest(containers []container.Summary) []string {
	return activeProjectNamesFromContainers(containers)
}

func BuildPMAConfigContentForTest() string {
	return buildPMAConfigContent()
}
