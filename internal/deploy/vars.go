package deploy

import (
	"errors"
	"regexp"
	"sort"
	"strings"

	"govard/internal/conventions"
	"govard/internal/engine/remote"
)

// ErrUnknownVariable is returned when a command template references a variable
// the executor did not define. Failing loudly is deliberate: a silently empty
// substitution turns a deploy command into a destructive one.
var ErrUnknownVariable = errors.New("unknown deploy variable")

// Vars is the variable set available to recipe commands and hooks.
//
// Values are shell-quoted on substitution, so a value can never break out of
// its argument. Paths are quoted differently on purpose: `~` inside single
// quotes is not expanded by the remote shell, so a path keeps its `$HOME/`
// prefix (the same rule remote.QuoteRemotePath already encodes).
type Vars struct {
	values map[string]string
	paths  map[string]bool
}

// NewVars returns an empty variable set.
func NewVars() Vars {
	return Vars{values: map[string]string{}, paths: map[string]bool{}}
}

// Set records a plain value.
func (v Vars) Set(key, value string) Vars {
	v.values[key] = value
	return v
}

// SetPath records a value that is a filesystem path.
func (v Vars) SetPath(key, value string) Vars {
	v.values[key] = value
	v.paths[key] = true
	return v
}

var varPattern = regexp.MustCompile(`\{\{([A-Za-z0-9_.]+)\}\}`)

// Expand substitutes every `{{name}}` with its quoted value. Substitution is
// single-pass: substituted text is never rescanned.
func (v Vars) Expand(raw string) (string, error) {
	var failure error
	expanded := varPattern.ReplaceAllStringFunc(raw, func(match string) string {
		name := strings.TrimSpace(match[2 : len(match)-2])
		value, ok := v.values[name]
		if !ok {
			if failure == nil {
				failure = ErrUnknownVariable
			}
			return match
		}
		if v.paths[name] {
			return remote.QuoteRemotePath(value)
		}
		return conventions.ShellQuote(value)
	})
	if failure != nil {
		return "", failure
	}
	return expanded, nil
}

// Has reports whether a variable is defined.
func (v Vars) Has(name string) bool {
	_, ok := v.values[name]
	return ok
}

// Names returns the defined variable names, sorted, for `deploy plan` output.
func (v Vars) Names() []string {
	names := make([]string, 0, len(v.values))
	for name := range v.values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
