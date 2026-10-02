package cmd

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"
	engineremote "govard/internal/engine/remote"

	"github.com/pterm/pterm"
)

const (
	openDBLocalTarget = "local"
)

func runOpenDBTarget(config engine.Config, requestedEnvironment string, pmaFlag bool, clientFlag bool) error {
	environment, isRemote, err := resolveOpenDBEnvironment(config, requestedEnvironment)
	if err != nil {
		return err
	}

	// Keep CLI behavior stable: local `open db` defaults to PMA, remote defaults to client tunnel.
	usePma := !isRemote

	if pmaFlag {
		usePma = true
	} else if clientFlag {
		usePma = false
	}

	if !isRemote {
		containerName := dbContainerName(config)

		if usePma {
			credentials := resolveLocalDBCredentials(config, containerName)
			target := buildOpenDBPMAURL(config.ProjectName, credentials.Database)
			pterm.Info.Printf("Opening %s\n", target)
			return openURL(target)
		} else {
			if err := ensureLocalDBRunning(containerName); err != nil {
				return err
			}
			credentials := resolveLocalDBCredentials(config, containerName)
			// Assuming Ward doesn't bind MySQL locally by default, we build a connect string to point to 127.0.0.1:3306
			// A local developer might have overriden their proxy map or bound it to the host.
			connectionURL := buildOpenDBConnectionURL(credentials, conventions.MySQLPort)
			pterm.Info.Printf("Opening DB URL %s\n", redactedOpenDBConnectionURL(credentials, conventions.MySQLPort))
			return openURL(connectionURL)
		}
	}

	remoteCfg, err := resolveDBRemote(config, environment, false)
	if err != nil {
		return err
	}
	credentials, probeErr := resolveRemoteDBCredentials(config, environment, remoteCfg)
	if probeErr != nil {
		pterm.Warning.Println(formatRemoteDBProbeWarning(environment, probeErr))
	}
	credentials = credentials.withDefaults()

	remoteHost := strings.TrimSpace(credentials.Host)
	if remoteHost == "" {
		remoteHost = "127.0.0.1"
	}
	remotePort := credentials.Port
	if remotePort <= 0 {
		remotePort = conventions.MySQLPort
	}

	localPort, err := findAvailableLocalPort(remotePort)
	if err != nil {
		return err
	}

	connectionURL := buildOpenDBConnectionURL(credentials, localPort)
	pterm.Info.Printf(
		"Starting SSH tunnel for remote '%s' (127.0.0.1:%d -> %s:%d).\n",
		environment,
		localPort,
		remoteHost,
		remotePort,
	)
	tunnelCmd := buildOpenDBTunnelCommand(environment, remoteCfg, localPort, remoteHost, remotePort)
	tunnelCmd.Stdin = os.Stdin
	tunnelCmd.Stdout = os.Stdout
	tunnelCmd.Stderr = os.Stderr

	if err := tunnelCmd.Start(); err != nil {
		return err
	}

	if err := waitForOpenDBTunnel(localPort, 5*time.Second); err != nil {
		_ = tunnelCmd.Process.Kill()
		_ = tunnelCmd.Wait()
		return fmt.Errorf("wait for DB tunnel: %w", err)
	}

	pterm.Info.Printf("Opening DB URL %s\n", redactedOpenDBConnectionURL(credentials, localPort))
	if err := openURL(connectionURL); err != nil {
		_ = tunnelCmd.Process.Kill()
		_ = tunnelCmd.Wait()
		return err
	}

	pterm.Info.Println("Tunnel active. Press Ctrl+C to close.")
	return waitForOpenDBTunnelExit(tunnelCmd)
}

func resolveOpenDBEnvironment(config engine.Config, requestedEnvironment string) (string, bool, error) {
	requested := strings.ToLower(strings.TrimSpace(requestedEnvironment))
	if requested == "" {
		return openDBLocalTarget, false, nil
	}

	if requested == openDBLocalTarget {
		return openDBLocalTarget, false, nil
	}

	remoteName, ok := findRemoteByNameOrEnvironment(config, requested)
	if !ok {
		return "", false, fmt.Errorf("unknown remote environment %q", requestedEnvironment)
	}
	return remoteName, true, nil
}

func buildOpenDBConnectionURL(credentials dbCredentials, localPort int) string {
	return openDBConnectionURL(credentials, localPort, credentials.withDefaults().Password)
}

// redactedOpenDBConnectionURL is the display form of buildOpenDBConnectionURL:
// the same URL with the password replaced by "***" (no password section when
// none is set). The real URL goes only to openURL, never to the terminal.
func redactedOpenDBConnectionURL(credentials dbCredentials, localPort int) string {
	if strings.TrimSpace(credentials.withDefaults().Password) == "" {
		return openDBConnectionURL(credentials, localPort, "")
	}
	// url.URL escapes "*" in userinfo, so swap the escaped mask back for display.
	return strings.Replace(openDBConnectionURL(credentials, localPort, "***"), ":%2A%2A%2A@", ":***@", 1)
}

func openDBConnectionURL(credentials dbCredentials, localPort int, password string) string {
	credentials = credentials.withDefaults()
	connectionURL := &url.URL{
		Scheme: "mysql",
		Host:   net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)),
		Path:   "/" + credentials.Database,
	}
	if strings.TrimSpace(password) == "" {
		connectionURL.User = url.User(credentials.Username)
	} else {
		connectionURL.User = url.UserPassword(credentials.Username, password)
	}
	return connectionURL.String()
}

func buildOpenDBPMAURL(project string, database string) string {
	baseURL := "https://pma.govard.test"
	values := url.Values{}

	if normalizedProject := strings.TrimSpace(project); normalizedProject != "" {
		values.Set("project", normalizedProject)
	}

	if normalizedDatabase := strings.TrimSpace(database); normalizedDatabase != "" {
		values.Set("db", normalizedDatabase)
	}

	if len(values) == 0 {
		return baseURL
	}

	return baseURL + "/?" + values.Encode()
}

func findAvailableLocalPort(startPort int) (int, error) {
	if startPort <= 0 {
		startPort = conventions.MySQLPort
	}

	for port := startPort; port <= 65535; port++ {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		_ = listener.Close()
		return port, nil
	}

	return 0, fmt.Errorf("could not find a free local TCP port starting from %d", startPort)
}

func buildOpenDBTunnelCommand(remoteName string, remoteCfg engine.RemoteConfig, localPort int, remoteHost string, remotePort int) *exec.Cmd {
	args := engineremote.BuildSSHArgs(remoteName, remoteCfg, false, false)
	args = append(args, "-L", fmt.Sprintf("%d:%s:%d", localPort, remoteHost, remotePort), "-N", engineremote.RemoteTarget(remoteCfg))
	return exec.Command("ssh", args...)
}

func waitForOpenDBTunnel(localPort int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	target := net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort))
	for {
		conn, err := net.DialTimeout("tcp", target, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for tunnel on %s", target)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func waitForOpenDBTunnelExit(tunnelCmd *exec.Cmd) error {
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- tunnelCmd.Wait()
	}()

	interruptCh := make(chan os.Signal, 1)
	signal.Notify(interruptCh, os.Interrupt)
	defer signal.Stop(interruptCh)

	for {
		select {
		case err := <-waitCh:
			if err == nil || isInterruptExit(err) {
				return nil
			}
			return err
		case <-interruptCh:
			if tunnelCmd.Process != nil {
				_ = tunnelCmd.Process.Signal(os.Interrupt)
			}
		}
	}
}

func ResolveOpenDBEnvironmentForTest(config engine.Config, requestedEnvironment string) (string, bool, error) {
	return resolveOpenDBEnvironment(config, requestedEnvironment)
}

func RedactedOpenDBConnectionURLForTest(username string, password string, database string, localPort int) string {
	return redactedOpenDBConnectionURL(dbCredentials{
		Username: username,
		Password: password,
		Database: database,
	}, localPort)
}

func BuildOpenDBConnectionURLForTest(username string, password string, database string, localPort int) string {
	return buildOpenDBConnectionURL(dbCredentials{
		Username: username,
		Password: password,
		Database: database,
	}, localPort)
}
