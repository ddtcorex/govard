package desktop

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Internal log streaming logic

func (s *LogService) StartLogStream(project string) (string, error) {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	if s.streamCancel != nil {
		s.streamCancel()
		s.streamCancel = nil
	}

	streamCtx, cancel := context.WithCancel(s.ctx)
	s.streamCancel = cancel

	go s.streamLogs(streamCtx, project, "")
	return "Live logs started", nil
}

func (s *LogService) StartLogStreamForService(project string, service string) (string, error) {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	if s.streamCancel != nil {
		s.streamCancel()
		s.streamCancel = nil
	}

	streamCtx, cancel := context.WithCancel(s.ctx)
	s.streamCancel = cancel

	go s.streamLogs(streamCtx, project, service)
	return "Live logs started", nil
}

func (s *LogService) StopLogStream() (string, error) {
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	if s.streamCancel != nil {
		s.streamCancel()
		s.streamCancel = nil
		return "Live logs stopped", nil
	}
	return "Live logs already stopped", nil
}

func (s *LogService) streamLogs(ctx context.Context, project string, service string) {
	info, err := loadProjectInfo(project)
	if err != nil {
		runtime.EventsEmit(s.ctx, "logs:error", map[string]interface{}{
			"message": fmt.Sprintf("Failed to load project info for %s: %s", project, err.Error()),
		})
		return
	}

	containerName := resolveLogContainer(info, service)
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "100", "-f", containerName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		runtime.EventsEmit(s.ctx, "logs:error", "Failed to stream logs: "+err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		runtime.EventsEmit(s.ctx, "logs:error", "Failed to stream logs: "+err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		runtime.EventsEmit(s.ctx, "logs:error", "Failed to start log stream: "+err.Error())
		return
	}

	runtime.EventsEmit(s.ctx, "logs:status", fmt.Sprintf("Streaming logs from %s", containerName))

	done := make(chan struct{}, 2)
	go scanLogPipe(s.ctx, stdout, "logs:line", done)
	go scanLogPipe(s.ctx, stderr, "logs:line", done)

	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	case <-done:
	}
}

func scanLogPipe(ctx context.Context, pipe interface{}, event string, done chan<- struct{}) {
	reader, ok := pipe.(interface {
		Read(p []byte) (n int, err error)
	})
	if !ok {
		runtime.EventsEmit(ctx, "logs:error", "Failed to read log stream")
		done <- struct{}{}
		return
	}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		runtime.EventsEmit(ctx, event, scanner.Text())
	}
	done <- struct{}{}
}

func resolveLogContainer(info *projectInfo, service string) string {
	if service != "" {
		return info.name + "-" + service + "-1"
	}
	// Pick first running service or default to php
	if info.services["php"] {
		return info.name + "-php-1"
	}
	for s := range info.services {
		return info.name + "-" + s + "-1"
	}
	return info.name
}

func getLogs(project string, lines int) (string, error) {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}
	containerName := resolveLogContainer(info, "")
	output, err := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerName).CombinedOutput()
	return string(output), err
}

func getLogsForService(project string, service string, lines int) (string, error) {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}
	containerName := resolveLogContainer(info, service)
	output, err := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerName).CombinedOutput()
	return string(output), err
}

func resolveRequestedLogTargets(service string, discovered []string) []string {
	requested := strings.ToLower(strings.TrimSpace(service))
	if requested == "" || requested == "all" {
		if len(discovered) == 0 {
			return []string{"web"}
		}
		return discovered
	}
	return []string{requested}
}

func prefixServiceLogLines(service string, raw string) string {
	trimmedService := strings.TrimSpace(service)
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	for index, line := range lines {
		lines[index] = fmt.Sprintf("[%s] %s", trimmedService, line)
	}
	return strings.Join(lines, "\n")
}
