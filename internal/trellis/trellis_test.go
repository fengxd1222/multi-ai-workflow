package trellis

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Detect(dir); ok {
		t.Fatal("should not detect .trellis in an empty dir")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".trellis"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := Detect(dir); !ok {
		t.Fatal("should detect .trellis after it exists")
	}
}

func writeTask(t *testing.T, root, slug string, files map[string]string) {
	t.Helper()
	td := filepath.Join(root, ".trellis", "tasks", slug)
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(td, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadTask(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "02-27-user-login", map[string]string{
		"task.json": `{"id":"02-27-user-login","title":"Add login validation","status":"in_progress","branch":"feat/login","base_branch":"main","meta":{}}`,
		"prd.md":    "# Login\n\nValidate inputs. Acceptance: empty -> E_BADPASS.\n",
		"implement.jsonl": `{"file":".trellis/spec/style-guide.md","reason":"style"}
{"file":"src/auth/service.js","reason":"target"}

// a comment line
{"bad json
{"file":"","reason":"empty file ignored"}
`,
	})

	proj, ok := Detect(root)
	if !ok {
		t.Fatal("detect")
	}
	task, err := proj.LoadTask("02-27-user-login")
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Add login validation" || task.Status != "in_progress" ||
		task.Branch != "feat/login" || task.BaseBranch != "main" {
		t.Fatalf("task.json fields wrong: %+v", task)
	}
	if task.PRD == "" || task.PRD[0] != '#' {
		t.Fatalf("prd not loaded: %q", task.PRD)
	}
	want := []string{".trellis/spec/style-guide.md", "src/auth/service.js"}
	if len(task.ContextRefs) != len(want) {
		t.Fatalf("context refs = %v, want %v", task.ContextRefs, want)
	}
	for i := range want {
		if task.ContextRefs[i] != want[i] {
			t.Fatalf("context ref[%d]=%q want %q", i, task.ContextRefs[i], want[i])
		}
	}
}

func TestLoadTask_Missing(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".trellis"), 0o755)
	proj, _ := Detect(root)
	if _, err := proj.LoadTask("nope"); err == nil {
		t.Fatal("missing task should error")
	}
}

func TestLoadTask_MinimalNoJSONL(t *testing.T) {
	root := t.TempDir()
	writeTask(t, root, "03-01-x", map[string]string{
		"task.json": `{"id":"03-01-x","title":"X","status":"planning"}`,
	})
	proj, _ := Detect(root)
	task, err := proj.LoadTask("03-01-x")
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "X" || len(task.ContextRefs) != 0 || task.PRD != "" {
		t.Fatalf("minimal task wrong: %+v", task)
	}
}
