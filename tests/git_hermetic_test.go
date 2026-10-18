package tests

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// hermeticGitEnv layers the per-test core.excludesFile pin on top of the
// suite-wide gitHousekeepingBase guards. os/exec takes the LAST
// GIT_CONFIG_COUNT in the environment, so base and pin travel as one entry
// whose count derives from the base length — never a second literal.
func hermeticGitEnv() []string {
	pairs := append(append([][2]string{}, gitHousekeepingBase...), [2]string{"core.excludesFile", "/dev/null"})
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT="+strconv.Itoa(len(pairs)),
	)
	for i, pair := range pairs {
		env = append(env,
			"GIT_CONFIG_KEY_"+strconv.Itoa(i)+"="+pair[0],
			"GIT_CONFIG_VALUE_"+strconv.Itoa(i)+"="+pair[1],
		)
	}
	return env
}

// The hermetic git helpers must layer core.excludesFile on top of the
// suite-wide housekeeping guards (gc.auto=0, maintenance.auto=false): exec
// takes the LAST GIT_CONFIG_COUNT in the environment, so a COUNT=1 override
// silently drops the TestMain guards.
func TestHermeticGitEnvKeepsHousekeepingGuards(t *testing.T) {
	base := gitHousekeepingBase
	env := hermeticGitEnv()
	count := ""
	keys := map[string]string{}
	values := map[string]string{}
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "GIT_CONFIG_COUNT="); ok {
			count = v
		}
		for i := 0; i < len(base)+1; i++ {
			key := "GIT_CONFIG_KEY_" + strconv.Itoa(i)
			if v, ok := strings.CutPrefix(kv, key+"="); ok {
				keys[key] = v
			}
			vkey := "GIT_CONFIG_VALUE_" + strconv.Itoa(i)
			if v, ok := strings.CutPrefix(kv, vkey+"="); ok {
				values[vkey] = v
			}
		}
	}
	// A third guard added to the suite-wide base must flow through here
	// automatically: the count derives from the base length, it is not a
	// second literal to keep in sync.
	if want := strconv.Itoa(len(base) + 1); count != want {
		t.Fatalf("GIT_CONFIG_COUNT = %q, want %q (base guards + excludesFile)", count, want)
	}
	for i, pair := range base {
		if keys["GIT_CONFIG_KEY_"+strconv.Itoa(i)] != pair[0] {
			t.Fatalf("base guard %d lost: keys=%v", i, keys)
		}
		if values["GIT_CONFIG_VALUE_"+strconv.Itoa(i)] != pair[1] {
			t.Fatalf("base guard %d value lost: values=%v", i, values)
		}
	}
	last := len(base)
	if keys["GIT_CONFIG_KEY_"+strconv.Itoa(last)] != "core.excludesFile" || values["GIT_CONFIG_VALUE_"+strconv.Itoa(last)] != "/dev/null" {
		t.Fatalf("excludesFile pin missing: keys=%v values=%v", keys, values)
	}
}
