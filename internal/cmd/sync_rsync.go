package cmd

import (
	"fmt"
	"govard/internal/conventions"
	"os"
	"os/exec"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
)

func buildRsyncForEndpoints(
	source SyncEndpoint,
	destination SyncEndpoint,
	sourcePath string,
	destinationPath string,
	isDir bool,
	deleteFiles bool,
	resume bool,
	noCompress bool,
	includePatterns []string,
	excludePatterns []string,
) (*exec.Cmd, string, error) {
	if source.IsLocal == destination.IsLocal {
		return nil, "", fmt.Errorf("synchronization only supports transfers between local and remote environments")
	}

	if isDir {
		sourcePath = ensureTrailingSlash(sourcePath)
		destinationPath = ensureTrailingSlash(destinationPath)
	}

	if source.IsLocal {
		cmd := remote.BuildRsyncCommand(
			destination.Name,
			sourcePath,
			remote.RemoteTarget(destination.RemoteCfg)+":"+remote.QuoteRemotePath(destinationPath),
			destination.RemoteCfg,
			deleteFiles,
			resume,
			noCompress,
			includePatterns,
			excludePatterns,
		)
		return cmd, cmd.String(), nil
	}

	cmd := remote.BuildRsyncCommand(
		source.Name,
		remote.RemoteTarget(source.RemoteCfg)+":"+remote.QuoteRemotePath(sourcePath),
		destinationPath,
		source.RemoteCfg,
		deleteFiles,
		resume,
		noCompress,
		includePatterns,
		excludePatterns,
	)
	withPullSymlinkPolicy(cmd)
	return cmd, cmd.String(), nil
}

// syncResolveSymlinks is set from `sync --resolve-symlinks` and read when a pull
// is built. It is off by default on purpose: following a link that points
// outside the synced tree copies whatever that link names on the remote.
var syncResolveSymlinks bool

// SetSyncResolveSymlinksForTest sets the opt-in for tests.
func SetSyncResolveSymlinksForTest(v bool) { syncResolveSymlinks = v }

// withPullSymlinkPolicy decides how a pull treats symlinks that point outside
// the transferred tree (for example a release file linked into a shared
// directory).
//
// By default the receiver ignores them (--safe-links): archive mode would
// otherwise copy such a link as a symlink whose target does not exist locally,
// replacing a real local file with a dangling one. Ignoring leaves the local
// file untouched and never reads anything outside the synced path.
//
// With --resolve-symlinks the content is copied instead
// (--copy-unsafe-links). That follows any link that leaves the tree, so it can
// read files elsewhere on the remote; it is for known links only. Links that
// stay inside the tree are preserved either way, and pushes are unchanged so a
// deploy layout on the remote is not flattened.
func withPullSymlinkPolicy(cmd *exec.Cmd) {
	flag := "--safe-links"
	if syncResolveSymlinks {
		flag = "--copy-unsafe-links"
	}
	// Insert after the mode flag (args[1]) so the argv still starts with it.
	if len(cmd.Args) < 2 {
		return
	}
	args := append([]string{}, cmd.Args[:2]...)
	args = append(args, flag)
	cmd.Args = append(args, cmd.Args[2:]...)
}

// BuildRsyncForEndpointsForTest exposes the rsync builder for tests.
func BuildRsyncForEndpointsForTest(source SyncEndpoint, destination SyncEndpoint, sourcePath string, destinationPath string, isDir bool) (*exec.Cmd, string, error) {
	return buildRsyncForEndpoints(source, destination, sourcePath, destinationPath, isDir, false, false, false, nil, nil)
}

func ensureTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	}
	return path + "/"
}

func buildDatabaseSyncAction(config engine.Config, source SyncEndpoint, destination SyncEndpoint, noNoise bool, noPII bool) (string, func() error, error) {
	localDBContainer := fmt.Sprintf("%s%s", config.ProjectName, conventions.DBSuffix)
	localCredentials := resolveLocalDBCredentials(config, localDBContainer)

	switch {
	case !source.IsLocal && destination.IsLocal:
		remoteCredentials, probeErr := resolveRemoteDBCredentials(config, source.Name, source.RemoteCfg)
		if probeErr != nil {
			return "", nil, fmt.Errorf("cannot sync database: %s", formatRemoteDBProbeWarning(source.Name, probeErr))
		}
		dumpCmdStr := buildRemoteMySQLDumpCommandString(remoteCredentials, noNoise, noPII, config.Framework, true)
		importCmdStr := buildLocalMySQLClientCommandScript(localCredentials, true)

		// The description is shown to people (plan, confirmation), so it is
		// built from redacted credentials; only the closure below runs the
		// real command.
		displayDumpCmdStr := buildRemoteMySQLDumpCommandString(remoteCredentials.forDisplay(), noNoise, noPII, config.Framework, true)
		desc := fmt.Sprintf("ssh %s \"%s\" | docker exec -i %s sh -lc \"%s\"", remote.RemoteTarget(source.RemoteCfg), displayDumpCmdStr, localDBContainer, importCmdStr)

		return desc, func() error {
			// The local container is the import target here, so it must be up
			// before we start streaming into it - otherwise `docker exec`
			// against it can behave unpredictably (e.g. hang) instead of
			// failing fast with a clear error (see runStreamDBImport/
			// buildDBImportCommand, which already guard this for `db import`).
			if err := ensureLocalDBRunning(localDBContainer); err != nil {
				return err
			}

			spinner, _ := pterm.DefaultSpinner.Start("Fetching remote database size...")
			totalSize, _ := GetDatabaseSize(config, source.Name, source.RemoteCfg, remoteCredentials, noNoise, noPII)
			spinner.Success()

			dumpCmd := remote.BuildSSHExecCommand(source.Name, source.RemoteCfg, true, dumpCmdStr)
			importCmd := buildLocalDBImportCommand(localDBContainer, localCredentials)
			poller := &finalizePoller{config: config, remoteName: "local", credentials: localCredentials, noNoise: noNoise, noPII: noPII}
			return RunDumpToImportWithProgress(dumpCmd, importCmd, totalSize, true, os.Stdout, os.Stderr, poller.size)
		}, nil
	case source.IsLocal && !destination.IsLocal:
		remoteCredentials, probeErr := resolveRemoteDBCredentials(config, destination.Name, destination.RemoteCfg)
		if probeErr != nil {
			return "", nil, fmt.Errorf("cannot sync database: %s", formatRemoteDBProbeWarning(destination.Name, probeErr))
		}
		dumpCmdStr := buildLocalMySQLDumpCommandScript(localCredentials, noNoise, noPII, config.Framework)
		importCmdStr := buildRemoteMySQLImportCommandString(remoteCredentials)

		// Display-only form, see the remote-to-local branch above.
		displayImportCmdStr := buildRemoteMySQLImportCommandString(remoteCredentials.forDisplay())
		desc := fmt.Sprintf("docker exec -i %s sh -lc \"%s\" | ssh %s \"%s\"", localDBContainer, dumpCmdStr, remote.RemoteTarget(destination.RemoteCfg), displayImportCmdStr)

		return desc, func() error {
			// The local container is the dump source here, so it must be up
			// before we start reading from it - see the comment in the
			// remote-to-local branch above for why this matters.
			if err := ensureLocalDBRunning(localDBContainer); err != nil {
				return err
			}

			spinner, _ := pterm.DefaultSpinner.Start("Fetching local database size...")
			totalSize, _ := GetDatabaseSize(config, "local", engine.RemoteConfig{}, localCredentials, noNoise, noPII)
			spinner.Success()

			dumpCmd := buildLocalDBDumpCommand(localDBContainer, localCredentials, noNoise, noPII, config.Framework)
			importCmd := remote.BuildSSHExecCommand(destination.Name, destination.RemoteCfg, true, importCmdStr)
			poller := &finalizePoller{config: config, remoteName: destination.Name, remoteCfg: destination.RemoteCfg, credentials: remoteCredentials, noNoise: noNoise, noPII: noPII}
			return RunDumpToImportWithProgress(dumpCmd, importCmd, totalSize, true, os.Stdout, os.Stderr, poller.size)
		}, nil
	default:
		return "", nil, fmt.Errorf("database synchronization only supports transfers between local and remote environments")
	}
}
