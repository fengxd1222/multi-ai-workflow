package integrate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/scope"
)

func gitR(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitR(t, dir, "init", "-q")
	gitR(t, dir, "config", "user.email", "t@t.t")
	gitR(t, dir, "config", "user.name", "t")
	write(t, dir, "src/a.ts", "base\n")
	gitR(t, dir, "add", "-A")
	gitR(t, dir, "commit", "-qm", "base")
	return dir
}

func write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// makeBranch creates job/<jid> with the given files committed (via a temp worktree).
func makeBranch(t *testing.T, repoDir, jid string, files map[string]string) {
	t.Helper()
	wt := filepath.Join(repoDir, ".wt-"+jid)
	gitR(t, repoDir, "worktree", "add", "-q", "-b", "job/"+jid, wt, "HEAD")
	for rel, content := range files {
		write(t, wt, rel, content)
	}
	gitR(t, wt, "add", "-A")
	gitR(t, wt, "commit", "-qm", "job "+jid)
	gitR(t, repoDir, "worktree", "remove", "--force", wt)
}

func TestIntegrate_MergesNonConflicting(t *testing.T) {
	dir := repo(t)
	makeBranch(t, dir, "J-1", map[string]string{"src/x.ts": "x\n"})
	makeBranch(t, dir, "J-2", map[string]string{"src/y.ts": "y\n"})

	sc := model.Scope{Allowed: []string{"src/**"}}
	res, err := IntegrateTask(dir, "T-1", []JobBranch{
		{JobID: "J-1", Branch: "job/J-1", Scope: sc},
		{JobID: "J-2", Branch: "job/J-2", Scope: sc},
	}, scope.Reserved{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || len(res.Merged) != 2 {
		t.Fatalf("expected clean merge of 2: %+v", res)
	}
	// integration branch contains both files
	out, _ := exec.Command("git", "-C", dir, "ls-tree", "-r", "--name-only", res.IntegrationBranch).Output()
	if got := string(out); !contains(got, "src/x.ts") || !contains(got, "src/y.ts") {
		t.Fatalf("integration branch missing files:\n%s", got)
	}
}

func TestIntegrate_RejectsDeniedChange(t *testing.T) {
	dir := repo(t)
	makeBranch(t, dir, "J-3", map[string]string{"package.json": "{}\n"})
	sc := model.Scope{Allowed: []string{"src/**"}, Denied: []string{"package.json"}}
	res, err := IntegrateTask(dir, "T-1", []JobBranch{{JobID: "J-3", Branch: "job/J-3", Scope: sc}}, scope.Reserved{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.DeniedJob != "J-3" || res.OK() {
		t.Fatalf("expected denied rejection: %+v", res)
	}
}

func TestIntegrate_AbortsOnConflict(t *testing.T) {
	dir := repo(t)
	// both modify the same committed file differently -> conflict
	makeBranch(t, dir, "J-1", map[string]string{"src/a.ts": "v1\n"})
	makeBranch(t, dir, "J-2", map[string]string{"src/a.ts": "v2\n"})
	sc := model.Scope{Allowed: []string{"src/**"}}
	res, err := IntegrateTask(dir, "T-1", []JobBranch{
		{JobID: "J-1", Branch: "job/J-1", Scope: sc},
		{JobID: "J-2", Branch: "job/J-2", Scope: sc},
	}, scope.Reserved{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ConflictJob != "J-2" {
		t.Fatalf("expected conflict on J-2: %+v", res)
	}
	// main working tree must not be left mid-merge
	st, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if len(st) != 0 {
		t.Fatalf("main tree dirtied by integrate:\n%s", st)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
