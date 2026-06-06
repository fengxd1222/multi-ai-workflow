// Package trellis reads a co-located Trellis workspace (.trellis/) so harness can
// act as the execution backend for a Trellis task: it consumes the task's PRD
// (goal/acceptance) and implement.jsonl (files the worker should Read) without
// re-implementing Trellis's formats. Read-only here; write-back (status/journal)
// goes through Trellis's own scripts (task.py / add_session.py) separately.
//
// Schema reference: Trellis docs appendix-a (paths) and appendix-c (task.json).
package trellis

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Project is a detected Trellis workspace rooted at a git repo.
type Project struct {
	Root string // repo root that contains .trellis/
}

func dir(root string) string { return filepath.Join(root, ".trellis") }

// Detect reports whether repoRoot contains a .trellis/ workspace.
func Detect(repoRoot string) (Project, bool) {
	if fi, err := os.Stat(dir(repoRoot)); err == nil && fi.IsDir() {
		return Project{Root: repoRoot}, true
	}
	return Project{}, false
}

// taskJSON mirrors the subset of Trellis task.json that harness consumes.
// Unknown fields are ignored, so Trellis schema additions don't break us.
type taskJSON struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Branch     string `json:"branch"`
	BaseBranch string `json:"base_branch"`
}

// Task is the harness-facing view of a Trellis task.
type Task struct {
	Slug        string
	Title       string
	Status      string // planning | in_progress | completed | review
	Branch      string
	BaseBranch  string
	PRD         string   // prd.md contents (requirements + acceptance criteria)
	ContextRefs []string // repo-relative files from implement.jsonl the worker should Read
}

func (p Project) taskDir(slug string) string {
	return filepath.Join(dir(p.Root), "tasks", slug)
}

// LoadTask reads tasks/<slug>/{task.json, prd.md, implement.jsonl}. Only task.json
// is treated as authoritative metadata; prd.md and implement.jsonl are optional.
func (p Project) LoadTask(slug string) (Task, error) {
	td := p.taskDir(slug)
	if fi, err := os.Stat(td); err != nil || !fi.IsDir() {
		return Task{}, fmt.Errorf("trellis task %q not found under %s", slug, td)
	}
	t := Task{Slug: slug}

	if data, err := os.ReadFile(filepath.Join(td, "task.json")); err == nil {
		var tj taskJSON
		if err := json.Unmarshal(data, &tj); err == nil {
			t.Title, t.Status, t.Branch, t.BaseBranch = tj.Title, tj.Status, tj.Branch, tj.BaseBranch
		}
	}
	if data, err := os.ReadFile(filepath.Join(td, "prd.md")); err == nil {
		t.PRD = strings.TrimSpace(string(data))
	}
	t.ContextRefs = readJSONLFiles(filepath.Join(td, "implement.jsonl"))
	return t, nil
}

// readJSONLFiles parses a Trellis context manifest: one JSON object per line with
// a "file" field (rows: {"file":"...","reason":"..."}). Blank/comment lines are
// skipped; malformed lines are ignored rather than failing the whole read.
func readJSONLFiles(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}
		var row struct {
			File string `json:"file"`
		}
		if json.Unmarshal([]byte(line), &row) == nil && row.File != "" {
			out = append(out, row.File)
		}
	}
	return out
}
