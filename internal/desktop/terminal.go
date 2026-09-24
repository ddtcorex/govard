package desktop

import (
	"fmt"
	"govard/internal/conventions"
	"os/exec"
	"strings"
)

// LogService methods

func (s *LogService) GetLogs(project string, lines int) (res string, err error) {
	defer RecoverPanic(&err, "GetLogs")
	output, err := getLogs(project, lines)
	if err != nil {
		return "", err
	}
	return output, nil
}

func (s *LogService) GetLogsForService(project string, service string, lines int) (res string, err error) {
	defer RecoverPanic(&err, "GetLogsForService")
	output, err := getLogsForService(project, service, lines)
	if err != nil {
		return "", err
	}
	return output, nil
}

func (s *LogService) StartServiceTerminalInOS(project, service, user, shell string) (res string, err error) {
	defer RecoverPanic(&err, "StartServiceTerminalInOS")
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}

	targetService := resolveShellServiceName(info, service)
	containerName := resolveShellContainer(info, targetService)
	chosenShell := normalizeShell(shell)
	chosenUser := normalizeShellUser(info, targetService, user)

	args := []string{"exec", "-it", "-w", conventions.DefaultWorkDir, "-e", "TERM=screen-256color"}
	if chosenUser != "" {
		args = append(args, "-u", chosenUser)
	}
	args = append(args, containerName, chosenShell)

	// Combine into a single command string for LaunchInTerminal
	dockerBinary, err := exec.LookPath("docker")
	if err != nil {
		return "", fmt.Errorf("docker binary not found: %w", err)
	}

	fullCmd := dockerBinary + " " + strings.Join(args, " ")
	err = LaunchInTerminal(info.workingDir, fullCmd)
	if err != nil {
		return "", err
	}

	return "Terminal launched in OS window", nil
}
