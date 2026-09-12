package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine/remote"
)

// The artifact contract. `govard deploy build` writes it, `govard deploy
// --artifact-dir` consumes it, and the manifest is the only thing that travels
// between the two jobs besides the files themselves.
const (
	// ArtifactManifestName is the manifest file at the root of an artifact.
	ArtifactManifestName = "manifest.json"
	// ArtifactManifestSchema is the manifest format version. A consumer refuses
	// a version it does not know rather than reading fields that moved.
	ArtifactManifestSchema = 1
	// ArtifactRecordName is the manifest's name inside a release, next to the
	// release record. It is not ArtifactManifestName because a release already
	// holds a record of its own and two files called manifest.json in one
	// directory hierarchy is how an operator reads the wrong one.
	ArtifactRecordName = "artifact-manifest.json"
)

// ArtifactFile is one entry of an artifact's file list.
type ArtifactFile struct {
	Path string `json:"path"`
	// SHA256 is the hex digest of a regular file's contents. It is empty for a
	// symlink, whose Target is what identifies it: hashing through a link would
	// make the manifest depend on wherever the link pointed at build time.
	SHA256 string `json:"sha256,omitempty"`
	// Target is the link target of a symlink entry.
	Target string `json:"target,omitempty"`
	Size   int64  `json:"size"`
}

// ArtifactManifest is the evidence `govard deploy build` leaves beside the
// files it produced. The deploy job reads it to prove it is shipping the
// revision it thinks it is, and to compare the PHP it was built for with the
// PHP the target runs.
type ArtifactManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Tool          string `json:"tool"`
	BuildMode     string `json:"build_mode"`
	Revision      string `json:"revision"`
	CreatedAt     string `json:"created_at"`
	// PHPVersion is the PHP that built the artifact, or empty when the build
	// machine had none. An empty value disables the parity gate rather than
	// blocking a project that is not a PHP project.
	PHPVersion string `json:"php_version,omitempty"`
	// ComposerLockSHA256 identifies the dependency set the artifact was built
	// against, which is what makes "same revision, different vendor/" visible.
	ComposerLockSHA256 string         `json:"composer_lock_sha256,omitempty"`
	FileCount          int            `json:"file_count"`
	TotalBytes         int64          `json:"total_bytes"`
	Files              []ArtifactFile `json:"files"`
}

// ManifestPath is the manifest file inside an artifact directory.
func ManifestPath(root string) string { return filepath.Join(root, ArtifactManifestName) }

// BuildManifest walks a built artifact and records every file it holds.
//
// The walk never follows a symlink: a link into the build machine's filesystem
// must not turn into a permission error or, worse, into content the artifact
// does not actually contain.
func BuildManifest(root, revision, phpVersion string) (*ArtifactManifest, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect artifact directory %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifact path %s is not a directory", root)
	}

	manifest := &ArtifactManifest{
		SchemaVersion: ArtifactManifestSchema,
		Tool:          ReleaseTool,
		BuildMode:     BuildArtifact,
		Revision:      strings.TrimSpace(revision),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		PHPVersion:    strings.TrimSpace(phpVersion),
		Files:         []ArtifactFile{},
	}

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		// The manifest describes the payload; it is not part of it. A rebuild
		// after a previous manifest was written must produce the same list.
		if relative == ArtifactManifestName {
			return nil
		}

		file := ArtifactFile{Path: relative}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return fmt.Errorf("read link %s: %w", relative, err)
			}
			file.Target = target
		case entry.Type().IsRegular():
			digest, size, err := hashFile(path)
			if err != nil {
				return fmt.Errorf("hash %s: %w", relative, err)
			}
			file.SHA256, file.Size = digest, size
		default:
			// A device, socket or fifo has no content to hash and no place in
			// a release. Recording it with a size of zero keeps the file list
			// faithful without pretending it is verifiable.
		}

		manifest.Files = append(manifest.Files, file)
		manifest.FileCount++
		manifest.TotalBytes += file.Size
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan artifact %s: %w", root, err)
	}

	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })

	if lock, _, err := hashFile(filepath.Join(root, "composer.lock")); err == nil {
		manifest.ComposerLockSHA256 = lock
	}
	return manifest, nil
}

// BuildManifestForTest exposes BuildManifest to the tests/ package.
func BuildManifestForTest(root, revision, phpVersion string) (*ArtifactManifest, error) {
	return BuildManifest(root, revision, phpVersion)
}

// WriteManifest stores the manifest at the root of the artifact it describes.
func WriteManifest(root string, manifest *ArtifactManifest) error {
	if manifest == nil {
		return fmt.Errorf("write artifact manifest: nil manifest")
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode artifact manifest: %w", err)
	}
	if err := os.WriteFile(ManifestPath(root), append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write artifact manifest: %w", err)
	}
	return nil
}

// ReadManifest loads the manifest from an artifact directory.
func ReadManifest(root string) (*ArtifactManifest, error) {
	payload, err := os.ReadFile(ManifestPath(root))
	if err != nil {
		return nil, fmt.Errorf("read artifact manifest %s: %w", ManifestPath(root), err)
	}
	var manifest ArtifactManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return nil, fmt.Errorf("parse artifact manifest %s: %w", ManifestPath(root), err)
	}
	if manifest.SchemaVersion != ArtifactManifestSchema {
		return nil, fmt.Errorf("artifact manifest %s has schema version %d; this govard understands %d",
			ManifestPath(root), manifest.SchemaVersion, ArtifactManifestSchema)
	}
	return &manifest, nil
}

// Verify re-reads every file the manifest lists and reports the first entry
// that is missing or no longer matches.
//
// It runs where the artifact is, before anything is uploaded: a truncated CI
// cache download or a half-written build is caught on the runner rather than
// discovered by a visitor.
func (m *ArtifactManifest) Verify(root string) error {
	for _, file := range m.Files {
		absolute := filepath.Join(root, filepath.FromSlash(file.Path))
		info, err := os.Lstat(absolute)
		if err != nil {
			return fmt.Errorf("artifact is missing %s: %w", file.Path, err)
		}

		if file.Target != "" {
			if info.Mode()&fs.ModeSymlink == 0 {
				return fmt.Errorf("artifact entry %s is no longer a symlink", file.Path)
			}
			target, err := os.Readlink(absolute)
			if err != nil {
				return fmt.Errorf("artifact link %s is unreadable: %w", file.Path, err)
			}
			if target != file.Target {
				return fmt.Errorf("artifact link %s points at %q, want %q", file.Path, target, file.Target)
			}
			continue
		}

		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact entry %s is no longer a regular file", file.Path)
		}
		if info.Size() != file.Size {
			return fmt.Errorf("artifact file %s is %d bytes, want %d", file.Path, info.Size(), file.Size)
		}
		digest, _, err := hashFile(absolute)
		if err != nil {
			return fmt.Errorf("hash artifact file %s: %w", file.Path, err)
		}
		if digest != file.SHA256 {
			return fmt.Errorf("artifact file %s was modified after the build (sha256 %s, want %s)", file.Path, digest, file.SHA256)
		}
	}
	return nil
}

// hashFile returns the hex sha256 and the size of one file.
func hashFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

// BuildRequest is one `govard deploy build` invocation.
type BuildRequest struct {
	// Recipe is the resolved framework recipe; Hooks are the project's.
	Recipe Recipe
	Hooks  []Hook
	// Options carries the revision, the settings and the timeout. Build is
	// ignored: a build always runs the build tasks.
	Options Options
	Vars    Vars
	// WorkDir is the local checkout whose revision is materialised. Empty
	// means the process working directory.
	WorkDir string
	// OutputDir receives the artifact. It must be absent or empty unless Force
	// is set.
	OutputDir string
	Force     bool
	Out       io.Writer
	// Runner executes the local commands. It defaults to LocalRunner; tests
	// substitute it to observe what the build actually runs.
	Runner Runner
}

// BuildArtifactDir materialises one revision into an artifact directory, runs
// the recipe's build tasks there, and writes the manifest.
//
// It runs entirely on the machine that invokes govard — a CI runner with the
// project's toolchain — which is what lets the deploy job that consumes the
// result need nothing but govard, ssh and rsync.
func BuildArtifactDir(ctx context.Context, req BuildRequest) (*ArtifactManifest, error) {
	out := req.Out
	if out == nil {
		out = io.Discard
	}
	if strings.TrimSpace(req.OutputDir) == "" {
		return nil, fmt.Errorf("an output directory is required: pass --output <dir>")
	}
	output := filepath.Clean(req.OutputDir)

	if err := prepareOutputDir(output, req.Force); err != nil {
		return nil, err
	}

	workDir := req.WorkDir
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	runner := req.Runner
	if runner == nil {
		runner = LocalRunner{}
	}
	timeout := req.Options.CommandTimeout
	if timeout <= 0 {
		timeout = DefaultCommandTimeout
	}

	revision, err := resolveBuildRevision(ctx, runner, workDir, req.Options)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "▶ build %s (%s)\n", revisionOrShort(revision), output)

	if err := materialiseRevision(ctx, runner, workDir, output, revision, timeout); err != nil {
		return nil, err
	}

	vars := req.Vars.Clone().
		SetPath("release_path", output).
		SetPath("deploy_path", filepath.Dir(output))

	if err := runBuildTasks(ctx, runner, req, vars, output, out); err != nil {
		return nil, err
	}

	// PHP is only meaningful for a project that has one. Probing an arbitrary
	// project would let a CI image that happens to ship PHP impose a PHP gate
	// on a project that has none.
	phpVersion := ""
	if projectUsesPHP(output) {
		if phpVersion = probeLocalPHP(ctx, runner, req.Options, workDir); phpVersion == "" {
			fmt.Fprintf(out, "  note: this is a Composer project but no local PHP was found; the artifact records no PHP version, so the target's PHP is not compared\n")
		}
	}

	manifest, err := BuildManifest(output, revision, phpVersion)
	if err != nil {
		return nil, err
	}
	if err := WriteManifest(output, manifest); err != nil {
		return nil, err
	}
	fmt.Fprintf(out, "  artifact: %d files, %.1f MiB, revision %s\n",
		manifest.FileCount, float64(manifest.TotalBytes)/1024/1024, revisionOrShort(manifest.Revision))
	return manifest, nil
}

// prepareOutputDir guarantees the artifact holds exactly what this build
// produced. A stale file from an earlier build would ship silently, so a
// non-empty directory is refused unless --force says to clear it.
func prepareOutputDir(output string, force bool) error {
	entries, err := os.ReadDir(output)
	if err == nil && len(entries) > 0 {
		if !force {
			return fmt.Errorf("output directory %s is not empty; pass --force to replace its contents", output)
		}
		if err := os.RemoveAll(output); err != nil {
			return fmt.Errorf("clear output directory %s: %w", output, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect output directory %s: %w", output, err)
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return fmt.Errorf("create output directory %s: %w", output, err)
	}
	return nil
}

// resolveBuildRevision uses the explicit revision, then the tag, then the local
// HEAD. A revision that is not in the checkout is reported here rather than as
// an empty artifact.
func resolveBuildRevision(ctx context.Context, runner Runner, workDir string, opts Options) (string, error) {
	revision := strings.TrimSpace(opts.Revision)
	if revision == "" {
		revision = strings.TrimSpace(opts.Tag)
	}
	if revision != "" {
		return revision, nil
	}
	result, err := runner.Run(ctx, "git -C "+conventions.ShellQuote(workDir)+" rev-parse HEAD", RunOptions{Timeout: shortCommandTimeout})
	if err != nil {
		return "", fmt.Errorf("resolve the local revision (pass --revision in CI): %w", err)
	}
	resolved := strings.TrimSpace(result.Stdout)
	if resolved == "" {
		return "", fmt.Errorf("resolve the local revision: git returned an empty value (pass --revision in CI)")
	}
	return resolved, nil
}

// materialiseRevision extracts exactly the tracked files of one revision. The
// artifact starts from the checkout, not from a working tree: an uncommitted
// edit on the build machine must not reach production.
func materialiseRevision(ctx context.Context, runner Runner, workDir, output, revision string, timeout time.Duration) error {
	command := fmt.Sprintf(
		"git -C %s cat-file -e %s^{commit} && git -C %s archive --format=tar %s | tar -x -C %s",
		conventions.ShellQuote(workDir),
		conventions.ShellQuote(revision),
		conventions.ShellQuote(workDir),
		conventions.ShellQuote(revision),
		conventions.ShellQuote(output),
	)
	if _, err := runner.Run(ctx, command, RunOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("materialise %s from %s (is the revision fetched?): %w", revision, workDir, err)
	}
	return nil
}

// runBuildTasks runs the build stage in the artifact directory, through the same
// plan a server build would run. That is the parity guarantee: one task list,
// one expansion, two places to execute it.
func runBuildTasks(ctx context.Context, runner Runner, req BuildRequest, vars Vars, output string, out io.Writer) error {
	plan, err := BuildPlan(req.Recipe, req.Hooks, "build")
	if err != nil {
		return err
	}
	// A build always runs the build tasks: `deploy:artifact` is excluded
	// because this command is what produces the artifact.
	plan = plan.ForBuildMode(BuildServer)

	for _, step := range plan.Steps {
		if step.Stage != StageBuild {
			continue
		}
		if !step.Implemented() {
			continue
		}
		if step.core != nil {
			return fmt.Errorf("the build stage declares the core step %s; `govard deploy build` runs shell tasks only", step.ID)
		}
		stepVars := vars.Set("release", "").SetPath("release_path", output)
		expanded, err := stepVars.Expand(step.Command)
		if err != nil {
			return fmt.Errorf("expand %s: %w", step.ID, err)
		}
		fmt.Fprintf(out, "  → %s\n", step.ID)
		if _, err := runner.Run(ctx, expanded, RunOptions{Dir: output, Timeout: buildStepTimeout(req.Options)}); err != nil {
			return fmt.Errorf("%s: %w", step.ID, err)
		}
	}
	return nil
}

func buildStepTimeout(opts Options) time.Duration {
	if opts.CommandTimeout > 0 {
		return opts.CommandTimeout
	}
	return DefaultCommandTimeout
}

// probeLocalPHP records the PHP the artifact was built for. It is what the
// deploy job compares the target against without needing PHP of its own.
func probeLocalPHP(ctx context.Context, runner Runner, opts Options, workDir string) string {
	phpBin := settingsString(opts.Settings, "php_bin")
	if phpBin == "" {
		phpBin = "php"
	}
	result, err := runner.Run(ctx, phpBin+" -r 'echo PHP_VERSION;'", RunOptions{Dir: workDir, Timeout: shortCommandTimeout})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(result.Stdout)
}

// projectUsesPHP reports whether a built tree is a Composer project. The
// manifest is what the deploy job's parity gate reads, and it must only record a
// PHP version for a project that actually has one.
func projectUsesPHP(root string) bool {
	for _, name := range []string{"composer.json", "composer.lock"} {
		if info, err := os.Stat(filepath.Join(root, name)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func revisionOrShort(revision string) string {
	if revision == "" {
		return "unknown"
	}
	if len(revision) > 8 {
		return revision[:8]
	}
	return revision
}

// CoreArtifact receives a prebuilt artifact into the release directory.
//
// It is the artifact branch of the build stage: the build tasks are skipped by
// the plan in this mode, and this step is what puts their output there instead.
// Everything it needs is local (the artifact directory) plus one transfer, so
// the job that runs it needs no PHP, no Composer and no Node.
func CoreArtifact(ctx context.Context, sc *StepContext) error {
	// Belt and braces: a plan built without ForBuildMode still must not try to
	// upload an artifact a server build never made.
	if sc.Opts.Build != BuildArtifact {
		return nil
	}

	artifactDir := strings.TrimSpace(sc.Opts.ArtifactDir)
	if artifactDir == "" {
		return fmt.Errorf("artifact mode needs an artifact directory: pass --artifact-dir <dir> or set deploy.artifact_dir")
	}
	manifest, err := ReadManifest(artifactDir)
	if err != nil {
		return err
	}
	if manifest.FileCount == 0 && len(manifest.Files) == 0 {
		return fmt.Errorf("the artifact at %s holds no files; run `govard deploy build` first", artifactDir)
	}
	// The hashes are checked here, on the machine the artifact is on, before
	// anything is transferred: a truncated cache download or a half-written
	// build is otherwise indistinguishable from a good one until the site serves
	// it.
	if err := manifest.Verify(artifactDir); err != nil {
		return fmt.Errorf("the artifact at %s does not match its own manifest: %w", artifactDir, err)
	}

	revision := strings.TrimSpace(sc.Opts.Revision)
	if revision == "" {
		revision = strings.TrimSpace(sc.Opts.Tag)
	}
	if manifest.Revision != "" && revision != "" && manifest.Revision != revision {
		return fmt.Errorf(
			"the artifact at %s was built for revision %s but %s is being deployed; rebuild it with `govard deploy build --revision %s` or deploy the revision it was built for",
			artifactDir, manifest.Revision, revision, manifest.Revision)
	}

	releasePath := releasePathOf(sc)
	if releasePath == "" {
		return fmt.Errorf("release path is unknown; deploy:release must run first")
	}

	if _, err := sc.Runner.Run(ctx, "mkdir -p "+Shell(releasePath), RunOptions{Timeout: shortCommandTimeout, Out: sc.Live}); err != nil {
		return fmt.Errorf("prepare the release directory for the artifact: %w", err)
	}
	// The transfer runs on the machine that invoked govard, so it goes through
	// a local runner whatever the target is: for a remote target that command
	// is rsync, for a local one it is a copy.
	command := ArtifactUploadCommand(sc.Host, artifactDir, releasePath, len(progressArgs(sc)) > 0)
	if _, err := (LocalRunner{}).Run(ctx, command, RunOptions{Timeout: buildStepTimeout(sc.Opts), Out: sc.Live}); err != nil {
		return fmt.Errorf("upload the artifact to %s: %w", releasePath, err)
	}

	payload, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode the artifact manifest: %w", err)
	}
	if err := writeTargetFile(ctx, sc.Host, path.Join(releasePath, ".dep", ArtifactRecordName), payload); err != nil {
		return err
	}

	if sc.Release != nil {
		sc.Release.Build = BuildRecord{
			Mode:             BuildArtifact,
			ArtifactRevision: manifest.Revision,
			ArtifactFiles:    manifest.FileCount,
			ArtifactBytes:    manifest.TotalBytes,
		}
	}
	return nil
}

// ArtifactUploadCommand returns the command that copies an artifact root onto a
// target path.
//
// A local target is the same machine, where `cp` is enough and where requiring
// rsync would add a dependency the pipeline does not otherwise have. A remote
// target gets rsync, with the manifest excluded: it is written separately to
// `.dep/` so the release keeps one copy of it, next to its record.
func ArtifactUploadCommand(host Host, source, destination string, progress bool) string {
	root := strings.TrimRight(source, "/")
	if host.Local || strings.TrimSpace(host.Remote.Host) == "" {
		return fmt.Sprintf("cp -a %s %s && rm -f %s",
			conventions.ShellQuote(root+"/."),
			conventions.ShellQuote(destination),
			conventions.ShellQuote(path.Join(destination, ArtifactManifestName)))
	}

	sshArgs := append([]string{"ssh"}, remote.BuildSSHArgs(host.Name, host.Remote, false, false)...)
	target := remote.RemoteTarget(host.Remote) + ":" + destination + "/"
	// Progress only when a terminal is watching: rsync cannot redraw its progress
	// line without one, and an artifact upload is exactly the transfer that takes
	// long enough to want it (see rsyncProgressArgs).
	progressFlag := ""
	if progress {
		progressFlag = "--info=progress2 "
	}
	return fmt.Sprintf("rsync -az --numeric-ids "+progressFlag+"--exclude=%s -e %s %s %s",
		conventions.ShellQuote(ArtifactManifestName),
		conventions.ShellQuote(strings.Join(sshArgs, " ")),
		conventions.ShellQuote(root+"/"),
		conventions.ShellQuote(target))
}

// ArtifactUploadCommandForTest exposes ArtifactUploadCommand to the tests/ package.
func ArtifactUploadCommandForTest(host Host, source, destination string, progress bool) string {
	return ArtifactUploadCommand(host, source, destination, progress)
}

// writeTargetFile writes a file on the target atomically: a heredoc into a
// temporary path followed by a rename. A reader must never observe a
// half-written record, and the payload is a single line so a quoted delimiter
// cannot be terminated early by its content.
func writeTargetFile(ctx context.Context, host Host, target string, payload []byte) error {
	temporary := target + ".tmp"
	// The payload is the command's standard input, never part of its argv. A
	// record carrying the captured output of a chatty step used to be embedded in
	// the command text, and a payload past the kernel's argument limit made the
	// command unlaunchable — the deploy then failed to write the very record that
	// says what happened. Standard input has no such limit, and it also keeps the
	// payload out of the target's process list.
	command := fmt.Sprintf(
		"mkdir -p %s && cat > %s && mv %s %s",
		Shell(path.Dir(target)),
		Shell(temporary),
		Shell(temporary),
		Shell(target),
	)
	if _, err := host.Runner().Run(ctx, command, RunOptions{Timeout: shortCommandTimeout, Stdin: string(payload)}); err != nil {
		return fmt.Errorf("write %s on %s: %w", target, host.Name, err)
	}
	return nil
}
