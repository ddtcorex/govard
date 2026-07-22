package cmd

import (
	"fmt"
	"strings"

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
	DBDump          string
	HyvaInstall     bool
	HyvaToken       string
	MageUsername    string
	MagePassword    string
	AssumeYes       bool
	Plan            bool
	NoNoise         bool
	NoPII           bool
	DeleteSync      bool
	NoCompress      bool
	ExcludePatterns []string
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

func validateBootstrapFrameworkVersion(framework string, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil
	}

	comparison, comparable := compareNumericDotVersions(version, "0.0.0")
	if !comparable || comparison < 0 {
		return fmt.Errorf("invalid --framework-version value %q (must be a numeric dotted version)", version)
	}
	if strings.EqualFold(strings.TrimSpace(framework), "magento2") || strings.EqualFold(strings.TrimSpace(framework), "magento") {
		comparison, _ = compareNumericDotVersions(version, "2.0.0")
		if comparison < 0 {
			return fmt.Errorf("invalid --framework-version value %q (must be Magento 2.0.0+)", version)
		}
	}
	return nil
}

// ValidateBootstrapFrameworkVersionForTest exposes framework-aware fresh
// version validation for tests in /tests.
func ValidateBootstrapFrameworkVersionForTest(framework string, version string) error {
	return validateBootstrapFrameworkVersion(framework, version)
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
