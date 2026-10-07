package tests

import (
	"reflect"
	"strings"
	"testing"

	"govard/internal/engine"
	"govard/internal/verify"
)

// verify's children have no tty, so a REMOTE-WRITE bootstrap that waits for a
// confirmation always exits 1 ("confirmation required to proceed with
// bootstrap"). The two rows carry -y so an opted-in run really bootstraps, and
// their titles name that exact argv.
func TestRemoteWriteBootstrapRowsConfirmWithYes(t *testing.T) {
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	cfg := engine.Config{Framework: "magento2"}
	argvs := captureItemArgvs(t, cfg, verify.VerifyOpts{Remote: "sandbox", ProjectRoot: t.TempDir()})
	byID := map[string]verify.Item{}
	for _, it := range verify.RegistryFor(cfg) {
		byID[it.ID] = it
	}
	for id, want := range map[string][]string{
		"P2-05": {"bootstrap", "-e", "sandbox", "--no-noise", "-y"},
		"P2-08": {"bootstrap", "--clone", "-e", "sandbox", "--no-noise", "-y"},
	} {
		if got := argvs[id]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s argv = %v, want %v", id, got, want)
		}
		title := byID[id].Title
		command := strings.ReplaceAll(strings.Join(want, " "), "sandbox", "<remote>")
		if !strings.Contains(title, command) {
			t.Errorf("%s title %q does not contain its argv `%s`", id, title, command)
		}
	}
}
