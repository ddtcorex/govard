package cmd

import (
	"context"
	"io"
	"time"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

type UpReadinessCheckForTest struct {
	Service        string
	ContainerName  string
	RequireHealthy bool
}

// RemoteDBCredentialsForTest is the observable subset of resolved remote DB
// credentials. It avoids exposing the command package's internal credential
// type while preserving the table-prefix contract in external tests.
type RemoteDBCredentialsForTest struct {
	Host        string
	Port        int
	Username    string
	Database    string
	TablePrefix string
}

// ProjectRemoteDBCredentialsForTest exposes framework-aware remote metadata
// projection without creating an SSH connection.
func ProjectRemoteDBCredentialsForTest(config engine.Config, metadata remote.RemoteDatabaseMetadata, useConfigTablePrefix bool) RemoteDBCredentialsForTest {
	credentials := projectRemoteDBCredentials(config, metadata, useConfigTablePrefix)
	return RemoteDBCredentialsForTest{
		Host:        credentials.Host,
		Port:        credentials.Port,
		Username:    credentials.Username,
		Database:    credentials.Database,
		TablePrefix: credentials.TablePrefix,
	}
}

type UpCrossProjectRefreshDependenciesForTest struct {
	GetRunningProjectNames     func(context.Context) ([]string, error)
	ReadProjectRegistryEntries func() ([]engine.ProjectRegistryEntry, error)
	LoadConfigFromDir          func(string, bool) (engine.Config, []string, error)
	RenderBlueprint            func(string, engine.Config) error
	RunCompose                 func(context.Context, engine.ComposeOptions) error
}

// FindWailsCLIForTest exposes Wails binary discovery for external tests.
func FindWailsCLIForTest() (string, error) {
	return findWailsCLI()
}

// DesktopBinaryArgsForTest exposes govard-desktop argument construction for tests.
func DesktopBinaryArgsForTest(background bool) []string {
	return buildDesktopBinaryArgs(background)
}

// FindDesktopBinaryForTest exposes desktop binary discovery for external tests.
func FindDesktopBinaryForTest() (string, error) {
	return findDesktopBinary()
}

// SetDesktopExecutablePathForTest overrides os.Executable usage for tests.
func SetDesktopExecutablePathForTest(fn func() (string, error)) func() {
	previous := desktopExecutablePath
	desktopExecutablePath = fn
	return func() {
		desktopExecutablePath = previous
	}
}

// SetDesktopLookPathForTest overrides exec.LookPath usage for tests.
func SetDesktopLookPathForTest(fn func(file string) (string, error)) func() {
	previous := desktopBinaryLookPath
	desktopBinaryLookPath = fn
	return func() {
		desktopBinaryLookPath = previous
	}
}

// DesktopProductionBuildTagsForTest exposes desktop production build tags.
func DesktopProductionBuildTagsForTest() string {
	return desktopBuildTags(true)
}

// BuildUpReadinessChecksForTest exposes startup readiness planning for tests.
func BuildUpReadinessChecksForTest(projectRoot string, config engine.Config) ([]UpReadinessCheckForTest, error) {
	checks, err := buildUpReadinessChecks(projectRoot, config)
	if err != nil {
		return nil, err
	}
	result := make([]UpReadinessCheckForTest, 0, len(checks))
	for _, check := range checks {
		result = append(result, UpReadinessCheckForTest{
			Service:        check.Service,
			ContainerName:  check.ContainerName,
			RequireHealthy: check.RequireHealthy,
		})
	}
	return result, nil
}

// WaitForUpRuntimeReadinessForTest exposes readiness waiting for tests.
func WaitForUpRuntimeReadinessForTest(projectRoot string, config engine.Config, timeout time.Duration) error {
	return waitForUpRuntimeReadiness(projectRoot, config, timeout)
}

// SetUpReadinessProbeRunnerForTest overrides the probe runner used by readiness checks.
func SetUpReadinessProbeRunnerForTest(fn func(containerName string, probeArgs []string) error) func() {
	previous := upReadinessProbeRunner
	if fn == nil {
		upReadinessProbeRunner = previous
		return func() {
			upReadinessProbeRunner = previous
		}
	}
	upReadinessProbeRunner = fn
	return func() {
		upReadinessProbeRunner = previous
	}
}

// SetUpContainerStateRunnerForTest overrides container state inspection during readiness checks.
func SetUpContainerStateRunnerForTest(fn func(containerName string) (string, error)) func() {
	previous := upContainerStateRunner
	if fn == nil {
		upContainerStateRunner = previous
		return func() {
			upContainerStateRunner = previous
		}
	}
	upContainerStateRunner = fn
	return func() {
		upContainerStateRunner = previous
	}
}

// SetUpReadinessProbeIntervalForTest overrides readiness retry intervals.
func SetUpReadinessProbeIntervalForTest(interval time.Duration) func() {
	previous := upReadinessProbeInterval
	upReadinessProbeInterval = interval
	return func() {
		upReadinessProbeInterval = previous
	}
}

// SetUpReadinessSleepForTest overrides readiness sleeping behavior.
func SetUpReadinessSleepForTest(fn func(time.Duration)) func() {
	previous := upReadinessSleep
	if fn == nil {
		upReadinessSleep = time.Sleep
		return func() {
			upReadinessSleep = previous
		}
	}
	upReadinessSleep = fn
	return func() {
		upReadinessSleep = previous
	}
}

// RefreshCrossProjectRuntimeHostsForTest exposes cross-project PHP runtime host refreshes for tests.
func RefreshCrossProjectRuntimeHostsForTest(ctx context.Context, currentProjectRoot string, currentConfig engine.Config) error {
	return refreshCrossProjectRuntimeHosts(ctx, currentProjectRoot, currentConfig, io.Discard, io.Discard)
}

// SetUpCrossProjectRefreshDependenciesForTest overrides cross-project runtime refresh dependencies.
func SetUpCrossProjectRefreshDependenciesForTest(deps UpCrossProjectRefreshDependenciesForTest) func() {
	previousGetRunningProjectNames := upRefreshRunningProjectNames
	previousReadProjectRegistryEntries := upRefreshReadProjectRegistryEntries
	previousLoadConfigFromDir := upRefreshLoadConfigFromDir
	previousRenderBlueprint := upRefreshRenderBlueprint
	previousRunCompose := upRefreshRunCompose

	if deps.GetRunningProjectNames != nil {
		upRefreshRunningProjectNames = deps.GetRunningProjectNames
	}
	if deps.ReadProjectRegistryEntries != nil {
		upRefreshReadProjectRegistryEntries = deps.ReadProjectRegistryEntries
	}
	if deps.LoadConfigFromDir != nil {
		upRefreshLoadConfigFromDir = deps.LoadConfigFromDir
	}
	if deps.RenderBlueprint != nil {
		upRefreshRenderBlueprint = deps.RenderBlueprint
	}
	if deps.RunCompose != nil {
		upRefreshRunCompose = deps.RunCompose
	}

	return func() {
		upRefreshRunningProjectNames = previousGetRunningProjectNames
		upRefreshReadProjectRegistryEntries = previousReadProjectRegistryEntries
		upRefreshLoadConfigFromDir = previousLoadConfigFromDir
		upRefreshRenderBlueprint = previousRenderBlueprint
		upRefreshRunCompose = previousRunCompose
	}
}
