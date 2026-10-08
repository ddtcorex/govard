package deploy

import (
	"fmt"
	"os"
	"path/filepath"
)

// sandboxSeedPendingName is the marker a seed writes before it starts and
// removes when it completes. It lives in the sandbox state directory, which
// goes away with `down --purge`.
const sandboxSeedPendingName = "seed-pending"

func sandboxSeedPendingPath(projectRoot string) string {
	return filepath.Join(SandboxStateDir(projectRoot), sandboxSeedPendingName)
}

func sandboxSeedPending(projectRoot string) bool {
	_, err := os.Stat(sandboxSeedPendingPath(projectRoot))
	return err == nil
}

func markSandboxSeedPending(projectRoot string) error {
	path := sandboxSeedPendingPath(projectRoot)
	if err := os.WriteFile(path, []byte("the sandbox seed started and has not finished\n"), 0o644); err != nil {
		return fmt.Errorf("record the pending seed in %s: %w", path, err)
	}
	return nil
}

func clearSandboxSeedPending(projectRoot string) {
	_ = os.Remove(sandboxSeedPendingPath(projectRoot))
}
