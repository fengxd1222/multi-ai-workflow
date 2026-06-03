// Package integrate merges completed write-job branches into a dedicated
// integration branch in its own worktree (rev3 §7, §11). The human's main
// working tree is never touched; the deliverable is the harness/integration/<tid>
// branch. Denied changes are rejected before merge (R4) and conflicts abort
// cleanly so no tree is left half-merged (N17).
package integrate

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/scope"
)

// JobBranch is one completed write job to integrate.
type JobBranch struct {
	JobID  string
	Branch string
	Scope  model.Scope
}

// Result reports the integration outcome.
type Result struct {
	IntegrationBranch string
	Merged            []string
	DeniedJob         string          // job rejected pre-merge for out-of-scope changes
	DeniedPaths       []scope.Verdict
	ConflictJob       string          // job whose merge conflicted (then aborted)
}

// OK reports a fully successful integration.
func (r Result) OK() bool { return r.DeniedJob == "" && r.ConflictJob == "" }

// IntegrateTask merges each job branch into harness/integration/<tid> in an
// isolated integration worktree, returning early (without merging) on the first
// denied-change or conflict so the caller can open a gate.
func IntegrateTask(repoRoot, tid string, jobs []JobBranch, rsv scope.Reserved, ignorecase bool) (Result, error) {
	intBranch := "harness/integration/" + tid
	intPath := filepath.Join(repoRoot, ".worktrees", "__int__"+tid)
	res := Result{IntegrationBranch: intBranch}

	mainHEAD, err := git(repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return res, fmt.Errorf("no HEAD to integrate onto")
	}
	if err := setupWorktree(repoRoot, intBranch, intPath, mainHEAD); err != nil {
		return res, err
	}
	defer teardownWorktree(repoRoot, intPath) // keep the branch, drop the worktree

	for _, jb := range jobs {
		// R4: reject changes that touch denied/reserved/out-of-scope before merging.
		if v := diffClassify(repoRoot, mainHEAD, jb.Branch, jb.Scope, rsv, ignorecase); len(v) > 0 {
			res.DeniedJob = jb.JobID
			res.DeniedPaths = v
			return res, nil
		}
		if out, err := git(intPath, "merge", "--no-edit", jb.Branch); err != nil {
			_, _ = git(intPath, "merge", "--abort") // never leave a half-merged tree (N17)
			res.ConflictJob = jb.JobID
			_ = out
			return res, nil
		}
		res.Merged = append(res.Merged, jb.JobID)
	}
	return res, nil
}

// diffClassify returns every non-Allow verdict for the changes a branch
// introduces relative to base.
func diffClassify(repoRoot, base, branch string, sc model.Scope, rsv scope.Reserved, ignorecase bool) []scope.Verdict {
	out, err := git(repoRoot, "diff", "--name-status", "-M", base, branch, "--")
	if err != nil || out == "" {
		return nil
	}
	var violations []scope.Verdict
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		paths := f[1:]
		if f[0][0] == 'R' || f[0][0] == 'C' {
			paths = f[1:] // old + new ends
		}
		for _, p := range paths {
			if v := scope.Classify(p, sc, rsv, ignorecase); v.Decision != scope.Allow {
				violations = append(violations, v)
			}
		}
	}
	return violations
}

func setupWorktree(repoRoot, branch, path, base string) error {
	_, _ = git(repoRoot, "worktree", "remove", "--force", path)
	_, _ = git(repoRoot, "worktree", "prune")
	_, _ = git(repoRoot, "branch", "-D", branch)
	if out, err := git(repoRoot, "worktree", "add", "-b", branch, path, base); err != nil {
		return fmt.Errorf("integration worktree add: %s", out)
	}
	return nil
}

func teardownWorktree(repoRoot, path string) {
	_, _ = git(repoRoot, "worktree", "remove", "--force", path)
	_, _ = git(repoRoot, "worktree", "prune")
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
