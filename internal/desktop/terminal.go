package desktop

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/creack/pty"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type terminalSession struct {
	id     string
	pty    io.ReadWriteCloser
	cmd    *exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
}

type TerminalOptions struct {
	User  string `json:"user"`
	Shell string `json:"shell"`
}

var (
	sessions   = make(map[string]*terminalSession)
	sessionsMu sync.Mutex
)

// LogService methods

func (s *LogService) StartTerminal(project string, service string, user string, shell string) (string, error) {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}

	targetService := resolveShellServiceName(info, service)
	containerName := resolveShellContainer(info, targetService)
	chosenShell := normalizeShell(shell)
	chosenUser := normalizeShellUser(info, targetService, user)

	args := []string{"exec", "-it"}
	if chosenUser != "" {
		args = append(args, "-u", chosenUser)
	}
	args = append(args, containerName, chosenShell)

	cmd := exec.Command("docker", args...)
	cmd.Dir = filepath.Clean(info.workingDir)

	sessionID := fmt.Sprintf("%s-%s", project, targetService)
	return s.startSession(sessionID, cmd)
}

func (s *LogService) StartGovardTerminal(project string, args []string) (string, error) {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "", err
	}

	binary, err := exec.LookPath("govard")
	if err != nil {
		binary, _ = os.Executable()
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = filepath.Clean(info.workingDir)

	remoteName := "exec"
	for i, arg := range args {
		if arg == "-e" || arg == "--environment" {
			if i+1 < len(args) {
				remoteName = args[i+1]
			}
			break
		}
	}
	sessionID := fmt.Sprintf("remote-%s-%s", project, remoteName)
	return s.startSession(sessionID, cmd)
}

func (s *LogService) WriteTerminal(sessionID string, data string) {
	sessionsMu.Lock()
	session, ok := sessions[sessionID]
	sessionsMu.Unlock()

	if ok {
		_, _ = session.pty.Write([]byte(data))
	}
}

func (s *LogService) ResizeTerminal(sessionID string, cols int, rows int) {
	sessionsMu.Lock()
	session, ok := sessions[sessionID]
	sessionsMu.Unlock()

	if ok {
		_ = pty.Setsize(session.pty.(*os.File), &pty.Winsize{
			Cols: uint16(cols),
			Rows: uint16(rows),
		})
	}
}

func (s *LogService) GetLogs(project string, lines int) (string, error) {
	output, err := getLogs(project, lines)
	if err != nil {
		return "", err
	}
	return output, nil
}

func (s *LogService) GetLogsForService(project string, service string, lines int) (string, error) {
	output, err := getLogsForService(project, service, lines)
	if err != nil {
		return "", err
	}
	return output, nil
}

// Internal session management

func (s *LogService) startSession(sessionID string, cmd *exec.Cmd) (string, error) {
	f, err := pty.Start(cmd)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(s.ctx)

	sessionsMu.Lock()
	if old, ok := sessions[sessionID]; ok {
		old.cancel()
		old.pty.Close()
	}
	sessions[sessionID] = &terminalSession{
		id:     sessionID,
		pty:    f,
		cmd:    cmd,
		ctx:    ctx,
		cancel: cancel,
	}
	sessionsMu.Unlock()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				runtime.EventsEmit(s.ctx, "terminal:output", map[string]interface{}{
					"id":   sessionID,
					"data": string(buf[:n]),
				})
			}
			if err != nil {
				break
			}
		}
		sessionsMu.Lock()
		if sess, ok := sessions[sessionID]; ok && sess.pty == f {
			delete(sessions, sessionID)
		}
		sessionsMu.Unlock()
		runtime.EventsEmit(s.ctx, "terminal:exit", map[string]interface{}{
			"id": sessionID,
		})
	}()

	return sessionID, nil
}
