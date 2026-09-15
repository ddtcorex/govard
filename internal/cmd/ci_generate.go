package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"govard/internal/cli"
	"govard/internal/conventions"
	"govard/internal/deploy"
	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

var ciGenerateCmd = &cobra.Command{
	// Generating reads .govard.yml only: no containers, no network, no
	// targets. That is what makes it usable in CI before a deploy and on a
	// host that has nothing installed.
	Annotations: map[string]string{
		runtime.AnnotationRequires: string(runtime.CapNone),
	},
	Use:   "generate",
	Short: "Render a CI pipeline from .govard.yml",
	Args:  cobra.NoArgs,
	RunE:  runCIGenerate,
}

func init() {
	ciGenerateCmd.Flags().String("provider", "", "CI provider (supported: gitlab)")
	ciGenerateCmd.Flags().String("output", "", "Write the pipeline file here (default: stdout)")
	ciGenerateCmd.Flags().Bool("check", false, "Fail if the file at --output differs from the rendered pipeline")
	ciCmd.AddCommand(ciGenerateCmd)
}

// CIGenerateCommand exposes the generate subcommand for tests.
func CIGenerateCommand() *cobra.Command { return ciGenerateCmd }

func runCIGenerate(cmd *cobra.Command, _ []string) error {
	provider, _ := cmd.Flags().GetString("provider")
	if provider != "gitlab" {
		return fmt.Errorf("unsupported CI provider %q; use gitlab", provider)
	}
	output, _ := cmd.Flags().GetString("output")
	check, _ := cmd.Flags().GetBool("check")
	if check && strings.TrimSpace(output) == "" {
		return fmt.Errorf("--check needs --output: there is nothing to compare stdout against")
	}
	// A .govard.yml problem is a configuration error (exit 4): the fix is a
	// file edit, the same classification resolveDeployOptions uses.
	config, err := loadFullConfig()
	if err != nil {
		return &cli.ConfigError{Err: err}
	}
	// Like every command that describes a deploy, generation starts from the
	// recipe: its CI lint defaults layer under the project's settings, so
	// the emitter sees the values the deploy would use.
	config = deploy.WithCILintDefaults(config, recipeFor(config).Defaults)
	// The version comes from the release stamp, never from a literal: a dev
	// binary records "dev", a release binary its tag.
	rendered, err := deploy.RenderGitLabPipelineWithGovardVersion(config, Version)
	if err != nil {
		return &cli.ConfigError{Err: err}
	}
	if strings.TrimSpace(output) == "" {
		fmt.Fprint(cmd.OutOrStdout(), rendered)
		return nil
	}
	if check {
		return checkCIOutput(cmd, output, rendered)
	}
	if dir := filepath.Dir(output); dir != "" {
		if err := os.MkdirAll(dir, conventions.DefaultDirPerm); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
	}
	if err := os.WriteFile(output, []byte(rendered), conventions.DefaultFilePerm); err != nil {
		return fmt.Errorf("write CI file: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", output)
	return nil
}

// checkCIOutput byte-compares the rendered pipeline with the file at output.
// A mismatch is an execution failure (exit 1): the pipeline drifted from
// .govard.yml, and the fix is to regenerate, never to hand-edit.
func checkCIOutput(cmd *cobra.Command, output, rendered string) error {
	existing, err := os.ReadFile(output)
	if err != nil {
		return fmt.Errorf("CI file drifted from .govard.yml (cannot read %s: %v) - regenerate with: govard ci generate --provider gitlab --output %s", output, err, output)
	}
	if string(existing) == rendered {
		return nil
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "CI file drifted from .govard.yml - regenerate with: govard ci generate --provider gitlab --output %s\n", output)
	for _, line := range ciDiffExcerpt(rendered, string(existing)) {
		fmt.Fprintln(out, line)
	}
	return fmt.Errorf("CI file %s drifted from .govard.yml", output)
}

// ciDiffExcerpt returns at most the first 20 differing lines with numbers, so
// a drifted file prints evidence instead of one bare verdict.
func ciDiffExcerpt(rendered, existing string) []string {
	want := strings.Split(rendered, "\n")
	got := strings.Split(existing, "\n")
	var excerpt []string
	for i := 0; i < len(want) || i < len(got); i++ {
		var w, g string
		if i < len(want) {
			w = want[i]
		}
		if i < len(got) {
			g = got[i]
		}
		if w != g {
			excerpt = append(excerpt, fmt.Sprintf("line %d:\n- %s\n+ %s", i+1, w, g))
			if len(excerpt) >= 20 {
				break
			}
		}
	}
	return excerpt
}
