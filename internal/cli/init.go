package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	harness "github.com/fengxd1222/multi-ai-workflow"
	"github.com/fengxd1222/multi-ai-workflow/internal/store"
)

const workflowContract = `# Workflow Contract (harness v1)

LLM proposes, CLI disposes. State machine / scope / gate / phase / verify
decisions are made only by the deterministic CLI; worker self-reports
(changed_files / verification / usage) are informational-only.

Roles: analysis · implementation · test · review · verification · integration
  write roles (implementation/test/integration) run in a worktree;
  read-only roles run in the shared main tree.

events/<actor>.jsonl is the single source of truth; views/ are rebuildable.
harness never writes your main working tree: results land on
harness/integration/<task> for you to merge.

Exit codes: 0 ok · 10 blocked-policy · 12 needs-human · 20 verify-failed ·
22 result-invalid · 30 state-corrupt · 31 lock-timeout · 32 cas-retry ·
40 runtime-exec-failed · 41 budget-exceeded · 42 delegation-loop
`

// Init initializes .harness/ inside the git repo containing dir (rev3 §2, §16).
// It refuses to run outside a git repo and refuses if .harness is tracked.
func Init(dir string) error {
	root, err := repoRoot(dir)
	if err != nil {
		return coded(ExitUsage,
			"%s is not inside a git repository; run `git init` first (harness will not do it for you)", dir)
	}
	if isTracked(root, ".harness") {
		return coded(ExitStateCorrupt,
			".harness/ is tracked by git; run `git rm -r --cached .harness` then retry")
	}

	l := store.NewLayout(root)
	if err := os.MkdirAll(l.Schemas(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(l.Current(), 0o755); err != nil {
		return err
	}

	if err := writeTemplates(l); err != nil {
		return err
	}
	if err := store.WriteAtomic(l.Contract(), []byte(workflowContract), 0o644); err != nil {
		return err
	}
	if err := ensureGitignore(root); err != nil {
		return err
	}

	fmt.Printf("initialized harness at %s\n", l.StateRoot)
	return nil
}

// writeTemplates copies every embedded schemas/* file into .harness/, routing
// reserved.json to the state root and *.schema.json to schemas/.
func writeTemplates(l store.Layout) error {
	entries, err := harness.Templates.ReadDir("schemas")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := harness.Templates.ReadFile("schemas/" + e.Name())
		if err != nil {
			return err
		}
		dst := filepath.Join(l.Schemas(), e.Name())
		if e.Name() == "reserved.json" {
			dst = l.Reserved()
		}
		if err := store.WriteAtomic(dst, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ensureGitignore appends .harness/ and .worktrees/ if absent (rev3 §2).
func ensureGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	existing := ""
	if f, err := os.Open(path); err == nil {
		b, _ := io.ReadAll(f)
		f.Close()
		existing = string(b)
	}
	have := map[string]bool{}
	for _, line := range strings.Split(existing, "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var add []string
	for _, want := range []string{".harness/", ".worktrees/"} {
		if !have[want] {
			add = append(add, want)
		}
	}
	if len(add) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString(existing)
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\n# harness runtime state (never commit)\n")
	b.WriteString(strings.Join(add, "\n"))
	b.WriteString("\n")
	return store.WriteAtomic(path, []byte(b.String()), 0o644)
}
