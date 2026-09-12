package deploy

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// SettingKind is the shape a setting's value must have.
type SettingKind string

const (
	SettingString        SettingKind = "string"
	SettingBool          SettingKind = "bool"
	SettingInt           SettingKind = "int"
	SettingStringList    SettingKind = "string-list"
	SettingStringListMap SettingKind = "string-list-map"
	// SettingArgs is a value the recipe renders into a "<key>_args" argument
	// list (see ArgsSpec), so it may be a string, a list or a map.
	SettingArgs SettingKind = "args"
	// SettingCommand is a shell fragment — a command line the recipe runs — and
	// is substituted verbatim rather than quoted. Quoting it turns
	// `npm ci && npm run build` into one word, and the shell reports a command
	// that does not exist.
	SettingCommand SettingKind = "command"
	// SettingUnsupported is a key the recipe knows about and does not implement.
	// Naming it is better than leaving it unknown, and much better than letting
	// a project believe it is in effect.
	SettingUnsupported SettingKind = "unsupported"
)

// Setting declares one key a recipe understands in `deploy.settings`.
type Setting struct {
	Key   string
	Kind  SettingKind
	Title string
}

// SettingText renders a setting value as the string a command template
// substitutes. Booleans and numbers are rendered too: a recipe that guards a step
// with `[ {{settings.worker_control}} = true ]` must not fail with "unknown
// variable" just because the configuration expressed the value as a bool.
func SettingText(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case bool:
		return strconv.FormatBool(typed), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10), true
		}
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	default:
		return "", false
	}
}

// engineSettings are the settings the engine itself reads. They are declared
// once here rather than by each recipe, because every recipe gets them through
// DefaultRecipe.
var engineSettings = []Setting{
	{Key: "shared_files", Kind: SettingStringList, Title: "files linked from shared/ into the release"},
	{Key: "shared_dirs", Kind: SettingStringList, Title: "directories linked from shared/ into the release"},
	{Key: "writable_dirs", Kind: SettingStringList, Title: "paths made writable in the release"},
	{Key: "writable_mode", Kind: SettingString, Title: "chmod, chown, chmod+chown or skip"},
	{Key: "writable_permissions", Kind: SettingString, Title: "the chmod mode, for example 775"},
	{Key: "owner", Kind: SettingString, Title: "user:group applied to the writable paths"},
	{Key: "sync_paths", Kind: SettingStringList, Title: "paths copied into an in-place docroot"},
	{Key: "php_bin", Kind: SettingString, Title: "the PHP interpreter on the target"},
	{Key: "php_version", Kind: SettingString, Title: "the PHP series the project expects"},
	{Key: "composer_bin", Kind: SettingString, Title: "the Composer binary on the target"},
	{Key: "content_version", Kind: SettingString, Title: "static content version; defaults to the revision"},
}

// ValidateRecipe refuses a recipe that declares the same setting twice.
//
// The declarations behave as a map — the last one wins — so a duplicate is a
// copy-paste bug that silently changes a setting's shape or its description, and
// `ValidateSettings` would validate values against one declaration while
// `deploy plan` prints the other. It is a recipe bug rather than a project
// configuration error, so it is a plain refusal, raised wherever a plan is built
// (every command goes through one).
func ValidateRecipe(recipe Recipe) error {
	seen := make(map[string]bool, len(recipe.Settings))
	for _, setting := range recipe.Settings {
		if seen[setting.Key] {
			return fmt.Errorf("recipe %q declares deploy.settings.%s twice; the second declaration would silently win", recipe.ID, setting.Key)
		}
		seen[setting.Key] = true
	}
	return nil
}

// ValidateSettings refuses an unknown or misshapen `deploy.settings` key.
//
// Spec 5.2 makes this the recipe's job and calls the outcome a configuration
// error: the core never interprets the map, so only the recipe can say whether a
// key exists. Catching it at plan time is the difference between "this deploy
// cannot work, and here is the typo" and a deploy that quietly ignores what the
// operator configured — a `static_content_locale` misspelling used to ship
// content in every locale the recipe defaults to.
//
// A key the recipe renders itself ("<key>_args") is accepted when its base key is
// declared.
func ValidateSettings(recipe Recipe, settings map[string]any) error {
	if len(settings) == 0 {
		return nil
	}
	known := make(map[string]Setting, len(recipe.Settings))
	for _, setting := range recipe.Settings {
		known[setting.Key] = setting
	}

	for _, key := range sortedSettingKeys(settings) {
		setting, declared := known[key]
		if !declared {
			if base, isDerived := strings.CutSuffix(key, "_args"); isDerived {
				if parent, ok := known[base]; ok && parent.Kind == SettingArgs {
					continue
				}
			}
			return fmt.Errorf("%w: deploy.settings.%s is not a setting this recipe knows%s",
				ErrInvalidConfiguration, key, nearestSetting(key, known))
		}
		if setting.Kind == SettingUnsupported {
			return fmt.Errorf("%w: deploy.settings.%s is not implemented yet (%s)",
				ErrInvalidConfiguration, key, setting.Title)
		}
		if err := validateSettingValue(key, setting, settings[key]); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidConfiguration, err)
		}
	}
	return nil
}

func validateSettingValue(key string, setting Setting, value any) error {
	describe := func(want string) error {
		return fmt.Errorf("deploy.settings.%s must be %s (%s), got %T", key, want, setting.Title, value)
	}
	switch setting.Kind {
	case SettingString:
		// Strictly a string, because that is what the readers do: the engine's
		// `settingsString` asserts `value.(string)`, so `php_version: 8.2`
		// unquoted reads as empty and the version check silently compares
		// nothing. Refusing it here says so instead.
		if _, ok := value.(string); ok {
			return nil
		}
		return describe("a quoted string")
	case SettingBool:
		switch typed := value.(type) {
		case bool:
			return nil
		case string:
			if _, err := strconv.ParseBool(strings.TrimSpace(typed)); err == nil {
				return nil
			}
		}
		return describe("true or false")
	case SettingInt:
		switch typed := value.(type) {
		case int, int64, float64:
			return nil
		case string:
			if _, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
				return nil
			}
		}
		return describe("a number")
	case SettingStringList:
		if _, ok := settingStringListValue(value); !ok {
			return describe("a list of strings")
		}
		return nil
	case SettingStringListMap:
		mapped, ok := value.(map[string]any)
		if !ok {
			return describe("a map of lists")
		}
		for entryKey, entry := range mapped {
			if _, ok := settingStringListValue(entry); !ok {
				return fmt.Errorf("deploy.settings.%s.%s must be a list of strings, got %T", key, entryKey, entry)
			}
		}
		return nil
	case SettingCommand:
		if _, ok := value.(string); ok {
			return nil
		}
		return describe("a command line")
	case SettingArgs:
		if settingArgsValue(value) {
			return nil
		}
		return describe("a string, a list of strings or a map of lists")
	default:
		return nil
	}
}

// settingStringListValue reports whether a value is a list of strings, or the
// single string a shared-path reader also accepts. Non-string items are refused
// rather than skipped: `settingsStringList` ignores them, so a number in the list
// is a path govard would silently not link.
func settingStringListValue(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case string:
		return []string{typed}, true
	case []any:
		rendered := make([]string, 0, len(typed))
		for _, entry := range typed {
			text, ok := entry.(string)
			if !ok {
				return nil, false
			}
			rendered = append(rendered, text)
		}
		return rendered, true
	default:
		return nil, false
	}
}

// settingArgsValue reports whether a value is one RenderSettingArgs can turn into
// an argument list: a raw string, a list of strings, or a map of lists.
func settingArgsValue(value any) bool {
	if _, ok := value.(string); ok {
		return true
	}
	if _, ok := value.(map[string][]string); ok {
		return true
	}
	if mapped, ok := value.(map[string]any); ok {
		for _, entry := range mapped {
			if _, ok := settingStringListValue(entry); !ok {
				return false
			}
		}
		return true
	}
	if _, ok := value.([]any); ok {
		_, ok := settingStringListValue(value)
		return ok
	}
	_, ok := value.([]string)
	return ok
}

// SettingsDeclare reports whether the recipe knows this setting. It exists so a
// test can assert that a setting the recipe reads is one it declared.
func (r Recipe) SettingsDeclare(key string) bool {
	for _, setting := range r.Settings {
		if setting.Key == key {
			return true
		}
	}
	return false
}

func sortedSettingKeys(settings map[string]any) []string {
	keys := make([]string, 0, len(settings))
	for key := range settings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// nearestSetting names the declared key closest to a typo, or nothing when none
// is close enough to be worth guessing at.
func nearestSetting(key string, known map[string]Setting) string {
	best, bestDistance := "", 3
	for candidate := range known {
		if distance := editDistance(key, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	if best == "" {
		return ""
	}
	return "; did you mean " + best + "?"
}

// editDistance is the Levenshtein distance, which is enough to suggest a
// misspelled key without pulling in a dependency.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func min(values ...int) int {
	smallest := values[0]
	for _, value := range values[1:] {
		if value < smallest {
			smallest = value
		}
	}
	return smallest
}
