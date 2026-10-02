package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// Reusing static content on the sandbox. The task that deploys it takes 30 to
// 40 s and its output is a function of the artifact and of the command's own
// text, so a second deploy of the same artifact to the throwaway sandbox target
// can hardlink the earlier release's directories instead. It is limited to the
// sandbox: a real remote always runs the task, because content that silently
// diverged from what the command would produce is worse there than the wait.

const (
	// sandboxAssetsRecordName is the record of what the last run of the task
	// produced, in the layout's bookkeeping directory.
	sandboxAssetsRecordName = "sandbox-assets.json"
	// sandboxReuseSetting is the deploy setting that switches the reuse off.
	sandboxReuseSetting = "sandbox_reuse_assets"
)

type sandboxAssetsRecord struct {
	Fingerprint string `json:"fingerprint"`
	Release     string `json:"release"`
}

// AssetsFingerprint identifies what the assets task would produce: the digest of
// the artifact manifest (PHP, lock file and every file's digest, without the build
// time) and the rendered command, which carries the settings that shape the
// output.
func AssetsFingerprint(manifestDigest, command string) string {
	hash := sha256.New()
	for _, value := range []string{manifestDigest, command} {
		fmt.Fprintf(hash, "%d:%s;", len(value), value)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// AssetsFingerprintForTest exposes AssetsFingerprint to the tests/ package.
func AssetsFingerprintForTest(manifestDigest, command string) string {
	return AssetsFingerprint(manifestDigest, command)
}

// sandboxAssetsEligible reports whether the reuse applies to this step at all.
func (e *Executor) sandboxAssetsEligible(step Step) bool {
	if step.ID != TaskAssets || len(step.ReusableOutputs) == 0 || !e.host.Remote.Sandbox {
		return false
	}
	if value, ok := e.opts.Settings[sandboxReuseSetting]; ok {
		if enabled, isBool := value.(bool); isBool && !enabled {
			return false
		}
	}
	return true
}

// sandboxAssetsFingerprint digests the manifest the release carries on the
// target and fingerprints it together with the step's rendered command. An empty result
// means "no basis for reuse": a server build has no manifest, and a manifest
// that cannot be read is treated the same.
func (e *Executor) sandboxAssetsFingerprint(ctx context.Context, step Step, vars Vars, release *Release) string {
	// The release directory is in the rendered command and differs on every run;
	// what the command does is the same, so the release-derived variables are
	// pinned before the text is fingerprinted.
	command, err := vars.Clone().Set("release", "<release>").SetPath("release_path", "<release>").Expand(step.Command)
	if err != nil {
		return ""
	}
	// The manifest of a storefront is megabytes, far past the runner's capture
	// limit, so the target digests it instead of sending it back. The build time
	// is the one field that differs between two builds of the same files.
	manifestPath := path.Join(e.host.ReleasePath(release.Release), ".dep", ArtifactRecordName)
	result, err := e.host.Runner().Run(ctx,
		// A pipeline reports its last stage, so a missing manifest (a server
		// build has none) would otherwise digest the empty input and look like a
		// valid fingerprint.
		`test -f `+Shell(manifestPath)+` && sed 's/"created_at":"[^"]*",//' `+Shell(manifestPath)+` | sha256sum | cut -d' ' -f1`,
		RunOptions{Timeout: shortCommandTimeout})
	digest := strings.TrimSpace(result.Stdout)
	if err != nil || len(digest) != sha256.Size*2 {
		return ""
	}
	return AssetsFingerprint(digest, command)
}

// reuseSandboxAssets hardlinks the recorded release's outputs into this release
// when the fingerprints match and every directory still exists. It reports why
// the step may be skipped, or false when the task has to run. Any failure to
// reuse falls back to running the task: a deploy never fails because of this.
func (e *Executor) reuseSandboxAssets(ctx context.Context, step Step, vars Vars, release *Release) (string, bool) {
	if !e.sandboxAssetsEligible(step) {
		return "", false
	}
	fingerprint := e.sandboxAssetsFingerprint(ctx, step, vars, release)
	if fingerprint == "" {
		return "", false
	}
	runner := e.host.Runner()
	recordPath := path.Join(e.host.DepPath(), sandboxAssetsRecordName)
	raw, err := runner.Run(ctx, "cat "+Shell(recordPath), RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return "", false
	}
	var record sandboxAssetsRecord
	if json.Unmarshal([]byte(raw.Stdout), &record) != nil || record.Fingerprint != fingerprint || record.Release == "" || record.Release == release.Release {
		return "", false
	}
	previous := e.host.ReleasePath(record.Release)
	current := e.host.ReleasePath(release.Release)

	var command strings.Builder
	for _, output := range step.ReusableOutputs {
		fmt.Fprintf(&command, "test -d %s || exit 3; ", Shell(path.Join(previous, output)))
	}
	for _, output := range step.ReusableOutputs {
		target := path.Join(current, output)
		fmt.Fprintf(&command, "mkdir -p %s && rm -rf %s && cp -al %s %s || exit 4; ",
			Shell(path.Dir(target)), Shell(target), Shell(path.Join(previous, output)), Shell(target))
	}
	if _, err := runner.Run(ctx, strings.TrimSuffix(command.String(), " "), RunOptions{Timeout: shortCommandTimeout}); err != nil {
		return "", false
	}
	return fmt.Sprintf("sandbox: static content reused from release %s (same artifact fingerprint)", record.Release), true
}

// recordSandboxAssets stores what the task just produced, so the next deploy to
// the sandbox can reuse it.
func (e *Executor) recordSandboxAssets(ctx context.Context, step Step, vars Vars, release *Release) {
	if !e.sandboxAssetsEligible(step) {
		return
	}
	fingerprint := e.sandboxAssetsFingerprint(ctx, step, vars, release)
	if fingerprint == "" {
		return
	}
	payload, err := json.Marshal(sandboxAssetsRecord{Fingerprint: fingerprint, Release: release.Release})
	if err != nil {
		return
	}
	// A record that cannot be written costs the next deploy its shortcut only.
	_ = writeTargetFile(ctx, e.host, path.Join(e.host.DepPath(), sandboxAssetsRecordName), payload)
}
