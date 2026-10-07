package verify

import (
	"context"
	"os/exec"
	"strings"
)

// diffBaseCandidates is the order the diff-scope row looks for a base when the
// run was not given --base. The first ref that really exists wins.
var diffBaseCandidates = []string{"origin/master", "origin/main", "master", "main"}

// ResolveDiffBase picks the base ref for a diff-scope audit. An explicit base
// always wins and is passed through untouched (the audit judges it). Without
// one the first of origin/master, origin/main, master and main that resolves to
// a commit in root is used. ok is false when none exists or root is not a git
// repository: `audit run --scope diff` silently widens the scope to the whole
// project when its base ref is missing, so the caller must not guess one.
func ResolveDiffBase(ctx context.Context, root, explicit string) (string, bool) {
	if base := strings.TrimSpace(explicit); base != "" {
		return base, true
	}
	for _, ref := range diffBaseCandidates {
		cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--quiet", ref+"^{commit}")
		cmd.Dir = root
		if cmd.Run() == nil {
			return ref, true
		}
	}
	return "", false
}
