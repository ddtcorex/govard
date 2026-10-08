package remote

import (
	"fmt"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
	"govard/internal/engine"
)

var validSnapshotNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateSnapshotName ensures the snapshot name is safe and doesn't allow path traversal.
func ValidateSnapshotName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("snapshot name cannot be empty")
	}
	if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.Contains(trimmed, "..") {
		return fmt.Errorf("snapshot name must not contain path separators or '..'")
	}
	if !validSnapshotNamePattern.MatchString(trimmed) {
		return fmt.Errorf("snapshot name contains invalid characters: %q", trimmed)
	}
	return nil
}

// RemoteSnapshotRoot returns the legacy remote snapshot root: inside the remote
// path itself. On a deploy layout that path is the served release, so new
// snapshots no longer go there (see SnapshotRootForLayout); the legacy root is
// still read by list, restore, delete and pull so existing snapshots stay
// visible.
func RemoteSnapshotRoot(remoteCfg engine.RemoteConfig) string {
	return strings.TrimRight(remoteCfg.Path, "/") + "/.govard/snapshots"
}

// Snapshot layouts reported by SnapshotLayoutProbeCommand.
const (
	SnapshotLayoutDeploy = "deploy"
	SnapshotLayoutLegacy = "legacy"
)

// SnapshotDeployPath returns the deploy root used to detect a deploy layout.
// The per-remote deploy_path wins, then the project-level one (configured),
// then it is derived from the remote path: the parent when the path is the
// `current` link, the path itself otherwise.
func SnapshotDeployPath(remoteCfg engine.RemoteConfig, configured string) string {
	if remoteCfg.Deploy != nil && strings.TrimSpace(remoteCfg.Deploy.DeployPath) != "" {
		return strings.TrimSpace(remoteCfg.Deploy.DeployPath)
	}
	if strings.TrimSpace(configured) != "" {
		return strings.TrimSpace(configured)
	}
	trimmed := strings.TrimRight(remoteCfg.Path, "/")
	if path.Base(trimmed) == "current" {
		return path.Dir(trimmed)
	}
	return trimmed
}

// snapshotLayoutTest is the shell test for "this directory is a deploy root":
// releases/ and shared/ directories plus a current link.
const snapshotLayoutTest = `[ -d "$D/releases" ] && [ -d "$D/shared" ] && [ -L "$D/current" ]`

// SnapshotLayoutProbeCommand returns a shell command that prints "deploy" when
// deployPath holds a deploy layout and "legacy" otherwise. It always exits 0.
func SnapshotLayoutProbeCommand(deployPath string) string {
	return fmt.Sprintf("D=%s; if %s; then echo %s; else echo %s; fi",
		QuoteRemotePath(deployPath), snapshotLayoutTest, SnapshotLayoutDeploy, SnapshotLayoutLegacy)
}

// SnapshotRootForLayout is where new snapshots are written: shared/.govard/snapshots
// under the deploy path on a deploy layout (survives release switches and is
// never served), the legacy root otherwise.
func SnapshotRootForLayout(remoteCfg engine.RemoteConfig, deployPath string, layout string) string {
	if layout == SnapshotLayoutDeploy {
		return strings.TrimRight(deployPath, "/") + "/shared/.govard/snapshots"
	}
	return RemoteSnapshotRoot(remoteCfg)
}

// snapshotRootsPrelude defines N (deploy-layout root, empty when the remote has
// no deploy layout) and O (legacy root) in the remote shell. Lookups try N first.
func snapshotRootsPrelude(remoteCfg engine.RemoteConfig) string {
	deployPath := SnapshotDeployPath(remoteCfg, "")
	return fmt.Sprintf("D=%s; N=''; if %s; then N=\"$D/shared/.govard/snapshots\"; fi; O=%s; ",
		QuoteRemotePath(deployPath), snapshotLayoutTest, QuoteRemotePath(RemoteSnapshotRoot(remoteCfg)))
}

// RemoteSnapshotDir returns the full remote path for a named snapshot.
func RemoteSnapshotDir(remoteCfg engine.RemoteConfig, name string) string {
	return RemoteSnapshotRoot(remoteCfg) + "/" + name
}

// BuildRemoteSnapshotCreateCommand builds the SSH command to create a snapshot on the remote.
// It creates the directory, dumps the DB to db.sql.gz, and tars media to media.tar.gz.
//
// This writes to the legacy root; callers that probe the layout use
// BuildRemoteSnapshotCreateCommandAtRoot.
func BuildRemoteSnapshotCreateCommand(
	remoteCfg engine.RemoteConfig,
	name string,
	framework string,
	dbDumpCommandStr string,
	remoteMediaPath string,
) string {
	return BuildRemoteSnapshotCreateCommandAtRoot(RemoteSnapshotRoot(remoteCfg), name, framework, dbDumpCommandStr, remoteMediaPath)
}

// BuildRemoteSnapshotCreateCommandAtRoot is BuildRemoteSnapshotCreateCommand for an explicit snapshot root.
func BuildRemoteSnapshotCreateCommandAtRoot(
	root string,
	name string,
	framework string,
	dbDumpCommandStr string,
	remoteMediaPath string,
) string {
	snapshotDir := strings.TrimRight(root, "/") + "/" + name
	quoted := QuoteRemotePath(snapshotDir)

	parts := []string{
		fmt.Sprintf("mkdir -p %s", quoted),
	}

	// DB dump
	if dbDumpCommandStr != "" {
		dbPath := snapshotDir + "/db.sql.gz"
		parts = append(parts,
			fmt.Sprintf("{ %s; } > %s", dbDumpCommandStr, engine.ShellQuote(dbPath)),
		)
	}

	// Media tar
	if strings.TrimSpace(remoteMediaPath) != "" {
		mediaTar := snapshotDir + "/media.tar.gz"
		parts = append(parts,
			fmt.Sprintf("if [ -d %s ]; then tar -czf %s -C %s .; fi",
				engine.ShellQuote(remoteMediaPath),
				engine.ShellQuote(mediaTar),
				engine.ShellQuote(remoteMediaPath),
			),
		)
	}

	// Write metadata. The timestamp is computed in its own step so no %
	// sequence ever reaches printf as a format, and every line is passed as a
	// quoted %s argument so name and framework are never expanded or
	// interpreted. Values are YAML single-quoted so any text round-trips.
	metaPath := snapshotDir + "/metadata.yml"
	yamlQuote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	parts = append(parts,
		"ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)",
		// db and media describe what this run actually captured, so a project
		// with no media directory (or no dump command) does not claim archives
		// that were never written.
		fmt.Sprintf("if [ -s %s ]; then hasdb=true; else hasdb=false; fi", engine.ShellQuote(snapshotDir+"/db.sql.gz")),
		fmt.Sprintf("if [ -s %s ]; then hasmedia=true; else hasmedia=false; fi", engine.ShellQuote(snapshotDir+"/media.tar.gz")),
		fmt.Sprintf("printf '%%s\\n' %s \"created_at: $ts\" %s \"db: $hasdb\" \"media: $hasmedia\" > %s",
			engine.ShellQuote("name: "+yamlQuote(name)),
			engine.ShellQuote("framework: "+yamlQuote(framework)),
			engine.ShellQuote(metaPath),
		),
	)

	// umask 077 first: the dump, the media archive and any directory this
	// creates (e.g. the snapshots root) are owner-only on the remote.
	return "umask 077; " + strings.Join(parts, " && ")
}

// BuildRemoteSnapshotListCommand builds the SSH command to list snapshots on the remote.
// It reads the deploy-layout root first (when the remote has one), then the
// legacy root. Each root is announced with an "@@root <path>" line so the caller
// can say where a snapshot lives.
func BuildRemoteSnapshotListCommand(remoteCfg engine.RemoteConfig) string {
	return snapshotRootsPrelude(remoteCfg) +
		`for r in "$N" "$O"; do [ -n "$r" ] && [ -d "$r" ] || continue; echo "@@root $r"; ` +
		`for d in "$r"/*/; do [ -d "$d" ] && cat "$d/metadata.yml" 2>/dev/null && echo '---'; done; done; true`
}

// BuildRemoteSnapshotLocateCommand prints the directory of a named snapshot
// (deploy-layout root first, legacy second) and fails when neither holds it.
func BuildRemoteSnapshotLocateCommand(remoteCfg engine.RemoteConfig, name string) string {
	return snapshotRootsPrelude(remoteCfg) + fmt.Sprintf(
		`for r in "$N" "$O"; do [ -n "$r" ] && [ -d "$r"/%[1]s ] && { printf '%%s\n' "$r"/%[1]s; exit 0; }; done; exit 1`,
		engine.ShellQuote(name))
}

// BuildRemoteSnapshotDeleteCommand builds the SSH command to delete a snapshot on the remote.
// The snapshot is removed from every location that holds it.
func BuildRemoteSnapshotDeleteCommand(remoteCfg engine.RemoteConfig, name string) string {
	return snapshotRootsPrelude(remoteCfg) + fmt.Sprintf(
		`for r in "$N" "$O"; do [ -n "$r" ] && [ -d "$r"/%[1]s ] && rm -rf "$r"/%[1]s; done; true`,
		engine.ShellQuote(name))
}

// BuildRemoteSnapshotRestoreCommand builds the SSH command to restore a snapshot on the remote.
func BuildRemoteSnapshotRestoreCommand(
	remoteCfg engine.RemoteConfig,
	name string,
	framework string,
	dbImportCommandStr string,
	remoteMediaPath string,
	dbOnly bool,
	mediaOnly bool,
) string {
	// S is the snapshot directory, found in the deploy-layout root first and
	// the legacy root second.
	lookup := snapshotRootsPrelude(remoteCfg) + fmt.Sprintf(
		`S=''; for r in "$N" "$O"; do [ -n "$r" ] && [ -d "$r"/%s ] && { S="$r"/%s; break; }; done; [ -n "$S" ]`,
		engine.ShellQuote(name), engine.ShellQuote(name))
	parts := []string{lookup}

	// Restore DB
	if !mediaOnly && dbImportCommandStr != "" {
		parts = append(parts,
			fmt.Sprintf(`if [ -f "$S/db.sql.gz" ]; then zcat "$S/db.sql.gz" | %s; fi`, dbImportCommandStr),
		)
	}

	// Restore media
	if !dbOnly && strings.TrimSpace(remoteMediaPath) != "" {
		parts = append(parts,
			fmt.Sprintf(`if [ -f "$S/media.tar.gz" ]; then mkdir -p %s && tar -xzf "$S/media.tar.gz" -C %s; fi`,
				engine.ShellQuote(remoteMediaPath),
				engine.ShellQuote(remoteMediaPath),
			),
		)
	}

	return strings.Join(parts, " && ")
}

// BuildRemoteSnapshotPullCommand builds the rsync command to download a snapshot from remote to local.
func BuildRemoteSnapshotPullCommand(
	remoteName string,
	remoteCfg engine.RemoteConfig,
	name string,
	localSnapshotDir string,
) *exec.Cmd {
	return BuildRemoteSnapshotPullCommandAt(remoteName, remoteCfg, RemoteSnapshotDir(remoteCfg, name), localSnapshotDir)
}

// BuildRemoteSnapshotPullCommandAt pulls an explicit remote snapshot directory.
func BuildRemoteSnapshotPullCommandAt(
	remoteName string,
	remoteCfg engine.RemoteConfig,
	remoteDir string,
	localSnapshotDir string,
) *exec.Cmd {
	remoteSnapshotDir := strings.TrimRight(remoteDir, "/") + "/"
	source := fmt.Sprintf("%s:%s", RemoteTarget(remoteCfg), remoteSnapshotDir)
	return BuildRsyncCommand(remoteName, source, localSnapshotDir+"/", remoteCfg, false, true, false, nil, nil)
}

// BuildRemoteSnapshotPushCommand builds the rsync command to upload a local snapshot to remote.
func BuildRemoteSnapshotPushCommand(
	remoteName string,
	remoteCfg engine.RemoteConfig,
	name string,
	localSnapshotDir string,
) *exec.Cmd {
	return BuildRemoteSnapshotPushCommandAt(remoteName, remoteCfg, RemoteSnapshotDir(remoteCfg, name), localSnapshotDir)
}

// BuildRemoteSnapshotPushCommandAt pushes into an explicit remote snapshot directory.
func BuildRemoteSnapshotPushCommandAt(
	remoteName string,
	remoteCfg engine.RemoteConfig,
	remoteDir string,
	localSnapshotDir string,
) *exec.Cmd {
	remoteSnapshotDir := strings.TrimRight(remoteDir, "/") + "/"
	destination := fmt.Sprintf("%s:%s", RemoteTarget(remoteCfg), remoteSnapshotDir)
	return BuildRsyncCommand(remoteName, localSnapshotDir+"/", destination, remoteCfg, false, true, false, nil, nil)
}

// ParseRemoteSnapshotList parses the output of the remote listing command.
func ParseRemoteSnapshotList(raw string) ([]engine.SnapshotMetadata, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "EMPTY" {
		return []engine.SnapshotMetadata{}, nil
	}

	documents := strings.Split(trimmed, "---")
	snapshots := make([]engine.SnapshotMetadata, 0, len(documents))
	for _, doc := range documents {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var meta engine.SnapshotMetadata
		if err := yaml.Unmarshal([]byte(doc), &meta); err != nil {
			continue
		}
		if meta.Name == "" {
			continue
		}
		snapshots = append(snapshots, meta)
	}
	return snapshots, nil
}

// RemoteSnapshotEntry is a snapshot together with the remote root it was found in.
type RemoteSnapshotEntry struct {
	Meta engine.SnapshotMetadata
	Root string
}

// ParseRemoteSnapshotListEntries parses the output of BuildRemoteSnapshotListCommand,
// keeping the order of the roots (deploy layout first, legacy second).
func ParseRemoteSnapshotListEntries(raw string) ([]RemoteSnapshotEntry, error) {
	entries := []RemoteSnapshotEntry{}
	root := ""
	var doc []string
	flush := func() {
		text := strings.TrimSpace(strings.Join(doc, "\n"))
		doc = doc[:0]
		if text == "" || text == "EMPTY" {
			return
		}
		var meta engine.SnapshotMetadata
		if err := yaml.Unmarshal([]byte(text), &meta); err != nil || meta.Name == "" {
			return
		}
		entries = append(entries, RemoteSnapshotEntry{Meta: meta, Root: root})
	}
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "@@root "):
			flush()
			root = strings.TrimSpace(strings.TrimPrefix(trimmed, "@@root "))
		case trimmed == "---":
			flush()
		default:
			doc = append(doc, line)
		}
	}
	flush()
	return entries, nil
}

// ForTest wrappers

func ValidateSnapshotNameForTest(name string) error {
	return ValidateSnapshotName(name)
}

func BuildRemoteSnapshotCreateCommandForTest(remoteName string, remoteCfg engine.RemoteConfig, name string, framework string) string {
	_, mediaPath := engine.ResolveRemotePathsForConfig(framework, remoteCfg)
	dbDump := "echo 'mock-dump'"
	return BuildRemoteSnapshotCreateCommand(remoteCfg, name, framework, dbDump, mediaPath)
}

func BuildRemoteSnapshotListCommandForTest(remoteCfg engine.RemoteConfig) string {
	return BuildRemoteSnapshotListCommand(remoteCfg)
}

func BuildRemoteSnapshotDeleteCommandForTest(remoteCfg engine.RemoteConfig, name string) string {
	return BuildRemoteSnapshotDeleteCommand(remoteCfg, name)
}

func BuildRemoteSnapshotRestoreCommandForTest(remoteCfg engine.RemoteConfig, name string, framework string, dbOnly bool, mediaOnly bool) string {
	_, mediaPath := engine.ResolveRemotePathsForConfig(framework, remoteCfg)
	dbImport := "mysql -u app app"
	return BuildRemoteSnapshotRestoreCommand(remoteCfg, name, framework, dbImport, mediaPath, dbOnly, mediaOnly)
}

func ParseRemoteSnapshotListForTest(raw string) ([]engine.SnapshotMetadata, error) {
	return ParseRemoteSnapshotList(raw)
}
