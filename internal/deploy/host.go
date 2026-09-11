package deploy

import (
	"path"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

// Host is the deployment target. It is either the local machine (`Local`, used
// for rehearsals and the hermetic tests) or an SSH remote described by the
// project's `remotes` configuration.
type Host struct {
	Name        string
	Local       bool
	Remote      engine.RemoteConfig
	DeployPath  string
	CurrentPath string
	Repository  string

	// runner overrides the default transport. The executor always goes through
	// Runner(), which is what lets the same pipeline run over SSH in production
	// and over a local shell in tests.
	runner Runner
}

// Runner returns the transport for this host.
func (h Host) Runner() Runner {
	if h.runner != nil {
		return h.runner
	}
	return SSHRunner{RemoteName: h.Name, Config: h.Remote}
}

// WithRunner returns a copy of the host bound to a specific transport.
func (h Host) WithRunner(runner Runner) Host {
	h.runner = runner
	return h
}

// DeployerLockPath is where the other deploy tool keeps its lock. Govard never
// writes it, but it refuses to run while it is held.
func (h Host) DeployerLockPath() string { return path.Join(h.DepPath(), "deploy.lock") }

// DepPath is govard's own state directory inside the deploy path.
func (h Host) DepPath() string { return path.Join(h.DeployPath, ".dep") }

// ReleasesPath holds one directory per release.
func (h Host) ReleasesPath() string { return path.Join(h.DeployPath, "releases") }

// SharedPath holds files and directories shared between releases.
func (h Host) SharedPath() string { return path.Join(h.DeployPath, "shared") }

// RepoPath is the bare mirror used to materialise a revision without cloning.
func (h Host) RepoPath() string { return path.Join(h.DeployPath, "repo.git") }

// LockPath is the govard lock directory. It is a directory so acquisition can
// be an atomic mkdir instead of a read-then-write race.
func (h Host) LockPath() string { return path.Join(h.DepPath(), "govard.lock") }

// LockOwnerPath is the JSON file describing the current lock holder.
func (h Host) LockOwnerPath() string { return path.Join(h.LockPath(), "owner.json") }

// HistoryPath is the append-only deploy history.
func (h Host) HistoryPath() string { return path.Join(h.DepPath(), "history.jsonl") }

// ReleasePath is the directory of one release.
func (h Host) ReleasePath(release string) string {
	return path.Join(h.ReleasesPath(), release)
}

// ReleaseRecordPath is the govard record of one release.
func (h Host) ReleaseRecordPath(release string) string {
	return path.Join(h.ReleasePath(release), ".dep", "release.json")
}

// SharedBackupPath is where a pre-migration database dump for a release lives.
func (h Host) SharedBackupPath(release string) string {
	return path.Join(h.SharedPath(), "backups", "deploy", release)
}

// Shell returns a path quoted for the target shell. A path under `~` keeps a
// `$HOME/` prefix, because a tilde inside single quotes is not expanded.
func Shell(p string) string { return remote.QuoteRemotePath(p) }

// HostForTest builds a local host rooted at deployPath. Tests use it to run the
// real pipeline against a temporary directory.
func HostForTest(deployPath string, runner Runner) Host {
	host := Host{
		Name:        "local",
		Local:       true,
		DeployPath:  deployPath,
		CurrentPath: path.Join(deployPath, "current"),
		runner:      runner,
	}
	return host
}

// HostForConfig builds the host for one configured remote, resolving the deploy
// path from configuration and the current path from the remote's `path`.
func HostForConfig(cfg engine.Config, remoteName string, opts Options) (Host, error) {
	name := strings.ToLower(strings.TrimSpace(remoteName))
	remoteCfg, ok := cfg.Remotes[name]
	if !ok {
		return Host{}, errUnknownRemote(name, cfg)
	}
	host := Host{
		Name:        name,
		Local:       remoteCfg.Local,
		Remote:      remoteCfg,
		DeployPath:  remoteCfg.DeployPath,
		CurrentPath: remoteCfg.Path,
		Repository:  opts.Repository,
	}
	if host.Local {
		host.runner = LocalRunner{}
	}
	return host, nil
}

// HostForConfigForTest exposes HostForConfig to the tests/ package.
func HostForConfigForTest(cfg engine.Config, remoteName string, opts Options) (Host, error) {
	return HostForConfig(cfg, remoteName, opts)
}
