package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"govard/internal/engine"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

const globalServicesComposeProjectName = "proxy"
const routingConflictWarningPrefix = "Port conflict "

var errDesktopGlobalServicesNotInitialized = errors.New("global services are not initialized")
var ssProcessPattern = regexp.MustCompile(`users:\(\("([^"]+)",pid=(\d+)`)

type portProbeTarget struct {
	Port     int
	Protocol string
}

type hostPortOwner struct {
	Port     int
	Protocol string
	Command  string
	PID      string
	User     string
}

var routingPortProbeTargets = []portProbeTarget{
	{Port: 53, Protocol: "udp"},
	{Port: 80, Protocol: "tcp"},
	{Port: 443, Protocol: "tcp"},
}

var routingServiceExpectedPublishedPorts = map[string][]string{
	"caddy":   {"80/tcp", "443/tcp"},
	"dnsmasq": {"53/udp", "53/tcp"},
}

type globalServiceSpec struct {
	ID             string
	Name           string
	ComposeService string
	ContainerName  string
	URLHost        string
}

var globalServiceSpecs = []globalServiceSpec{
	{
		ID:             "caddy",
		Name:           "Caddy Proxy",
		ComposeService: "caddy",
		ContainerName:  "govard-proxy-caddy",
	},
	{
		ID:             "mail",
		Name:           "Mailpit",
		ComposeService: "mail",
		ContainerName:  "govard-proxy-mail",
		URLHost:        "mail",
	},
	{
		ID:             "pma",
		Name:           "PHPMyAdmin",
		ComposeService: "pma",
		ContainerName:  "govard-proxy-pma",
		URLHost:        "pma",
	},
	{
		ID:             "portainer",
		Name:           "Portainer",
		ComposeService: "portainer",
		ContainerName:  "govard-proxy-portainer",
		URLHost:        "portainer",
	},
	{
		ID:             "dnsmasq",
		Name:           "DNSMasq",
		ComposeService: "dnsmasq",
		ContainerName:  "govard-proxy-dnsmasq",
	},
}

var defaultEnsureGlobalServicesForDesktop = func() error {
	if err := ensureGlobalComposeFileExists(); err == nil {
		return nil
	}

	if err := engine.EnsureGlobalProxy(); err != nil {
		return fmt.Errorf("ensure global proxy: %w", err)
	}
	return ensureGlobalComposeFileExists()
}

var ensureGlobalServicesForDesktop = defaultEnsureGlobalServicesForDesktop

var defaultRunGlobalServicesComposeForDesktop = func(args ...string) (string, error) {
	composeFile := globalServicesComposeFilePath()
	composeDir := globalServicesComposeDirPath()

	if err := ensureGlobalComposeFileExists(); err != nil {
		return "", err
	}

	dockerArgs := []string{
		"compose",
		"--project-directory",
		composeDir,
		"-p",
		globalServicesComposeProjectName,
		"-f",
		composeFile,
	}
	dockerArgs = append(dockerArgs, args...)

	command := exec.Command("docker", dockerArgs...)
	output, err := command.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed != "" {
			return "", fmt.Errorf("%w: %s", err, trimmed)
		}
		return "", err
	}
	return trimmed, nil
}

var runGlobalServicesComposeForDesktop = defaultRunGlobalServicesComposeForDesktop

var defaultRunHostPortProbeForDesktop = func(binary string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, binary, args...)
	output, err := command.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed != "" {
			return trimmed, fmt.Errorf("%w: %s", err, trimmed)
		}
		return trimmed, err
	}
	return trimmed, nil
}

var runHostPortProbeForDesktop = defaultRunHostPortProbeForDesktop

func (s *GlobalServiceService) GetGlobalServices() (GlobalServicesSnapshot, error) {
	snapshot := GlobalServicesSnapshot{
		Total:    len(globalServiceSpecs),
		Services: make([]GlobalService, 0, len(globalServiceSpecs)),
	}

	containersByName := map[string]container.Summary{}
	allContainers := []container.Summary{}
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		snapshot.Warnings = append(snapshot.Warnings, "Docker client error: "+err.Error())
	} else if containers, listErr := cli.ContainerList(ctx, container.ListOptions{All: true}); listErr != nil {
		snapshot.Warnings = append(snapshot.Warnings, "Docker unavailable: "+listErr.Error())
	} else {
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
	}

	for _, spec := range globalServiceSpecs {
		service := GlobalService{
			ID:             spec.ID,
			Name:           spec.Name,
			ComposeService: spec.ComposeService,
			ContainerName:  spec.ContainerName,
			Status:         "missing",
			State:          "not-created",
			Health:         "unknown",
			StatusText:     "Container not created",
			Running:        false,
			Openable:       spec.URLHost != "",
		}
		if spec.URLHost != "" {
			service.URL = buildProxyURL(spec.URLHost)
		}

		if c, ok := containersByName[spec.ContainerName]; ok {
			status, health, running := deriveGlobalContainerStatus(c.State, c.Status)
			service.Status = status
			service.State = strings.TrimSpace(c.State)
			if service.State == "" {
				service.State = "unknown"
			}
			service.Health = health
			service.StatusText = strings.TrimSpace(c.Status)
			if service.StatusText == "" {
				service.StatusText = service.State
			}
			service.Running = running
		}

		if service.Running {
			snapshot.Active++
		}
		snapshot.Services = append(snapshot.Services, service)
	}

	routingBindingWarnings := detectRoutingPublishedPortBindingWarnings(snapshot.Services, containersByName)
	snapshot.Warnings = append(snapshot.Warnings, routingBindingWarnings...)

	if hasRoutingConflictSignal(snapshot.Services) || len(routingBindingWarnings) > 0 {
		snapshot.Warnings = append(snapshot.Warnings, detectRoutingPortConflictWarnings(allContainers)...)
	}

	snapshot.Warnings = uniqueStrings(snapshot.Warnings)
	snapshot.Summary = fmt.Sprintf("%d/%d global services running", snapshot.Active, snapshot.Total)
	return snapshot, nil
}

func (s *GlobalServiceService) StartGlobalServices() (string, error) {
	root, err := resolveDesktopGovardCommandDir()
	if err != nil {
		return "", err
	}
	output, err := runGovardCommandForDesktop(root, []string{"svc", "up"})
	if err != nil {
		return "", err
	}
	return withCommandOutput("Global services started.", output), nil
}

func (s *GlobalServiceService) StopGlobalServices() (string, error) {
	root, err := resolveDesktopGovardCommandDir()
	if err != nil {
		return "", err
	}
	output, err := runGovardCommandForDesktop(root, []string{"svc", "stop"})
	if err != nil {
		return "", err
	}
	return withCommandOutput("Global services stopped.", output), nil
}

func (s *GlobalServiceService) RestartGlobalServices() (string, error) {
	root, err := resolveDesktopGovardCommandDir()
	if err != nil {
		return "", err
	}
	output, err := runGovardCommandForDesktop(root, []string{"svc", "restart"})
	if err != nil {
		return "", err
	}
	return withCommandOutput("Global services restarted.", output), nil
}

func (s *GlobalServiceService) PullGlobalServices() (string, error) {
	root, err := resolveDesktopGovardCommandDir()
	if err != nil {
		return "", err
	}
	output, err := runGovardCommandForDesktop(root, []string{"svc", "pull"})
	if err != nil {
		return "", err
	}
	return withCommandOutput("Global services images pulled.", output), nil
}

func (s *GlobalServiceService) StartGlobalService(serviceID string) (string, error) {
	spec, err := resolveGlobalServiceSpec(serviceID)
	if err != nil {
		return "", err
	}
	if err := ensureGlobalServicesForDesktop(); err != nil {
		return "", err
	}
	out, err := runGlobalServicesComposeForDesktop("up", "-d", spec.ComposeService)
	if err != nil {
		return "", fmt.Errorf("start %s: %w", spec.Name, err)
	}
	return withCommandOutput(fmt.Sprintf("%s started.", spec.Name), out), nil
}

func (s *GlobalServiceService) StopGlobalService(serviceID string) (string, error) {
	spec, err := resolveGlobalServiceSpec(serviceID)
	if err != nil {
		return "", err
	}
	if err := ensureGlobalServicesForDesktop(); err != nil {
		return "", err
	}
	out, err := runGlobalServicesComposeForDesktop("stop", spec.ComposeService)
	if err != nil {
		return "", fmt.Errorf("stop %s: %w", spec.Name, err)
	}
	return withCommandOutput(fmt.Sprintf("%s stopped.", spec.Name), out), nil
}

func (s *GlobalServiceService) RestartGlobalService(serviceID string) (string, error) {
	spec, err := resolveGlobalServiceSpec(serviceID)
	if err != nil {
		return "", err
	}
	if err := ensureGlobalServicesForDesktop(); err != nil {
		return "", err
	}
	out, err := runGlobalServicesComposeForDesktop("restart", spec.ComposeService)
	if err != nil {
		return "", fmt.Errorf("restart %s: %w", spec.Name, err)
	}
	return withCommandOutput(fmt.Sprintf("%s restarted.", spec.Name), out), nil
}

func (s *GlobalServiceService) OpenGlobalService(serviceID string) (string, error) {
	spec, err := resolveGlobalServiceSpec(serviceID)
	if err != nil {
		return "", err
	}
	if spec.URLHost == "" {
		return "", fmt.Errorf("%s has no web interface", spec.Name)
	}

	url := buildProxyURL(spec.URLHost)
	if err := openURLWithPreferences(s.ctx, url); err != nil {
		return "Open manually: " + url, nil
	}
	return "Opening " + url + "...", nil
}

func resolveGlobalServiceSpec(serviceID string) (globalServiceSpec, error) {
	normalized := strings.ToLower(strings.TrimSpace(serviceID))
	for _, spec := range globalServiceSpecs {
		if spec.ID == normalized {
			return spec, nil
		}
	}
	return globalServiceSpec{}, fmt.Errorf("unknown global service: %s", serviceID)
}

func deriveGlobalContainerStatus(state string, statusText string) (string, string, bool) {
	normalizedState := strings.ToLower(strings.TrimSpace(state))
	status := "stopped"
	running := false

	switch normalizedState {
	case "running":
		status = "running"
		running = true
	case "restarting":
		status = "restarting"
	case "paused":
		status = "paused"
	case "created":
		status = "created"
	case "dead":
		status = "dead"
	case "exited":
		status = "stopped"
	default:
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(statusText)), "up ") {
			status = "running"
			running = true
		}
	}

	return status, deriveGlobalContainerHealth(statusText), running
}

func deriveGlobalContainerHealth(statusText string) string {
	normalized := strings.ToLower(strings.TrimSpace(statusText))
	switch {
	case strings.Contains(normalized, "(healthy)"):
		return "healthy"
	case strings.Contains(normalized, "(unhealthy)"):
		return "unhealthy"
	case strings.Contains(normalized, "health: starting"):
		return "starting"
	default:
		return "unknown"
	}
}

func isGlobalServiceStopLike(service GlobalService) bool {
	status := strings.ToLower(strings.TrimSpace(service.Status))
	state := strings.ToLower(strings.TrimSpace(service.State))

	switch status {
	case "stopped", "exited", "created", "dead":
		return true
	}

	return strings.Contains(state, "stopped") ||
		strings.Contains(state, "exited") ||
		strings.Contains(state, "created") ||
		strings.Contains(state, "dead")
}

func hasRoutingConflictSignal(services []GlobalService) bool {
	for _, service := range services {
		if service.ID != "caddy" && service.ID != "dnsmasq" {
			continue
		}
		if service.Running {
			continue
		}
		if isGlobalServiceStopLike(service) {
			return true
		}
	}
	return false
}

func detectRoutingPortConflictWarnings(containers []container.Summary) []string {
	warnings := []string{}
	warnings = append(warnings, detectDockerPortConflictWarnings(containers)...)
	warnings = append(warnings, detectHostPortConflictWarnings()...)
	warnings = uniqueStrings(warnings)
	sort.Strings(warnings)

	if len(warnings) > 0 {
		return warnings
	}

	return []string{
		"Routing services are degraded. Check listeners on ports 53/80/443, then retry Restart All.",
	}
}

func detectRoutingPublishedPortBindingWarnings(
	services []GlobalService,
	containersByName map[string]container.Summary,
) []string {
	warnings := []string{}

	for _, service := range services {
		expectedPorts, tracked := routingServiceExpectedPublishedPorts[service.ID]
		if !tracked || !service.Running {
			continue
		}

		containerSummary, ok := containersByName[service.ContainerName]
		if !ok {
			for _, expectedPort := range expectedPorts {
				warnings = append(
					warnings,
					fmt.Sprintf(
						"%s%s: %s is running but container %s was not found for port verification",
						routingConflictWarningPrefix,
						expectedPort,
						service.Name,
						service.ContainerName,
					),
				)
			}
			continue
		}

		publishedPorts := buildPublishedPortKeySet(containerSummary.Ports)
		for _, expectedPort := range expectedPorts {
			if publishedPorts[expectedPort] {
				continue
			}
			warnings = append(
				warnings,
				fmt.Sprintf(
					"%s%s: %s is running but %s is not published on host",
					routingConflictWarningPrefix,
					expectedPort,
					service.Name,
					service.ContainerName,
				),
			)
		}
	}

	return uniqueStrings(warnings)
}

func detectDockerPortConflictWarnings(containers []container.Summary) []string {
	if len(containers) == 0 {
		return nil
	}

	targets := routingPortTargetSet()
	globalContainers := map[string]bool{}
	for _, spec := range globalServiceSpecs {
		globalContainers[spec.ContainerName] = true
	}

	warnings := []string{}
	for _, c := range containers {
		if strings.ToLower(strings.TrimSpace(c.State)) != "running" {
			continue
		}

		projectName := strings.TrimSpace(c.Labels["com.docker.compose.project"])
		if strings.EqualFold(projectName, globalServicesComposeProjectName) {
			continue
		}

		containerName := firstContainerName(c)
		if containerName == "" {
			containerName = strings.TrimSpace(c.ID)
			if len(containerName) > 12 {
				containerName = containerName[:12]
			}
		}
		if globalContainers[containerName] {
			continue
		}

		for _, published := range c.Ports {
			if published.PublicPort <= 0 {
				continue
			}

			protocol := strings.ToLower(strings.TrimSpace(published.Type))
			if protocol == "" {
				protocol = "tcp"
			}
			targetKey := fmt.Sprintf("%d/%s", published.PublicPort, protocol)
			if !targets[targetKey] {
				continue
			}

			details := []string{}
			if projectName != "" {
				details = append(details, "project: "+projectName)
			}
			if composeService := strings.TrimSpace(c.Labels["com.docker.compose.service"]); composeService != "" {
				details = append(details, "service: "+composeService)
			}

			suffix := ""
			if len(details) > 0 {
				suffix = " (" + strings.Join(details, ", ") + ")"
			}

			warnings = append(
				warnings,
				fmt.Sprintf(
					"%s%s: docker container %s%s",
					routingConflictWarningPrefix,
					targetKey,
					containerName,
					suffix,
				),
			)
		}
	}

	return uniqueStrings(warnings)
}

func detectHostPortConflictWarnings() []string {
	allOwners := []hostPortOwner{}
	for _, target := range routingPortProbeTargets {
		allOwners = append(allOwners, detectHostPortOwners(target.Port, target.Protocol)...)
	}

	return formatHostPortConflictWarnings(allOwners)
}

func detectHostPortOwners(port int, protocol string) []hostPortOwner {
	normalizedProtocol := strings.ToLower(strings.TrimSpace(protocol))
	if normalizedProtocol == "" {
		normalizedProtocol = "tcp"
	}

	lsofArgs := []string{
		"-nP",
		fmt.Sprintf("-i%s:%d", strings.ToUpper(normalizedProtocol), port),
	}
	if normalizedProtocol == "tcp" {
		lsofArgs = append(lsofArgs, "-sTCP:LISTEN")
	}
	lsofOutput, lsofErr := runHostPortProbeForDesktop("lsof", lsofArgs...)
	owners := parseLsofPortOwners(lsofOutput, port, normalizedProtocol)
	if len(owners) > 0 {
		return owners
	}
	if lsofErr == nil {
		return nil
	}

	ssArgs := []string{fmt.Sprintf("sport = :%d", port)}
	if normalizedProtocol == "tcp" {
		ssArgs = append([]string{"-ltnp"}, ssArgs...)
	} else {
		ssArgs = append([]string{"-lunp"}, ssArgs...)
	}

	ssOutput, ssErr := runHostPortProbeForDesktop("ss", ssArgs...)
	owners = parseSSPortOwners(ssOutput, port, normalizedProtocol)
	if len(owners) > 0 {
		return owners
	}
	if ssErr != nil {
		return nil
	}
	return nil
}

func parseLsofPortOwners(output string, port int, protocol string) []hostPortOwner {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}

	owners := []hostPortOwner{}
	seen := map[string]bool{}

	for _, rawLine := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "COMMAND ") || strings.HasPrefix(line, "COMMAND\t") {
			continue
		}
		if strings.EqualFold(protocol, "tcp") && !strings.Contains(strings.ToUpper(line), "LISTEN") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		command := strings.TrimSpace(fields[0])
		pid := strings.TrimSpace(fields[1])
		user := ""
		if len(fields) >= 3 {
			user = strings.TrimSpace(fields[2])
		}

		key := strings.Join([]string{command, pid, user, strconv.Itoa(port), protocol}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true

		owners = append(owners, hostPortOwner{
			Port:     port,
			Protocol: protocol,
			Command:  command,
			PID:      pid,
			User:     user,
		})
	}

	return owners
}

func parseSSPortOwners(output string, port int, protocol string) []hostPortOwner {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}

	owners := []hostPortOwner{}
	seen := map[string]bool{}
	portToken := ":" + strconv.Itoa(port)

	for _, rawLine := range strings.Split(trimmed, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "netid") {
			continue
		}
		if !strings.Contains(line, portToken) {
			continue
		}

		matches := ssProcessPattern.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			key := strings.Join([]string{"unknown", "", strconv.Itoa(port), protocol}, "|")
			if seen[key] {
				continue
			}
			seen[key] = true
			owners = append(owners, hostPortOwner{
				Port:     port,
				Protocol: protocol,
				Command:  "unknown",
				PID:      "",
			})
			continue
		}

		for _, match := range matches {
			if len(match) < 3 {
				continue
			}

			command := strings.TrimSpace(match[1])
			pid := strings.TrimSpace(match[2])
			key := strings.Join([]string{command, pid, strconv.Itoa(port), protocol}, "|")
			if seen[key] {
				continue
			}
			seen[key] = true

			owners = append(owners, hostPortOwner{
				Port:     port,
				Protocol: protocol,
				Command:  command,
				PID:      pid,
			})
		}
	}

	return owners
}

func formatHostPortConflictWarnings(owners []hostPortOwner) []string {
	warnings := []string{}

	for _, owner := range owners {
		command := strings.TrimSpace(owner.Command)
		if command == "" {
			command = "unknown"
		}

		normalizedProtocol := strings.ToLower(strings.TrimSpace(owner.Protocol))
		if normalizedProtocol == "" {
			normalizedProtocol = "tcp"
		}

		details := []string{}
		if pid := strings.TrimSpace(owner.PID); pid != "" {
			details = append(details, "pid: "+pid)
		}
		if user := strings.TrimSpace(owner.User); user != "" {
			details = append(details, "user: "+user)
		}

		suffix := ""
		if len(details) > 0 {
			suffix = " (" + strings.Join(details, ", ") + ")"
		}

		warnings = append(
			warnings,
			fmt.Sprintf(
				"%s%d/%s: host process %s%s",
				routingConflictWarningPrefix,
				owner.Port,
				normalizedProtocol,
				command,
				suffix,
			),
		)
	}

	return uniqueStrings(warnings)
}

func firstContainerName(c container.Summary) string {
	for _, rawName := range c.Names {
		name := strings.TrimSpace(strings.TrimPrefix(rawName, "/"))
		if name != "" {
			return name
		}
	}
	return ""
}

func routingPortTargetSet() map[string]bool {
	targets := map[string]bool{}
	for _, target := range routingPortProbeTargets {
		key := fmt.Sprintf(
			"%d/%s",
			target.Port,
			strings.ToLower(strings.TrimSpace(target.Protocol)),
		)
		targets[key] = true
	}
	return targets
}

func buildPublishedPortKeySet(ports []container.Port) map[string]bool {
	published := map[string]bool{}
	for _, port := range ports {
		if port.PublicPort <= 0 {
			continue
		}

		protocol := strings.ToLower(strings.TrimSpace(port.Type))
		if protocol == "" {
			protocol = "tcp"
		}

		key := fmt.Sprintf("%d/%s", port.PublicPort, protocol)
		published[key] = true
	}
	return published
}

func resolveDesktopGovardCommandDir() (string, error) {
	// For global services, we can run from home or any repo path.
	// Usually ~/.govard/proxy, but the CLI handles it via internal engine.
	// We just need a working directory where the CLI can function.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home, nil
}

func withCommandOutput(base string, commandOutput string) string {
	trimmed := strings.TrimSpace(commandOutput)
	if trimmed == "" {
		return base
	}
	return base + "\n" + trimmed
}

func globalServicesComposeDirPath() string {
	return filepath.Join(os.Getenv("HOME"), ".govard", "proxy")
}

func globalServicesComposeFilePath() string {
	return filepath.Join(globalServicesComposeDirPath(), "docker-compose.yml")
}

func ensureGlobalComposeFileExists() error {
	composeFile := globalServicesComposeFilePath()
	if _, err := os.Stat(composeFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", errDesktopGlobalServicesNotInitialized, composeFile)
		}
		return fmt.Errorf("stat global compose file: %w", err)
	}
	return nil
}
