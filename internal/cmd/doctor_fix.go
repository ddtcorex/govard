package cmd

import (
	"fmt"
	"govard/internal/conventions"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/engine"
	"govard/internal/frameworks"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"
)

const (
	DoctorFixStatusApplied     = "applied"
	DoctorFixStatusFailed      = "failed"
	DoctorFixStatusSkipped     = "skipped"
	DoctorFixStatusUnavailable = "unavailable"
)

// DoctorFixResult captures the outcome of a single doctor --fix attempt.
type DoctorFixResult struct {
	CheckID string   `json:"check_id"`
	Title   string   `json:"title"`
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Actions []string `json:"actions,omitempty"`
}

var doctorDependencies = engine.DoctorDependencies{}

var doctorRunDiagnostics = engine.RunDoctorDiagnostics

type doctorFixHandler func(check engine.DoctorCheck) DoctorFixResult

var doctorFixHandlers = map[string]doctorFixHandler{
	"host.govard.home":        fixGovardHomeDirectory,
	"host.search.index_block": unblockSearchIndex,
	"host.compose.spam":       purgeStaleComposeFiles,
	"host.govard.registry":    fixGovardRegistry,
	"project.profile.sync":    tuneProjectProfile,
	"project.runtime.images":  pullRuntimeImages,
	"project.config.audit":    fixLegacyConfig,
	"project.config.drift":    fixConfigDrift,
}

func runDoctorDiagnostics() engine.DoctorReport {
	return doctorRunDiagnostics(doctorDependencies)
}

func applyDoctorSafeFixes(report engine.DoctorReport, skippedCheckIDs map[string]bool) []DoctorFixResult {
	results := make([]DoctorFixResult, 0)
	seen := map[string]bool{}

	for _, check := range report.Checks {
		if check.Status == engine.DoctorStatusPass {
			continue
		}

		checkID := strings.TrimSpace(check.ID)
		if checkID == "" || seen[checkID] || skippedCheckIDs[checkID] {
			continue
		}
		seen[checkID] = true

		handler, ok := doctorFixHandlers[checkID]
		if !ok {
			results = append(results, DoctorFixResult{
				CheckID: checkID,
				Title:   strings.TrimSpace(check.Title),
				Status:  DoctorFixStatusUnavailable,
				Message: "No safe automatic fix available.",
			})
			continue
		}

		res := handler(check)
		if res.Status == DoctorFixStatusSkipped {
			skippedCheckIDs[checkID] = true
		}
		results = append(results, res)
	}

	return results
}

func fixGovardHomeDirectory(check engine.DoctorCheck) DoctorFixResult {
	result := DoctorFixResult{
		CheckID: strings.TrimSpace(check.ID),
		Title:   strings.TrimSpace(check.Title),
		Status:  DoctorFixStatusApplied,
		Message: "Govard runtime directories prepared.",
		Actions: []string{},
	}

	homeDir := filepath.Clean(engine.GovardHomeDir())
	dirs := []string{
		homeDir,
		filepath.Join(homeDir, "compose"),
		filepath.Join(homeDir, "diagnostics"),
	}

	for _, dir := range dirs {
		result.Actions = append(result.Actions, fmt.Sprintf("mkdir -p %s", dir))
		if err := os.MkdirAll(dir, conventions.SecretDirPerm); err != nil {
			result.Status = DoctorFixStatusSkipped
			result.Message = err.Error()
			return result
		}
		result.Actions = append(result.Actions, fmt.Sprintf("chmod 700 %s", dir))
		if err := os.Chmod(dir, conventions.SecretDirPerm); err != nil {
			result.Status = DoctorFixStatusSkipped
			result.Message = err.Error()
			return result
		}
	}

	result.Actions = append(result.Actions, fmt.Sprintf("write probe file in %s", homeDir))
	if err := engine.CheckGovardHomeWritable(); err != nil {
		result.Status = DoctorFixStatusSkipped
		result.Message = err.Error()
		return result
	}

	return result
}

func unblockSearchIndex(check engine.DoctorCheck) DoctorFixResult {
	result := DoctorFixResult{
		CheckID: strings.TrimSpace(check.ID),
		Title:   strings.TrimSpace(check.Title),
		Status:  DoctorFixStatusApplied,
		Message: "Elasticsearch/OpenSearch index unblocked.",
		Actions: []string{},
	}

	config := loadConfig()
	definition, ok := frameworks.Get(config.Framework)
	if !ok || definition.UnblockSearchIndex == nil {
		result.Status = DoctorFixStatusUnavailable
		result.Message = fmt.Sprintf("%s does not support this search-index fix.", config.Framework)
		return result
	}
	result.Actions = append(result.Actions, "unblock search index via docker exec curl")
	if err := definition.UnblockSearchIndex(config); err != nil {
		result.Status = DoctorFixStatusSkipped
		result.Message = err.Error()
		return result
	}

	return result
}

func purgeStaleComposeFiles(check engine.DoctorCheck) DoctorFixResult {
	result := DoctorFixResult{
		CheckID: strings.TrimSpace(check.ID),
		Title:   strings.TrimSpace(check.Title),
		Status:  DoctorFixStatusApplied,
		Message: "Stale govard compose files purged.",
		Actions: []string{},
	}

	result.Actions = append(result.Actions, "Purging compose files older than 7 days")
	count, err := engine.CleanupStaleComposeFiles(7 * 24 * time.Hour)
	if err != nil {
		result.Status = DoctorFixStatusSkipped
		result.Message = err.Error()
		return result
	}

	result.Message = fmt.Sprintf("Removed %d stale compose file(s).", count)
	return result
}

func fixGovardRegistry(check engine.DoctorCheck) DoctorFixResult {
	result := DoctorFixResult{
		CheckID: strings.TrimSpace(check.ID),
		Title:   strings.TrimSpace(check.Title),
		Status:  DoctorFixStatusApplied,
		Message: "Project registry path restored.",
		Actions: []string{},
	}

	path := engine.ProjectRegistryPath()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return result
		}
		result.Status = DoctorFixStatusSkipped
		result.Message = err.Error()
		return result
	}

	if !info.IsDir() {
		return result
	}

	result.Actions = append(result.Actions, fmt.Sprintf("rm -rf %s", path))
	if err := os.RemoveAll(path); err != nil {
		result.Status = DoctorFixStatusSkipped
		result.Message = err.Error()
		return result
	}

	return result
}

func tuneProjectProfile(check engine.DoctorCheck) DoctorFixResult {
	return tuneProjectProfileCore(check, false, false)
}

func tuneProjectProfileCore(check engine.DoctorCheck, forceTune bool, forceOverride bool) DoctorFixResult {
	result := DoctorFixResult{
		CheckID: strings.TrimSpace(check.ID),
		Title:   strings.TrimSpace(check.Title),
		Status:  DoctorFixStatusApplied,
		Message: "Environment profile synchronized.",
		Actions: []string{},
	}

	if !forceTune && !confirmAction("Do you want to automatically tune the framework runtime profile now?") {
		result.Status = DoctorFixStatusSkipped
		result.Message = "Skipped by user."
		return result
	}

	result.Actions = append(result.Actions, "Tune environment services to match framework profile")
	wd, _ := os.Getwd()
	config, _, err := engine.LoadConfigFromDir(wd, false)
	if err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = err.Error()
		return result
	}

	// Read original bytes for no-op detection
	originalBytes, _ := os.ReadFile(conventions.BaseConfigFile)

	metadata := engine.DetectFramework(wd)
	version := strings.TrimSpace(metadata.Version)
	if version == "" {
		version = strings.TrimSpace(config.FrameworkVersion)
	}

	profileResult, err := engine.ResolveRuntimeProfile(config.Framework, version)
	if err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = err.Error()
		return result
	}

	existingPHPVersion := strings.TrimSpace(config.Stack.PHPVersion)
	existingNodeVersion := strings.TrimSpace(config.Stack.NodeVersion)
	existingComposerVersion := strings.TrimSpace(config.Stack.ComposerVersion)
	existingNginxVersion := strings.TrimSpace(config.Stack.NginxVersion)
	existingApacheVersion := strings.TrimSpace(config.Stack.ApacheVersion)
	existingDB := strings.TrimSpace(config.Stack.Services.DB)
	existingDBVersion := strings.TrimSpace(config.Stack.DBVersion)
	existingCacheService := strings.TrimSpace(config.Stack.Services.Cache)
	existingCacheVersion := strings.TrimSpace(config.Stack.CacheVersion)
	existingSearchService := strings.TrimSpace(config.Stack.Services.Search)
	existingSearchVersion := strings.TrimSpace(config.Stack.SearchVersion)
	existingQueueService := strings.TrimSpace(config.Stack.Services.Queue)
	existingQueueVersion := strings.TrimSpace(config.Stack.QueueVersion)
	existingVarnishVersion := strings.TrimSpace(config.Stack.VarnishVersion)
	existingWebServer := strings.TrimSpace(config.Stack.Services.WebServer)

	engine.ApplyRuntimeProfileToConfig(&config, profileResult.Profile)

	type manualOverride struct {
		Field    string
		UserVal  string
		Standard string
	}
	overrides := []manualOverride{}

	if existingPHPVersion != "" && existingPHPVersion != profileResult.Profile.PHPVersion {
		overrides = append(overrides, manualOverride{"PHP version", existingPHPVersion, profileResult.Profile.PHPVersion})
	}
	if existingNodeVersion != "" && existingNodeVersion != profileResult.Profile.NodeVersion {
		overrides = append(overrides, manualOverride{"Node version", existingNodeVersion, profileResult.Profile.NodeVersion})
	}
	if existingDBVersion != "" && existingDBVersion != profileResult.Profile.DBVersion {
		overrides = append(overrides, manualOverride{"DB version", existingDBVersion, profileResult.Profile.DBVersion})
	}
	if existingDB != "" && existingDB != profileResult.Profile.DB {
		overrides = append(overrides, manualOverride{"DB", existingDB, profileResult.Profile.DB})
	}
	if existingSearchService != "" && existingSearchService != profileResult.Profile.Search {
		overrides = append(overrides, manualOverride{"Search service", existingSearchService, profileResult.Profile.Search})
	}
	if existingCacheService != "" && existingCacheService != profileResult.Profile.Cache {
		overrides = append(overrides, manualOverride{"Cache service", existingCacheService, profileResult.Profile.Cache})
	}
	if existingQueueService != "" && existingQueueService != profileResult.Profile.Queue {
		overrides = append(overrides, manualOverride{"Queue service", existingQueueService, profileResult.Profile.Queue})
	}
	if existingVarnishVersion != "" && existingVarnishVersion != profileResult.Profile.VarnishVersion {
		overrides = append(overrides, manualOverride{"Varnish version", existingVarnishVersion, profileResult.Profile.VarnishVersion})
	}
	if existingWebServer != "" && existingWebServer != profileResult.Profile.WebServer {
		overrides = append(overrides, manualOverride{"Web server", existingWebServer, profileResult.Profile.WebServer})
	}

	canOverrideExplicts := forceOverride
	if !forceOverride && len(overrides) > 0 {
		pterm.Info.Println("Detected manual overrides that deviate from the framework profile:")
		for _, o := range overrides {
			pterm.Println(fmt.Sprintf("  - %s: %s (Standard: %s)", o.Field, o.UserVal, o.Standard))
		}
		if confirmAction("Would you like to reset these manual settings to standard profile defaults?") {
			canOverrideExplicts = true
			result.Actions = append(result.Actions, "Overriding manual version settings with profile standards")
		}
	}

	if existingPHPVersion != "" && !canOverrideExplicts {
		config.Stack.PHPVersion = existingPHPVersion
	} else if existingPHPVersion == "" && !canOverrideExplicts {
		config.Stack.PHPVersion = ""
	}

	if existingNodeVersion != "" && !canOverrideExplicts {
		config.Stack.NodeVersion = existingNodeVersion
	} else if existingNodeVersion == "" && !canOverrideExplicts {
		config.Stack.NodeVersion = ""
	}

	if existingComposerVersion != "" {
		config.Stack.ComposerVersion = existingComposerVersion
	}

	activeWebServer := strings.ToLower(strings.TrimSpace(config.Stack.Services.WebServer))

	if existingDBVersion != "" && !canOverrideExplicts {
		config.Stack.DBVersion = existingDBVersion
	} else if existingDBVersion == "" && !canOverrideExplicts {
		config.Stack.DBVersion = ""
	}

	engine.NormalizeConfig(&config, wd)

	// FINAL MINIMALIST FILTER:
	// We run this AFTER NormalizeConfig to ensure we strictly preserve 'none' state
	// even if the normalization or profile auto-injection tried to re-enable them.
	// If a user consciously chose 'none', we don't force a service on them.
	if existingCacheService == "none" || existingCacheService == "" {
		config.Stack.Services.Cache = "none"
		config.Stack.CacheVersion = ""
		config.Stack.Features.Cache = false
	} else if !canOverrideExplicts {
		config.Stack.Services.Cache = existingCacheService
		if existingCacheVersion == "" {
			config.Stack.CacheVersion = ""
		} else {
			config.Stack.CacheVersion = existingCacheVersion
		}
	}

	if existingSearchService == "none" || existingSearchService == "" {
		config.Stack.Services.Search = "none"
		config.Stack.SearchVersion = ""
		config.Stack.Features.Search = false
	} else if !canOverrideExplicts {
		config.Stack.Services.Search = existingSearchService
		if existingSearchVersion == "" {
			config.Stack.SearchVersion = ""
		} else {
			config.Stack.SearchVersion = existingSearchVersion
		}
	}

	if existingQueueService == "none" || existingQueueService == "" {
		config.Stack.Services.Queue = "none"
		config.Stack.QueueVersion = ""
		config.Stack.Features.Queue = false
	} else if !canOverrideExplicts {
		config.Stack.Services.Queue = existingQueueService
		if existingQueueVersion == "" {
			config.Stack.QueueVersion = ""
		} else {
			config.Stack.QueueVersion = existingQueueVersion
		}
	}

	if existingVarnishVersion == "" || existingVarnishVersion == "none" {
		config.Stack.VarnishVersion = ""
		config.Stack.Features.Varnish = false
	} else if !canOverrideExplicts {
		config.Stack.VarnishVersion = existingVarnishVersion
	}

	if !canOverrideExplicts {
		if activeWebServer == "nginx" || activeWebServer == "hybrid" {
			if existingNginxVersion == "" {
				config.Stack.NginxVersion = ""
			} else {
				config.Stack.NginxVersion = existingNginxVersion
			}
		}
		if activeWebServer == "apache" || activeWebServer == "hybrid" {
			if existingApacheVersion == "" {
				config.Stack.ApacheVersion = ""
			} else {
				config.Stack.ApacheVersion = existingApacheVersion
			}
		}
	}

	if existingWebServer != "" && existingWebServer != "none" && !canOverrideExplicts {
		config.Stack.Services.WebServer = existingWebServer
	}

	writableConfig := engine.PrepareConfigForWrite(config)
	updated, err := yaml.Marshal(&writableConfig)
	if err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = fmt.Sprintf("failed to marshal config: %v", err)
		return result
	}

	// If the config hasn't actually changed (because all framework recommendations were overridden
	// by explicit user settings), skip the update and return "Skipped" to break the infinite fix loop.
	if string(updated) == string(originalBytes) {
		result.Status = DoctorFixStatusSkipped
		result.Message = "Advisory: Explicit user settings preserved (no safe profile changes to apply)."
		return result
	}

	if doctorDryRun {
		result.Message = fmt.Sprintf("[dry-run] would update %s profile (%s)", config.Framework, profileResult.Source)
		result.Actions = append(result.Actions, "dry-run: no files written")
		return result
	}

	if err := os.WriteFile(conventions.BaseConfigFile, updated, conventions.DefaultFilePerm); err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = fmt.Sprintf("failed to write updated config: %v", err)
		return result
	}

	result.Message = fmt.Sprintf("Environment updated to %s profile (%s)", config.Framework, profileResult.Source)
	result.Status = DoctorFixStatusApplied
	return result
}

func pullRuntimeImages(check engine.DoctorCheck) DoctorFixResult {
	result := DoctorFixResult{
		CheckID: strings.TrimSpace(check.ID),
		Title:   strings.TrimSpace(check.Title),
		Status:  DoctorFixStatusApplied,
		Message: "Missing runtime images pulled.",
		Actions: []string{},
	}

	if !confirmAction("Do you want to pull missing Docker images now? This may take several minutes.") {
		result.Status = DoctorFixStatusSkipped
		result.Message = "Skipped by user."
		return result
	}

	result.Actions = append(result.Actions, "Inspecting required runtime images")

	wd, err := os.Getwd()
	if err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = fmt.Sprintf("failed to read working directory: %v", err)
		return result
	}

	config, _, err := engine.LoadConfigFromDir(wd, true)
	if err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = fmt.Sprintf("could not load configuration: %v", err)
		return result
	}

	required := engine.RequiredRuntimeImages(config, wd)
	var missing []string
	for _, image := range required {
		if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
			missing = append(missing, image)
		}
	}

	composePath := engine.ComposeFilePathWithProfile(wd, config.ProjectName, config.Profile)
	rebuildIncompatible := func() bool {
		if _, statErr := os.Stat(composePath); statErr != nil {
			return false
		}
		built, rebuildErr := engine.RebuildIncompatibleGovardImagesFromCompose(composePath, os.Stdout, os.Stderr)
		if rebuildErr != nil {
			result.Status = DoctorFixStatusFailed
			result.Message = fmt.Sprintf("failed to rebuild incompatible local images: %v", rebuildErr)
			return true
		}
		if len(built) > 0 {
			result.Actions = append(result.Actions, fmt.Sprintf("Rebuilt incompatible local images: %s", strings.Join(built, ", ")))
		}
		return false
	}

	if len(missing) == 0 {
		if failed := rebuildIncompatible(); failed {
			return result
		}
		if len(result.Actions) > 1 {
			result.Message = "Required images are present; incompatible local images were rebuilt."
		} else {
			result.Message = "All required images are already present."
		}
		return result
	}

	for _, image := range missing {
		pterm.Info.Printf("Pulling %s...\n", image)
		cmd := exec.Command("docker", "pull", image)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			pterm.Warning.Printf("Failed to pull %s. Attempting local build fallback...\n", image)
			built, fallbackErr := engine.FallbackBuildMissingGovardImagesFromCompose(composePath, os.Stdout, os.Stderr)
			if fallbackErr != nil {
				result.Status = DoctorFixStatusFailed
				result.Message = fmt.Sprintf("failed to pull image %s and local fallback failed: %v", image, fallbackErr)
				return result
			}

			// Verify if the build resolved it
			if errCheck := exec.Command("docker", "image", "inspect", image).Run(); errCheck != nil {
				result.Status = DoctorFixStatusFailed
				result.Message = fmt.Sprintf("failed to pull image %s and it was not resolved by local fallback", image)
				return result
			}

			result.Actions = append(result.Actions, fmt.Sprintf("Built %s locally (fallback %d images)", image, len(built)))
		} else {
			result.Actions = append(result.Actions, fmt.Sprintf("Pulled %s", image))
		}
	}
	if failed := rebuildIncompatible(); failed {
		return result
	}

	return result
}

func fixLegacyConfig(check engine.DoctorCheck) DoctorFixResult {
	result := DoctorFixResult{
		CheckID: strings.TrimSpace(check.ID),
		Title:   strings.TrimSpace(check.Title),
		Status:  DoctorFixStatusApplied,
		Message: "Legacy configuration migrated to new standard.",
		Actions: []string{},
	}

	data, err := os.ReadFile(conventions.BaseConfigFile)
	if err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = fmt.Sprintf("failed to read config: %v", err)
		return result
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = fmt.Sprintf("failed to parse yaml: %v", err)
		return result
	}

	stackRaw, ok := raw["stack"].(map[string]interface{})
	if !ok {
		result.Status = DoctorFixStatusUnavailable
		result.Message = "No stack configuration found to migrate."
		return result
	}

	// Ensure services block exists
	servicesRaw, ok := stackRaw["services"].(map[string]interface{})
	if !ok {
		servicesRaw = make(map[string]interface{})
		stackRaw["services"] = servicesRaw
	}

	// 1. Move stack.db to services.db
	if dbType, ok := stackRaw["db"]; ok {
		if servicesRaw["db"] == nil || servicesRaw["db"] == "" {
			servicesRaw["db"] = dbType
			result.Actions = append(result.Actions, fmt.Sprintf("Moved db '%v' to services.db", dbType))
		}
		delete(stackRaw, "db")
	}

	// 2. Handle legacy features (redis, elasticsearch, rabbitmq)
	if featuresRaw, ok := stackRaw["features"].(map[string]interface{}); ok {
		// Map redis -> services.cache
		if val, exists := featuresRaw["redis"]; exists {
			if b, ok := val.(bool); ok && b {
				if servicesRaw["cache"] == nil || servicesRaw["cache"] == "" {
					servicesRaw["cache"] = "redis"
					result.Actions = append(result.Actions, "Migrated feature 'redis' to services.cache='redis'")
				}
			}
			delete(featuresRaw, "redis")
		}

		// Map elasticsearch -> services.search
		if val, exists := featuresRaw["elasticsearch"]; exists {
			if b, ok := val.(bool); ok && b {
				if servicesRaw["search"] == nil || servicesRaw["search"] == "" {
					servicesRaw["search"] = "elasticsearch"
					result.Actions = append(result.Actions, "Migrated feature 'elasticsearch' to services.search='elasticsearch'")
				}
			}
			delete(featuresRaw, "elasticsearch")
		}

		// Map rabbitmq -> services.queue
		if val, exists := featuresRaw["rabbitmq"]; exists {
			if b, ok := val.(bool); ok && b {
				if servicesRaw["queue"] == nil || servicesRaw["queue"] == "" {
					servicesRaw["queue"] = "rabbitmq"
					result.Actions = append(result.Actions, "Migrated feature 'rabbitmq' to services.queue='rabbitmq'")
				}
			}
			delete(featuresRaw, "rabbitmq")
		}

		// If features block is now empty (or only contains ignored internal keys), we'll let PrepareConfigForWrite handle it
	}

	// Re-marshal via Config struct to ensure standard field ordering is preserved
	var cfg engine.Config
	tempData, _ := yaml.Marshal(&raw)
	_ = yaml.Unmarshal(tempData, &cfg)

	// Clean up empty services/features if they match defaults via PrepareConfigForWrite
	writableConfig := engine.PrepareConfigForWrite(cfg)
	updated, err := yaml.Marshal(&writableConfig)
	if err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = fmt.Sprintf("failed to marshal updated config: %v", err)
		return result
	}

	if doctorDryRun {
		result.Message = "[dry-run] would migrate legacy config"
		result.Actions = append(result.Actions, "dry-run: no files written")
		return result
	}

	if err := os.WriteFile(conventions.BaseConfigFile, updated, conventions.DefaultFilePerm); err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = fmt.Sprintf("failed to write updated config: %v", err)
		return result
	}

	return result
}

func fixConfigDrift(check engine.DoctorCheck) DoctorFixResult {
	result := DoctorFixResult{
		CheckID: strings.TrimSpace(check.ID),
		Title:   strings.TrimSpace(check.Title),
		Status:  DoctorFixStatusApplied,
		Message: "Configuration drift synchronized.",
		Actions: []string{},
	}
	wd, _ := os.Getwd()
	changes, err := engine.SyncConfigDriftForTest(wd, doctorDryRun, doctorCommit)
	if err != nil {
		result.Status = DoctorFixStatusFailed
		result.Message = err.Error()
		return result
	}
	if len(changes) == 0 {
		result.Status = DoctorFixStatusSkipped
		result.Message = "No drift detected."
		return result
	}
	for _, c := range changes {
		result.Actions = append(result.Actions, fmt.Sprintf("yq update .govard.yml %s", c))
	}
	if doctorDryRun {
		result.Message = fmt.Sprintf("[dry-run] would sync: %s", strings.Join(changes, ", "))
		result.Actions = append(result.Actions, "dry-run: no files written")
		return result
	}
	if doctorCommit {
		result.Actions = append(result.Actions, fmt.Sprintf("git add %s && git commit", conventions.BaseConfigFile))
	}
	result.Message = fmt.Sprintf("Synced %s", strings.Join(changes, ", "))
	return result
}

func renderDoctorFixResults(results []DoctorFixResult) {
	if len(results) == 0 {
		pterm.Info.Println("No safe fixes were available.")
		return
	}

	applied := 0
	failed := 0
	unavailable := 0

	for _, result := range results {
		line := fmt.Sprintf("%s (%s): %s", result.Title, result.CheckID, result.Message)
		switch result.Status {
		case DoctorFixStatusApplied:
			applied++
			pterm.Success.Println(line)
		case DoctorFixStatusFailed:
			failed++
			pterm.Error.Println(line)
		case DoctorFixStatusSkipped:
			pterm.Info.Println(line)
		default:
			unavailable++
			pterm.Warning.Println(line)
		}

		for _, action := range result.Actions {
			pterm.Info.Printf("Action: %s\n", action)
		}
	}

	pterm.Info.Printf(
		"Fix summary: applied=%d failed=%d unavailable=%d\n",
		applied,
		failed,
		unavailable,
	)
}

func summarizeDoctorFixResults(results []DoctorFixResult) []string {
	if len(results) == 0 {
		return []string{"Doctor --fix: no safe fixes were available."}
	}

	lines := make([]string, 0, len(results)+1)
	applied := 0
	failed := 0
	unavailable := 0

	for _, result := range results {
		switch result.Status {
		case DoctorFixStatusApplied:
			applied++
		case DoctorFixStatusFailed:
			failed++
		case DoctorFixStatusSkipped:
			// No count increment for skips
		default:
			unavailable++
		}
		prefix := "Doctor --fix"
		if result.Status == DoctorFixStatusSkipped {
			prefix = "Doctor --skip"
		}
		lines = append(lines, fmt.Sprintf("%s: %s (%s): %s", prefix, result.Title, result.CheckID, result.Message))
		for _, action := range result.Actions {
			lines = append(lines, fmt.Sprintf("Doctor --fix action: %s", action))
		}
	}

	lines = append(lines, fmt.Sprintf("Doctor --fix summary: applied=%d failed=%d unavailable=%d", applied, failed, unavailable))
	return lines
}

// SetDoctorDependenciesForTest overrides doctor diagnostics dependencies for tests.
func SetDoctorDependenciesForTest(dependencies engine.DoctorDependencies) func() {
	previous := doctorDependencies
	doctorDependencies = dependencies
	return func() {
		doctorDependencies = previous
	}
}

// ApplyDoctorSafeFixesForTest exposes doctor fix planning/execution for tests.
func ApplyDoctorSafeFixesForTest(report engine.DoctorReport, skippedCheckIDs map[string]bool) []DoctorFixResult {
	if skippedCheckIDs == nil {
		skippedCheckIDs = make(map[string]bool)
	}
	return applyDoctorSafeFixes(report, skippedCheckIDs)
}

func confirmAction(message string) bool {
	if os.Getenv("GOVARD_ASSUME_YES") == "true" {
		return true
	}
	confirmed, _ := pterm.DefaultInteractiveConfirm.
		WithDefaultValue(true).
		Show(message)
	return confirmed
}
