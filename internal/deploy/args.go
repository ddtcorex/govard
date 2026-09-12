package deploy

import (
	"sort"
	"strings"

	"govard/internal/conventions"
)

// ArgsSpec declares that a recipe turns a *list* or *map* setting into command
// arguments. The flag is framework knowledge, so the recipe owns it and the
// engine only knows how to render the values.
//
//	Defaults: {"themes": ArgsSpec{Flag: "-t"}}
//
// A project that writes `themes: {Vendor/theme: [en_US]}` gets
// `-t Vendor/theme --language en_US` in `{{settings.themes_args}}` when the spec
// declares ValueFlag. A project that writes a plain string is passed through
// verbatim, because an operator who typed flags meant those flags.
type ArgsSpec struct {
	Flag string
	// ValueFlag is the flag a *map*'s values are rendered with, and it exists
	// because a bare value is not always a value: a command may take
	// `-t <key>` plus a separate positional argument that its own parser prefers
	// to the option, in which case `-t key value` does not mean "this key with
	// this value" — it means "these keys, with `value` in the positional
	// argument instead of whatever the option said". Declaring ValueFlag renders
	// the map's values as repeated options (`-t a -t b --language en_US
	// --language de_DE`) and leaves the positional argument empty. Empty keeps
	// the loose-argument shape.
	ValueFlag string
	// Words renders the value as a list of shell-quoted words instead of flags,
	// for a recipe that iterates them itself. A path list needs this: the values
	// come from the project, so they must be quoted, and they are not arguments
	// to any flag.
	Words bool
	// Default is the value rendered when the project configured nothing, so a
	// recipe can supply a framework default (the admin theme, for instance)
	// without writing it into every project's configuration.
	Default any
	// DefaultFrom names another setting whose configured value becomes this
	// one's default. It is how "the backend languages default to the frontend
	// ones" is expressed: the two settings must agree unless a project says
	// otherwise.
	DefaultFrom string
}

// RenderSettingArgs renders a setting value as a raw shell argument string.
//
// A string is passed through verbatim — an operator who typed flags meant those
// flags. A list repeats the flag per element (`-t a -t b`). A map repeats the
// flag with the key as its first argument; the key's values follow it, or, when
// ValueFlag is declared, each of them repeats that flag instead (`-t a -t b
// --language en_US`). The values of a map are deduplicated, because a value flag
// is global: a project whose two themes share en_US wants it once.
//
// A spec with Words renders the whole value as shell-quoted words, for a recipe
// that consumes the list itself.
//
// The result is empty when nothing was configured, so a command template can
// reference the variable unconditionally and expand to nothing.
func RenderSettingArgs(spec ArgsSpec, value any) string {
	if spec.Words {
		return shellQuotedWords(argWordsAll(value))
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	if groups, ok := argGroups(value); ok {
		return renderArgGroups(spec, groups)
	}

	words := argWords(value)
	parts := make([]string, 0, len(words))
	for _, word := range words {
		parts = append(parts, flagWords(spec.Flag, []string{word}))
	}
	return strings.Join(parts, " ")
}

// renderArgGroups renders the map shape: one group per key, with the key's values
// either trailing the key or repeated under ValueFlag.
func renderArgGroups(spec ArgsSpec, groups map[string][]string) string {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if spec.ValueFlag == "" {
			parts = append(parts, flagWords(spec.Flag, append([]string{key}, groups[key]...)))
			continue
		}
		parts = append(parts, flagWords(spec.Flag, []string{key}))
	}
	if spec.ValueFlag == "" {
		return strings.Join(parts, " ")
	}

	for _, value := range distinctGroupValues(groups) {
		parts = append(parts, flagWords(spec.ValueFlag, []string{value}))
	}
	return strings.Join(parts, " ")
}

// distinctGroupValues collects a map's values once each, sorted, so the rendered
// argument list is stable and a shared value is not repeated.
func distinctGroupValues(groups map[string][]string) []string {
	seen := map[string]bool{}
	values := make([]string, 0, len(groups))
	for _, group := range groups {
		for _, value := range group {
			if seen[value] {
				continue
			}
			seen[value] = true
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}

// shellQuotedWords renders words as one shell-quoted list, for a recipe that
// iterates it. Quoting is not optional here: a value that came from the project
// could otherwise add a command of its own to the loop.
func shellQuotedWords(words []string) string {
	quoted := make([]string, 0, len(words))
	for _, word := range words {
		quoted = append(quoted, conventions.ShellQuote(word))
	}
	return strings.Join(quoted, " ")
}

func flagWords(flag string, words []string) string {
	if strings.TrimSpace(flag) == "" {
		return strings.Join(words, " ")
	}
	return strings.TrimSpace(flag + " " + strings.Join(words, " "))
}

// RenderSettingArgsForTest exposes RenderSettingArgs to the tests/ package.
func RenderSettingArgsForTest(spec ArgsSpec, value any) string {
	return RenderSettingArgs(spec, value)
}

// argWordsAll flattens any supported shape into the words it names: a string
// splits on whitespace, a list contributes its elements, and a map contributes
// each key followed by its values.
func argWordsAll(value any) []string {
	groups, ok := argGroups(value)
	if !ok {
		return argWords(value)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	words := make([]string, 0, len(keys))
	for _, key := range keys {
		words = append(words, key)
		words = append(words, groups[key]...)
	}
	return cleanWords(words)
}

// argWords flattens a scalar or list setting into words. A string is split on
// whitespace so an operator can write raw flags, and a list contributes one word
// per element.
func argWords(value any) []string {
	switch typed := value.(type) {
	case string:
		return strings.Fields(typed)
	case []string:
		return cleanWords(typed)
	case []any:
		words := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				words = append(words, text)
			}
		}
		return cleanWords(words)
	default:
		return nil
	}
}

// argGroups flattens a map setting into one word list per key, which is the
// "theme -> locales" shape a per-theme deploy needs.
func argGroups(value any) (map[string][]string, bool) {
	switch typed := value.(type) {
	case map[string]any:
		groups := make(map[string][]string, len(typed))
		for key, entry := range typed {
			groups[key] = argWords(entry)
		}
		return groups, true
	case map[string][]string:
		groups := make(map[string][]string, len(typed))
		for key, entry := range typed {
			groups[key] = cleanWords(entry)
		}
		return groups, true
	default:
		return nil, false
	}
}

func cleanWords(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
