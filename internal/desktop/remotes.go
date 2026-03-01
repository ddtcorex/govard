package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"govard/internal/engine"
	engineremote "govard/internal/engine/remote"

	"gopkg.in/yaml.v3"
)

const remoteMagentoAdminProbeScript = `$c=@include "app/etc/env.php"; if(!is_array($c)){fwrite(STDERR,"env.php not found"); exit(2);} echo (string)($c["backend"]["frontName"] ?? "admin");`
const remoteLastSyncReadLimit = 5000

type SyncInput struct {
	Project    string          `json:"project"`
	RemoteName string          `json:"remoteName"`
	Preset     string          `json:"preset"`
	Options    map[string]bool `json:"options"`
}

var defaultRunGovardCommandForDesktop = func(root string, args []string) (string, error) {
	binary, err := exec.LookPath("govard")
	if err != nil {
		return "", fmt.Errorf("govard CLI not found in PATH")
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = filepath.Clean(root)
	output, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(output))
	if err != nil {
		if trimmed != "" {
			return "", fmt.Errorf("%v: %s", err, trimmed)
		}
		return "", err
	}
	return trimmed, nil
}

var runGovardCommandForDesktop = defaultRunGovardCommandForDesktop

var defaultSyncSanitizeExcludePatterns = []string{
	".env",
	"*.pem",
	"*.key",
}

var defaultSyncLogExcludePatterns = []string{
	"var/log/**",
	"storage/logs/**",
}

func listProjectRemotes(project string) (RemoteSnapshot, error) {
	root, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return RemoteSnapshot{}, err
	}
	return listProjectRemotesByPath(root)
}

func listProjectRemotesByPath(root string) (RemoteSnapshot, error) {
	cleanRoot := filepath.Clean(strings.TrimSpace(root))
	cfg, _, err := engine.LoadConfigFromDir(cleanRoot, true)
	if err != nil {
		return RemoteSnapshot{}, fmt.Errorf("load config for remotes: %w", err)
	}

	projectName := strings.TrimSpace(cfg.ProjectName)
	if projectName == "" {
		projectName = filepath.Base(cleanRoot)
	}
	lastSyncByEnvironment := buildRemoteLastSyncLabels(projectName, time.Now().UTC())

	return RemoteSnapshot{
		Project:  projectName,
		Remotes:  buildRemoteEntries(cfg.Remotes, lastSyncByEnvironment),
		Warnings: []string{},
	}, nil
}

func testRemote(project string, remoteName string) (string, error) {
	startedAt := time.Now()
	status := engine.OperationStatusFailure
	category := "runtime"
	message := ""
	defer func() {
		writeDesktopOperationEvent(
			"desktop.remote.test",
			status,
			project,
			remoteName,
			"",
			message,
			category,
			time.Since(startedAt),
		)
	}()

	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		category = "validation"
		message = "remote name is required"
		return "", fmt.Errorf("%s", message)
	}

	root, err := resolveProjectRootForRemotes(project)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	snapshot, err := listProjectRemotesByPath(root)
	if err != nil {
		message = err.Error()
		return "", err
	}
	if !hasRemoteName(snapshot.Remotes, remoteName) {
		category = "validation"
		message = fmt.Sprintf("unknown remote: %s", remoteName)
		return "", fmt.Errorf("%s", message)
	}

	output, err := runGovardCommandForDesktop(root, []string{"remote", "test", remoteName})
	if err != nil {
		message = err.Error()
		return "", fmt.Errorf("remote test failed: %w", err)
	}
	if output == "" {
		output = fmt.Sprintf("Remote '%s' test completed.", remoteName)
	}

	status = engine.OperationStatusSuccess
	category = ""
	message = "remote test completed"
	return output, nil
}

func openRemoteURL(project string, remoteName string, ctx context.Context) (string, error) {
	startedAt := time.Now()
	status := engine.OperationStatusFailure
	category := "runtime"
	message := ""
	defer func() {
		writeDesktopOperationEvent(
			"desktop.remote.open_url",
			status,
			project,
			remoteName,
			"",
			message,
			category,
			time.Since(startedAt),
		)
	}()

	targetURL, resolvedRemoteName, err := resolveRemoteAdminURL(project, remoteName)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	result, err := openDestination(
		ctx,
		targetURL,
		fmt.Sprintf("Opening %s...", targetURL),
	)
	if err != nil {
		message = err.Error()
		return "", err
	}

	status = engine.OperationStatusSuccess
	category = ""
	message = fmt.Sprintf("remote URL opened for %s", resolvedRemoteName)
	return result, nil
}

func resolveRemoteAdminURL(project string, remoteName string) (string, string, error) {
	trimmedRemoteName := strings.TrimSpace(remoteName)
	if trimmedRemoteName == "" {
		return "", "", fmt.Errorf("remote name is required")
	}

	root, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return "", "", err
	}

	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		return "", "", fmt.Errorf("load config for remotes: %w", err)
	}

	resolvedRemoteName, remoteCfg, err := resolveRemoteConfigForOpen(cfg, trimmedRemoteName)
	if err != nil {
		return "", "", err
	}

	adminPath := "admin"
	if strings.EqualFold(strings.TrimSpace(cfg.Framework), "magento2") {
		detectedPath, probeErr := detectRemoteMagentoAdminPathForDesktop(resolvedRemoteName, remoteCfg)
		if probeErr == nil {
			adminPath = detectedPath
		}
	}

	return buildRemoteAdminURLForDesktop(remoteCfg, adminPath), resolvedRemoteName, nil
}

func resolveRemoteConfigForOpen(
	cfg engine.Config,
	requestedRemoteName string,
) (string, engine.RemoteConfig, error) {
	trimmedRequested := strings.TrimSpace(requestedRemoteName)
	if trimmedRequested == "" {
		return "", engine.RemoteConfig{}, fmt.Errorf("remote name is required")
	}

	if remoteCfg, ok := cfg.Remotes[trimmedRequested]; ok {
		if !engine.RemoteCapabilityEnabled(remoteCfg, engine.RemoteCapabilityFiles) {
			return "", engine.RemoteConfig{}, fmt.Errorf(
				"remote '%s' does not allow files operations (capabilities: %s)",
				trimmedRequested,
				strings.Join(engine.RemoteCapabilityList(remoteCfg), ","),
			)
		}
		return trimmedRequested, remoteCfg, nil
	}

	normalizedRequested := engine.NormalizeRemoteEnvironment(trimmedRequested)
	for name, remoteCfg := range cfg.Remotes {
		if engine.NormalizeRemoteEnvironment(name) != normalizedRequested {
			continue
		}
		if !engine.RemoteCapabilityEnabled(remoteCfg, engine.RemoteCapabilityFiles) {
			return "", engine.RemoteConfig{}, fmt.Errorf(
				"remote '%s' does not allow files operations (capabilities: %s)",
				name,
				strings.Join(engine.RemoteCapabilityList(remoteCfg), ","),
			)
		}
		return name, remoteCfg, nil
	}

	return "", engine.RemoteConfig{}, fmt.Errorf("unknown remote: %s", trimmedRequested)
}

func detectRemoteMagentoAdminPathForDesktop(
	remoteName string,
	remoteCfg engine.RemoteConfig,
) (string, error) {
	remoteCommand := "php -r " + shellQuoteForDesktop(remoteMagentoAdminProbeScript)
	if path := strings.TrimSpace(remoteCfg.Path); path != "" {
		remoteCommand = "cd " + shellQuoteForDesktop(path) + " && " + remoteCommand
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

func buildRemoteAdminURLForDesktop(remoteCfg engine.RemoteConfig, adminPath string) string {
	base := strings.TrimSpace(remoteCfg.URL)
	if base == "" {
		base = strings.TrimSpace(remoteCfg.Host)
		if base == "" {
			base = "localhost"
		}
		if !strings.HasPrefix(strings.ToLower(base), "http://") &&
			!strings.HasPrefix(strings.ToLower(base), "https://") {
			base = "https://" + base
		}
	}

	base = strings.TrimRight(base, "/")
	trimmedPath := strings.Trim(strings.TrimSpace(adminPath), "/")
	if trimmedPath == "" {
		trimmedPath = "admin"
	}

	return base + "/" + trimmedPath
}

func shellQuoteForDesktop(raw string) string {
	if raw == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(raw, "'", `'"'"'`) + "'"
}

func runRemoteSyncPresetWithOptions(
	project string,
	remoteName string,
	preset string,
	options map[string]bool,
) (string, error) {
	startedAt := time.Now()
	status := engine.OperationStatusFailure
	category := "runtime"
	message := ""
	defer func() {
		writeDesktopOperationEvent(
			"desktop.remote.sync.plan",
			status,
			project,
			remoteName,
			"local",
			message,
			category,
			time.Since(startedAt),
		)
	}()

	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		category = "validation"
		message = "remote name is required"
		return "", fmt.Errorf("%s", message)
	}

	root, err := resolveProjectRootForRemotes(project)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	snapshot, err := listProjectRemotesByPath(root)
	if err != nil {
		message = err.Error()
		return "", err
	}
	if !hasRemoteName(snapshot.Remotes, remoteName) {
		category = "validation"
		message = fmt.Sprintf("unknown remote: %s", remoteName)
		return "", fmt.Errorf("%s", message)
	}

	normalizedPreset, err := normalizeRemoteSyncPreset(preset)
	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	var args []string
	if normalizedPreset == "full" {
		args, err = buildBootstrapArgsWithOptions(remoteName, options, true)
	} else {
		args, err = buildRemoteSyncArgsWithOptions(remoteName, preset, options, true)
	}

	if err != nil {
		category = "validation"
		message = err.Error()
		return "", err
	}

	output, err := runGovardCommandForDesktop(root, args)
	if err != nil {
		message = err.Error()
		return "", fmt.Errorf("sync plan failed: %w", err)
	}
	if output == "" {
		output = "Sync plan generated."
	}

	status = engine.OperationStatusPlan
	category = ""
	message = "sync plan generated"
	return output, nil
}

func runRemoteSyncBackgroundWithOptions(
	ctx context.Context,
	project string,
	remoteName string,
	preset string,
	options map[string]bool,
) error {
	remoteName = strings.TrimSpace(remoteName)
	if remoteName == "" {
		return fmt.Errorf("remote name is required")
	}

	root, err := resolveProjectRootForRemotes(project)
	if err != nil {
		return err
	}

	normalizedPreset, err := normalizeRemoteSyncPreset(preset)
	if err != nil {
		return err
	}

	var args []string
	if normalizedPreset == "full" {
		args, err = buildBootstrapArgsWithOptions(remoteName, options, false)
	} else {
		args, err = buildRemoteSyncArgsWithOptions(remoteName, preset, options, false)
	}

	if err != nil {
		return err
	}

	binary, err := exec.LookPath("govard")
	if err != nil {
		return fmt.Errorf("govard CLI not found in PATH")
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = filepath.Clean(root)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to pipe stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to pipe stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background sync: %w", err)
	}

	done := make(chan struct{}, 2)
	go scanLogPipe(ctx, stdout, "sync:output", done)
	go scanLogPipe(ctx, stderr, "sync:output", done)

	go func() {
		<-done
		<-done
		err := cmd.Wait()
		if err != nil {
			emitEvent(ctx, "sync:failed", fmt.Sprintf("Sync failed: %v", err))
		} else {
			emitEvent(ctx, "sync:completed", "Sync completed successfully")
		}
	}()

	return nil
}

func resolveProjectRootForRemotes(project string) (string, error) {
	trimmedProject := strings.TrimSpace(project)
	if trimmedProject == "" {
		return "", fmt.Errorf("project is required")
	}

	if pathHasBaseConfig(trimmedProject) {
		return filepath.Clean(trimmedProject), nil
	}

	if entries, err := engine.ReadProjectRegistryEntries(); err == nil {
		for _, entry := range entries {
			if !projectRegistryMatches(entry, trimmedProject) {
				continue
			}
			if pathHasBaseConfig(entry.Path) {
				return filepath.Clean(entry.Path), nil
			}
		}
	}

	if info, err := loadProjectInfo(trimmedProject); err == nil {
		if configPath := strings.TrimSpace(info.configPath); configPath != "" {
			root := filepath.Dir(configPath)
			if pathHasBaseConfig(root) {
				return filepath.Clean(root), nil
			}
		}
	}

	return "", fmt.Errorf("unable to resolve project path for '%s'", trimmedProject)
}

func projectRegistryMatches(entry engine.ProjectRegistryEntry, project string) bool {
	if strings.TrimSpace(entry.ProjectName) == project {
		return true
	}
	if strings.TrimSpace(entry.Domain) == project {
		return true
	}
	return filepath.Base(strings.TrimSpace(entry.Path)) == project
}

func pathHasBaseConfig(root string) bool {
	configPath := filepath.Join(filepath.Clean(strings.TrimSpace(root)), engine.BaseConfigFile)
	info, err := os.Stat(configPath)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func buildRemoteEntries(
	remotes map[string]engine.RemoteConfig,
	lastSyncByEnvironment map[string]string,
) []RemoteEntry {
	if len(remotes) == 0 {
		return []RemoteEntry{}
	}

	names := make([]string, 0, len(remotes))
	for name := range remotes {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]RemoteEntry, 0, len(names))
	for _, name := range names {
		cfg := remotes[name]
		environment := engine.NormalizeRemoteEnvironment(name)
		port := cfg.Port
		if port <= 0 {
			port = 22
		}

		capabilities := engine.RemoteCapabilityList(cfg)
		if len(capabilities) == 1 && capabilities[0] == "none" {
			capabilities = []string{}
		}

		effectiveProtected, _ := engine.RemoteWriteBlocked(name, cfg)

		entry := RemoteEntry{
			Name:         strings.TrimSpace(name),
			Host:         strings.TrimSpace(cfg.Host),
			User:         strings.TrimSpace(cfg.User),
			Path:         strings.TrimSpace(cfg.Path),
			Port:         port,
			Environment:  environment,
			LastSync:     strings.TrimSpace(lastSyncByEnvironment[environment]),
			Protected:    effectiveProtected,
			AuthMethod:   engineremote.NormalizeAuthMethod(cfg.Auth.Method),
			Capabilities: append([]string{}, capabilities...),
		}
		if entry.AuthMethod == "" {
			entry.AuthMethod = engineremote.AuthMethodKeychain
		}
		entries = append(entries, entry)
	}
	return entries
}

func buildRemoteLastSyncLabels(project string, now time.Time) map[string]string {
	events, err := engine.ReadOperationEvents(remoteLastSyncReadLimit)
	if err != nil {
		return map[string]string{}
	}
	return buildRemoteLastSyncLabelsFromEvents(project, events, now)
}

func buildRemoteLastSyncLabelsFromEvents(
	project string,
	events []engine.OperationEvent,
	now time.Time,
) map[string]string {
	trimmedProject := strings.TrimSpace(project)
	latestByEnvironment := map[string]time.Time{}

	for _, event := range events {
		if !isRemoteSyncOperation(event.Operation) {
			continue
		}
		if event.Status != engine.OperationStatusSuccess {
			continue
		}

		eventProject := strings.TrimSpace(event.Project)
		if trimmedProject != "" && eventProject != "" && !strings.EqualFold(eventProject, trimmedProject) {
			continue
		}

		source := strings.TrimSpace(event.Source)
		if source == "" || strings.EqualFold(source, "local") {
			continue
		}

		timestamp, ok := parseOperationTimestamp(event.Timestamp)
		if !ok {
			continue
		}

		environment := engine.NormalizeRemoteEnvironment(source)
		current, exists := latestByEnvironment[environment]
		if !exists || timestamp.After(current) {
			latestByEnvironment[environment] = timestamp
		}
	}

	labels := make(map[string]string, len(latestByEnvironment))
	for environment, timestamp := range latestByEnvironment {
		labels[environment] = formatLastSyncLabel(timestamp, now)
	}
	return labels
}

func isRemoteSyncOperation(operation string) bool {
	switch strings.TrimSpace(operation) {
	case "sync.run", "bootstrap.run":
		return true
	default:
		return false
	}
}

func parseOperationTimestamp(raw string) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}

	if parsed, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}

func formatLastSyncLabel(timestamp time.Time, now time.Time) string {
	ts := timestamp.UTC()
	ref := now.UTC()
	if ref.IsZero() {
		ref = time.Now().UTC()
	}
	if ref.Before(ts) {
		return "just now"
	}

	elapsed := ref.Sub(ts)
	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed/time.Minute))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed/time.Hour))
	case elapsed < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(elapsed/(24*time.Hour)))
	case elapsed < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(elapsed/(7*24*time.Hour)))
	default:
		return ts.Format("2006-01-02")
	}
}

func buildRemoteSyncPlanArgs(remoteName string, preset string) ([]string, error) {
	return buildRemoteSyncPlanArgsWithOptions(
		remoteName,
		preset,
		map[string]bool{"compress": true},
	)
}

func buildRemoteSyncPlanArgsWithOptions(
	remoteName string,
	preset string,
	options map[string]bool,
) ([]string, error) {
	return buildRemoteSyncArgsWithOptions(remoteName, preset, options, true)
}

func buildRemoteSyncArgsWithOptions(
	remoteName string,
	preset string,
	options map[string]bool,
	planOnly bool,
) ([]string, error) {
	normalizedPreset, err := normalizeRemoteSyncPreset(preset)
	if err != nil {
		return nil, err
	}

	args := []string{"sync", "--source", strings.TrimSpace(remoteName), "--destination", "local"}
	switch normalizedPreset {
	case "files":
		args = append(args, "--file")
	case "media":
		args = append(args, "--media")
	case "db":
		args = append(args, "--db")
	case "full":
		args = append(args, "--full")
	default:
		return nil, fmt.Errorf("unsupported sync preset '%s'", normalizedPreset)
	}

	if options["sanitize"] {
		for _, pattern := range defaultSyncSanitizeExcludePatterns {
			args = append(args, "--exclude", pattern)
		}
	}

	if options["excludeLogs"] {
		for _, pattern := range defaultSyncLogExcludePatterns {
			args = append(args, "--exclude", pattern)
		}
	}

	// Default to true for compress unless explicitly set to false
	// or wait, it's safer to just check if it's set to true or false.
	// We'll mimic the previous behavior where `options.Compress` defaults to true.
	compress, hasCompress := options["compress"]
	// compression is default in rsync, only append no-compress if false
	if hasCompress && !compress {
		args = append(args, "--no-compress")
	}

	// we don't have a no-stream-db flag on `sync` command yet! The `sync` command uses db internally.
	if options["delete"] {
		args = append(args, "--delete")
	}

	if planOnly {
		args = append(args, "--plan")
	}
	return args, nil
}

type presetSyncOptions struct {
	Preset  string            `json:"preset"`
	Command string            `json:"command"`
	Options []presetOptionDef `json:"options"`
}

type presetOptionDef struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	DefaultValue bool   `json:"defaultValue"`
}

func buildPresetSyncOptionDefs(preset string) presetSyncOptions {
	normalizedPreset, _ := normalizeRemoteSyncPreset(preset)

	switch normalizedPreset {
	case "db":
		return presetSyncOptions{
			Preset:  "db",
			Command: "sync",
			Options: []presetOptionDef{
				{Key: "compress", Label: "Use Compression", Description: "Compress data during transfer", DefaultValue: true},
				{Key: "noStreamDb", Label: "Disable Stream DB", Description: "Do not stream database via pipe, use intermediate files instead", DefaultValue: false},
			},
		}
	case "media":
		return presetSyncOptions{
			Preset:  "media",
			Command: "sync",
			Options: []presetOptionDef{
				{Key: "compress", Label: "Use Compression", Description: "Compress data during transfer", DefaultValue: true},
				{Key: "includeProduct", Label: "Include Product Images", Description: "Include product images in media sync", DefaultValue: false},
				{Key: "delete", Label: "Delete Missing Files", Description: "Delete files on destination that are missing on source", DefaultValue: false},
			},
		}
	case "full":
		return presetSyncOptions{
			Preset:  "full",
			Command: "bootstrap",
			Options: []presetOptionDef{
				{Key: "noDb", Label: "Skip DB Import", Description: "Do not import the database", DefaultValue: false},
				{Key: "noMedia", Label: "Skip Media Sync", Description: "Do not sync media files", DefaultValue: false},
				{Key: "noComposer", Label: "Skip Composer", Description: "Do not run composer install", DefaultValue: false},
				{Key: "noAdmin", Label: "Skip Admin Creation", Description: "Do not create an admin user", DefaultValue: false},
				{Key: "skipUp", Label: "Skip Govard Up", Description: "Do not run govard up before bootstrap", DefaultValue: false},
				{Key: "noStreamDb", Label: "Disable Stream DB", Description: "Do not stream database via pipe, use intermediate files instead", DefaultValue: false},
				{Key: "includeProduct", Label: "Include Product Images", Description: "Include product images in media sync", DefaultValue: false},
				{Key: "assumeYes", Label: "Assume Yes", Description: "Automatically answer yes to all prompts", DefaultValue: false},
			},
		}
	default:
		// Fallback for "files" or unknown presets
		return presetSyncOptions{
			Preset:  normalizedPreset,
			Command: "sync",
			Options: []presetOptionDef{
				{Key: "compress", Label: "Use Compression", Description: "Compress data during transfer", DefaultValue: true},
			},
		}
	}
}

func buildBootstrapArgsWithOptions(remoteName string, options map[string]bool, planOnly bool) ([]string, error) {
	args := []string{"bootstrap", "--environment", strings.TrimSpace(remoteName)}

	if planOnly {
		args = append(args, "--plan")
	}

	if options["noDb"] {
		args = append(args, "--no-db")
	}
	if options["noMedia"] {
		args = append(args, "--no-media")
	}
	if options["noComposer"] {
		args = append(args, "--no-composer")
	}
	if options["noAdmin"] {
		args = append(args, "--no-admin")
	}
	if options["skipUp"] {
		args = append(args, "--skip-up")
	}
	if options["noStreamDb"] {
		args = append(args, "--no-stream-db")
	}
	if options["includeProduct"] {
		args = append(args, "--include-product")
	}
	if options["assumeYes"] {
		args = append(args, "--yes")
	}

	return args, nil
}

func normalizeRemoteSyncPreset(preset string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "", "file", "files", "code", "source":
		return "files", nil
	case "media", "assets":
		return "media", nil
	case "db", "database":
		return "db", nil
	case "full", "all":
		return "full", nil
	default:
		return "", fmt.Errorf("unsupported sync preset '%s'", preset)
	}
}

func writeBaseConfig(root string, config engine.Config) error {
	writableConfig := engine.PrepareConfigForWrite(config)
	data, err := yaml.Marshal(&writableConfig)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, engine.BaseConfigFile), data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", engine.BaseConfigFile, err)
	}
	return nil
}

func hasRemoteName(remotes []RemoteEntry, name string) bool {
	for _, remote := range remotes {
		if remote.Name == name {
			return true
		}
	}
	return false
}

// RemoteService methods

func (s *RemoteService) GetRemotes(project string) (RemoteSnapshot, error) {
	snapshot, err := listProjectRemotes(project)
	if err != nil {
		return RemoteSnapshot{
			Project:  project,
			Remotes:  []RemoteEntry{},
			Warnings: []string{},
		}, err
	}
	return snapshot, nil
}

func (s *RemoteService) TestRemote(project string, remoteName string) (string, error) {
	message, err := testRemote(project, remoteName)
	if err != nil {
		return "", err
	}
	return message, nil
}

func (s *RemoteService) OpenRemoteURL(project string, remoteName string) (string, error) {
	message, err := openRemoteURL(project, remoteName, s.ctx)
	if err != nil {
		return "", err
	}
	return message, nil
}

func (s *RemoteService) GetSyncOptions(preset string) presetSyncOptions {
	return buildPresetSyncOptionDefs(preset)
}

func (s *RemoteService) RunRemoteSyncPreset(project string, remoteName string, preset string, options map[string]bool) (string, error) {
	message, err := runRemoteSyncPresetWithOptions(project, remoteName, preset, options)
	if err != nil {
		return "", err
	}
	return message, nil
}

func (s *RemoteService) RunRemoteSync(project string, remoteName string, preset string, options map[string]bool) (string, error) {
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	err := runRemoteSyncBackgroundWithOptions(ctx, project, remoteName, preset, options)
	if err != nil {
		return "", err
	}
	return "Sync started", nil
}

func writeDesktopOperationEvent(
	operation string,
	status engine.OperationStatus,
	project string,
	source string,
	destination string,
	message string,
	category string,
	duration time.Duration,
) {
	_ = engine.WriteOperationEvent(engine.OperationEvent{
		Operation:   operation,
		Status:      status,
		Project:     strings.TrimSpace(project),
		Source:      strings.TrimSpace(source),
		Destination: strings.TrimSpace(destination),
		Message:     strings.TrimSpace(message),
		Category:    strings.TrimSpace(category),
		DurationMS:  duration.Milliseconds(),
	})
}
