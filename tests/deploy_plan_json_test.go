package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/cmd"
)

// planDocument is the machine-readable plan, decoded the way a CI job would:
// by field, not by position, and without knowing which steps this project has.
type planDocument struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	Remote        string `json:"remote"`
	Branch        string `json:"branch"`
	Revision      string `json:"revision"`
	Build         struct {
		Mode string `json:"mode"`
	} `json:"build"`
	Publish struct {
		Strategy  string `json:"strategy"`
		DecidedBy string `json:"decided_by"`
	} `json:"publish"`
	Steps []struct {
		Index            int    `json:"index"`
		ID               string `json:"id"`
		Kind             string `json:"kind"`
		Stage            string `json:"stage"`
		Title            string `json:"title"`
		RunOn            string `json:"run_on"`
		Source           string `json:"source"`
		Implementation   string `json:"implementation"`
		Command          string `json:"command"`
		Skipped          bool   `json:"skipped"`
		SkipReason       string `json:"skip_reason"`
		NeedsApplication bool   `json:"needs_application"`
	} `json:"steps"`
}

func (d planDocument) step(t *testing.T, id string) planDocumentStep {
	t.Helper()
	for _, step := range d.Steps {
		if step.ID == id {
			return step
		}
	}
	t.Fatalf("the plan has no step %q", id)
	return planDocumentStep{}
}

type planDocumentStep = struct {
	Index            int    `json:"index"`
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Stage            string `json:"stage"`
	Title            string `json:"title"`
	RunOn            string `json:"run_on"`
	Source           string `json:"source"`
	Implementation   string `json:"implementation"`
	Command          string `json:"command"`
	Skipped          bool   `json:"skipped"`
	SkipReason       string `json:"skip_reason"`
	NeedsApplication bool   `json:"needs_application"`
}

// planProject writes a project whose plan exercises every fact the document has
// to carry: a recipe step the engine implements, a build task the mode skips, a
// task no recipe fills, and a hook the project asked to run locally.
func planProject(t *testing.T) {
	t.Helper()
	origin, _ := seedGitRepo(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: laravel
domain: sample.test
deploy:
  hooks:
    - name: purge
      on: "publish:activate"
      position: after
      order: 10
      run: "varnishadm ban req.url ~ /"
      run_on: local
remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: `+filepath.Join(root, "public_html")+`
    local: true
    deploy:
      deploy_path: `+filepath.Join(root, ".deployer")+`
      branch: main
      repository: `+origin+`
`)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	// The cobra commands are package-level, so a flag this test sets stays set
	// for the next one — which is how this file first discovered that a plan
	// without --json printed the document anyway. Reset everything it touches.
	t.Cleanup(func() {
		flags := cmd.DeployPlanCommand().Flags()
		_ = flags.Set("build", "auto")
		_ = flags.Set("artifact-dir", "")
		_ = flags.Set("json", "false")
	})
}

// runPlanJSON runs `govard deploy plan local --json` in the requested build mode
// and returns stdout. The mode is always explicit: `auto` resolves by presence,
// so a test that relied on the ambient flag value would be testing the last test.
func runPlanJSON(t *testing.T, mode string) string {
	t.Helper()
	args := []string{"deploy", "plan", "local", "--json", "--build", mode}
	if mode == "artifact" {
		args = append(args, "--artifact-dir", "artifacts")
	}
	out := &bytes.Buffer{}
	command := cmd.RootCommandForTest()
	command.SetArgs(args)
	command.SetOut(out)
	command.SetErr(io.Discard)
	if err := command.Execute(); err != nil {
		t.Fatalf("deploy plan --json: %v\n%s", err, out.String())
	}
	return out.String()
}

// The flag was registered with the help text "Emit machine-readable output" and
// never read, so a pipeline that asked for JSON got the human tree. Parsing the
// whole of stdout is the assertion that matters: the document has to be the only
// thing on it, or every consumer needs to know where the document starts.
func TestDeployPlanJSONIsTheOnlyThingOnStdout(t *testing.T) {
	planProject(t)

	printed := runPlanJSON(t, "server")

	var document planDocument
	if err := json.Unmarshal([]byte(printed), &document); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, printed)
	}
	if document.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", document.SchemaVersion)
	}
	// The run's document carries schema_version too and has no kind, so the
	// discriminator is what tells the two apart without guessing by shape.
	if document.Kind != "plan" {
		t.Errorf("kind = %q, want %q", document.Kind, "plan")
	}
	if document.Remote != "local" {
		t.Errorf("remote = %q, want %q", document.Remote, "local")
	}
	if document.Branch != "main" {
		t.Errorf("branch = %q, want %q", document.Branch, "main")
	}
	if document.Revision == "" {
		t.Error("revision is empty; the header prints it, so the document has to as well")
	}
	// The resolved mode, not the requested one: `auto` is decided by presence
	// before the plan is built, and the document says what will happen.
	if document.Build.Mode != "server" {
		t.Errorf("build.mode = %q, want the resolved mode %q", document.Build.Mode, "server")
	}
	// --publish defaults to auto, and the strategy genuinely is not known until
	// the target is read: reporting a resolved-looking value would be a guess.
	if document.Publish.Strategy != "auto" || document.Publish.DecidedBy != "target" {
		t.Errorf("publish = %+v, want strategy auto decided by the target", document.Publish)
	}
	if len(document.Steps) == 0 {
		t.Fatal("the document carries no steps")
	}
	for idx, step := range document.Steps {
		if step.Index != idx+1 {
			t.Fatalf("step %d has index %d; the document must line up with the human tree", idx, step.Index)
		}
	}
}

func TestDeployPlanJSONCarriesWhatTheHumanTreeCannot(t *testing.T) {
	planProject(t)
	document := planDocument{}
	if err := json.Unmarshal([]byte(runPlanJSON(t, "server")), &document); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// A recipe task the engine implements: no command, but it runs.
	check := document.step(t, "deploy:check")
	if check.Implementation != "engine" || check.Skipped {
		t.Errorf("deploy:check = %+v, want the engine implementation and not skipped", check)
	}
	if check.RunOn != "remote" || check.Source != "recipe" || check.Kind != "task" {
		t.Errorf("deploy:check provenance = %+v, want a remote recipe task", check)
	}

	// A build task the recipe implements: it runs on the target in server mode.
	vendors := document.step(t, "build:vendors")
	if vendors.Skipped || vendors.Implementation != "command" || vendors.Command == "" {
		t.Errorf("build:vendors = %+v, want a build step that runs its command", vendors)
	}

	// A task no recipe fills: the executor skips it, so the document must too,
	// with the reason the human tree prints.
	compile := document.step(t, "build:compile")
	if !compile.Skipped || compile.Implementation != "none" || compile.SkipReason == "" {
		t.Errorf("build:compile = %+v, want skipped with no implementation and a reason", compile)
	}

	// The fact the human tree cannot express: where a step runs. A hook the
	// project runs locally looks exactly like a remote one in `deploy plan`.
	hook := document.step(t, "hook:purge")
	if hook.Kind != "hook" || hook.RunOn != "local" || hook.Source != "config" {
		t.Errorf("hook:purge = %+v, want a locally run project hook", hook)
	}
	if hook.Skipped {
		t.Error("a hook the project declared is never skipped by the plan")
	}
}

// Artifact mode leaves a build step its command and skips it anyway: both facts
// have to survive into the document, because "what would this step do" and "will
// it do it" are different questions.
func TestDeployPlanJSONKeepsTheCommandOfAStepTheModeSkips(t *testing.T) {
	planProject(t)
	document := planDocument{}
	if err := json.Unmarshal([]byte(runPlanJSON(t, "artifact")), &document); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if document.Build.Mode != "artifact" {
		t.Fatalf("build.mode = %q, want %q", document.Build.Mode, "artifact")
	}

	vendors := document.step(t, "build:vendors")
	if !vendors.Skipped {
		t.Errorf("build:vendors = %+v, want it skipped in artifact mode", vendors)
	}
	if vendors.Implementation != "command" || vendors.Command == "" {
		t.Errorf("build:vendors = %+v, want the command it would have run", vendors)
	}
	if vendors.SkipReason == "" {
		t.Error("a skipped step without a reason leaves a pipeline unable to say why it will not run")
	}
}

// A document a CI job can diff: the only inputs are the project, the recipe, the
// hooks and the flags, so two runs of the same command agree byte for byte. A
// timestamp anywhere in it would break that silently.
func TestDeployPlanJSONIsDeterministic(t *testing.T) {
	planProject(t)

	first := runPlanJSON(t, "server")
	second := runPlanJSON(t, "server")

	if first != second {
		t.Fatalf("two runs of the same plan differ:\n--- first\n%s\n--- second\n%s", first, second)
	}
}

// The human tree stays the default: the flag adds a document, it does not
// replace the output an operator reads.
func TestDeployPlanWithoutTheFlagStillPrintsTheTree(t *testing.T) {
	planProject(t)

	out := &bytes.Buffer{}
	command := cmd.RootCommandForTest()
	command.SetArgs([]string{"deploy", "plan", "local", "--build", "server"})
	command.SetOut(out)
	command.SetErr(io.Discard)
	if err := command.Execute(); err != nil {
		t.Fatalf("deploy plan: %v\n%s", err, out.String())
	}

	printed := out.String()
	for _, want := range []string{"Deploy plan for local", "Build mode:", "Publish strategy:", "deploy:check"} {
		if !bytes.Contains([]byte(printed), []byte(want)) {
			t.Errorf("the human plan no longer prints %q:\n%s", want, printed)
		}
	}
	if json.Valid([]byte(printed)) {
		t.Error("without --json the output must not be a JSON document")
	}
}
