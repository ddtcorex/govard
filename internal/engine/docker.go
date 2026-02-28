package engine

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

func CheckDockerStatus(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cli, err := GetDockerClient()
	if err != nil {
		return err
	}

	_, err = cli.Ping(ctx)
	return err
}

func CheckPort(port string) error {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return err
	}
	ln.Close()
	return nil
}

// CheckPortForGovardProxy returns true when the port is available OR already
// bound by the Govard proxy container (proxy-caddy), which is an expected state.
// It includes a retry logic to handle race conditions during container restarts
// and properly handles permission errors for privileged ports.
func CheckPortForGovardProxy(ctx context.Context, port string) bool {
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		err := CheckPort(port)
		if err == nil {
			return true
		}

		// If it's a permission denied error, we can't listen on this port,
		// but it doesn't necessarily mean it's in use. We rely on Docker check.
		if strings.Contains(err.Error(), "permission denied") {
			if isPortBoundByGovardProxy(ctx, port) {
				return true
			}
			// If not bound by our proxy, but we can't check further,
			// we check if ANY other container is binding it.
			if isPortBoundByOtherContainer(ctx, port) {
				return false
			}
			// If no other container is binding it, we assume we might be good
			// but we still wait a bit in case of transition.
			if i == maxRetries-1 {
				return true // Best effort: assume it's free if Docker says so
			}
			select {
			case <-ctx.Done():
				return false
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}

		// If the error is "address already in use", check if it's our proxy
		if isPortBoundByGovardProxy(ctx, port) {
			return true
		}

		// If we are here, port is in use but not by Govard proxy (yet).
		// Give it a moment to settle (e.g., during restart) before failing.
		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	return false
}

func isPortBoundByOtherContainer(ctx context.Context, port string) bool {
	targetPort, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cli, err := GetDockerClient()
	if err != nil {
		return false
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return false
	}

	for _, c := range containers {
		// Skip our own proxy
		if isGovardProxyContainer(c.Names) {
			continue
		}
		for _, published := range c.Ports {
			if published.Type == "tcp" && int(published.PublicPort) == targetPort {
				return true
			}
		}
	}

	return false
}

func isPortBoundByGovardProxy(ctx context.Context, port string) bool {
	targetPort, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cli, err := GetDockerClient()
	if err != nil {
		return false
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return false
	}

	for _, c := range containers {
		if !isGovardProxyContainer(c.Names) {
			continue
		}
		for _, published := range c.Ports {
			if published.Type == "tcp" && int(published.PublicPort) == targetPort {
				return true
			}
		}
	}

	return false
}

func isGovardProxyContainer(names []string) bool {
	for _, name := range names {
		clean := strings.TrimPrefix(name, "/")
		if clean == "proxy-caddy-1" || clean == "govard-proxy-caddy" {
			return true
		}
	}
	return false
}

func IsContainerRunning(ctx context.Context, name string) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cli, err := GetDockerClient()
	if err != nil {
		return false
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return false
	}

	for _, c := range containers {
		for _, cname := range c.Names {
			if strings.TrimPrefix(cname, "/") == name {
				return true
			}
		}
	}

	return false
}

func CheckDockerComposePlugin(ctx context.Context) error {
	command := exec.CommandContext(ctx, "docker", "compose", "version")
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return fmt.Errorf("docker compose plugin is not available: %w", err)
	}
	return fmt.Errorf("docker compose plugin is not available: %w (%s)", err, trimmed)
}

func GetRunningProjectNames(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cli, err := GetDockerClient()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	containers, err := cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	projectMap := make(map[string]bool)
	for _, c := range containers {
		for _, name := range c.Names {
			cleanName := strings.TrimPrefix(name, "/")
			// Standard Govard naming pattern: projectname-service-1
			parts := strings.Split(cleanName, "-")
			if len(parts) >= 3 {
				projectName := strings.Join(parts[:len(parts)-2], "-")
				// Basic filtering - only consider it a govard project if it has standard services
				serviceName := parts[len(parts)-2]
				if serviceName == "web" || serviceName == "php" || serviceName == "db" {
					projectMap[projectName] = true
				}
			}
		}
	}

	projects := make([]string, 0, len(projectMap))
	for p := range projectMap {
		projects = append(projects, p)
	}
	return projects, nil
}
