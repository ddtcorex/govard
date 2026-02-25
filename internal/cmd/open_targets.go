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

	url := openAdminURL(config)
	if isRemote {
		remoteCfg, err := ensureOpenRemote(config, environment, engine.RemoteCapabilityFiles)
		if err != nil {
			return err
		}
		adminPath, probeErr := detectRemoteMagentoAdminPath(config, environment, remoteCfg)
		if probeErr != nil {
			pterm.Warning.Printf("Could not auto-detect admin path for '%s': %v\n", environment, probeErr)
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

	remoteCommand := buildRemoteShellCommand(remoteCfg.Path)
	shellCmd := engineremote.BuildSSHExecCommand(environment, remoteCfg, true, remoteCommand)
	shellCmd.Stdin = os.Stdin
	shellCmd.Stdout = os.Stdout
	shellCmd.Stderr = os.Stderr

	pterm.Info.Printf("Opening remote shell on '%s'.\n", environment)
	return shellCmd.Run()
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

func runOpenPMATarget(config engine.Config, requestedEnvironment string) error {
	_, isRemote, err := resolveOpenDBEnvironment(config, requestedEnvironment)
	if err != nil {
		return err
	}
	if isRemote {
		return fmt.Errorf("open pma is local-only; use `govard open db -e <remote>` for remote DB access")
	}

	url := "https://pma.govard.test"
	pterm.Info.Printf("Opening %s\n", url)
	return openURL(url)
}

func openAdminURL(config engine.Config) string {
	return "https://" + config.Domain + "/admin"
}

func detectLocalAdminURL(config engine.Config) string {
	baseURL := "https://" + strings.TrimSpace(config.Domain)
	if strings.ToLower(strings.TrimSpace(config.Recipe)) != "magento2" {
		return joinURLWithPath(baseURL, "admin")
	}

	projectRoot, _ := os.Getwd()
	frontName, tablePrefix := detectLocalMagentoAdminMeta(projectRoot)
	dbValues := readLocalMagentoAdminDBValues(config, tablePrefix)
	return resolveMagentoAdminURL(baseURL, frontName, dbValues)
}

func runOpenLocalShell(config engine.Config) error {
	containerName := fmt.Sprintf("%s-php-1", config.ProjectName)
	user := resolveProjectExecUser(config, "www-data")

	if err := runInContainer(containerName, user, "bash", []string{}); err == nil {
		return nil
	}
	return runInContainer(containerName, user, "sh", []string{})
}

func buildRemoteShellCommand(projectPath string) string {
	trimmedPath := strings.TrimSpace(projectPath)
	if trimmedPath == "" {
		return "(bash -l || sh)"
	}
	return "cd " + shellQuote(trimmedPath) + " && (bash -l || sh)"
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
	remoteCfg, err := ensureRemoteKnown(config, name)
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
		trimmedPath = "admin"
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

	credentials := resolveLocalDBCredentials(containerName)
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
		frontName = "admin"
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
	if strings.ToLower(strings.TrimSpace(config.Recipe)) != "magento2" {
		return "admin", nil
	}

	phpScript := `$c=@include "app/etc/env.php"; if(!is_array($c)){fwrite(STDERR,"env.php not found"); exit(2);} echo (string)($c["backend"]["frontName"] ?? "admin");`
	remoteCommand := "php -r " + shellQuote(phpScript)
	if path := strings.TrimSpace(remoteCfg.Path); path != "" {
		remoteCommand = "cd " + shellQuote(path) + " && " + remoteCommand
	}

	probeCmd := engineremote.BuildSSHExecCommand(remoteName, remoteCfg, true, remoteCommand)
	output, err := probeCmd.CombinedOutput()
	if err != nil {
		return "admin", fmt.Errorf("probe failed: %w", err)
	}

	value := strings.Trim(strings.TrimSpace(string(output)), "/")
	if value == "" {
		value = "admin"
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
