package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

func TestSessionStart_NotInitialized(t *testing.T) {
	dir := gitRepo(t)
	if _, err := SessionStart(dir); CodeOf(err) != ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}

func TestSessionStart_EmptyRepo_NoHead(t *testing.T) {
	dir := gitRepo(t) // no commits -> HEAD empty
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	sid, err := SessionStart(dir)
	if err != nil {
		t.Fatalf("session start on empty repo: %v", err)
	}
	l := store.NewLayout(canonRoot(t, dir))
	var bl baseline
	if err := store.ReadJSON(l.SessionBaseline(sid), &bl); err != nil {
		t.Fatal(err)
	}
	if bl.HEAD != "" {
		t.Fatalf("expected empty HEAD on commit-less repo, got %q", bl.HEAD)
	}
	// ignorecase helper should not panic on a real repo
	_ = ignorecase(canonRoot(t, dir))
}

func TestRecover_NoSessions(t *testing.T) {
	dir := gitRepo(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := Recover(dir, ""); CodeOf(err) != ExitUsage {
		t.Fatalf("want ExitUsage (no sessions), got %v", err)
	}
}

func TestRecover_LatestSession(t *testing.T) {
	dir := gitRepo(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := SessionStart(dir); err != nil {
		t.Fatal(err)
	}
	// empty sid -> picks the latest session
	if err := Recover(dir, ""); err != nil {
		t.Fatalf("recover latest: %v", err)
	}
}

func TestRecover_LockBusy(t *testing.T) {
	dir := gitRepo(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := SessionStart(dir); err != nil {
		t.Fatal(err)
	}
	l := store.NewLayout(canonRoot(t, dir))

	held, err := store.AcquireLock(l.RecoverLock())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	if err := Recover(dir, ""); CodeOf(err) != ExitLockTimeout {
		t.Fatalf("want ExitLockTimeout while recover.lock held, got %v", err)
	}
}

func TestInit_Idempotent(t *testing.T) {
	dir := gitRepo(t)
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	if err := Init(dir); err != nil {
		t.Fatalf("second init should be idempotent: %v", err)
	}
	root := canonRoot(t, dir)
	gi, err := os.ReadFile(root + "/.gitignore")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(gi), ".harness/"); n != 1 {
		t.Fatalf(".harness/ duplicated in .gitignore (count=%d):\n%s", n, gi)
	}
}
