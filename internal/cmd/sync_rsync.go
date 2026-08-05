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
	return cmd, cmd.String(), nil
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

		desc := fmt.Sprintf("ssh %s \"%s\" | docker exec -i %s sh -lc \"%s\"", remote.RemoteTarget(source.RemoteCfg), dumpCmdStr, localDBContainer, importCmdStr)

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

		desc := fmt.Sprintf("docker exec -i %s sh -lc \"%s\" | ssh %s \"%s\"", localDBContainer, dumpCmdStr, remote.RemoteTarget(destination.RemoteCfg), importCmdStr)

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
