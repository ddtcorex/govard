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
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

func CheckDockerStatus() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer func() {
		_ = cli.Close()
	}()

	_, err = cli.Ping(ctx)
	return err
}

func CheckPort(port string) bool {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// CheckPortForGovardProxy returns true when the port is available OR already
// bound by the Govard proxy container (proxy-caddy), which is an expected state.
func CheckPortForGovardProxy(port string) bool {
	if CheckPort(port) {
		return true
	}
	return isPortBoundByGovardProxy(port)
}

func isPortBoundByGovardProxy(port string) bool {
	targetPort, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return false
	}
	defer func() {
		_ = cli.Close()
	}()

	// Optimize: only fetch containers that match our proxy names
	args := filters.NewArgs()
	args.Add("name", "proxy-caddy-1")
	args.Add("name", "govard-proxy-caddy")

	containers, err := cli.ContainerList(ctx, container.ListOptions{Filters: args})
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

func CheckDockerComposePlugin() error {
	command := exec.Command("docker", "compose", "version")
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
