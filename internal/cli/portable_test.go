package cli

import (
	"os"
	"testing"
)

// TestBootID_StableNonEmpty pins the invariant the pid-reuse guard relies on
// (rev3 N1/N5): within one boot, bootID() must return a stable, non-empty
// identifier. Empty or drifting values silently disable stale-job detection.
// Runs on whatever platform the suite executes on, exercising that platform's
// implementation (bootid_unix.go / bootid_windows.go).
func TestBootID_StableNonEmpty(t *testing.T) {
	first := bootID()
	if first == "" {
		t.Fatal("bootID() returned empty; pid-reuse guard would degrade to a bare pid check")
	}
	if second := bootID(); second != first {
		t.Fatalf("bootID() not stable within a boot: %q != %q", first, second)
	}
}

// TestProcessAlive_Self confirms the platform liveness probe (live_unix.go /
// live_windows.go) reports this very process as alive and a never-valid pid as
// dead, with matching boot ids so the boot-guard does not short-circuit.
func TestProcessAlive_Self(t *testing.T) {
	boot := bootID()
	if !processAlive(os.Getpid(), boot, boot) {
		t.Fatal("processAlive() must report the current process as alive")
	}
	if processAlive(-1, boot, boot) {
		t.Fatal("processAlive() must report a non-positive pid as dead")
	}
}

// TestProcessAlive_BootMismatch confirms a pid recorded under a different boot
// is treated as dead even if that pid currently exists — the core of cross-
// reboot pid-reuse protection.
func TestProcessAlive_BootMismatch(t *testing.T) {
	if processAlive(os.Getpid(), "boot-A", "boot-B") {
		t.Fatal("a pid from a different boot must be treated as dead")
	}
}
