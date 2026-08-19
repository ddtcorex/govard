package types

import (
	"text/template"

	"govard/internal/engine"
	"govard/internal/engine/bootstrap"
	"govard/internal/engine/remote"
	"govard/internal/engine/tunnel"

	"github.com/spf13/cobra"
)

// Override represents an explicit change to an inherited definition value.
// Its zero value means inherit. Set applies the supplied value, while Clear
// explicitly applies T's zero value. The distinction is essential for function
// capabilities and scalar defaults, where a plain zero value is ambiguous.
type Override[T any] struct {
	set   bool
	value T
}

// Set returns an override that replaces an inherited value.
func Set[T any](value T) Override[T] {
	return Override[T]{set: true, value: value}
}

// Clear returns an override that deliberately removes an inherited value.
func Clear[T any]() Override[T] {
	return Override[T]{set: true}
}

func (o Override[T]) apply(target *T) {
	if o.set {
		*target = o.value
	}
}

// FrameworkPatch contains the first set of inheritable definition fields.
// Additional fields are added here as their consumers move to the resolved
// capability model. Keeping every change explicit prevents child frameworks
// from silently inheriting or clearing policy through Go zero values.
type FrameworkPatch struct {
	DisplayName                             Override[string]
	MigrationTypes                          Override[MigrationTypes]
	Config                                  Override[engine.FrameworkConfig]
	Manifest                                Override[engine.FrameworkManifestConfig]
	DefaultDBCredentials                    Override[DefaultDBCredentials]
	DefaultChownDirectories                 Override[[]string]
	PHPStanPaths                            Override[[]string]
	AuditLint                               Override[*AuditLintProfile]
	AuditTargetResolver                     Override[AuditTargetResolver]
	ComposerCodingStandard                  Override[ComposerCodingStandard]
	ComposerAuth                            Override[ComposerAuthRequirement]
	ToolCommands                            Override[[]ToolCommand]
	DefaultTestCommand                      Override[TestCommand]
	TestSuiteCommands                       Override[map[string]TestCommand]
	Detect                                  Override[engine.DetectionSpec]
	Bootstrap                               Override[BootstrapFactory]
	BaseURLManager                          Override[func() tunnel.BaseURLManager]
	SupportsBootstrap                       Override[bool]
	SupportsFreshInstall                    Override[bool]
	MinimumBootstrapVersion                 Override[string]
	DefaultFreshMetaPackage                 Override[string]
	PrepareComposer                         Override[func(engine.Config) error]
	RequiresComposerManifestForDumpAutoload Override[bool]
	FreshInstall                            Override[func(bootstrap.Options, string, bootstrap.CmdHelpers) error]
	FreshInstallNeedsDB                     Override[bool]
	FreshInstallNeedsDomain                 Override[bool]
	FreshInstallManagesOwnEnvUp             Override[bool]
	PreConfigureHook                        Override[func(bootstrap.Options, string, bootstrap.CmdHelpers) error]
	PostCloneHook                           Override[func(bootstrap.Options, string, bootstrap.CmdHelpers) error]
	IgnorePostCloneError                    Override[func(error, string) bool]
	PHPImageVariant                         Override[string]
	NodeImageFlavor                         Override[string]
	VarnishTemplateFramework                Override[string]
	DBDriverCategory                        Override[string]
	Upgrade                                 Override[engine.UpgradeFunc]
	RunMappingAssetPreparer                 Override[engine.RunMappingAssetPreparer]
	TablePrefixDetector                     Override[engine.TablePrefixDetector]
	ResolveBootstrapTablePrefix             Override[func(string) (string, error)]
	BuildDeployLocalesQuery                 Override[func(string) string]
	BootstrapPlanSteps                      Override[func(bool) []BootstrapPlanStep]
	EnableVarnishOnInit                     Override[bool]
	VersionProfileResolver                  Override[engine.VersionProfileResolver]
	TemplateFuncs                           Override[template.FuncMap]
	ProbeRemoteDB                           Override[func(string, engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error)]
	ProbeRemoteBootstrapMetadata            Override[func(string, engine.RemoteConfig) (remote.RemoteDatabaseMetadata, error)]
	RemoteDBUsesConfigTablePrefix           Override[bool]
	DefaultAdminPath                        Override[string]
	ResolveRemoteAdminPath                  Override[func(string, engine.RemoteConfig) (string, error)]
	DetectLocalAdminMetadata                Override[func(string) (string, string)]
	BuildLocalAdminSettingsQuery            Override[func(string) string]
	ResolveLocalAdminURL                    Override[func(string, string, map[string]string) string]
	PostEnvironmentUp                       Override[func(engine.Config) error]
	ConfigureAfterProfileShift              Override[func(engine.Config, *engine.ProfileShiftInfo) error]
	PostSync                                Override[func(engine.Config) error]
	UnblockSearchIndex                      Override[func(engine.Config) error]
	BuildSearchHostFixSQL                   Override[func(engine.Config) string]
	BootstrapEnvironmentPath                Override[string]
	BootstrapEnvironmentMetadataKey         Override[string]
	RenderBootstrapEnvironment              Override[func(string, BootstrapEnvironmentDatabase, string) string]
	AutoConfigure                           Override[func(*cobra.Command, engine.Config) error]
}

// FrameworkSpec is a partial framework declaration. Root specs supply a full
// Definition. Child specs identify a Parent, supply their identity in
// Definition (Name and Aliases), and state every inherited-field delta in Patch.
type FrameworkSpec struct {
	Parent     string
	Definition FrameworkDefinition
	Patch      FrameworkPatch
}

// Resolve merges this spec onto parent. Callers must validate Parent and graph
// ordering before calling Resolve.
func (s FrameworkSpec) Resolve(parent FrameworkDefinition) FrameworkDefinition {
	if s.Parent == "" {
		return cloneDefinition(s.Definition)
	}

	resolved := cloneDefinition(parent)
	resolved.Name = s.Definition.Name
	resolved.Aliases = cloneStrings(s.Definition.Aliases)
	resolved.Parent = s.Parent
	s.Patch.DisplayName.apply(&resolved.DisplayName)
	s.Patch.MigrationTypes.apply(&resolved.MigrationTypes)
	s.Patch.Config.apply(&resolved.Config)
	s.Patch.Manifest.apply(&resolved.Manifest)
	s.Patch.DefaultDBCredentials.apply(&resolved.DefaultDBCredentials)
	s.Patch.DefaultChownDirectories.apply(&resolved.DefaultChownDirectories)
	s.Patch.PHPImageVariant.apply(&resolved.PHPImageVariant)
	s.Patch.NodeImageFlavor.apply(&resolved.NodeImageFlavor)
	s.Patch.VarnishTemplateFramework.apply(&resolved.VarnishTemplateFramework)
	s.Patch.PHPStanPaths.apply(&resolved.PHPStanPaths)
	s.Patch.AuditLint.apply(&resolved.AuditLint)
	s.Patch.AuditTargetResolver.apply(&resolved.AuditTargetResolver)
	s.Patch.ComposerCodingStandard.apply(&resolved.ComposerCodingStandard)
	s.Patch.ComposerAuth.apply(&resolved.ComposerAuth)
	s.Patch.ToolCommands.apply(&resolved.ToolCommands)
	s.Patch.DefaultTestCommand.apply(&resolved.DefaultTestCommand)
	s.Patch.TestSuiteCommands.apply(&resolved.TestSuiteCommands)
	s.Patch.Detect.apply(&resolved.Detect)
	s.Patch.Bootstrap.apply(&resolved.Bootstrap)
	s.Patch.BaseURLManager.apply(&resolved.BaseURLManager)
	s.Patch.SupportsBootstrap.apply(&resolved.SupportsBootstrap)
	s.Patch.SupportsFreshInstall.apply(&resolved.SupportsFreshInstall)
	s.Patch.MinimumBootstrapVersion.apply(&resolved.MinimumBootstrapVersion)
	s.Patch.DefaultFreshMetaPackage.apply(&resolved.DefaultFreshMetaPackage)
	s.Patch.PrepareComposer.apply(&resolved.PrepareComposer)
	s.Patch.RequiresComposerManifestForDumpAutoload.apply(&resolved.RequiresComposerManifestForDumpAutoload)
	s.Patch.FreshInstall.apply(&resolved.FreshInstall)
	s.Patch.FreshInstallNeedsDB.apply(&resolved.FreshInstallNeedsDB)
	s.Patch.FreshInstallNeedsDomain.apply(&resolved.FreshInstallNeedsDomain)
	s.Patch.FreshInstallManagesOwnEnvUp.apply(&resolved.FreshInstallManagesOwnEnvUp)
	s.Patch.PreConfigureHook.apply(&resolved.PreConfigureHook)
	s.Patch.PostCloneHook.apply(&resolved.PostCloneHook)
	s.Patch.IgnorePostCloneError.apply(&resolved.IgnorePostCloneError)
	s.Patch.DBDriverCategory.apply(&resolved.DBDriverCategory)
	s.Patch.Upgrade.apply(&resolved.Upgrade)
	s.Patch.RunMappingAssetPreparer.apply(&resolved.RunMappingAssetPreparer)
	s.Patch.TablePrefixDetector.apply(&resolved.TablePrefixDetector)
	s.Patch.ResolveBootstrapTablePrefix.apply(&resolved.ResolveBootstrapTablePrefix)
	s.Patch.BuildDeployLocalesQuery.apply(&resolved.BuildDeployLocalesQuery)
	s.Patch.BootstrapPlanSteps.apply(&resolved.BootstrapPlanSteps)
	s.Patch.EnableVarnishOnInit.apply(&resolved.EnableVarnishOnInit)
	s.Patch.VersionProfileResolver.apply(&resolved.VersionProfileResolver)
	s.Patch.TemplateFuncs.apply(&resolved.TemplateFuncs)
	s.Patch.ProbeRemoteDB.apply(&resolved.ProbeRemoteDB)
	s.Patch.ProbeRemoteBootstrapMetadata.apply(&resolved.ProbeRemoteBootstrapMetadata)
	s.Patch.RemoteDBUsesConfigTablePrefix.apply(&resolved.RemoteDBUsesConfigTablePrefix)
	s.Patch.DefaultAdminPath.apply(&resolved.DefaultAdminPath)
	s.Patch.ResolveRemoteAdminPath.apply(&resolved.ResolveRemoteAdminPath)
	s.Patch.DetectLocalAdminMetadata.apply(&resolved.DetectLocalAdminMetadata)
	s.Patch.BuildLocalAdminSettingsQuery.apply(&resolved.BuildLocalAdminSettingsQuery)
	s.Patch.ResolveLocalAdminURL.apply(&resolved.ResolveLocalAdminURL)
	s.Patch.PostEnvironmentUp.apply(&resolved.PostEnvironmentUp)
	s.Patch.ConfigureAfterProfileShift.apply(&resolved.ConfigureAfterProfileShift)
	s.Patch.PostSync.apply(&resolved.PostSync)
	s.Patch.UnblockSearchIndex.apply(&resolved.UnblockSearchIndex)
	s.Patch.BuildSearchHostFixSQL.apply(&resolved.BuildSearchHostFixSQL)
	s.Patch.BootstrapEnvironmentPath.apply(&resolved.BootstrapEnvironmentPath)
	s.Patch.BootstrapEnvironmentMetadataKey.apply(&resolved.BootstrapEnvironmentMetadataKey)
	s.Patch.RenderBootstrapEnvironment.apply(&resolved.RenderBootstrapEnvironment)
	s.Patch.AutoConfigure.apply(&resolved.AutoConfigure)
	return cloneDefinition(resolved)
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneDefinition(def FrameworkDefinition) FrameworkDefinition {
	cloned := def
	cloned.Aliases = cloneStrings(def.Aliases)
	cloned.MigrationTypes.DDEV = cloneStrings(def.MigrationTypes.DDEV)
	cloned.MigrationTypes.Warden = cloneStrings(def.MigrationTypes.Warden)
	cloned.PHPStanPaths = cloneStrings(def.PHPStanPaths)
	cloned.AuditLint = cloneAuditLintProfile(def.AuditLint)
	cloned.DefaultChownDirectories = cloneStrings(def.DefaultChownDirectories)
	cloned.ToolCommands = cloneToolCommands(def.ToolCommands)
	cloned.DefaultTestCommand.Args = cloneStrings(def.DefaultTestCommand.Args)
	cloned.TestSuiteCommands = cloneTestSuiteCommands(def.TestSuiteCommands)
	cloned.Config.Includes = cloneStrings(def.Config.Includes)
	cloned.Manifest.Ignored = cloneStrings(def.Manifest.Ignored)
	cloned.Manifest.Sensitive = cloneStrings(def.Manifest.Sensitive)
	cloned.Manifest.Paths.WebRootCandidates = append([]engine.FrameworkWebRootCandidate(nil), def.Manifest.Paths.WebRootCandidates...)
	cloned.Manifest.Sync.NoiseExcludes = cloneStrings(def.Manifest.Sync.NoiseExcludes)
	cloned.Manifest.Sync.MediaExcludes.NonAll = cloneStrings(def.Manifest.Sync.MediaExcludes.NonAll)
	cloned.Manifest.Sync.MediaExcludes.Optimized = cloneStrings(def.Manifest.Sync.MediaExcludes.Optimized)
	cloned.Manifest.Sync.MediaExcludes.Minimal = cloneStrings(def.Manifest.Sync.MediaExcludes.Minimal)
	cloned.Detect.ComposerPackages = cloneStrings(def.Detect.ComposerPackages)
	cloned.Detect.PackageJSONDeps = cloneStrings(def.Detect.PackageJSONDeps)
	cloned.Detect.AuthJSONHosts = cloneStrings(def.Detect.AuthJSONHosts)
	cloned.Detect.FilePaths = cloneStrings(def.Detect.FilePaths)
	if def.TemplateFuncs != nil {
		cloned.TemplateFuncs = make(template.FuncMap, len(def.TemplateFuncs))
		for name, fn := range def.TemplateFuncs {
			cloned.TemplateFuncs[name] = fn
		}
	}
	return cloned
}

func cloneToolCommands(commands []ToolCommand) []ToolCommand {
	if commands == nil {
		return nil
	}
	cloned := make([]ToolCommand, len(commands))
	for index, command := range commands {
		cloned[index] = command
		cloned[index].Aliases = cloneStrings(command.Aliases)
		cloned[index].PrependArgs = cloneStrings(command.PrependArgs)
	}
	return cloned
}

func cloneTestSuiteCommands(commands map[string]TestCommand) map[string]TestCommand {
	if commands == nil {
		return nil
	}
	cloned := make(map[string]TestCommand, len(commands))
	for name, command := range commands {
		command.Args = cloneStrings(command.Args)
		cloned[name] = command
	}
	return cloned
}

// CloneDefinition returns a deep-enough copy for registry callers. Functions
// remain shared because they are immutable values; every slice and map exposed
// by the current definition contract receives its own backing storage.
func CloneDefinition(def FrameworkDefinition) FrameworkDefinition {
	return cloneDefinition(def)
}
