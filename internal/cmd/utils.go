package cmd

import (
	"context"
	"errors"
	"fmt"
	"govard/internal/conventions"
	"govard/internal/engine"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"syscall"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

func loadConfig() engine.Config {
	return loadConfigWithProfile("")
}

// loadConfigWithProfile loads config with a specific profile (used by env proxy).
func loadConfigWithProfile(profile string) engine.Config {
	wd, _ := os.Getwd()
	// Resolve profile: explicit flag > project registry (last-used) > empty (default)
	resolvedProfile := engine.ResolveEffectiveProfile(wd, profile)
	config, _, err := engine.LoadConfigFromDirWithProfile(wd, false, resolvedProfile)
	if err != nil {
		pterm.Warning.Printf("Failed to load layered config: %v\n", err)
		return engine.Config{}
	}
	return config
}

func loadFullConfig() (engine.Config, error) {
	return loadFullConfigWithProfile("")
}

func loadFullConfigWithProfile(profile string) (engine.Config, error) {
	wd, _ := os.Getwd()
	// Resolve profile: explicit flag > project registry (last-used) > empty (default)
	resolvedProfile := engine.ResolveEffectiveProfile(wd, profile)
	config, _, err := engine.LoadConfigFromDirWithProfile(wd, true, resolvedProfile)
	if err != nil {
		return engine.Config{}, fmt.Errorf("could not load config: %w", err)
	}
	return config, nil
}

func loadWritableConfig() (engine.Config, error) {
	wd, _ := os.Getwd()
	config, err := engine.LoadBaseConfigFromDir(wd, true)
	if err != nil {
		return engine.Config{}, fmt.Errorf("could not load %s: %w", conventions.BaseConfigFile, err)
	}
	return config, nil
}

func saveConfig(config engine.Config) {
	if err := engine.ValidateConfig(config); err != nil {
		pterm.Error.Printf("Config validation failed: %v\n", err)
		return
	}
	writableConfig := engine.PrepareConfigForWrite(config)

	data, err := yaml.Marshal(&writableConfig)
	if err != nil {
		pterm.Error.Printf("Failed to marshal config: %v\n", err)
		return
	}
	err = os.WriteFile(conventions.BaseConfigFile, data, conventions.DefaultFilePerm)
	if err != nil {
		pterm.Error.Printf("Failed to write %s: %v\n", conventions.BaseConfigFile, err)
	}
}

func runUp() {
	// debug on/off call this outside cobra ExecuteContext, so cmd.Context() would be nil
	// and engine.CheckDockerStatus(nil) panics on context.WithTimeout(nil, ...).
	// Ensure a valid context before invoking upCmd.RunE.
	ctx := upCmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	upCmd.SetContext(ctx)
	_ = upCmd.RunE(upCmd, []string{})
}

var govardSubcommandRunner = func(cmd *cobra.Command, args ...string) error {
	executablePath, err := os.Executable()
	commandPath := "govard"
	if err == nil && strings.TrimSpace(executablePath) != "" {
		commandPath = executablePath
	}

	command := exec.Command(commandPath, args...)
	command.Dir, _ = os.Getwd()
	command.Stdin = os.Stdin
	command.Stdout = cmd.OutOrStdout()
	command.Stderr = cmd.ErrOrStderr()
	return command.Run()
}

func runGovardSubcommand(cmd *cobra.Command, args ...string) error {
	// If the current command has a profile set, pass it to the subcommand
	if cmd != nil {
		if profile, err := cmd.Flags().GetString("profile"); err == nil && profile != "" {
			// Find a safe spot to insert the flag. After 'db' but before 'query'?
			// Actually, govard commands are flexible with flag positions.
			// Appending is usually safe.
			args = append(args, "--profile", profile)
		}
	}
	return govardSubcommandRunner(cmd, args...)
}

func runGovardSubcommandSilent(cmd *cobra.Command, args ...string) error {
	executablePath, err := os.Executable()
	commandPath := "govard"
	if err == nil && strings.TrimSpace(executablePath) != "" {
		commandPath = executablePath
	}

	command := exec.Command(commandPath, args...)
	command.Dir, _ = os.Getwd()
	command.Stdin = os.Stdin
	// Stdout and Stderr are intentionally left as nil to discard output
	return command.Run()
}

// rebrandComposeHelp runs `docker compose --help`, rebrands the output to use govard command names,
// and prints it to the command's stdout.
func rebrandComposeHelp(cmd *cobra.Command, govardCmdName string) {
	// First, print our own Govard-specific help header if available
	printGovardHelpHeader(cmd)

	// Determine the subcommand by looking at os.Args
	dockerArgs := []string{"compose"}
	detectedSubcommand := ""
	for i, arg := range os.Args {
		if arg == govardCmdName {
			// Append subsequent args to get subcommand-specific help
			for j := i + 1; j < len(os.Args); j++ {
				candidate := os.Args[j]
				if candidate != "--help" && candidate != "-h" && candidate != "help" && !strings.HasPrefix(candidate, "-") {
					if detectedSubcommand == "" {
						detectedSubcommand = candidate
					}
					dockerArgs = append(dockerArgs, candidate)
				}
			}
			break
		}
	}
	dockerArgs = append(dockerArgs, "--help")

	c := exec.Command("docker", dockerArgs...)
	out, err := c.CombinedOutput()
	if err != nil {
		pterm.Error.Printf("Failed to get Docker Compose help: %v\n", err)
		return
	}

	helpText := string(out)

	// Rebrand: replace `docker compose` with `govard [govardCmdName]`
	helpText = strings.ReplaceAll(helpText, "docker compose", "govard "+govardCmdName)
	suppressedFlags := suppressedComposeFlags(cmd, govardCmdName, detectedSubcommand)
	helpText = filterComposeHelpText(helpText, suppressedFlags)
	fmt.Fprintln(cmd.OutOrStdout(), helpText)
	appendGovardSpecificOptions(cmd, govardCmdName, detectedSubcommand, cmd.OutOrStdout())
}

func printGovardHelpHeader(cmd *cobra.Command) {
	if cmd.Long != "" {
		fmt.Fprintln(cmd.OutOrStdout(), cmd.Long)
		fmt.Fprintln(cmd.OutOrStdout())
	} else if cmd.Short != "" {
		fmt.Fprintln(cmd.OutOrStdout(), cmd.Short)
		fmt.Fprintln(cmd.OutOrStdout())
	}

	if cmd.Example != "" {
		pterm.DefaultSection.WithLevel(2).Println("Examples:")
		fmt.Fprintln(cmd.OutOrStdout(), cmd.Example)
		fmt.Fprintln(cmd.OutOrStdout())
	}
}

func suppressedComposeFlags(cmd *cobra.Command, govardCmdName, detectedSubcommand string) map[string]struct{} {
	flags := map[string]struct{}{
		"file":              {},
		"project-name":      {},
		"project-directory": {},
	}

	if cmd != nil {
		cmd.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
			if flag.Name == "help" {
				return
			}
			flags[flag.Name] = struct{}{}
		})
	}

	if govardCmdName == "svc" {
		switch detectedSubcommand {
		case "up", "restart":
			flags["pull"] = struct{}{}
			flags["no-trust"] = struct{}{}
			flags["no-fallback"] = struct{}{}
		}
	}

	if govardCmdName == "env" && detectedSubcommand == "pull" {
		flags["no-fallback"] = struct{}{}
	}

	return flags
}

func runGovardSubcommandSkippable(cmd *cobra.Command, args ...string) (bool, error) {
	// Trap interrupt signal in the parent so we can decide to skip instead of exiting the whole bootstrap process.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	err := runGovardSubcommand(cmd, args...)
	if err == nil {
		return false, nil
	}

	// Check if the child process was interrupted (Exit Code 130)
	if isInterruptExit(err) {
		return true, nil
	}

	return false, err
}

func isInterruptExit(err error) bool {
	if err == nil {
		return false
	}

	// Check for string "signal: interrupt" pattern
	if strings.Contains(err.Error(), "signal: interrupt") {
		return true
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	// WaitStatus might be able to tell us
	if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
		if status.Signaled() && status.Signal() == os.Interrupt {
			return true
		}
	}

	// Fallback to exit code 130 convention from shell wrappers
	return exitErr.ExitCode() == 130
}

func filterComposeHelpText(helpText string, suppressedFlags map[string]struct{}) string {
	lines := strings.Split(helpText, "\n")
	filtered := make([]string, 0, len(lines))

	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if shouldSkipComposeOption(trimmed, suppressedFlags) {
			i++
			for i < len(lines) {
				nextTrimmed := strings.TrimSpace(lines[i])
				if nextTrimmed == "" {
					i++
					break
				}
				if strings.HasSuffix(nextTrimmed, ":") || strings.HasPrefix(nextTrimmed, "-") {
					break
				}
				if !strings.HasPrefix(lines[i], " ") && !strings.HasPrefix(lines[i], "\t") {
					break
				}
				i++
			}
			continue
		}
		filtered = append(filtered, lines[i])
		i++
	}

	return strings.Join(filtered, "\n")
}

func shouldSkipComposeOption(line string, suppressedFlags map[string]struct{}) bool {
	if !strings.HasPrefix(line, "-") {
		return false
	}

	for _, token := range strings.Fields(line) {
		if !strings.HasPrefix(token, "--") {
			continue
		}
		name := strings.TrimPrefix(token, "--")
		name = strings.TrimRight(name, ",")
		if idx := strings.IndexAny(name, " [<"); idx >= 0 {
			name = name[:idx]
		}
		if _, ok := suppressedFlags[name]; ok {
			return true
		}
	}

	return false
}

type helpFlagSpec struct {
	Display string
	Usage   string
}

func appendGovardSpecificOptions(cmd *cobra.Command, govardCmdName, detectedSubcommand string, out interface{ Write([]byte) (int, error) }) {
	specs := collectGovardSpecificOptions(cmd, govardCmdName, detectedSubcommand)
	if len(specs) == 0 {
		return
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Govard-specific Options:")
	for _, spec := range specs {
		fmt.Fprintf(out, "  %-28s %s\n", spec.Display, spec.Usage)
	}
}

func collectGovardSpecificOptions(cmd *cobra.Command, govardCmdName, detectedSubcommand string) []helpFlagSpec {
	specs := make([]helpFlagSpec, 0)

	if cmd != nil {
		cmd.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
			if flag.Name == "help" {
				return
			}
			specs = append(specs, helpFlagSpec{
				Display: formatHelpFlagDisplay(flag),
				Usage:   formatHelpFlagUsage(flag),
			})
		})
	}

	if govardCmdName == "svc" {
		switch detectedSubcommand {
		case "up", "restart":
			specs = append(specs,
				helpFlagSpec{
					Display: "--pull",
					Usage:   "Pull latest images before startup.",
				},
				helpFlagSpec{
					Display: "--no-trust",
					Usage:   "Skip Govard Root CA trust installation.",
				},
				helpFlagSpec{
					Display: "--no-fallback",
					Usage:   "Disable the automatic local image build retry if pulls fail.",
				},
			)
		}
	}

	if govardCmdName == "env" && detectedSubcommand == "pull" {
		specs = append(specs, helpFlagSpec{
			Display: "--no-fallback",
			Usage:   "Disable the automatic local image build retry if pulls fail.",
		})
	}

	return specs
}

func formatHelpFlagDisplay(flag *pflag.Flag) string {
	parts := make([]string, 0, 2)
	if flag.Shorthand != "" {
		parts = append(parts, "-"+flag.Shorthand)
	}

	long := "--" + flag.Name
	if flag.Value.Type() != "bool" {
		long += " " + flag.Value.Type()
	}
	parts = append(parts, long)
	return strings.Join(parts, ", ")
}

func formatHelpFlagUsage(flag *pflag.Flag) string {
	usage := strings.TrimSpace(flag.Usage)
	if flag.DefValue == "" || flag.DefValue == "false" {
		return usage
	}
	return fmt.Sprintf("%s (default %s)", usage, flag.DefValue)
}

func boolFlagOrDefault(cmd *cobra.Command, name string, fallback bool) bool {
	if cmd == nil {
		return fallback
	}
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return fallback
	}
	value, err := cmd.Flags().GetBool(name)
	if err != nil {
		return fallback
	}
	return value
}

// isComposeMaintenanceCommand returns true if the command is a common Docker Compose maintenance command
// that accepts a service name at the end of its arguments.
func isComposeMaintenanceCommand(cmd string) bool {
	commands := map[string]bool{
		"ps":      true,
		"logs":    true,
		"top":     true,
		"stop":    true,
		"start":   true,
		"restart": true,
		"pause":   true,
		"unpause": true,
		"pull":    true,
		"build":   true,
		"port":    true,
		"images":  true,
		"rm":      true,
		"kill":    true,
	}
	return commands[cmd]
}

// proxyServiceToCompose forwards a command to Docker Compose for a specific service.
func proxyServiceToCompose(cmd *cobra.Command, service string, args []string) error {
	config := loadConfig()
	cwd, _ := os.Getwd()
	composePath := engine.ComposeFilePathWithProfile(cwd, config.ProjectName, config.Profile)

	subcommand := args[0]
	remainingArgs := args[1:]

	composeArgs := append([]string{subcommand}, remainingArgs...)
	composeArgs = append(composeArgs, service)

	return engine.RunCompose(cmd.Context(), engine.ComposeOptions{
		ProjectDir:  cwd,
		ProjectName: config.ProjectName,
		ComposeFile: composePath,
		Args:        composeArgs,
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
		Stdin:       os.Stdin,
	})
}
