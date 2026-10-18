package desktop

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"govard/internal/conventions"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Internal log streaming logic

func (s *LogService) StartLogStream(project string) (res string, err error) {
	defer RecoverPanic(&err, "StartLogStream")
	s.streamMu.Lock()
	defer s.streamMu.Unlock()

	if s.streamCancel != nil {
		s.streamCancel()
		s.streamCancel = nil
	}

	streamCtx, cancel := context.WithCancel(s.lifecycleContext())
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

	streamCtx, cancel := context.WithCancel(s.lifecycleContext())
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

	streamCtx, cancel := context.WithCancel(s.lifecycleContext())
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
		s.platform.Emit(EventLogsError, fmt.Sprintf("Failed to load project info for %s: %s", project, err.Error()))
		return
	}

	containerName := resolveLogContainer(info, service)
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "100", "-f", containerName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.platform.Emit(EventLogsError, "Failed to stream logs: "+err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.platform.Emit(EventLogsError, "Failed to stream logs: "+err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		s.platform.Emit(EventLogsError, "Failed to start log stream: "+err.Error())
		return
	}

	s.platform.Emit(EventLogsStatus, fmt.Sprintf("Streaming logs from %s", containerName))

	done := make(chan struct{}, 2)
	go scanLogPipe(s.lifecycleContext(), s.platform, stdout, EventLogsLine, done)
	go scanLogPipe(s.lifecycleContext(), s.platform, stderr, EventLogsLine, done)

	select {
	case <-ctx.Done():
		reapLogStreamProcess(cmd)
	case <-done:
	}
}

func (s *LogService) streamGlobalServiceLogs(ctx context.Context, spec globalServiceSpec) {
	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", "100", "-f", spec.ContainerName)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.platform.Emit(EventGlobalLogsError, "Failed to stream logs: "+err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.platform.Emit(EventGlobalLogsError, "Failed to stream logs: "+err.Error())
		return
	}

	if err := cmd.Start(); err != nil {
		s.platform.Emit(EventGlobalLogsError, "Failed to start log stream: "+err.Error())
		return
	}

	s.platform.Emit(EventGlobalLogsStatus,
		fmt.Sprintf("Streaming logs from %s", spec.ContainerName),
	)

	done := make(chan struct{}, 2)
	go scanLogPipe(s.lifecycleContext(), s.platform, stdout, EventGlobalLogsLine, done)
	go scanLogPipe(s.lifecycleContext(), s.platform, stderr, EventGlobalLogsLine, done)

	select {
	case <-ctx.Done():
		reapLogStreamProcess(cmd)
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
		p.Emit(EventLogsError, "Failed to read log stream")
		done <- struct{}{}
		return
	}

	streamSanitizedBatches(p, reader, func(batch []string) {
		p.Emit(event, strings.Join(batch, "\n"))
	})
	done <- struct{}{}
}

func sanitizeStreamLine(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	// Log lines keep their full length (max <= 0 disables truncation);
	// display caps belong to the toast/status-line callers.
	return sanitizeUIText(string(raw), 0)
}

// Shared pipe batching for scanLogPipe and scanSyncPipe: one \r/\n splitter,
// one sanitized-line pump, one throttle. Callers differ only in what a flush
// emits, so behavior stays identical whichever pipe a service streams.
const (
	// logFlushInterval throttles batch emission to prevent IPC saturation
	// which causes WebKitWebProcess crashes on high-volume syncs (e.g. large DB imports).
	logFlushInterval = 150 * time.Millisecond
	logMaxBatchSize  = 50
	// maxScanLineBytes fits very long log lines (e.g. detailed SQL or JSON).
	maxScanLineBytes = 1024 * 1024
)

// splitLogChunks treats \r as a line ending so progress bars and spinners,
// which redraw with \r, are reflected in the UI.
func splitLogChunks(data []byte, atEOF bool) (advance int, token []byte, err error) {
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
}

// streamSanitizedBatches pumps sanitized non-blank lines from pipe and calls
// flush with each throttled batch; it returns once the pipe is drained.
func streamSanitizedBatches(p Platform, pipe io.Reader, flush func(batch []string)) {
	scanner := bufio.NewScanner(pipe)
	scanner.Split(splitLogChunks)

	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxScanLineBytes)

	var batch []string
	ticker := time.NewTicker(logFlushInterval)
	defer ticker.Stop()

	flushBatch := func() {
		if len(batch) == 0 {
			return
		}
		flush(batch)
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
			p.Emit(EventLogsError, "Log scanner error: "+err.Error())
		}
		close(linesCh)
	}()

	for {
		select {
		case line, ok := <-linesCh:
			if !ok {
				// Scanner finished — flush remaining and return
				flushBatch()
				return
			}
			batch = append(batch, line)
			if len(batch) >= logMaxBatchSize {
				flushBatch()
			}
		case <-ticker.C:
			flushBatch()
		}
	}
}

// reapLogStreamProcess stops a streaming child and reaps it. Kill without
// Wait leaves a zombie behind per stop: every Start/Stop log cycle leaks
// one until the app exits.
func reapLogStreamProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	// Wait reaps the child and unblocks once the pipe scanners drain the
	// dead process's pipes to EOF; without it every stopped stream leaks a
	// zombie.
	_ = cmd.Wait()
}

func resolveLogContainer(info *projectInfo, service string) string {
	if service != "" && service != "all" {
		return info.name + "-" + service + conventions.ReplicaSuffix
	}
	// Pick first running service or default to php
	if info.services["php"] {
		return info.name + conventions.PHPSuffix
	}
	// Deterministic fallback (front-door order, then alphabetical): a map
	// hit here flipped between services across calls.
	if picked := stableServiceFallback(info.services); picked != "" {
		return info.name + "-" + picked + conventions.ReplicaSuffix
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
