package engine

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
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

	report := DoctorReport{
		Checks: make([]DoctorCheck, 0, 7),
	}

	if err := dependencies.CheckDockerStatus(); err != nil {
		report.Checks = append(report.Checks, DoctorCheck{
			ID:               "docker.daemon",
			Title:            "Docker daemon",
			Status:           DoctorStatusFail,
			Message:          fmt.Sprintf("Docker is not running or not accessible: %v", err),
			Hint:             "Start Docker Desktop/daemon and verify current user can access Docker socket.",
			SuggestedCommand: "govard doctor fix-deps",
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
			SuggestedCommand: "govard doctor fix-deps",
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
	report.IssueCards = BuildDoctorIssueCards(report.Checks)

	return report
}

func BuildDoctorIssueCards(checks []DoctorCheck) []DoctorIssueCard {
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

func CheckSearchIndexBlock() error {
	config := loadConfig()
	if config.Stack.Services.Search == "" || config.Stack.Services.Search == "none" {
		return nil
	}

	containerName := fmt.Sprintf("%s-elasticsearch-1", config.ProjectName)
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
	config, _, err := LoadConfigFromDir(wd, false)
	if err != nil {
		return Config{}
	}
	return config
}
