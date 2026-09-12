package deploy

import (
	"sort"
	"strings"
)

// ArgsSpec declares that a recipe turns a *list* or *map* setting into command
// arguments. The flag is framework knowledge, so the recipe owns it and the
// engine only knows how to render the values.
//
//	Defaults: {"themes": ArgsSpec{Flag: "-t"}}
//
// A project that writes `themes: {Vendor/theme: [en_US]}` gets
// `-t Vendor/theme en_US` in `{{settings.themes_args}}`. A project that writes a
// plain string is passed through verbatim, because an operator who typed flags
// meant those flags.
type ArgsSpec struct {
	Flag string
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
// flag with the key as its first argument and the key's values after it
// (`-t Vendor/theme en_US`), which is the per-theme locale shape.
//
// The result is empty when nothing was configured, so a command template can
// reference the variable unconditionally and expand to nothing.
func RenderSettingArgs(spec ArgsSpec, value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	if groups, ok := argGroups(value); ok {
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, flagWords(spec.Flag, append([]string{key}, groups[key]...)))
		}
		return strings.Join(parts, " ")
	}

	words := argWords(value)
	parts := make([]string, 0, len(words))
	for _, word := range words {
		parts = append(parts, flagWords(spec.Flag, []string{word}))
	}
	return strings.Join(parts, " ")
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
