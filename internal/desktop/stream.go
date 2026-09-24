package desktop

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"govard/internal/conventions"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
var orphanAnsiStylePattern = regexp.MustCompile(`\[(?:\d{1,3}(?:;\d{1,3})*)m`)

// Internal log streaming logic

func (s *LogService) StartLogStream(project string) (res string, err error) {
	defer RecoverPanic(&err, "StartLogStream")
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

func (s *LogService) StartLogStreamForService(project string, service string) (res string, err error) {
	defer RecoverPanic(&err, "StartLogStreamForService")
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

func (s *LogService) StopLogStream() (res string, err error) {
	defer RecoverPanic(&err, "StopLogStream")
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	if s.streamCancel != nil {
		s.streamCancel()
		s.streamCancel = nil
		return "Live logs stopped", nil
	}
	return "Live logs already stopped", nil
}

func (s *LogService) StartGlobalServiceLogStream(serviceID string) (res string, err error) {
	defer RecoverPanic(&err, "StartGlobalServiceLogStream")
	spec, err := resolveGlobalServiceSpec(serviceID)
	if err != nil {
		return "", err
	}

	s.globalStreamMu.Lock()
	defer s.globalStreamMu.Unlock()

	if s.globalStreamCancel != nil {
		s.globalStreamCancel()
		s.globalStreamCancel = nil
	}

	streamCtx, cancel := context.WithCancel(s.ctx)
	s.globalStreamCancel = cancel
	go s.streamGlobalServiceLogs(streamCtx, spec)

	return "Global service live logs started", nil
}

func (s *LogService) StopGlobalServiceLogStream() (res string, err error) {
	defer RecoverPanic(&err, "StopGlobalServiceLogStream")
	s.globalStreamMu.Lock()
	defer s.globalStreamMu.Unlock()

	if s.globalStreamCancel != nil {
		s.globalStreamCancel()
		s.globalStreamCancel = nil
		return "Global service live logs stopped", nil
	}
	return "Global service live logs already stopped", nil
}

func (s *LogService) GetGlobalServiceLogs(serviceID string, lines int) (res string, err error) {
	defer RecoverPanic(&err, "GetGlobalServiceLogs")
	spec, err := resolveGlobalServiceSpec(serviceID)
	if err != nil {
		return "", err
	}
	if lines <= 0 {
		lines = 200
	}

	output, err := exec.Command(
		"docker",
		"logs",
		"--tail",
		fmt.Sprintf("%d", lines),
		spec.ContainerName,
	).CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}

func (s *LogService) streamLogs(ctx context.Context, project string, service string) {
	info, err := loadProjectInfo(project)
	if err != nil {
		s.platform.Emit("logs:error", map[string]interface{}{
			"message": fmt.Sprintf("Failed to load project info for %s: %s", project, err.Error()),
		})
		return
	}

	containerName := resolveLogContainer(info, service)
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "100", "-f", containerName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.platform.Emit("logs:error", "Failed to stream logs: "+err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.platform.Emit("logs:error", "Failed to stream logs: "+err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		s.platform.Emit("logs:error", "Failed to start log stream: "+err.Error())
		return
	}

	s.platform.Emit("logs:status", fmt.Sprintf("Streaming logs from %s", containerName))

	done := make(chan struct{}, 2)
	go scanLogPipe(s.ctx, s.platform, stdout, "logs:line", done)
	go scanLogPipe(s.ctx, s.platform, stderr, "logs:line", done)

	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	case <-done:
	}
}

func (s *LogService) streamGlobalServiceLogs(ctx context.Context, spec globalServiceSpec) {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "100", "-f", spec.ContainerName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.platform.Emit("global-logs:error", "Failed to stream logs: "+err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.platform.Emit("global-logs:error", "Failed to stream logs: "+err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		s.platform.Emit("global-logs:error", "Failed to start log stream: "+err.Error())
		return
	}

	s.platform.Emit("global-logs:status",
		fmt.Sprintf("Streaming logs from %s", spec.ContainerName),
	)

	done := make(chan struct{}, 2)
	go scanLogPipe(s.ctx, s.platform, stdout, "global-logs:line", done)
	go scanLogPipe(s.ctx, s.platform, stderr, "global-logs:line", done)

	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	case <-done:
	}
}

// scanLogPipe streams one pipe into the UI. The context is not read here: it
// is kept so every pipe started by a service carries the same cancellation
// parent as the exec.CommandContext that produced it. Emission goes through p.
func scanLogPipe(ctx context.Context, p Platform, pipe interface{}, event string, done chan<- struct{}) {
	reader, ok := pipe.(interface {
		Read(p []byte) (n int, err error)
	})
	if !ok {
		p.Emit("logs:error", "Failed to read log stream")
		done <- struct{}{}
		return
	}

	scanner := bufio.NewScanner(reader)
	// Progress bars and spinners use \r to update the same line.
	// We want to treat \r as a line ending so that progress is reflected in the UI.
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
			if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[0:i], nil
			}
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data, nil
		}
		return 0, nil, nil
	})

	// Increase buffer for very long log lines (e.g. detailed SQL or JSON)
	const maxLogLine = 1024 * 1024
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxLogLine)

	// Throttle: batch lines and emit at most every 150ms to prevent IPC saturation
	// which causes WebKitWebProcess crashes on high-volume syncs (e.g. large DB imports).
	const (
		flushInterval = 150 * time.Millisecond
		maxBatchSize  = 50
	)
	var batch []string
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		p.Emit(event, strings.Join(batch, "\n"))
		batch = batch[:0]
	}

	linesCh := make(chan string, 512)
	go func() {
		for scanner.Scan() {
			line := sanitizeStreamLine(scanner.Bytes())
			if strings.TrimSpace(line) == "" {
				continue
			}
			linesCh <- line
		}
		if err := scanner.Err(); err != nil {
			p.Emit("logs:error", "Log scanner error: "+err.Error())
		}
		close(linesCh)
	}()

	for {
		select {
		case line, ok := <-linesCh:
			if !ok {
				// Scanner finished — flush remaining and signal done
				flushBatch()
				done <- struct{}{}
				return
			}
			batch = append(batch, line)
			if len(batch) >= maxBatchSize {
				flushBatch()
			}
		case <-ticker.C:
			flushBatch()
		}
	}
}

func sanitizeStreamLine(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	valid := bytes.ToValidUTF8(raw, []byte{})
	line := string(valid)
	line = ansiEscapePattern.ReplaceAllString(line, "")
	line = orphanAnsiStylePattern.ReplaceAllString(line, "")

	var builder strings.Builder
	builder.Grow(len(line))
	for _, r := range line {
		if r == '\t' || (r >= 0x20 && r != 0x7f) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func resolveLogContainer(info *projectInfo, service string) string {
	if service != "" && service != "all" {
		return info.name + "-" + service + conventions.ReplicaSuffix
	}
	// Pick first running service or default to php
	if info.services["php"] {
		return info.name + conventions.PHPSuffix
	}
	for s := range info.services {
		return info.name + "-" + s + conventions.ReplicaSuffix
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

	discovered := discoveredLogTargets(info)
	targets := resolveRequestedLogTargets(service, discovered)
	var chunks []string
	successCount := 0

	for _, target := range targets {
		containerName := resolveLogContainer(info, target)
		output, err := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerName).CombinedOutput()
		if err != nil {
			continue
		}
		successCount++

		text := strings.TrimSpace(string(output))
		if text == "" {
			continue
		}

		if len(targets) > 1 {
			text = prefixServiceLogLines(target, text)
		}
		chunks = append(chunks, text)
	}

	if len(chunks) == 0 {
		if successCount > 0 {
			return "", nil
		}
		containerName := resolveLogContainer(info, service)
		output, err := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerName).CombinedOutput()
		return string(output), err
	}

	return strings.Join(chunks, "\n"), nil
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

func discoveredLogTargets(info *projectInfo) []string {
	if info == nil || len(info.services) == 0 {
		return nil
	}
	targets := make([]string, 0, len(info.services))
	for service := range info.services {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		targets = append(targets, service)
	}
	sort.Strings(targets)
	return targets
}

func prefixServiceLogLines(service string, raw string) string {
	trimmedService := strings.TrimSpace(service)
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	for index, line := range lines {
		lines[index] = fmt.Sprintf("[%s] %s", trimmedService, line)
	}
	return strings.Join(lines, "\n")
}
