package engine

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"govard/internal/conventions"
)

type DoctorCheckStatus string

const (
	DoctorStatusPass DoctorCheckStatus = "pass"
	DoctorStatusWarn DoctorCheckStatus = "warn"
	DoctorStatusFail DoctorCheckStatus = "fail"
)

type DoctorCheck struct {
	ID               string            `json:"id" yaml:"id"`
	Title            string            `json:"title" yaml:"title"`
	Status           DoctorCheckStatus `json:"status" yaml:"status"`
	Message          string            `json:"message" yaml:"message"`
	Hint             string            `json:"hint,omitempty" yaml:"hint,omitempty"`
	SuggestedCommand string            `json:"suggested_command,omitempty" yaml:"suggested_command,omitempty"`
}

type DoctorReport struct {
	Checks     []DoctorCheck     `json:"checks" yaml:"checks"`
	IssueCards []DoctorIssueCard `json:"issue_cards" yaml:"issue_cards"`
	Passed     int               `json:"passed" yaml:"passed"`
	Warnings   int               `json:"warnings" yaml:"warnings"`
	Failures   int               `json:"failures" yaml:"failures"`
}

type DoctorIssueCard struct {
	ID               string `json:"id" yaml:"id"`
	Severity         string `json:"severity" yaml:"severity"`
	Status           string `json:"status" yaml:"status"`
	Title            string `json:"title" yaml:"title"`
	Message          string `json:"message" yaml:"message"`
	Hint             string `json:"hint,omitempty" yaml:"hint,omitempty"`
	SuggestedCommand string `json:"suggested_command,omitempty" yaml:"suggested_command,omitempty"`
}

func (report DoctorReport) HasFailures() bool {
	return report.Failures > 0
}

type DoctorDependencies struct {
	CheckDockerStatus        func() error
	CheckDockerComposePlugin func() error
	CheckPortAvailable       func(port string) bool
	CheckDiskScratch         func() error
	CheckGovardHomeWritable  func() error
	CheckNetworkConnectivity func() error
	CheckSearchIndexBlock    func() error
	CheckSSHAgentStatus      func() (string, error)
	CheckComposeSpam         func() error
	CheckGovardRegistry      func() error
	CheckProfileSync         func() error
	CheckSystemDependencies  func() []string
	CheckRuntimeImages       func() ([]string, error)
	CheckLegacyConfig        func() error
	CheckConfigDrift         func() error
}

func RunDoctorDiagnostics(dependencies DoctorDependencies) DoctorReport {
	if dependencies.CheckDockerStatus == nil {
		dependencies.CheckDockerStatus = func() error { return CheckDockerStatus(context.Background()) }
	}
	if dependencies.CheckDockerComposePlugin == nil {
		dependencies.CheckDockerComposePlugin = func() error { return CheckDockerComposePlugin(context.Background()) }
	}
	if dependencies.CheckPortAvailable == nil {
		dependencies.CheckPortAvailable = func(port string) bool { return CheckPortForGovardProxy(context.Background(), port) }
	}
	if dependencies.CheckDiskScratch == nil {
		dependencies.CheckDiskScratch = CheckDiskScratchWrite
	}
	if dependencies.CheckGovardHomeWritable == nil {
		dependencies.CheckGovardHomeWritable = CheckGovardHomeWritable
	}
	if dependencies.CheckNetworkConnectivity == nil {
		dependencies.CheckNetworkConnectivity = CheckNetworkConnectivity
	}
	if dependencies.CheckSearchIndexBlock == nil {
		dependencies.CheckSearchIndexBlock = CheckSearchIndexBlock
	}
	if dependencies.CheckSSHAgentStatus == nil {
		dependencies.CheckSSHAgentStatus = CheckSSHAgentStatus
	}
	if dependencies.CheckComposeSpam == nil {
		dependencies.CheckComposeSpam = func() error { return CheckComposeSpam(1000) }
	}
	if dependencies.CheckGovardRegistry == nil {
		dependencies.CheckGovardRegistry = checkGovardRegistry
	}
	if dependencies.CheckProfileSync == nil {
		dependencies.CheckProfileSync = checkProfileSync
	}
	if dependencies.CheckSystemDependencies == nil {
		dependencies.CheckSystemDependencies = checkSystemDependencies
	}
	if dependencies.CheckRuntimeImages == nil {
		dependencies.CheckRuntimeImages = checkRuntimeImages
	}
	if dependencies.CheckLegacyConfig == nil {
		dependencies.CheckLegacyConfig = checkLegacyConfig
	}
	if dependencies.CheckConfigDrift == nil {
		dependencies.CheckConfigDrift = checkConfigDrift
	}

	report := DoctorReport{
		Checks: make([]DoctorCheck, 0, 10),
	}

	if err := dependencies.CheckDockerStatus(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "docker.daemon",
			Title:            "Docker daemon",
			Status:           DoctorStatusFail,
			Message:          fmt.Sprintf("Docker is not running or not accessible: %v", err),
			Hint:             "Start Docker Desktop/daemon and verify current user can access Docker socket.",
			SuggestedCommand: "systemctl start docker",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "docker.daemon",
			Title:   "Docker daemon",
			Status:  DoctorStatusPass,
			Message: "Docker is running.",
		})
	}

	if err := dependencies.CheckDockerComposePlugin(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "docker.compose",
			Title:            "Docker Compose plugin",
			Status:           DoctorStatusFail,
			Message:          err.Error(),
			Hint:             "Install or enable the Docker Compose v2 plugin.",
			SuggestedCommand: "docker compose version",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "docker.compose",
			Title:   "Docker Compose plugin",
			Status:  DoctorStatusPass,
			Message: "Docker Compose plugin is available.",
		})
	}

	if dependencies.CheckPortAvailable("80") {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.port.80",
			Title:   "Host port 80",
			Status:  DoctorStatusPass,
			Message: "Port 80 is available.",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "host.port.80",
			Title:            "Host port 80",
			Status:           DoctorStatusWarn,
			Message:          "Port 80 is in use.",
			Hint:             "Stop or reconfigure the process currently binding port 80.",
			SuggestedCommand: "govard proxy status",
		})
	}

	if dependencies.CheckPortAvailable("443") {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.port.443",
			Title:   "Host port 443",
			Status:  DoctorStatusPass,
			Message: "Port 443 is available.",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "host.port.443",
			Title:            "Host port 443",
			Status:           DoctorStatusWarn,
			Message:          "Port 443 is in use.",
			Hint:             "Stop or reconfigure the process currently binding port 443.",
			SuggestedCommand: "govard proxy status",
		})
	}

	if err := dependencies.CheckDiskScratch(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "host.disk.scratch",
			Title:            "Disk scratch write",
			Status:           DoctorStatusFail,
			Message:          fmt.Sprintf("Failed to write temp file: %v", err),
			Hint:             "Check disk space and write permissions on temporary directory.",
			SuggestedCommand: "govard doctor",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.disk.scratch",
			Title:   "Disk scratch write",
			Status:  DoctorStatusPass,
			Message: "Temporary directory is writable.",
		})
	}

	if err := dependencies.CheckGovardHomeWritable(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "host.govard.home",
			Title:            "Govard home directory",
			Status:           DoctorStatusWarn,
			Message:          fmt.Sprintf("Govard home directory is not ready: %v", err),
			Hint:             "Run doctor --fix to create or repair safe Govard runtime directories.",
			SuggestedCommand: "govard doctor --fix",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.govard.home",
			Title:   "Govard home directory",
			Status:  DoctorStatusPass,
			Message: "Govard home directory is writable.",
		})
	}

	if err := dependencies.CheckNetworkConnectivity(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "host.network.outbound",
			Title:            "Network outbound probe",
			Status:           DoctorStatusWarn,
			Message:          fmt.Sprintf("Could not complete outbound probe: %v", err),
			Hint:             "Check VPN/firewall settings and DNS/network routes.",
			SuggestedCommand: "govard remote test <name>",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.network.outbound",
			Title:   "Network outbound probe",
			Status:  DoctorStatusPass,
			Message: "Outbound network probe succeeded.",
		})
	}

	if err := dependencies.CheckSearchIndexBlock(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "host.search.index_block",
			Title:            "Search index block",
			Status:           DoctorStatusFail,
			Message:          fmt.Sprintf("Search index is blocked (read-only): %v", err),
			Hint:             "Run doctor --fix to unblock the search index.",
			SuggestedCommand: "govard doctor --fix",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.search.index_block",
			Title:   "Search index block",
			Status:  DoctorStatusPass,
			Message: "Search index is not blocked.",
		})
	}

	if msg, err := dependencies.CheckSSHAgentStatus(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "host.ssh.agent",
			Title:            "SSH Agent forwarding",
			Status:           DoctorStatusWarn,
			Message:          msg,
			Hint:             "Ensure your SSH agent is running (ssh-add -l) and SSH_AUTH_SOCK is exported.",
			SuggestedCommand: "ssh-add",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.ssh.agent",
			Title:   "SSH Agent forwarding",
			Status:  DoctorStatusPass,
			Message: msg,
		})
	}

	if err := dependencies.CheckComposeSpam(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "host.compose.spam",
			Title:            "Compose directory saturation",
			Status:           DoctorStatusWarn,
			Message:          fmt.Sprintf("Too many compose files found in ~/.govard/compose: %v", err),
			Hint:             "Run doctor --fix to purge stale compose files.",
			SuggestedCommand: "govard doctor --fix",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.compose.spam",
			Title:   "Compose directory saturation",
			Status:  DoctorStatusPass,
			Message: "Compose directory count is healthy.",
		})
	}

	if err := dependencies.CheckGovardRegistry(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "host.govard.registry",
			Title:            "Project registry file",
			Status:           DoctorStatusFail,
			Message:          err.Error(),
			Hint:             "Run doctor --fix to repair the corrupted registry directory.",
			SuggestedCommand: "govard doctor --fix",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.govard.registry",
			Title:   "Project registry file",
			Status:  DoctorStatusPass,
			Message: "Project registry is healthy.",
		})
	}

	if err := dependencies.CheckLegacyConfig(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "project.config.audit",
			Title:            "Configuration audit",
			Status:           DoctorStatusWarn,
			Message:          err.Error(),
			Hint:             "Run doctor --fix to automatically migrate your .govard.yml to the new standard.",
			SuggestedCommand: "govard doctor --fix",
		})
	} else {
		// Only report pass if base config exists and is healthy
		if _, err := os.Stat(BaseConfigFile); err == nil {
			report.Checks = append(report.Checks, DoctorCheck{
				ID:      "project.config.audit",
				Title:   "Configuration audit",
				Status:  DoctorStatusPass,
				Message: "Configuration file is using current naming standards.",
			})
		}
	}

	if err := dependencies.CheckConfigDrift(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "project.config.drift",
			Title:            "Configuration drift",
			Status:           DoctorStatusWarn,
			Message:          err.Error(),
			Hint:             "Run doctor --fix to sync .govard.yml with detected framework/profile versions. Use --dry-run to preview or --commit to auto-commit.",
			SuggestedCommand: "govard doctor --fix",
		})
	} else {
		if _, err := os.Stat(BaseConfigFile); err == nil {
			report.Checks = append(report.Checks, DoctorCheck{
				ID:      "project.config.drift",
				Title:   "Configuration drift",
				Status:  DoctorStatusPass,
				Message: "Configuration is in sync with framework profile.",
			})
		}
	}

	if err := dependencies.CheckProfileSync(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "project.profile.sync",
			Title:            "Framework version profile",
			Status:           DoctorStatusWarn,
			Message:          err.Error(),
			Hint:             "Run doctor --fix to automatically auto-tune your environment to the recommended profile.",
			SuggestedCommand: "govard doctor --fix",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "project.profile.sync",
			Title:   "Framework version profile",
			Status:  DoctorStatusPass,
			Message: "Runtime environment is synced with the recommended framework profile.",
		})
	}

	if missing := dependencies.CheckSystemDependencies(); len(missing) > 0 {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.system.deps",
			Title:   "Host system dependencies",
			Status:  DoctorStatusFail,
			Message: fmt.Sprintf("Missing required system dependencies: %s", strings.Join(missing, ", ")),
			Hint:    "Please install the missing tools using your system package manager.",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "host.system.deps",
			Title:   "Host system dependencies",
			Status:  DoctorStatusPass,
			Message: "All required system dependencies are installed.",
		})
	}

	if missingImages, err := dependencies.CheckRuntimeImages(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "project.runtime.images",
			Title:   "Docker runtime images",
			Status:  DoctorStatusWarn,
			Message: fmt.Sprintf("Failed to check runtime images: %v", err),
		})
	} else if len(missingImages) > 0 {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "project.runtime.images",
			Title:            "Docker runtime images",
			Status:           DoctorStatusWarn,
			Message:          fmt.Sprintf("Missing %d required Docker image(s)", len(missingImages)),
			Hint:             fmt.Sprintf("Missing images: %s. Run doctor --fix to pull them automatically.", strings.Join(missingImages, ", ")),
			SuggestedCommand: "govard doctor --fix",
		})
	} else {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:      "project.runtime.images",
			Title:   "Docker runtime images",
			Status:  DoctorStatusPass,
			Message: "All required Docker runtime images are cached locally.",
		})
	}

	for _, check := range report.Checks {
		switch check.Status {
		case DoctorStatusPass:
			report.Passed++
		case DoctorStatusWarn:
			report.Warnings++
		case DoctorStatusFail:
			report.Failures++
		}
	}
	report.IssueCards = buildDoctorIssueCards(report.Checks)

	return report
}

func buildDoctorIssueCards(checks []DoctorCheck) []DoctorIssueCard {
	cards := make([]DoctorIssueCard, 0, len(checks))
	for _, check := range checks {
		if check.Status == DoctorStatusPass {
			continue
		}
		severity := "warning"
		if check.Status == DoctorStatusFail {
			severity = "error"
		}
		cards = append(cards, DoctorIssueCard{
			ID:               check.ID,
			Severity:         severity,
			Status:           string(check.Status),
			Title:            check.Title,
			Message:          check.Message,
			Hint:             check.Hint,
			SuggestedCommand: check.SuggestedCommand,
		})
	}
	return cards
}

func CheckDiskScratchWrite() error {
	file, err := os.CreateTemp("", "govard-doctor-*")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)

	if _, err := file.Write([]byte("ok")); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func CheckGovardHomeWritable() error {
	homeDir := GovardHomeDir()
	info, err := os.Stat(homeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("missing directory: %s", homeDir)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", homeDir)
	}

	file, err := os.CreateTemp(homeDir, "doctor-write-*")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)

	if _, err := file.Write([]byte("ok")); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func checkGovardRegistry() error {
	path := ProjectRegistryPath()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("registry file %s is a directory", path)
	}
	return nil
}

func CheckNetworkConnectivity() error {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", "1.1.1.1:53")
	if err != nil {
		return err
	}
	return conn.Close()
}

func CheckSSHAgentStatus() (string, error) {
	// 1. Check host environment
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return "SSH_AUTH_SOCK is not set on the host machine.", fmt.Errorf("missing SSH_AUTH_SOCK")
	}

	if _, err := os.Stat(sock); err != nil {
		return fmt.Sprintf("SSH_AUTH_SOCK is set but socket file is missing or inaccessible: %v", err), err
	}

	// 2. Check responsiveness on host
	hostCheck := exec.Command("ssh-add", "-l")
	if out, err := hostCheck.CombinedOutput(); err != nil {
		// Multi-language support: Check exit code instead of string.
		// Exit code 1 means agent is running but has no identities.
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			// Continue to container check
		} else {
			return fmt.Sprintf("SSH agent is not responding on host: %s", strings.TrimSpace(string(out))), err
		}
	}

	// 3. Optional: Check inside the PHP container if running
	config := loadConfig()
	if config.ProjectName != "" {
		containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.PHPSuffix)
		// Only check if container is running
		inspect := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName)
		if output, err := inspect.Output(); err == nil && strings.TrimSpace(string(output)) == "true" {
			containerCheck := exec.Command("docker", "exec", "-i", containerName, "ssh-add", "-l")
			if out, err := containerCheck.CombinedOutput(); err != nil {
				if exitError, ok := err.(*exec.ExitError); !ok || exitError.ExitCode() != 1 {
					return fmt.Sprintf("SSH agent is working on host but NOT inside container %s: %s", containerName, strings.TrimSpace(string(out))), err
				}
			}
			return "SSH agent is healthy on host and forwarded to container.", nil
		}
	}

	return "SSH agent is healthy on host.", nil
}

func CheckSearchIndexBlock() error {
	config := loadConfig()
	if config.Stack.Services.Search == "" || config.Stack.Services.Search == "none" {
		return nil
	}

	containerName := fmt.Sprintf("%s%s", config.ProjectName, conventions.ElasticsearchSuffix)
	// Check if container is running first
	inspect := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", containerName)
	if output, err := inspect.Output(); err != nil || strings.TrimSpace(string(output)) != "true" {
		return nil // skip if not running
	}

	// Query settings and look for read_only_allow_delete
	cmdArgs := []string{
		"exec", "-i", containerName,
		"curl", "-s", "-X", "GET", "http://localhost:9200/_all/_settings",
	}

	output, err := exec.Command("docker", cmdArgs...).CombinedOutput()
	if err != nil {
		return nil // skip if we can't query
	}

	if strings.Contains(string(output), `"read_only_allow_delete":"true"`) {
		return fmt.Errorf("index is in read-only mode")
	}

	return nil
}

func loadConfig() Config {
	wd, _ := os.Getwd()
	config, err := LoadRawConfigFromDir(wd, false)
	if err != nil {
		return Config{}
	}
	return config
}

func checkProfileSync() error {
	config := loadConfig()
	if config.Framework == "" || config.Framework == "generic" || config.Framework == "custom" {
		return nil
	}

	wd, _ := os.Getwd()
	metadata := DetectFramework(wd)

	warnings := CollectProfileSyncWarnings(config, metadata)
	if len(warnings) > 0 {
		version := strings.TrimSpace(metadata.Version)
		if version == "" {
			version = strings.TrimSpace(config.FrameworkVersion)
		}
		return fmt.Errorf("%s %s environment is out of sync: %s", config.Framework, version, strings.Join(warnings, ", "))
	}

	return nil
}

// CollectProfileSyncWarnings compares the raw user configuration against framework profile recommendations.
// It silences warnings for fields that were explicitly set by the user (non-empty in rawConfig).
func CollectProfileSyncWarnings(rawConfig Config, metadata ProjectMetadata) []string {
	if rawConfig.Framework == "" || rawConfig.Framework == "generic" || rawConfig.Framework == "custom" {
		return nil
	}

	version := strings.TrimSpace(metadata.Version)
	if version == "" {
		version = strings.TrimSpace(rawConfig.FrameworkVersion)
	}

	profileResult, err := ResolveRuntimeProfile(rawConfig.Framework, version)
	if err != nil {
		return nil // skip check if profile resolution fails
	}

	mismatches := []string{}
	p := profileResult.Profile

	if p.PHPVersion != "" {
		normalized := rawConfig
		NormalizeConfig(&normalized, "")
		if normalized.Stack.PHPVersion != p.PHPVersion {
			if rawConfig.Stack.PHPVersion != "" {
				mismatches = append(mismatches, fmt.Sprintf("PHP %s [Explicit] (advisory: recommended %s)", rawConfig.Stack.PHPVersion, p.PHPVersion))
			} else {
				mismatches = append(mismatches, fmt.Sprintf("PHP %s (expected %s)", normalized.Stack.PHPVersion, p.PHPVersion))
			}
		}
	}

	if p.DB != "" {
		if rawConfig.Stack.Services.DB != "" && rawConfig.Stack.Services.DB != p.DB {
			mismatches = append(mismatches, fmt.Sprintf("DB %s [Explicit] (expected %s)", rawConfig.Stack.Services.DB, p.DB))
		} else if p.DBVersion != "" {
			normalized := rawConfig
			NormalizeConfig(&normalized, "")
			if normalized.Stack.DBVersion != p.DBVersion {
				if rawConfig.Stack.DBVersion != "" {
					mismatches = append(mismatches, fmt.Sprintf("DB version %s [Explicit] (recommended %s)", rawConfig.Stack.DBVersion, p.DBVersion))
				} else {
					mismatches = append(mismatches, fmt.Sprintf("DB version %s (recommended %s)", normalized.Stack.DBVersion, p.DBVersion))
				}
			}
		}
	}

	if p.Search != "" {
		normalized := rawConfig
		NormalizeConfig(&normalized, "")
		if normalized.Stack.Services.Search != p.Search {
			if rawConfig.Stack.Services.Search != "" {
				mismatches = append(mismatches, fmt.Sprintf("Search %s [Explicit] (expected %s)", rawConfig.Stack.Services.Search, p.Search))
			} else {
				mismatches = append(mismatches, fmt.Sprintf("Search %s (expected %s)", normalized.Stack.Services.Search, p.Search))
			}
		}
	}

	if p.Cache != "" {
		normalized := rawConfig
		NormalizeConfig(&normalized, "")
		if normalized.Stack.Services.Cache != p.Cache {
			if rawConfig.Stack.Services.Cache != "" {
				mismatches = append(mismatches, fmt.Sprintf("Cache %s [Explicit] (expected %s)", rawConfig.Stack.Services.Cache, p.Cache))
			} else {
				mismatches = append(mismatches, fmt.Sprintf("Cache %s (expected %s)", normalized.Stack.Services.Cache, p.Cache))
			}
		}
	}

	return mismatches
}

func checkSystemDependencies() []string {
	missing := make([]string, 0, 4)

	if _, err := exec.LookPath("docker"); err != nil {
		missing = append(missing, "docker")
	} else if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		missing = append(missing, "docker compose plugin")
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		missing = append(missing, "ssh")
	}

	if _, err := exec.LookPath("rsync"); err != nil {
		missing = append(missing, "rsync")
	}

	return missing
}

func checkRuntimeImages() ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not read working directory: %w", err)
	}

	config, _, err := LoadConfigFromDir(cwd, true)
	if err != nil {
		if strings.Contains(err.Error(), BaseConfigFile+" not found") {
			return nil, nil // no project, no required images
		}
		return nil, err
	}

	required := RequiredRuntimeImages(config, cwd)
	if len(required) == 0 {
		return nil, nil
	}

	missing := make([]string, 0, len(required))
	for _, image := range required {
		if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
			missing = append(missing, image)
		}
	}
	return missing, nil
}

func checkLegacyConfig() error {
	data, err := os.ReadFile(BaseConfigFile)
	if err != nil {
		return nil // skip if no config
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil // invalid yaml will be caught by other checks
	}

	foundLegacy := []string{}
	legacyKeys := map[string]bool{

		"redis":         true,
		"elasticsearch": true,
		"rabbitmq":      true,
	}

	// Recursive node search to find legacy keys at any level
	var findLegacy func(*yaml.Node)
	findLegacy = func(node *yaml.Node) {
		switch node.Kind {
		case yaml.MappingNode:
			for i := 0; i < len(node.Content); i += 2 {
				keyNode := node.Content[i]
				valNode := node.Content[i+1]
				if keyNode.Kind == yaml.ScalarNode && legacyKeys[keyNode.Value] {
					foundLegacy = append(foundLegacy, keyNode.Value)
				}
				findLegacy(valNode)
			}
		case yaml.DocumentNode, yaml.SequenceNode:
			for _, child := range node.Content {
				findLegacy(child)
			}
		}
	}

	findLegacy(&root)

	if len(foundLegacy) > 0 {
		return fmt.Errorf("found %d legacy configuration key(s): %s", len(foundLegacy), strings.Join(foundLegacy, ", "))
	}

	return nil
}

func checkConfigDrift() error {
	wd, _ := os.Getwd()
	data, err := os.ReadFile(BaseConfigFile)
	if err != nil {
		return nil
	}
	// Quick check: if no config content, skip
	if len(data) == 0 {
		return nil
	}
	cfg, err := LoadRawConfigFromDir(wd, false)
	if err != nil {
		return nil
	}
	if cfg.Framework == "" || cfg.Framework == "generic" || cfg.Framework == "custom" {
		return nil
	}
	meta := DetectFramework(wd)
	warnings := CollectConfigDrift(cfg, meta)
	if msg := ComposerStaleWarning(wd); msg != "" {
		warnings = append(warnings, msg)
	}
	if len(warnings) > 0 {
		return fmt.Errorf("configuration drift: %s", strings.Join(warnings, ", "))
	}
	return nil
}
