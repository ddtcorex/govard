package cmd

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine"
	engineremote "govard/internal/engine/remote"

	"github.com/pterm/pterm"
)

const openLocalEnvironment = "local"

var (
	magentoFrontNamePattern   = regexp.MustCompile(`(?i)['"]frontName['"]\s*=>\s*['"]([^'"]+)['"]`)
	magentoTablePrefixPattern = regexp.MustCompile(`(?i)['"]table_prefix['"]\s*=>\s*['"]([^'"]*)['"]`)
)

func runOpenAdminTarget(config engine.Config, requestedEnvironment string) error {
	environment, isRemote, err := resolveOpenEnvironment(config, requestedEnvironment)
	if err != nil {
		return err
	}

	var url string
	if isRemote {
		remoteCfg, err := ensureOpenRemote(config, environment, engine.RemoteCapabilityFiles)
		if err != nil {
			return err
		}
		var adminPath string
		if strings.EqualFold(strings.TrimSpace(config.Framework), conventions.FrameworkEmdash) {
			adminPath = "_emdash/admin"
		} else {
			detectedAdminPath, probeErr := detectRemoteMagentoAdminPath(config, environment, remoteCfg)
			if probeErr != nil {
				adminPath = conventions.DefaultAdminPath
				pterm.Warning.Printf("Could not auto-detect admin path for '%s': %v\n", environment, probeErr)
			} else {
				adminPath = detectedAdminPath
			}
		}
		url = buildRemoteAdminURL(remoteCfg, adminPath)
	} else {
		url = detectLocalAdminURL(config)
	}

	pterm.Info.Printf("Opening %s\n", url)
	return openURL(url)
}

func runOpenShellTarget(config engine.Config, requestedEnvironment string) error {
	environment, isRemote, err := resolveOpenEnvironment(config, requestedEnvironment)
	if err != nil {
		return err
	}
	if !isRemote {
		return runOpenLocalShell(config)
	}

	remoteCfg, err := ensureOpenRemote(config, environment, engine.RemoteCapabilityFiles)
	if err != nil {
		return err
	}

	pterm.Info.Printf("Opening remote shell on '%s'.\n", environment)

	// Pre-flight SSH auth check: since RunRemoteShell uses syscall.Exec
	// (replacing the current process), we must probe auth and offer
	// ssh-copy-id *before* the handoff — there's no coming back after.
	if err := offerSSHKeyCopyOnAuthFailure(environment, remoteCfg); err != nil {
		return err
	}

	remoteCommand := buildRemoteShellCommand(remoteCfg.Path)
	return engineremote.RunRemoteShell(environment, remoteCfg, remoteCommand)
}

func runOpenSFTPTarget(config engine.Config, requestedEnvironment string) error {
	environment, isRemote, err := resolveOpenEnvironment(config, requestedEnvironment)
	if err != nil {
		return err
	}
	if !isRemote {
		pterm.Info.Println("SFTP is not supported for local target. Use `govard open sftp -e <remote>`.")
		return nil
	}

	remoteCfg, err := ensureOpenRemote(config, environment, engine.RemoteCapabilityFiles)
	if err != nil {
		return err
	}
	target := buildSFTPURL(remoteCfg)
	pterm.Info.Printf("Opening %s\n", target)
	return openURL(target)
}

func runOpenSearchTarget(config engine.Config, target string, requestedEnvironment string) error {
	_, isRemote, err := resolveOpenEnvironment(config, requestedEnvironment)
	if err != nil {
		return err
	}
	if isRemote {
		return fmt.Errorf("open %s with remote environment is not supported yet", target)
	}

	url := "https://elasticsearch.govard.test"
	if target == "opensearch" {
		url = "https://opensearch.govard.test"
	}
	pterm.Info.Printf("Opening %s\n", url)
	return openURL(url)
}

func runOpenMailTarget(config engine.Config, requestedEnvironment string) error {
	_, isRemote, err := resolveOpenEnvironment(config, requestedEnvironment)
	if err != nil {
		return err
	}
	if isRemote {
		return fmt.Errorf("open mail with remote environment is not supported yet")
	}

	url := "https://mail.govard.test"
	pterm.Info.Printf("Opening %s\n", url)
	return openURL(url)
}

func runOpenMFTFTarget(config engine.Config, requestedEnvironment string) error {
	_, isRemote, err := resolveOpenEnvironment(config, requestedEnvironment)
	if err != nil {
		return err
	}
	if isRemote {
		return fmt.Errorf("open mftf with remote environment is not supported yet")
	}

	url := "https://selenium.govard.test"
	pterm.Info.Printf("Opening Selenium VNC Viewer: %s\n", url)
	return openURL(url)
}

func runOpenPortainerTarget(config engine.Config, requestedEnvironment string) error {
	_, isRemote, err := resolveOpenEnvironment(config, requestedEnvironment)
	if err != nil {
		return err
	}
	if isRemote {
		return fmt.Errorf("open portainer is local-only")
	}

	url := "https://portainer.govard.test"
	pterm.Info.Printf("Opening %s\n", url)
	return openURL(url)
}

func openAdminURL(config engine.Config) string {
	baseURL := "https://" + strings.TrimSpace(config.Domain)
	if strings.EqualFold(strings.TrimSpace(config.Framework), conventions.FrameworkEmdash) {
		return joinURLWithPath(baseURL, "_emdash/admin")
	}
	return joinURLWithPath(baseURL, conventions.DefaultAdminPath)
}

func detectLocalAdminURL(config engine.Config) string {
	if strings.EqualFold(strings.TrimSpace(config.Framework), conventions.FrameworkEmdash) {
		return openAdminURL(config)
	}

	baseURL := "https://" + strings.TrimSpace(config.Domain)
	if !engine.IsMagento2Family(config.Framework) {
		return joinURLWithPath(baseURL, conventions.DefaultAdminPath)
	}

	projectRoot, _ := os.Getwd()
	frontName, tablePrefix := detectLocalMagentoAdminMeta(projectRoot)
	if tablePrefix == "" {
		tablePrefix = config.TablePrefix
	}
	dbValues := readLocalMagentoAdminDBValues(config, tablePrefix)
	return resolveMagentoAdminURL(baseURL, frontName, dbValues)
}

func runOpenLocalShell(config engine.Config) error {
	containerName, workdir, user := resolveShellExecution(config)

	if err := RunInContainerAt(containerName, user, workdir, "bash", []string{}); err == nil {
		return nil
	}
	return RunInContainerAt(containerName, user, workdir, "sh", []string{})
}

func buildRemoteShellCommand(projectPath string) string {
	trimmedPath := strings.TrimSpace(projectPath)
	cmd := "if command -v bash >/dev/null 2>&1; then exec bash -l; else exec sh; fi"
	if trimmedPath == "" {
		return cmd
	}
	return "cd " + engineremote.QuoteRemotePath(trimmedPath) + " && " + cmd
}

func resolveOpenEnvironment(config engine.Config, requestedEnvironment string) (string, bool, error) {
	requested := strings.ToLower(strings.TrimSpace(requestedEnvironment))
	if requested == "" || requested == openLocalEnvironment {
		return openLocalEnvironment, false, nil
	}

	remoteName, ok := findRemoteByNameOrEnvironment(config, requested)
	if !ok {
		return "", false, fmt.Errorf("unknown remote environment %q", requestedEnvironment)
	}
	return remoteName, true, nil
}

func ensureOpenRemote(config engine.Config, name string, capability string) (engine.RemoteConfig, error) {
	_, remoteCfg, err := ensureRemoteKnown(config, name)
	if err != nil {
		return engine.RemoteConfig{}, err
	}
	if capability != "" && !engine.RemoteCapabilityEnabled(remoteCfg, capability) {
		return engine.RemoteConfig{}, fmt.Errorf(
			"remote '%s' does not allow %s operations (capabilities: %s)",
			name,
			capability,
			strings.Join(engine.RemoteCapabilityList(remoteCfg), ","),
		)
	}
	return remoteCfg, nil
}

func buildRemoteAdminURL(remoteCfg engine.RemoteConfig, adminPath string) string {
	if remoteCfg.URL != "" {
		return joinURLWithPath(remoteCfg.URL, adminPath)
	}

	base := strings.TrimSpace(remoteCfg.Host)
	if base == "" {
		base = "localhost"
	}
	if !strings.HasPrefix(strings.ToLower(base), "http://") && !strings.HasPrefix(strings.ToLower(base), "https://") {
		base = "https://" + base
	}
	base = strings.TrimRight(base, "/")
	trimmedPath := strings.Trim(strings.TrimSpace(adminPath), "/")
	if trimmedPath == "" {
		trimmedPath = conventions.DefaultAdminPath
	}
	return base + "/" + trimmedPath
}

func detectLocalMagentoAdminMeta(projectRoot string) (string, string) {
	envPath := filepath.Join(projectRoot, "app", "etc", "env.php")
	content, err := os.ReadFile(envPath)
	if err != nil {
		return "", ""
	}

	raw := string(content)
	frontName := ""
	tablePrefix := ""

	if match := magentoFrontNamePattern.FindStringSubmatch(raw); len(match) == 2 {
		frontName = strings.Trim(strings.TrimSpace(match[1]), "/")
	}
	if match := magentoTablePrefixPattern.FindStringSubmatch(raw); len(match) == 2 {
		tablePrefix = strings.TrimSpace(match[1])
	}

	return frontName, tablePrefix
}

func readLocalMagentoAdminDBValues(config engine.Config, tablePrefix string) map[string]string {
	containerName := dbContainerName(config)
	if err := ensureLocalDBRunning(containerName); err != nil {
		return map[string]string{}
	}

	credentials := resolveLocalDBCredentials(config, containerName)
	table := tablePrefix + "core_config_data"
	query := "SELECT path, value FROM " + table +
		" WHERE path IN ('admin/url/use_custom','admin/url/use_custom_path','admin/url/custom','admin/url/custom_path')"

	args := []string{"exec", "-i"}
	if strings.TrimSpace(credentials.Password) != "" {
		args = append(args, "-e", "MYSQL_PWD="+credentials.Password)
	}
	args = append(args, containerName, "mysql", "-u", credentials.Username, "-N", "-B", credentials.Database, "-e", query)

	output, err := exec.Command("docker", args...).Output()
	if err != nil {
		return map[string]string{}
	}

	return parseMagentoAdminDBRows(string(output))
}

func parseMagentoAdminDBRows(raw string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return values
}

func resolveMagentoAdminURL(baseURL string, envFrontName string, dbValues map[string]string) string {
	frontName := strings.Trim(strings.TrimSpace(envFrontName), "/")
	if frontName == "" {
		frontName = conventions.DefaultAdminPath
	}

	if truthyMagentoConfig(dbValues["admin/url/use_custom_path"]) {
		if customPath := normalizeMagentoAdminTarget(dbValues["admin/url/custom_path"]); customPath != "" {
			if isURLTarget(customPath) {
				return customPath
			}
			return joinURLWithPath(baseURL, customPath)
		}
	}

	if truthyMagentoConfig(dbValues["admin/url/use_custom"]) {
		if custom := normalizeMagentoAdminTarget(dbValues["admin/url/custom"]); custom != "" {
			if isURLTarget(custom) {
				return custom
			}
			return joinURLWithPath(baseURL, custom)
		}
	}

	return joinURLWithPath(baseURL, frontName)
}

func truthyMagentoConfig(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeMagentoAdminTarget(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), "/")
}

func isURLTarget(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")
}

func joinURLWithPath(baseURL string, path string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	trimmedPath := strings.Trim(strings.TrimSpace(path), "/")
	if trimmedPath == "" {
		return base
	}
	return base + "/" + trimmedPath
}

func detectRemoteMagentoAdminPath(config engine.Config, remoteName string, remoteCfg engine.RemoteConfig) (string, error) {
	if !engine.IsMagento2Family(config.Framework) {
		return conventions.DefaultAdminPath, nil
	}

	phpScript := `$c=@include "app/etc/env.php"; if(!is_array($c)){fwrite(STDERR,"env.php not found"); exit(2);} echo (string)($c["backend"]["frontName"] ?? "` + conventions.DefaultAdminPath + `");`
	remoteCommand := "php -r " + engine.ShellQuote(phpScript)
	if path := strings.TrimSpace(remoteCfg.Path); path != "" {
		remoteCommand = "cd " + engineremote.QuoteRemotePath(path) + " && " + remoteCommand
	}

	probeCmd := engineremote.BuildSSHExecCommand(remoteName, remoteCfg, true, remoteCommand)
	output, err := probeCmd.CombinedOutput()
	if err != nil {
		return conventions.DefaultAdminPath, fmt.Errorf("probe failed: %w", err)
	}

	value := strings.Trim(strings.TrimSpace(string(output)), "/")
	if value == "" {
		value = conventions.DefaultAdminPath
	}
	return value, nil
}

func buildSFTPURL(remoteCfg engine.RemoteConfig) string {
	port := remoteCfg.Port
	if port <= 0 {
		port = 22
	}
	sftpURL := &url.URL{
		Scheme: "sftp",
		User:   url.User(remoteCfg.User),
		Host:   net.JoinHostPort(strings.TrimSpace(remoteCfg.Host), fmt.Sprintf("%d", port)),
		Path:   strings.TrimSpace(remoteCfg.Path),
	}
	return sftpURL.String()
}

func OpenAdminURLForTest(config engine.Config) string {
	return openAdminURL(config)
}

func DetectLocalAdminURLForTest(config engine.Config) string {
	return detectLocalAdminURL(config)
}

func ResolveOpenEnvironmentForTest(config engine.Config, requestedEnvironment string) (string, bool, error) {
	return resolveOpenEnvironment(config, requestedEnvironment)
}

func BuildRemoteAdminURLForTest(remoteCfg engine.RemoteConfig, adminPath string) string {
	return buildRemoteAdminURL(remoteCfg, adminPath)
}

func BuildSFTPURLForTest(remoteCfg engine.RemoteConfig) string {
	return buildSFTPURL(remoteCfg)
}

func ResolveMagentoAdminURLForTest(baseURL string, envFrontName string, dbValues map[string]string) string {
	return resolveMagentoAdminURL(baseURL, envFrontName, dbValues)
}
