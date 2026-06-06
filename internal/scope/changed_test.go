package scope

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	must(t, os.MkdirAll(filepath.Dir(p), 0o755))
	must(t, os.WriteFile(p, []byte(content), 0o644))
}

func head(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out[:len(out)-1])
}

// setupRepo creates a repo with src/auth/a.ts committed and .gitignore covering
// .env and dist/.
func setupRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	git(t, dir, "config", "user.email", "t@t.t")
	git(t, dir, "config", "user.name", "t")
	writeFile(t, dir, ".gitignore", ".env\ndist/\n")
	writeFile(t, dir, "src/auth/a.ts", "export const x = 1\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "base")
	return dir
}

// TestChangedSet_Case1_NewUntrackedFile: a worker-created file outside scope is
// caught via porcelain (git diff --name-only would miss it) — rev3 N14.
func TestChangedSet_Case1_NewUntrackedFile(t *testing.T) {
	dir := setupRepo(t)
	base := head(t, dir)
	writeFile(t, dir, "infra/deploy.sh", "rm -rf /\n")

	cs, err := ChangedSet(dir, base)
	must(t, err)
	if cs["infra/deploy.sh"] != StatusUntracked {
		t.Fatalf("new untracked not captured: %v", cs)
	}
	v := Classify("infra/deploy.sh", model.Scope{Allowed: []string{"src/**"}}, Reserved{}, false)
	if v.Decision != Gate {
		t.Fatalf("scope-outside new file should default-deny, got %s", v.Decision)
	}
}

// TestChangedSet_Case2_IgnoredFile: a .gitignore'd .env is invisible to diff but
// caught via --ignored, then hits the reserved layer — rev3 N15.
func TestChangedSet_Case2_IgnoredFile(t *testing.T) {
	dir := setupRepo(t)
	base := head(t, dir)
	writeFile(t, dir, ".env", "SECRET=1\n")

	cs, err := ChangedSet(dir, base)
	must(t, err)
	if cs[".env"] != StatusIgnored {
		t.Fatalf(".env (ignored) not captured: %v", cs)
	}
	v := Classify(".env", model.Scope{Allowed: []string{"**"}}, Reserved{Patterns: []string{"**/.env"}}, false)
	if v.Decision != DenyReserved {
		t.Fatalf(".env must hit reserved, got %s", v.Decision)
	}
}

// TestChangedSet_Case3_RenameBothEnds: git mv to an out-of-scope path surfaces
// both rename ends; the new end is out of scope — rev3 N18.
func TestChangedSet_Case3_RenameBothEnds(t *testing.T) {
	dir := setupRepo(t)
	base := head(t, dir)
	must(t, os.MkdirAll(filepath.Join(dir, "src", "payments"), 0o755))
	git(t, dir, "mv", "src/auth/a.ts", "src/payments/stolen.ts")

	cs, err := ChangedSet(dir, base)
	must(t, err)
	if cs["src/auth/a.ts"] != StatusRenamedFrom {
		t.Fatalf("rename old end missing: %v", cs)
	}
	if cs["src/payments/stolen.ts"] != StatusRenamedTo {
		t.Fatalf("rename new end missing: %v", cs)
	}
	v := Classify("src/payments/stolen.ts", model.Scope{Allowed: []string{"src/auth/**"}}, Reserved{}, false)
	if v.Decision != Gate {
		t.Fatalf("rename target out of scope should default-deny, got %s", v.Decision)
	}
}

func TestLoadReserved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "reserved.json")
	must(t, os.WriteFile(p, []byte(`{"_comment":"x","patterns":["**/.env"],"reserved_by_human":["secret.txt"]}`), 0o644))
	r, err := LoadReserved(p)
	must(t, err)
	if len(r.Patterns) != 1 || r.Patterns[0] != "**/.env" || r.ReservedByHuman[0] != "secret.txt" {
		t.Fatalf("loaded reserved wrong: %+v", r)
	}
}
