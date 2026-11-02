package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"govard/internal/cli"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/runtime"
	"govard/internal/try"
)

// defaultTryFramework is the stack a trial runs when the caller names none.
// Mage-OS is chosen because it installs without Adobe credentials, so the
// trial works on a machine that has never built a Magento project.
const defaultTryFramework = "mageos"

// tryOptions is what the command line resolved, passed to the runner.
type tryOptions struct {
	Framework string
	Dir       string
	Keep      bool
	Sample    bool
	Setup     bool
	Timeout   time.Duration
	Open      bool
}

var (
	tryDir      string
	tryKeep     bool
	tryNoSample bool
	trySetup    bool
	tryTimeout  time.Duration
	tryNoOpen   bool
)

var tryCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapDocker),
	},
	Use:   "try [framework]",
	Short: "Run a throwaway project to try Govard without touching anything",
	Long: `Create a temporary Mage-OS project, start it, and print the URL.

Nothing is written into your current directory and no Magento credentials are
needed. Sample data is installed by default; pass --no-sample to skip it. When
you exit, the trial is removed unless you pass --keep.

The trial needs the Govard proxy, a local CA and a resolution entry for the
project domain, which one run of 'govard try --setup' provides. Without them the
command stops before downloading anything and says which piece is missing.`,
	Example: `  # Mage-OS with sample data, removed on exit
  govard try

  # Keep the workspace and print the path to resume later
  govard try --keep

  # Prepare the proxy, local CA and domains first (privileged, one time)
  govard try --setup

  # Skip sample data
  govard try --no-sample`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := resolveTryOptions(args)
		if err != nil {
			return err
		}

		return runTry(cmd.Context(), opts)
	},
}

func init() {
	tryCmd.Flags().StringVar(&tryDir, "dir", "", "Use this directory as the trial workspace instead of a generated one")
	tryCmd.Flags().BoolVar(&tryKeep, "keep", false, "Keep the workspace when the trial ends")
	tryCmd.Flags().BoolVar(&tryNoSample, "no-sample", false, "Skip the sample data install")
	tryCmd.Flags().BoolVar(&trySetup, "setup", false, "Run the one-time privileged domain and CA setup first")
	tryCmd.Flags().DurationVar(&tryTimeout, "timeout", 20*time.Minute, "How long to wait for the storefront to answer")
	tryCmd.Flags().BoolVar(&tryNoOpen, "no-open", false, "Do not open the storefront in a browser")
}

// resolveTryOptions turns the parsed flags into the options the runner needs.
// It exists so the flag-to-behaviour mapping can be tested without running a
// trial, which is how the inverted --no-sample binding went unnoticed.
func resolveTryOptions(args []string) (tryOptions, error) {
	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}

	framework, err := resolveTryFramework(arg)
	if err != nil {
		return tryOptions{}, err
	}

	return tryOptions{
		Framework: framework,
		Dir:       tryDir,
		Keep:      tryKeep,
		Sample:    !tryNoSample,
		Setup:     trySetup,
		Timeout:   tryTimeout,
		Open:      !tryNoOpen,
	}, nil
}

// resolveTryFramework turns an argument or alias into a canonical framework
// key, defaulting to Mage-OS and rejecting anything the registry does not know.
func resolveTryFramework(arg string) (string, error) {
	name := strings.TrimSpace(arg)
	if name == "" {
		name = defaultTryFramework
	}

	def, ok := frameworks.Get(frameworks.Normalize(name))
	if !ok {
		names := make([]string, 0, len(frameworks.All()))
		for _, candidate := range frameworks.All() {
			names = append(names, candidate.Name)
		}
		return "", &cli.UsageError{Err: fmt.Errorf("unknown framework %q; try one of: %s", arg, strings.Join(names, ", "))}
	}

	return def.Name, nil
}

// TryDependenciesForTest replaces the side effects of a trial so the command
// can be exercised without Docker, git or a network. A nil field keeps the real
// implementation, mirroring SetEnvDependenciesForTest.
type TryDependenciesForTest struct {
	RunSelf          func(ctx context.Context, dir string, args ...string) error
	Probe            func(ctx context.Context, origin try.Origin, timeout time.Duration) error
	DeleteProject    func(ctx context.Context, dir string) error
	CollectArtifacts func(name, root string, profiles []string) engine.ProjectArtifacts
	ProxyUp          func() error
	FreeDisk         func(path string) (uint64, error)
	Now              func() time.Time
	Signal           <-chan os.Signal
	Stdin            io.Reader
	OpenURL          func(url string) error
}

var tryDeps = TryDependenciesForTest{
	RunSelf:          runSelfCommand,
	Probe:            probeOrigin,
	DeleteProject:    deleteProject,
	CollectArtifacts: engine.CollectProjectArtifacts,
	ProxyUp:          checkProxyReachable,
	FreeDisk:         diskFreeBytes,
	Now:              time.Now,
	OpenURL:          openURL,
}

// SetTryDependenciesForTest applies the given dependencies and returns a
// function that restores the previous set.
func SetTryDependenciesForTest(deps TryDependenciesForTest) func() {
	previous := tryDeps

	if deps.RunSelf != nil {
		tryDeps.RunSelf = deps.RunSelf
	}
	if deps.Probe != nil {
		tryDeps.Probe = deps.Probe
	}
	if deps.DeleteProject != nil {
		tryDeps.DeleteProject = deps.DeleteProject
	}
	if deps.CollectArtifacts != nil {
		tryDeps.CollectArtifacts = deps.CollectArtifacts
	}
	if deps.ProxyUp != nil {
		tryDeps.ProxyUp = deps.ProxyUp
	}
	if deps.FreeDisk != nil {
		tryDeps.FreeDisk = deps.FreeDisk
	}
	if deps.Now != nil {
		tryDeps.Now = deps.Now
	}
	if deps.Signal != nil {
		tryDeps.Signal = deps.Signal
	}
	if deps.Stdin != nil {
		tryDeps.Stdin = deps.Stdin
	}
	if deps.OpenURL != nil {
		tryDeps.OpenURL = deps.OpenURL
	}

	return func() { tryDeps = previous }
}
