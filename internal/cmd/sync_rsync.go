package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"govard/internal/engine"
	"govard/internal/engine/remote"

	"github.com/pterm/pterm"
)

func buildRsyncForEndpoints(
	source syncEndpoint,
	destination syncEndpoint,
	sourcePath string,
	destinationPath string,
	deleteFiles bool,
	resume bool,
	noCompress bool,
	includePatterns []string,
	excludePatterns []string,
) (*exec.Cmd, string, error) {
	if source.IsLocal == destination.IsLocal {
		return nil, "", fmt.Errorf("synchronization only supports transfers between local and remote environments")
	}

	if source.IsLocal {
		cmd := remote.BuildRsyncCommand(
			destination.Name,
			ensureTrailingSlash(sourcePath),
			remote.RemoteTarget(destination.RemoteCfg)+":"+ensureTrailingSlash(destinationPath),
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
		remote.RemoteTarget(source.RemoteCfg)+":"+ensureTrailingSlash(sourcePath),
		ensureTrailingSlash(destinationPath),
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

func buildDatabaseSyncAction(config engine.Config, source syncEndpoint, destination syncEndpoint) (string, func() error, error) {
	localDBContainer := fmt.Sprintf("%s-db-1", config.ProjectName)
	localCredentials := resolveLocalDBCredentials(localDBContainer)

	switch {
	case !source.IsLocal && destination.IsLocal:
		desc := fmt.Sprintf("ssh %s \"mysqldump ...\" | docker exec -i %s mysql ...", remote.RemoteTarget(source.RemoteCfg), localDBContainer)
		return desc, func() error {
			remoteCredentials, probeErr := resolveRemoteDBCredentials(config, source.Name, source.RemoteCfg)
			if probeErr != nil {
				pterm.Warning.Println(formatRemoteDBProbeWarning(source.Name, probeErr))
			}
			dumpCmd := remote.BuildSSHExecCommand(source.Name, source.RemoteCfg, true, buildRemoteMySQLDumpCommandString(remoteCredentials, false))
			importCmd := buildLocalDBImportCommand(localDBContainer, localCredentials)
			return RunDumpToImport(dumpCmd, importCmd, true, os.Stdout, os.Stderr)
		}, nil
	case source.IsLocal && !destination.IsLocal:
		desc := fmt.Sprintf("docker exec -i %s mysqldump ... | ssh %s \"mysql ...\"", localDBContainer, remote.RemoteTarget(destination.RemoteCfg))
		return desc, func() error {
			dumpCmd := buildLocalDBDumpCommand(localDBContainer, localCredentials, false)
			remoteCredentials, probeErr := resolveRemoteDBCredentials(config, destination.Name, destination.RemoteCfg)
			if probeErr != nil {
				pterm.Warning.Println(formatRemoteDBProbeWarning(destination.Name, probeErr))
			}
			importCmd := remote.BuildSSHExecCommand(destination.Name, destination.RemoteCfg, true, buildRemoteMySQLImportCommandString(remoteCredentials))
			return RunDumpToImport(dumpCmd, importCmd, true, os.Stdout, os.Stderr)
		}, nil
	default:
		return "", nil, fmt.Errorf("database synchronization only supports transfers between local and remote environments")
	}
}
