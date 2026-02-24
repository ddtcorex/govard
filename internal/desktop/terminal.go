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
)

type terminalSession struct {
	id     string
	pty    io.ReadWriteCloser
	cmd    *exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
}

var (
	sessions   = make(map[string]*terminalSession)
	sessionsMu sync.Mutex
)

func (app *App) StartTerminal(project string, service string, user string, shell string) string {
	info, err := loadProjectInfo(project)
	if err != nil {
		return "error: " + err.Error()
	}

	containerName := resolveShellContainer(info, service)
	chosenShell := normalizeShell(shell)
	chosenUser := normalizeShellUser(info, service, user)

	args := []string{"exec", "-it"}
	if chosenUser != "" {
		args = append(args, "-u", chosenUser)
	}
	args = append(args, containerName, chosenShell)

	cmd := exec.Command("docker", args...)
	cmd.Dir = filepath.Clean(info.workingDir)

	f, err := pty.Start(cmd)
	if err != nil {
		return "error: " + err.Error()
	}

	sessionID := fmt.Sprintf("%s-%s", project, service)
	ctx, cancel := context.WithCancel(app.ctx)

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

	// Read from PTY and emit to frontend
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				emitEvent(app.ctx, "terminal:output", map[string]interface{}{
					"id":   sessionID,
					"data": string(buf[:n]),
				})
			}
			if err != nil {
				break
			}
		}
		sessionsMu.Lock()
		delete(sessions, sessionID)
		sessionsMu.Unlock()
		emitEvent(app.ctx, "terminal:exit", map[string]interface{}{
			"id": sessionID,
		})
	}()

	return sessionID
}

func (app *App) WriteTerminal(sessionID string, data string) {
	sessionsMu.Lock()
	session, ok := sessions[sessionID]
	sessionsMu.Unlock()

	if ok {
		_, _ = session.pty.Write([]byte(data))
	}
}

func (app *App) ResizeTerminal(sessionID string, cols int, rows int) {
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
