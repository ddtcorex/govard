package cmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pterm/pterm"
)

func stubTunnelContainerRuntime(t *testing.T, available bool) {
	t.Helper()
	original := tunnelContainerRuntimeAvailable
	tunnelContainerRuntimeAvailable = func() bool { return available }
	t.Cleanup(func() { tunnelContainerRuntimeAvailable = original })
}

func TestTunnelStopWarnsWhenRevertFails(t *testing.T) {
	stubTunnelContainerRuntime(t, true)

	var output bytes.Buffer
	pterm.DisableColor()
	pterm.SetDefaultOutput(&output)
	t.Cleanup(func() {
		pterm.EnableColor()
		pterm.SetDefaultOutput(os.Stdout)
	})

	revertErr := errors.New("ssh: connection timed out")
	revertTunnelBaseURLOnStop(func() error { return revertErr })

	message := output.String()
	if !strings.Contains(message, "base URL was not reverted") {
		t.Fatalf("output %q does not warn about the base URL", message)
	}
	if !strings.Contains(message, revertErr.Error()) {
		t.Fatalf("output %q does not carry the revert error", message)
	}
}

// The revert runs `docker exec` inside the project container while `tunnel stop`
// declares no requirement, so on a host without a container runtime the command
// must skip it with a plain note instead of leaking a raw runtime error.
func TestTunnelStopSkipsRevertWithoutContainerRuntime(t *testing.T) {
	stubTunnelContainerRuntime(t, false)

	var output bytes.Buffer
	pterm.DisableColor()
	pterm.SetDefaultOutput(&output)
	t.Cleanup(func() {
		pterm.EnableColor()
		pterm.SetDefaultOutput(os.Stdout)
	})

	reverted := false
	revertTunnelBaseURLOnStop(func() error { reverted = true; return nil })
	if reverted {
		t.Fatal("the revert ran without a container runtime")
	}

	message := output.String()
	if !strings.Contains(message, "No container runtime available") {
		t.Fatalf("output %q does not explain the skip", message)
	}
	if strings.Contains(strings.ToLower(message), "docker") || strings.Contains(message, "missing capability") {
		t.Fatalf("output %q leaks the container runtime into a requirement-free command", message)
	}
}

// TestTunnelHostSlotIsRaceFree exercises the hand-off between the stderr monitor
// and the deferred cleanup, which run on different goroutines. Run with -race.
func TestTunnelHostSlotIsRaceFree(t *testing.T) {
	slot := &tunnelHostSlot{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			slot.set("sample.trycloudflare.com")
			_ = slot.get()
		}
	}()

	for i := 0; i < 200; i++ {
		_ = slot.get()
	}
	<-done

	if got := slot.get(); got != "sample.trycloudflare.com" {
		t.Fatalf("slot holds %q, want the last host set", got)
	}
}
