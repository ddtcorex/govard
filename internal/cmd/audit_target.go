package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"govard/internal/audit"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

type runtimePHPProbe func(context.Context, types.AuditTarget, engine.Config) (version string, running bool, err error)

type resolvedAuditTarget struct {
	Definition     types.FrameworkDefinition
	Target         types.AuditTarget
	Config         *engine.Config
	ProjectID      string
	Source         audit.SourceFingerprint
	PHPVersions    []string
	MatrixComplete bool
}

var phpMajorMinorPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

func resolveAuditTarget(ctx context.Context, start string, mode types.AuditTargetMode, requestedPHPVersions []string, probe runtimePHPProbe, resolvePHPPolicy bool) (resolvedAuditTarget, error) {
	definition, target, err := frameworks.ResolveAuditTarget(types.AuditTargetResolveRequest{StartPath: start, ModeOverride: mode})
	if err != nil {
		return resolvedAuditTarget{}, err
	}
	if definition.AuditLint == nil {
		return resolvedAuditTarget{}, fmt.Errorf("framework %q does not support lint audit (use govard tool php vendor/bin/phpcs, vendor/bin/phpstan or vendor/bin/pint directly)", definition.Name)
	}

	root := target.TargetPath
	var config *engine.Config
	if target.Mode != types.AuditTargetStandalone {
		resolvedProfile := engine.ResolveEffectiveProfile(target.ProjectRoot, "")
		loaded, _, loadErr := engine.LoadConfigFromDirWithProfile(target.ProjectRoot, true, resolvedProfile)
		if loadErr != nil {
			return resolvedAuditTarget{}, fmt.Errorf("load audit project config: %w", loadErr)
		}
		if !frameworks.IsA(definition.Name, loaded.Framework) {
			return resolvedAuditTarget{}, fmt.Errorf("audit target framework %q is incompatible with configured framework %q", definition.Name, loaded.Framework)
		}
		config = &loaded
		root = target.ProjectRoot
		if resolvePHPPolicy {
			if probe == nil {
				probe = probeRuntimePHP
			}
			runtimeVersion, running, probeErr := probe(ctx, target, loaded)
			if probeErr != nil {
				return resolvedAuditTarget{}, fmt.Errorf("audit PHP runtime probe: %w", probeErr)
			}
			if running && runtimeVersion != loaded.Stack.PHPVersion {
				return resolvedAuditTarget{}, fmt.Errorf("infrastructure: configured PHP %q differs from running PHP %q", sanitizePHPVersion(loaded.Stack.PHPVersion), sanitizePHPVersion(runtimeVersion))
			}
		}
	}

	var versions []string
	matrixComplete := false
	if resolvePHPPolicy {
		versions, matrixComplete, err = resolveAuditPHPVersions(target, definition.AuditLint, config, requestedPHPVersions)
		if err != nil {
			return resolvedAuditTarget{}, err
		}
	}
	source, repositoryIdentity := auditSourceFingerprint(root)
	return resolvedAuditTarget{
		Definition:     definition,
		Target:         target,
		Config:         config,
		ProjectID:      audit.ProjectID(root, repositoryIdentity),
		Source:         source,
		PHPVersions:    versions,
		MatrixComplete: matrixComplete,
	}, nil
}

func resolveAuditPHPVersions(target types.AuditTarget, profile *types.AuditLintProfile, config *engine.Config, requested []string) ([]string, bool, error) {
	if profile == nil {
		return nil, false, fmt.Errorf("framework %q does not support lint audit (use govard tool php vendor/bin/phpcs, vendor/bin/phpstan or vendor/bin/pint directly)", target.Framework)
	}
	if target.Mode != types.AuditTargetStandalone {
		if config == nil {
			return nil, false, fmt.Errorf("audit target %q has no project configuration", target.TargetPath)
		}
		active := strings.TrimSpace(config.Stack.PHPVersion)
		if !containsAuditPHPVersion(profile.ProjectPHPVersions, active) {
			return nil, false, fmt.Errorf("unsupported_php: project PHP %q is not supported", sanitizePHPVersion(active))
		}
		for _, version := range requested {
			if strings.TrimSpace(version) != active {
				return nil, false, fmt.Errorf("audit --php %q must match active project PHP %q", sanitizePHPVersion(version), sanitizePHPVersion(active))
			}
		}
		if len(requested) > 1 {
			return nil, false, fmt.Errorf("audit --php accepts only the active project PHP %q", sanitizePHPVersion(active))
		}
		return []string{active}, true, nil
	}

	versions := normalizeAuditPHPVersions(requested)
	if len(versions) == 0 {
		versions = append([]string(nil), profile.StandalonePHPVersions...)
	}
	seen := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		version = strings.TrimSpace(version)
		if !containsAuditPHPVersion(profile.StandalonePHPVersions, version) {
			return nil, false, fmt.Errorf("unsupported_php: standalone PHP %q is not supported", sanitizePHPVersion(version))
		}
		if _, ok := seen[version]; ok {
			return nil, false, fmt.Errorf("audit --php repeats PHP %q", sanitizePHPVersion(version))
		}
		seen[version] = struct{}{}
	}
	return versions, sameAuditPHPSet(versions, profile.StandalonePHPVersions), nil
}

func normalizeAuditPHPVersions(versions []string) []string {
	normalized := make([]string, len(versions))
	for index, version := range versions {
		normalized[index] = strings.TrimSpace(version)
	}
	return normalized
}

func probeRuntimePHP(ctx context.Context, _ types.AuditTarget, config engine.Config) (string, bool, error) {
	service := engine.ResolveFrameworkAppService(config.Framework)
	container := fmt.Sprintf("%s-%s-1", config.ProjectName, service)
	inspect := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", container)
	output, err := inspect.CombinedOutput()
	state := strings.ToLower(strings.TrimSpace(string(output)))
	if err != nil {
		if strings.Contains(strings.ToLower(string(output)), "no such object") {
			return "", false, nil
		}
		return "", false, fmt.Errorf("inspect application container: %w", err)
	}
	if state == "false" {
		return "", false, nil
	}
	if state != "true" {
		return "", false, fmt.Errorf("inspect application container returned %q", sanitizePHPVersion(state))
	}
	command := exec.CommandContext(ctx, "docker", "exec", container, "php", "-r", `echo PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION;`)
	runtime, err := command.Output()
	if err != nil {
		return "", true, fmt.Errorf("read running PHP version: %w", err)
	}
	version := strings.TrimSpace(string(runtime))
	if !phpMajorMinorPattern.MatchString(version) {
		return "", true, fmt.Errorf("read invalid running PHP version %q", sanitizePHPVersion(version))
	}
	return version, true, nil
}

func containsAuditPHPVersion(versions []string, want string) bool {
	for _, version := range versions {
		if version == want {
			return true
		}
	}
	return false
}

func sameAuditPHPSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	left = append([]string(nil), left...)
	right = append([]string(nil), right...)
	sort.Strings(left)
	sort.Strings(right)
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sanitizePHPVersion(version string) string {
	version = strings.TrimSpace(version)
	if phpMajorMinorPattern.MatchString(version) {
		return version
	}
	return "invalid"
}

func auditSourceFingerprint(root string) (audit.SourceFingerprint, string) {
	digest := auditManifestDigest(root)
	origin, _ := gitOutput(root, "config", "--get", "remote.origin.url")
	commit, commitErr := gitOutput(root, "rev-parse", "HEAD")
	if commitErr != nil {
		return audit.SourceFingerprint{Digest: digest}, auditRepositoryIdentity(root, origin)
	}
	dirty := false
	if command := exec.Command("git", "-C", root, "diff", "--quiet"); command.Run() != nil {
		dirty = true
	}
	return audit.SourceFingerprint{GitCommit: commit, GitDirty: dirty, Digest: digest}, auditRepositoryIdentity(root, origin)
}

func auditRepositoryIdentity(root, origin string) string {
	if strings.TrimSpace(origin) != "" {
		return strings.TrimSpace(origin)
	}
	contents, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return ""
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return ""
	}
	return strings.TrimSpace(manifest.Name)
}
