package deploy

import (
	"errors"
	"regexp"
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
	raw    map[string]bool
}

// NewVars returns an empty variable set.
func NewVars() Vars {
	return Vars{values: map[string]string{}, paths: map[string]bool{}, raw: map[string]bool{}}
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

// SetRaw records a value that is substituted verbatim, with no quoting. It
// exists for argument lists the recipe already rendered (a list of locales, a
// theme list): quoting them would turn several arguments into one. It is never
// used for a value that came from outside the project configuration.
func (v Vars) SetRaw(key, value string) Vars {
	v.values[key] = value
	v.raw[key] = true
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
		if v.raw[name] {
			return value
		}
		return conventions.ShellQuote(value)
	})
	if failure != nil {
		return "", failure
	}
	return expanded, nil
}

// Clone returns an independent copy of the set.
//
// Vars shares its maps between copies, so a caller that layers values onto a
// set it does not own must clone first. The zero value clones to an empty set
// rather than panicking, which is what makes a request struct safe to leave
// half-filled.
func (v Vars) Clone() Vars {
	clone := NewVars()
	for key, value := range v.values {
		clone.values[key] = value
	}
	for key := range v.paths {
		clone.paths[key] = true
	}
	for key := range v.raw {
		clone.raw[key] = true
	}
	return clone
}

// VarsFor builds the variable set available to recipe commands and hooks.
//
// It is the one mapping every runner uses: the CLI deploy, build and
// rollback paths and the desktop apply path all expand steps through it, so
// a step the desktop runs expands exactly like one the CLI runs. The quoting
// rules are load-bearing (see the method comments they mirror): a configured
// wrapper of several words has to reach the command as several words, a value
// carrying shell syntax must stay inert, argument lists render verbatim, and
// an empty shell fragment becomes `true` so `cmd && ` never reaches the shell.
func VarsFor(host Host, opts Options) Vars {
	vars := NewVars().
		SetPath("release_path", "").
		SetPath("previous_release", "").
		SetPath("deploy_path", host.DeployPath).
		SetPath("current_path", host.CurrentPath).
		SetPath("shared_path", host.SharedPath()).
		SetPath("repo_path", host.RepoPath()).
		Set("release", "").
		Set("branch", opts.Branch).
		Set("revision", opts.Revision).
		Set("tag", opts.Tag).
		Set("verify_url", opts.VerifyURL)

	// SetRaw with the words already quoted: a configured wrapper of several
	// words has to reach the command as several words, and a value carrying shell
	// syntax must stay inert. The probes render the same value through the same
	// helper, so `deploy:check` tests the binary the recipe runs.
	vars = vars.SetRaw("php_bin", CommandWords(opts.Settings, "php_bin", "php"))
	vars = vars.SetRaw("composer_bin", CommandWords(opts.Settings, "composer_bin", "composer"))
	for key, value := range opts.Settings {
		if text, ok := SettingText(value); ok {
			vars = vars.Set("settings."+key, text)
		}
	}
	// Argument lists and shell fragments are substituted verbatim: quoting an
	// argument list collapses several arguments into one, and quoting a command
	// turns `npm ci && npm run build` into a single word the shell cannot find.
	// Which settings are fragments is the recipe's declaration, carried on the
	// options (see SettingCommand).
	for key, value := range opts.Settings {
		text, ok := SettingText(value)
		if !ok {
			continue
		}
		switch {
		case strings.HasSuffix(key, "_args"):
			// An argument list renders to nothing when it is empty, because it
			// is appended to a command that already has its own arguments.
			vars = vars.SetRaw("settings."+key, text)
		case opts.RawSettings[key]:
			// A shell fragment is substituted verbatim, and an empty one becomes
			// `true`: rendering nothing at all leaves `cmd && ` behind, which is
			// a syntax error, and `''` is a command the shell cannot find.
			if strings.TrimSpace(text) == "" {
				text = "true"
			}
			vars = vars.SetRaw("settings."+key, text)
		}
	}
	// Deployer's content version was a timestamp, which busts every browser
	// cache on every deploy. The revision is deterministic: a retry or a resume
	// of the same revision writes the same static URLs, and a new revision
	// still changes them. A project can still override it.
	if text, ok := opts.Settings["content_version"].(string); ok && strings.TrimSpace(text) == "" {
		// Short, per spec 10.4: readable in an asset URL, and still different
		// for every revision so a deploy busts caches exactly once.
		vars = vars.Set("settings.content_version", ShortRevision(opts.Revision))
	}
	return vars
}
