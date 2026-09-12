package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
)

func recipeWithDefaults(defaults map[string]any) deploy.Recipe {
	recipe := deploy.DefaultRecipe()
	recipe.Defaults = defaults
	return recipe
}

func TestRecipeDefaultsAreLayeredUnderTheProjectConfiguration(t *testing.T) {
	recipe := recipeWithDefaults(map[string]any{"shared_dirs": []string{"var/log", "pub/media"}})

	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Settings: map[string]any{"shared_dirs": []string{"only/this"}},
	})
	if got := options.Settings["shared_dirs"]; !equalStrings(got, []string{"only/this"}) {
		t.Fatalf("shared_dirs = %#v, want the project's list to win outright", got)
	}

	options = deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{Settings: map[string]any{}})
	if got := options.Settings["shared_dirs"]; !equalStrings(got, []string{"var/log", "pub/media"}) {
		t.Fatalf("shared_dirs = %#v, want the recipe's default to fill an absent key", got)
	}
}

func TestRecipeDefaultsDoNotMutateTheCallersSettings(t *testing.T) {
	recipe := recipeWithDefaults(map[string]any{"frontend_dir": ""})
	original := map[string]any{"shared_files": []string{"app/etc/env.php"}}

	_ = deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{Settings: original})
	if _, leaked := original["frontend_dir"]; leaked {
		t.Fatal("WithRecipeDefaults wrote the recipe's defaults into the caller's map")
	}
}

func TestRecipeArgumentSpecsRenderEverySupportedShape(t *testing.T) {
	spec := deploy.ArgsSpec{Flag: "-t"}

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "absent", value: nil, want: ""},
		{name: "empty string", value: "", want: ""},
		{name: "string passes through verbatim", value: "-t Magento/luma en_US", want: "-t Magento/luma en_US"},
		{name: "list becomes repeated flags", value: []string{"a", "b"}, want: "-t a -t b"},
		{name: "interface list", value: []any{"a", "b"}, want: "-t a -t b"},
		{name: "map groups theme with locales", value: map[string]any{
			"Magento/luma":  []any{"en_US"},
			"Magento/blank": []any{"fr_FR", "de_DE"},
		}, want: "-t Magento/blank fr_FR de_DE -t Magento/luma en_US"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deploy.RenderSettingArgsForTest(spec, tt.value); got != tt.want {
				t.Fatalf("RenderSettingArgs(%#v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// A theme→locales map is the multi-website form: each site has its own theme with
// its own locales. Its values must render as their OWN flag, because a bare token
// after `-t <theme>` is not a second value for `-t` — it is Magento's positional
// `languages` argument, and `DeployStaticContentCommand::execute()` assigns that
// argument OVER `--language` (`$input->getArgument(LANGUAGES_ARGUMENT) ?:
// $languageOption`). Executed against Magento's own option definition,
// `--language en_US --language fr_FR -t Acme/a en_US` resolves to
// theme=[Acme/a] and language=[en_US]: the project's locale list is discarded.
func TestRecipeArgumentSpecsRenderMapValuesAsTheirOwnFlag(t *testing.T) {
	spec := deploy.ArgsSpec{Flag: "-t", ValueFlag: "--language"}

	got := deploy.RenderSettingArgsForTest(spec, map[string]any{
		"Magento/luma":  []any{"en_US"},
		"Magento/blank": []any{"fr_FR", "en_US"},
	})
	want := "-t Magento/blank -t Magento/luma --language en_US --language fr_FR"
	if got != want {
		t.Fatalf("RenderSettingArgs = %q, want %q", got, want)
	}

	// A recipe that declares no value flag keeps the older shape: its map values
	// really are loose arguments to it.
	plain := deploy.RenderSettingArgsForTest(deploy.ArgsSpec{Flag: "-t"}, map[string]any{"a": []any{"b"}})
	if plain != "-t a b" {
		t.Fatalf("a spec without ValueFlag must render as before, got %q", plain)
	}
}

func TestRecipeArgumentSpecsBecomeSettingsVariables(t *testing.T) {
	recipe := recipeWithDefaults(map[string]any{"magento_themes": deploy.ArgsSpec{Flag: "-t"}})

	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Settings: map[string]any{"magento_themes": map[string]any{"Magento/luma": []any{"en_US"}}},
	})
	if got, _ := options.Settings["magento_themes_args"].(string); got != "-t Magento/luma en_US" {
		t.Fatalf("magento_themes_args = %q, want %q", got, "-t Magento/luma en_US")
	}
	// The raw value must stay available: an operator reading the plan or the
	// settings sees what they configured, not a rendered argument string.
	if _, isSpec := options.Settings["magento_themes"].(deploy.ArgsSpec); isSpec {
		t.Fatal("the ArgsSpec itself was written into the settings map")
	}

	options = deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{Settings: map[string]any{}})
	got, _ := options.Settings["magento_themes_args"].(string)
	if got != "" {
		t.Fatalf("magento_themes_args = %q with nothing configured, want empty", got)
	}
}

func TestDeployVariablesSubstituteRenderedArgumentsVerbatim(t *testing.T) {
	vars := deploy.NewVars().Set("quoted", "one two").SetRaw("rendered", "-t a -t b")

	expanded, err := vars.Expand("cmd {{quoted}} {{rendered}}")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if !strings.Contains(expanded, "'one two'") {
		t.Fatalf("a plain value was not quoted: %q", expanded)
	}
	if !strings.HasSuffix(expanded, "cmd 'one two' -t a -t b") {
		t.Fatalf("a raw value was quoted: %q", expanded)
	}
}

func equalStrings(value any, want []string) bool {
	list, ok := value.([]string)
	if !ok || len(list) != len(want) {
		return false
	}
	for idx := range want {
		if list[idx] != want[idx] {
			return false
		}
	}
	return true
}

func TestSettingsBecomeVariablesWhateverTheirShape(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	vars := cmd.DeployVarsForTest(host, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{
			"worker_control":  false,
			"static_jobs":     4,
			"frontend_dir":    "src/theme",
			"themes_args":     "-t Vendor/theme en_US",
			"content_version": "",
		},
	})

	// A bool a recipe guards on must substitute, not fail as "unknown variable".
	expanded, err := vars.Expand(`if [ "{{settings.worker_control}}" = "true" ]; then echo yes; fi`)
	if err != nil {
		t.Fatalf("expand a bool setting: %v", err)
	}
	if !strings.Contains(expanded, "'false'") {
		t.Fatalf("expanded = %q, want the bool rendered as a string", expanded)
	}

	expanded, err = vars.Expand("{{settings.static_jobs}}")
	if err != nil {
		t.Fatalf("expand a numeric setting: %v", err)
	}
	if !strings.Contains(expanded, "'4'") {
		t.Fatalf("expanded = %q, want the number rendered as a string", expanded)
	}

	// An empty content_version defaults to the *short* revision (spec 10.4), so
	// a retry of the same revision writes the same static URLs instead of
	// busting every cache, and the URL stays readable.
	expanded, err = vars.Expand("{{settings.content_version}}")
	if err != nil {
		t.Fatalf("expand content_version: %v", err)
	}
	if !strings.Contains(expanded, "abcdef12") {
		t.Fatalf("content_version expanded to %q, want the short revision", expanded)
	}
	if strings.Contains(expanded, "abcdef123456") {
		t.Fatalf("content_version expanded to %q, want a short revision", expanded)
	}

	expanded, err = vars.Expand("{{settings.themes_args}}")
	if err != nil {
		t.Fatalf("expand themes_args: %v", err)
	}
	if !strings.HasSuffix(expanded, "-t Vendor/theme en_US") {
		t.Fatalf("a rendered argument list was quoted: %q", expanded)
	}
}
