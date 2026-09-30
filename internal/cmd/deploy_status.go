package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"govard/internal/cli"
	"govard/internal/deploy"
	"govard/internal/engine"
	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

var deployReleasesCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapSSH),
	},
	Use:   "releases [remote]",
	Short: "List the releases on a remote",
	Long: `List every release directory on the target, newest first, and say which one is
live.

A directory created by another deploy tool is listed as foreign. Govard never
deletes one, so seeing it here is how an operator learns that a server is still
being deployed to by something else.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeployReleases,
}

// DeployReleasesCommand exposes the releases subcommand for tests.
func DeployReleasesCommand() *cobra.Command { return deployReleasesCmd }

// DeployStatusCommand exposes the status subcommand for tests.
func DeployStatusCommand() *cobra.Command { return deployStatusCmd }

func runDeployReleases(cmd *cobra.Command, args []string) error {
	remote, err := deployRemoteName(cmd, args)
	if err != nil {
		return err
	}
	config, options, err := resolveDeployReadOptions(cmd, remote)
	if err != nil {
		return err
	}
	host, err := deployHostFor(cmd.Context(), config, remote, options, cmd.OutOrStdout())
	if err != nil {
		return err
	}

	entries, err := deploy.ListReleases(cmd.Context(), host)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s has no releases under %s\n", remote, host.ReleasesPath())
		return nil
	}

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSONLine(cmd, entries)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-8s %-10s %-24s %-10s %s\n", "RELEASE", "STATUS", "CREATED", "BY", "REVISION")
	for _, entry := range entries {
		marker := ""
		switch {
		case entry.Live:
			marker = "  (live)"
		case entry.Foreign:
			marker = "  (not managed by govard)"
		}
		revision := entry.Revision
		if revision == "" {
			revision = "-"
		}
		status := entry.Status
		if status == "" {
			status = "-"
		}
		created := entry.CreatedAt
		if created == "" {
			created = "-"
		}
		createdBy := entry.CreatedBy
		if createdBy == "" {
			createdBy = "-"
		}
		fmt.Fprintf(out, "%-8s %-10s %-24s %-10s %s%s\n", entry.Release, status, created, createdBy, revision, marker)
	}
	return nil
}

// statusResult is one environment's row in `deploy status --json`.
type statusResult struct {
	Remote   string `json:"remote"`
	Release  string `json:"release,omitempty"`
	Revision string `json:"revision,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

var deployStatusCmd = &cobra.Command{
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapSSH),
	},
	Use:   "status [remote]",
	Short: "Show which revision is live in which environment",
	Long: `Report the revision each environment is serving, read from the target itself.

With no remote, every configured remote is checked and an unreachable one is
reported as a row rather than aborting the report: "I cannot reach staging" is
information, not a failure of the command.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeployStatus,
}

func runDeployStatus(cmd *cobra.Command, args []string) error {
	config, err := loadFullConfig()
	if err != nil {
		return &cli.ConfigError{Err: err}
	}

	names, err := statusRemoteNames(cmd, args, config)
	if err != nil {
		return err
	}

	results := make([]statusResult, 0, len(names))
	for _, name := range names {
		result, err := deployStatusForRemote(cmd, config, name)
		if err != nil {
			return configOrUsageError(err)
		}
		results = append(results, result)
	}

	reachable := 0
	for _, result := range results {
		if result.Error == "" {
			reachable++
		}
	}
	// Both output modes end in the same error when nothing answered. In JSON mode
	// the document is written first, so a consumer still gets the per-remote rows
	// on stdout (one document) while the exit code reports the outage.
	errNoneReachable := fmt.Errorf("no configured remote could be reached")

	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		if err := writeJSONLine(cmd, results); err != nil {
			return err
		}
		if reachable == 0 {
			// Write the reason ourselves and mark the error as reported, so
			// --error-json does not append an envelope after the rows.
			fmt.Fprintln(cmd.ErrOrStderr(), errNoneReachable)
			return &reportedStatusError{err: errNoneReachable}
		}
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-16s %-10s %-10s %-12s %s\n", "REMOTE", "RELEASE", "REVISION", "STATUS", "BRANCH")
	for _, result := range results {
		if result.Error != "" {
			fmt.Fprintf(out, "%-16s %s\n", result.Remote, result.Error)
			continue
		}
		release := result.Release
		if release == "" {
			release = "-"
		}
		revision := result.Revision
		if revision == "" {
			revision = "-"
		}
		branch := result.Branch
		if branch == "" {
			branch = "-"
		}
		fmt.Fprintf(out, "%-16s %-10s %-10s %-12s %s\n", result.Remote, release, deploy.ShortRevision(revision), result.Status, branch)
	}
	if reachable == 0 {
		return errNoneReachable
	}
	return nil
}

// reportedStatusError is the --json form of "no remote reachable": a plain
// execution error (exit 1) whose message was already written to stderr, so the
// --error-json envelope is not printed as a second document on stdout.
type reportedStatusError struct{ err error }

func (e *reportedStatusError) Error() string         { return e.err.Error() }
func (e *reportedStatusError) Unwrap() error         { return e.err }
func (e *reportedStatusError) AlreadyReported() bool { return true }

// statusRemoteNames resolves which remotes to report on. Naming one is explicit;
// naming none means every configured remote.
func statusRemoteNames(cmd *cobra.Command, args []string, config engine.Config) ([]string, error) {
	flag, _ := cmd.Flags().GetString("remote")
	if len(args) > 0 && args[0] != "" {
		if flag != "" && flag != args[0] {
			return nil, &cli.UsageError{Err: fmt.Errorf("remote %q does not match --remote %q", args[0], flag)}
		}
		return []string{args[0]}, nil
	}
	if flag != "" {
		return []string{flag}, nil
	}
	names := make([]string, 0, len(config.Remotes))
	for name := range config.Remotes {
		names = append(names, name)
	}
	engine.SortRemoteNames(names)
	if len(names) == 0 {
		return nil, &cli.ConfigError{Err: fmt.Errorf("this project configures no remotes")}
	}
	return names, nil
}

// deployStatusForRemote reports one remote. The returned error is only ever a
// resolution failure that means the name is not configured: everything else — a
// target that refuses the connection, a layout that cannot be read — is a row,
// because a table of partial results is the point of the command and an early
// abort would throw away the remotes that did answer.
func deployStatusForRemote(cmd *cobra.Command, config engine.Config, name string) (statusResult, error) {
	result := statusResult{Remote: name}

	options, err := deploy.ResolveReadOptions(config, name, deploy.Overrides{Verify: boolPointer(false), Lock: boolPointer(false)})
	if err != nil {
		if errors.Is(err, deploy.ErrUnknownRemote) {
			return result, err
		}
		result.Status = "unknown"
		result.Error = err.Error()
		return result, nil
	}
	// No note here: this runs once per remote and its output is a table.
	host, err := deployHostFor(cmd.Context(), config, name, options, nil)
	if err != nil {
		result.Status = "unknown"
		result.Error = err.Error()
		return result, nil
	}

	release, err := deploy.LiveRelease(cmd.Context(), host)
	if err != nil {
		result.Status = "unknown"
		result.Error = err.Error()
		return result, nil
	}
	if release == nil {
		result.Status = "no release"
		return result, nil
	}
	result.Release = release.Release
	result.Revision = release.Revision
	result.Branch = release.Branch
	result.Status = release.Status
	return result, nil
}

func boolPointer(value bool) *bool { return &value }

func writeJSONLine(cmd *cobra.Command, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
	return nil
}
