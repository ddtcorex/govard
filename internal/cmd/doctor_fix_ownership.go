package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"govard/internal/engine"
)

// govardOwnershipHelperImage runs the chown: a small image, and the only
// thing it needs is busybox chown.
const govardOwnershipHelperImage = "alpine:3"

type govardOwnershipFixDeps struct {
	ProjectRoot      func() (string, error)
	LoadConfig       func(root string) (engine.Config, error)
	CurrentIDs       func() (int, int)
	RuntimeAvailable func() bool
	Foreign          func(root string, uid int) ([]engine.OwnedPath, error)
	Chown            func(ctx context.Context, govardDir string, entries []engine.OwnedPath) error
	ProbeWrite       func(dir string) error
	EnvRunning       func(root string, config engine.Config) bool
	ReadMount        func(ctx context.Context, container, path string) error
	HTTPStatus       func(ctx context.Context, url string) (int, error)
}

// GovardOwnershipFixDepsForTest replaces every host, Docker and network call
// of the ownership fix.
type GovardOwnershipFixDepsForTest = govardOwnershipFixDeps

var ownershipFixDeps = defaultGovardOwnershipFixDeps()

func defaultGovardOwnershipFixDeps() govardOwnershipFixDeps {
	return govardOwnershipFixDeps{
		ProjectRoot: os.Getwd,
		LoadConfig: func(root string) (engine.Config, error) {
			config, _, err := engine.LoadConfigFromDirWithProfile(root, false, engine.ResolveEffectiveProfile(root, ""))
			return config, err
		},
		CurrentIDs:       func() (int, int) { return os.Getuid(), os.Getgid() },
		RuntimeAvailable: func() bool { return engine.CheckDockerStatus(context.Background()) == nil },
		Foreign:          engine.ForeignOwnedGovardPaths,
		Chown:            chownGovardEntries,
		ProbeWrite:       probeWritableDir,
		EnvRunning: func(_ string, config engine.Config) bool {
			running, err := engine.GetRunningProjectNames(context.Background())
			if err != nil {
				return false
			}
			for _, name := range running {
				if name == config.ProjectName {
					return true
				}
			}
			return false
		},
		ReadMount: func(ctx context.Context, container, path string) error {
			out, err := runAuditProfilerDocker(ctx, "exec", container, "test", "-r", path)
			if err != nil {
				return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		},
		HTTPStatus: AuditProfilerHTTPGetForTest(),
	}
}

// SetGovardOwnershipFixDepsForTest swaps the dependencies and returns a restore.
func SetGovardOwnershipFixDepsForTest(deps govardOwnershipFixDeps) func() {
	previous := ownershipFixDeps
	ownershipFixDeps = deps
	return func() { ownershipFixDeps = previous }
}

// FixGovardOwnershipForTest runs the fix handler.
func FixGovardOwnershipForTest(check engine.DoctorCheck) DoctorFixResult {
	return fixGovardOwnership(check)
}

// GovardOwnershipChownScriptForTest exposes the helper container script.
func GovardOwnershipChownScriptForTest(govardDir string, entries []engine.OwnedPath) (string, error) {
	return govardOwnershipChownScript(govardDir, entries)
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// One `chown -h` per recorded path: never recursive, never through a link,
// so the restore can put back mixed original ownership exactly.
func govardOwnershipChownScript(govardDir string, entries []engine.OwnedPath) (string, error) {
	lines := make([]string, 0, len(entries)+1)
	lines = append(lines, "set -e")
	for _, entry := range entries {
		rel, err := filepath.Rel(govardDir, entry.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("refusing to change %s: it is outside %s", entry.Path, govardDir)
		}
		target := "/d"
		if rel != "." {
			target = "/d/" + filepath.ToSlash(rel)
		}
		lines = append(lines, fmt.Sprintf("chown -h %d:%d %s", entry.UID, entry.GID, shellSingleQuote(target)))
	}
	return strings.Join(lines, "\n"), nil
}

func chownGovardEntries(ctx context.Context, govardDir string, entries []engine.OwnedPath) error {
	script, err := govardOwnershipChownScript(govardDir, entries)
	if err != nil {
		return err
	}
	out, err := runAuditProfilerDocker(ctx, "run", "--rm", "-v", govardDir+":/d", govardOwnershipHelperImage, "sh", "-c", script)
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func probeWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	probe, err := os.CreateTemp(dir, ".govard-doctor-probe-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	_ = probe.Close()
	return os.Remove(name)
}

// fixGovardOwnership gives every foreign-owned path under .govard back to the
// invoking user through a short-lived container (no sudo), then proves the
// project still works; any failed post-check restores the recorded owners.
func fixGovardOwnership(check engine.DoctorCheck) DoctorFixResult {
	result := DoctorFixResult{CheckID: strings.TrimSpace(check.ID), Title: strings.TrimSpace(check.Title), Actions: []string{}}
	fail := func(status, message string) DoctorFixResult {
		result.Status = status
		result.Message = message
		return result
	}
	deps := ownershipFixDeps
	if !deps.RuntimeAvailable() {
		return fail(DoctorFixStatusUnavailable, "A container runtime is required to change ownership without sudo.")
	}
	root, err := deps.ProjectRoot()
	if err != nil {
		return fail(DoctorFixStatusFailed, err.Error())
	}
	config, err := deps.LoadConfig(root)
	if err != nil {
		return fail(DoctorFixStatusFailed, fmt.Sprintf("load project config: %v", err))
	}
	uid, gid := deps.CurrentIDs()
	govardDir := filepath.Join(root, ".govard")
	original, err := deps.Foreign(root, uid)
	if err != nil {
		return fail(DoctorFixStatusFailed, err.Error())
	}
	if len(original) == 0 {
		return fail(DoctorFixStatusApplied, "Every path under .govard already belongs to your user.")
	}
	if _, err := govardOwnershipChownScript(govardDir, original); err != nil {
		return fail(DoctorFixStatusFailed, err.Error())
	}
	target := make([]engine.OwnedPath, len(original))
	for i, entry := range original {
		target[i] = engine.OwnedPath{Path: entry.Path, UID: uid, GID: gid}
		rel, _ := filepath.Rel(root, entry.Path)
		result.Actions = append(result.Actions, fmt.Sprintf("chown %d:%d %s (was %d:%d)", uid, gid, rel, entry.UID, entry.GID))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := deps.Chown(ctx, govardDir, target); err != nil {
		return fail(DoctorFixStatusFailed, fmt.Sprintf("change ownership: %v (ownership left unchanged)", err))
	}

	rollback := func(reason string) DoctorFixResult {
		if err := deps.Chown(ctx, govardDir, original); err != nil {
			return fail(DoctorFixStatusFailed, fmt.Sprintf("%s; restoring the original owners also failed: %v", reason, err))
		}
		result.Actions = append(result.Actions, "restored the original owners")
		return fail(DoctorFixStatusFailed, reason+"; original owners restored")
	}

	mount, hasMount := engine.CustomConfigMount(root, config)
	if hasMount {
		if err := deps.ProbeWrite(mount.HostDir); err != nil {
			return rollback(fmt.Sprintf("write probe in %s failed: %v", mount.HostDir, err))
		}
		result.Actions = append(result.Actions, "write probe passed in "+mount.HostDir)
	}
	if !deps.EnvRunning(root, config) {
		result.Actions = append(result.Actions, "environment down: mount and HTTP checks skipped")
		result.Status = DoctorFixStatusApplied
		result.Message = fmt.Sprintf("%d paths under .govard now belong to your user.", len(original))
		return result
	}
	if hasMount {
		if err := deps.ReadMount(ctx, mount.Container, mount.ContainerPath); err != nil {
			return rollback(fmt.Sprintf("%s cannot read %s: %v", mount.Container, mount.ContainerPath, err))
		}
		result.Actions = append(result.Actions, fmt.Sprintf("%s still reads %s", mount.Container, mount.ContainerPath))
	}
	if domain := strings.TrimSpace(config.Domain); domain != "" {
		status, err := deps.HTTPStatus(ctx, "https://"+domain+"/")
		if err != nil || status != 200 {
			if err == nil {
				err = errors.New("unexpected status")
			}
			return rollback(fmt.Sprintf("https://%s/ answered %d after the fix (%v)", domain, status, err))
		}
		result.Actions = append(result.Actions, fmt.Sprintf("https://%s/ answered 200", domain))
	}
	result.Status = DoctorFixStatusApplied
	result.Message = fmt.Sprintf("%d paths under .govard now belong to your user.", len(original))
	return result
}
