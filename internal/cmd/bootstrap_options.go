package cmd

import (
	"fmt"
	"strings"

	"govard/internal/frameworks"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type BootstrapRuntimeOptions struct {
	Source          string
	Clone           bool
	CodeOnly        bool
	Fresh           bool
	IncludeSample   bool
	DBImport        bool
	MediaSync       string
	ComposerInstall bool
	AdminCreate     bool
	StreamDB        bool
	SkipUp          bool
	MetaPackage     string
	MetaVersion     string
	// MetaVersionSource says where MetaVersion came from (one of the
	// BootstrapVersionSource constants), so the plan and the log can name it.
	MetaVersionSource string
	// MetaVersionIgnored is set when the framework_version in .govard.yml was not
	// a plain numeric version (a detected composer constraint such as ^11.31) and
	// was ignored; it names the value and why, for the warning and the plan.
	MetaVersionIgnored string
	DBDump             string
	HyvaInstall        bool
	HyvaToken          string
	MageUsername       string
	MagePassword       string
	AssumeYes          bool
	Plan               bool
	NoNoise            bool
	NoPII              bool
	DeleteSync         bool
	NoCompress         bool
	ExcludePatterns    []string
}

func resolveBootstrapOptions(cmd *cobra.Command, args []string) (BootstrapRuntimeOptions, error) {
	opts := BootstrapRuntimeOptions{
		Source:          normalizeBootstrapSource(bootstrapEnv),
		Clone:           bootstrapClone,
		CodeOnly:        bootstrapCodeOnly,
		Fresh:           bootstrapFresh,
		IncludeSample:   bootstrapIncludeSample,
		DBImport:        !bootstrapSkipDB,
		MediaSync:       resolveBootstrapMediaMode(),
		ComposerInstall: !bootstrapSkipComposer,
		AdminCreate:     !bootstrapSkipAdmin,
		StreamDB:        !bootstrapNoStreamDB,
		SkipUp:          bootstrapSkipUp,
		MetaPackage:     strings.TrimSpace(bootstrapMetaPackage),
		MetaVersion:     strings.TrimSpace(bootstrapFrameworkVersion),
		DBDump:          strings.TrimSpace(bootstrapDBDump),
		HyvaInstall:     bootstrapHyvaInstall,
		HyvaToken:       strings.TrimSpace(bootstrapHyvaToken),
		MageUsername:    strings.TrimSpace(bootstrapMageUsername),
		MagePassword:    strings.TrimSpace(bootstrapMagePassword),
		AssumeYes:       bootstrapAssumeYes,
		Plan:            bootstrapPlan,
		NoNoise:         bootstrapNoNoise,
		NoPII:           bootstrapNoPII,
		DeleteSync:      bootstrapDelete,
		NoCompress:      bootstrapNoCompress,
		ExcludePatterns: bootstrapExclude,
	}
	opts.MediaSync = resolveMediaModeFlagValue(cmd, opts.MediaSync, args)

	if opts.MetaPackage == "" {
		opts.MetaPackage = defaultBootstrapMetaPackage
	}
	if opts.HyvaToken == "" {
		opts.HyvaToken = defaultBootstrapHyvaToken
	}
	cloneFlagExplicit := false
	if cmd != nil {
		cloneFlagExplicit = cmd.Flags().Changed("clone")
	}

	if opts.Fresh && opts.Clone {
		if cloneFlagExplicit {
			return BootstrapRuntimeOptions{}, fmt.Errorf("--fresh and --clone cannot be used together")
		}
		opts.Clone = false
	}
	if opts.CodeOnly && !opts.Clone {
		return BootstrapRuntimeOptions{}, fmt.Errorf("--code-only requires --clone")
	}
	if opts.Fresh {
		opts.ComposerInstall = false
		opts.DBImport = false
		opts.MediaSync = ""
	}
	if opts.Clone && opts.CodeOnly {
		opts.DBImport = false
		opts.MediaSync = ""
	}

	return opts, nil
}

// Where a bootstrap's meta version came from.
const (
	BootstrapVersionSourceFlag   = "--framework-version"
	BootstrapVersionSourceConfig = "framework_version in .govard.yml"
)

// resolveBootstrapMetaVersion picks the version a bootstrap installs. An
// explicit --framework-version always wins. Otherwise a fresh install defaults
// to the framework_version already recorded in .govard.yml (written by
// `govard init --framework-version`), because that version was chosen together
// with the PHP the config carries, and the latest release may not resolve on it.
// With neither, the version is empty and the framework installs its latest. The
// config value goes through the same validation as the flag, so there is one set
// of rules for both; the framework registry supplies them, not a name switch.
//
// A config value that is not a plain numeric version is not an error: `govard
// init` records the raw detected composer constraint (^11.31, ~2.4.7, 2.4.*),
// and the user never passed a flag to blame. It is ignored, ignored says so, and
// the install falls back to the latest release.
func resolveBootstrapMetaVersion(framework, flagVersion, configVersion string, fresh bool) (version, source, ignored string, err error) {
	flagVersion = strings.TrimSpace(flagVersion)
	if flagVersion != "" {
		if err := validateBootstrapFrameworkVersion(framework, flagVersion); err != nil {
			return "", "", "", err
		}
		return flagVersion, BootstrapVersionSourceFlag, "", nil
	}
	configVersion = strings.TrimSpace(configVersion)
	if !fresh || configVersion == "" {
		return "", "", "", nil
	}
	if !plainNumericVersion(configVersion) {
		return "", "", fmt.Sprintf("framework_version %q in .govard.yml is not a plain numeric version (composer constraints such as ^11.31 are not usable here), so it was ignored", configVersion), nil
	}
	if err := validateBootstrapFrameworkVersion(framework, configVersion); err != nil {
		return "", "", "", fmt.Errorf("%s: %w", BootstrapVersionSourceConfig, err)
	}
	return configVersion, BootstrapVersionSourceConfig, "", nil
}

// ResolveBootstrapMetaVersionForTest exposes resolveBootstrapMetaVersion for tests in /tests.
func ResolveBootstrapMetaVersionForTest(framework, flagVersion, configVersion string, fresh bool) (string, string, string, error) {
	return resolveBootstrapMetaVersion(framework, flagVersion, configVersion, fresh)
}

// plainNumericVersion reports whether raw is digits separated by dots, with no
// constraint operator, wildcard or suffix.
func plainNumericVersion(raw string) bool {
	if raw == "" {
		return false
	}
	for _, segment := range strings.Split(raw, ".") {
		if segment == "" {
			return false
		}
		for _, r := range segment {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func validateBootstrapFrameworkVersion(framework string, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil
	}

	comparison, comparable := compareNumericDotVersions(version, "0.0.0")
	if !comparable || comparison < 0 {
		return fmt.Errorf("invalid --framework-version value %q (must be a numeric dotted version)", version)
	}
	if definition, ok := frameworks.Get(framework); ok && definition.MinimumBootstrapVersion != "" {
		comparison, _ = compareNumericDotVersions(version, definition.MinimumBootstrapVersion)
		if comparison < 0 {
			return fmt.Errorf("invalid --framework-version value %q (must be %s %s+)", version, definition.DisplayName, definition.MinimumBootstrapVersion)
		}
	}
	return nil
}

func normalizeBootstrapSource(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func DefaultBootstrapRuntimeOptionsForTest() BootstrapRuntimeOptions {
	return BootstrapRuntimeOptions{
		Source:          "",
		DBImport:        true,
		MediaSync:       MediaSyncOptimized,
		ComposerInstall: true,
		AdminCreate:     true,
		StreamDB:        true,
		MetaPackage:     defaultBootstrapMetaPackage,
		HyvaToken:       defaultBootstrapHyvaToken,
	}
}

// ResetBootstrapFlags resets all package-level bootstrap flag variables.
// Used primarily for testing to ensure a clean state between runs.
func ResetBootstrapFlags() {
	bootstrapClone = false
	bootstrapCodeOnly = false
	bootstrapFresh = false
	bootstrapIncludeSample = false
	bootstrapSkipDB = false
	bootstrapSkipMedia = false
	bootstrapSkipComposer = false
	bootstrapSkipAdmin = false
	bootstrapNoStreamDB = false
	bootstrapEnv = ""
	bootstrapFramework = ""
	bootstrapFrameworkVersion = ""
	bootstrapSkipUp = false
	bootstrapMetaPackage = defaultBootstrapMetaPackage
	bootstrapDBDump = ""
	bootstrapHyvaInstall = false
	bootstrapHyvaToken = defaultBootstrapHyvaToken
	bootstrapMageUsername = ""
	bootstrapMagePassword = ""
	bootstrapAssumeYes = false
	bootstrapPlan = false
	bootstrapNoNoise = false
	bootstrapNoPII = false
	bootstrapDelete = false
	bootstrapNoCompress = false
	bootstrapExclude = []string{}

	bootstrapCmd.Flags().VisitAll(func(flag *pflag.Flag) {
		// Avoid resetting slice/array flags via Set(DefValue) if the DefValue is "[]",
		// as it can cause the literal string "[]" to be appended to the variable.
		if sliceValue, ok := flag.Value.(pflag.SliceValue); ok && (flag.DefValue == "" || flag.DefValue == "[]") {
			_ = sliceValue.Replace([]string{})
		} else {
			_ = flag.Value.Set(flag.DefValue)
		}
		flag.Changed = false
	})
	bootstrapMediaModeFlag = ""
}

func resolveBootstrapMediaMode() string {
	if bootstrapSkipMedia {
		return ""
	}
	if bootstrapMediaModeFlag != "" {
		return bootstrapMediaModeFlag
	}
	return MediaSyncOptimized
}
