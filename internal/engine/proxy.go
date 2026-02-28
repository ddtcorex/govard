package engine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	_ = os.MkdirAll(tempDir, 0755)

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
	if err := os.WriteFile(proxyFile, content, 0644); err != nil {
		return err
	}

	pmaConfigContent := `<?php
$projectsJson = @file_get_contents('/tmp/projects.json');
$dbMap = [
    'magento1' => 'magento',
    'magento2' => 'magento',
    'laravel' => 'laravel',
    'symfony' => 'symfony',
    'shopware' => 'shopware',
    'wordpress' => 'wordpress',
    'drupal' => 'drupal',
    'cakephp' => 'cakephp',
    'openmage' => 'openmage'
];

if ($projectsJson) {
    $projects = json_decode($projectsJson, true);
    if (isset($projects['projects']) && is_array($projects['projects'])) {
        $i = 1;
        foreach ($projects['projects'] as $p) {
            $name = isset($p['project_name']) ? $p['project_name'] : '';
            $framework = isset($p['framework']) ? $p['framework'] : '';
            if ($name) {
                $dbHost = $name . '-db-1';
                $dbName = isset($dbMap[$framework]) ? $dbMap[$framework] : 'app';
                
                $cfg['Servers'][$i]['host'] = $dbHost;
                $cfg['Servers'][$i]['verbose'] = $name;
                $cfg['Servers'][$i]['auth_type'] = 'config';
                $cfg['Servers'][$i]['user'] = $dbName;
                $cfg['Servers'][$i]['password'] = $dbName;
                $i++;
            }
        }
    }
}
`
	if err := os.WriteFile(filepath.Join(tempDir, "config.user.inc.php"), []byte(pmaConfigContent), 0644); err != nil {
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

	if err := proxy.EnsureTLS(); err != nil {
		pterm.Warning.Printf("Could not enable HTTPS for Govard Proxy: %v\n", err)
	}

	// 3. Register global domains (ensure they are always set)
	pterm.Debug.Println("Registering global service domains...")
	_ = proxy.RegisterDomain("mail.govard.test", "govard-proxy-mail:8025")
	_ = proxy.RegisterDomain("pma.govard.test", "govard-proxy-pma:80")

	return nil
}
