package deploy

import (
	"fmt"
	"strings"

	"govard/internal/engine"
)

// Position says where a hook is inserted relative to its anchor.
type Position string

const (
	PositionBefore Position = "before"
	PositionAfter  Position = "after"
)

// AnchorKind classifies what a hook is attached to.
type AnchorKind string

const (
	AnchorTask  AnchorKind = "task"
	AnchorStage AnchorKind = "stage"
	AnchorHook  AnchorKind = "hook"
)

// Hook is one project- or recipe-contributed step. Every hook has a stable id
// (`hook:<name>`), which is what lets another hook anchor on it.
type Hook struct {
	Name     string
	On       string
	Position Position
	Order    int
	Run      string
	RunOn    RunOn
	Optional bool
	Source   string
}

// HookID returns the plan step id of this hook.
func (h Hook) HookID() string { return "hook:" + h.Name }

// HookFromConfig converts a project hook from configuration into a Hook,
// applying the documented defaults: position after, run on the remote, and a
// required (non-optional) step.
func HookFromConfig(cfg engine.DeployHookConfig) (Hook, error) {
	hook := Hook{
		Name:     strings.TrimSpace(cfg.Name),
		On:       strings.TrimSpace(cfg.On),
		Position: Position(strings.ToLower(strings.TrimSpace(cfg.Position))),
		Order:    cfg.Order,
		Run:      cfg.Run,
		RunOn:    RunOn(strings.ToLower(strings.TrimSpace(cfg.RunOn))),
		Optional: cfg.Optional,
		Source:   "config",
	}
	if hook.Name == "" {
		return Hook{}, fmt.Errorf("deploy hook: name is required")
	}
	if hook.On == "" {
		return Hook{}, fmt.Errorf("deploy hook %q: on is required", hook.Name)
	}
	if strings.TrimSpace(hook.Run) == "" {
		return Hook{}, fmt.Errorf("deploy hook %q: run is required", hook.Name)
	}
	if hook.Position == "" {
		hook.Position = PositionAfter
	}
	if hook.Position != PositionBefore && hook.Position != PositionAfter {
		return Hook{}, fmt.Errorf("deploy hook %q: position must be %q or %q", hook.Name, PositionBefore, PositionAfter)
	}
	if hook.RunOn == "" {
		hook.RunOn = RunRemote
	}
	if hook.RunOn != RunRemote && hook.RunOn != RunLocal {
		return Hook{}, fmt.Errorf("deploy hook %q: run_on must be %q or %q", hook.Name, RunRemote, RunLocal)
	}
	return hook, nil
}

// parseAnchor classifies a hook's `on` value. The caller resolves the anchor
// against the plan and reports an unknown anchor at plan-build time.
func parseAnchor(raw string) (AnchorKind, string, error) {
	value := strings.TrimSpace(raw)
	switch {
	case value == "":
		return "", "", fmt.Errorf("empty hook anchor")
	case strings.HasPrefix(value, "stage:"):
		name := strings.TrimSpace(strings.TrimPrefix(value, "stage:"))
		if _, ok := stageByName(name); !ok {
			return "", "", fmt.Errorf("unknown stage %q", name)
		}
		return AnchorStage, name, nil
	case strings.HasPrefix(value, "hook:"):
		name := strings.TrimSpace(strings.TrimPrefix(value, "hook:"))
		if name == "" {
			return "", "", fmt.Errorf("empty hook anchor name")
		}
		return AnchorHook, name, nil
	default:
		return AnchorTask, value, nil
	}
}

func stageByName(name string) (Stage, bool) {
	for _, stage := range StageOrder {
		if string(stage) == name {
			return stage, true
		}
	}
	return "", false
}
