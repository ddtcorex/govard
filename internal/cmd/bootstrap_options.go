package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type bootstrapRuntimeOptions struct {
	Source          string
	Clone           bool
	CodeOnly        bool
	Fresh           bool
	IncludeSample   bool
	DBImport        bool
	MediaSync       bool
	ComposerInstall bool
	AdminCreate     bool
	StreamDB        bool
	SkipUp          bool
	MetaPackage     string
	MetaVersion     string
	DBDump          string
	FixDeps         bool
	HyvaInstall     bool
	HyvaToken       string
	MageUsername    string
	MagePassword    string
	AssumeYes       bool
	IncludeProduct  bool
}

func resolveBootstrapOptions(cmd *cobra.Command) (bootstrapRuntimeOptions, error) {
	opts := bootstrapRuntimeOptions{
		Source:          normalizeBootstrapSource(bootstrapEnv),
		Clone:           bootstrapClone,
		CodeOnly:        bootstrapCodeOnly,
		Fresh:           bootstrapFresh,
		IncludeSample:   bootstrapIncludeSample,
		DBImport:        !bootstrapSkipDB,
		MediaSync:       !bootstrapSkipMedia,
		ComposerInstall: !bootstrapSkipComposer,
		AdminCreate:     !bootstrapSkipAdmin,
		StreamDB:        !bootstrapNoStreamDB,
		SkipUp:          bootstrapSkipUp,
		MetaPackage:     strings.TrimSpace(bootstrapMetaPackage),
		MetaVersion:     strings.TrimSpace(bootstrapFrameworkVersion),
		DBDump:          strings.TrimSpace(bootstrapDBDump),
		FixDeps:         bootstrapFixDeps,
		HyvaInstall:     bootstrapHyvaInstall,
		HyvaToken:       strings.TrimSpace(bootstrapHyvaToken),
		MageUsername:    strings.TrimSpace(bootstrapMageUsername),
		MagePassword:    strings.TrimSpace(bootstrapMagePassword),
		AssumeYes:       bootstrapAssumeYes,
		IncludeProduct:  bootstrapIncludeProduct,
	}

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

	if !cloneFlagExplicit && !opts.Fresh {
		cwd, _ := os.Getwd()
		hasSource := fileExists(filepath.Join(cwd, "composer.json")) ||
			fileExists(filepath.Join(cwd, "package.json")) ||
			fileExists(filepath.Join(cwd, "wp-config.php"))
		if !hasSource {
			opts.Clone = true
		}
	}

	if opts.MetaVersion != "" {
		comparison, comparable := compareNumericDotVersions(opts.MetaVersion, "2.0.0")
		if !comparable || comparison < 0 {
			return bootstrapRuntimeOptions{}, fmt.Errorf("invalid --framework-version value %q (must be Magento 2.0.0+)", opts.MetaVersion)
		}
	}
	if opts.Fresh && opts.Clone {
		if cloneFlagExplicit {
			return bootstrapRuntimeOptions{}, fmt.Errorf("--fresh and --clone cannot be used together")
		}
		opts.Clone = false
	}
	if opts.CodeOnly && !opts.Clone {
		return bootstrapRuntimeOptions{}, fmt.Errorf("--code-only requires --clone")
	}
	if opts.Fresh {
		opts.ComposerInstall = false
		opts.DBImport = false
		opts.MediaSync = false
	}
	if opts.Clone && opts.CodeOnly {
		opts.DBImport = false
		opts.MediaSync = false
	}

	return opts, nil
}

func normalizeBootstrapSource(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "dev"
	}
	return value
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
	bootstrapEnv = "dev"
	bootstrapFramework = ""
	bootstrapFrameworkVersion = ""
	bootstrapSkipUp = false
	bootstrapMetaPackage = defaultBootstrapMetaPackage
	bootstrapDBDump = ""
	bootstrapFixDeps = false
	bootstrapHyvaInstall = false
	bootstrapHyvaToken = defaultBootstrapHyvaToken
	bootstrapMageUsername = ""
	bootstrapMagePassword = ""
	bootstrapAssumeYes = false
	bootstrapIncludeProduct = false
}
