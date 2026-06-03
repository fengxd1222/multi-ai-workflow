package scope

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

// Change statuses returned by ChangedSet.
const (
	StatusModified    = "modified"
	StatusAdded       = "added"
	StatusDeleted     = "deleted"
	StatusRenamedFrom = "renamed_from"
	StatusRenamedTo   = "renamed_to"
	StatusUntracked   = "untracked"
	StatusIgnored     = "ignored"
)

// ChangedSet returns the real change set in a git worktree, keyed by
// slash-relative path. It deliberately uses BOTH `git diff --name-status -M`
// (tracked changes incl. rename old+new) AND `git status --porcelain
// --untracked-files=all --ignored` (new untracked + .gitignore'd files) — never
// just `git diff --name-only`, which is blind to new and ignored files
// (rev3 §9 N14/N15/N18).
func ChangedSet(workdir, baseCommit string) (map[string]string, error) {
	out := map[string]string{}

	if baseCommit != "" {
		lines, err := gitLines(workdir, "diff", "--name-status", "-M", baseCommit, "--")
		if err != nil {
			return nil, err
		}
		for _, ln := range lines {
			f := strings.Split(ln, "\t")
			if len(f) < 2 {
				continue
			}
			switch f[0][0] {
			case 'A':
				out[f[1]] = StatusAdded
			case 'M', 'T':
				out[f[1]] = StatusModified
			case 'D':
				out[f[1]] = StatusDeleted
			case 'R', 'C':
				if len(f) >= 3 {
					out[f[1]] = StatusRenamedFrom // old end
					out[f[2]] = StatusRenamedTo   // new end
				}
			}
		}
	}

	lines, err := gitLines(workdir, "status", "--porcelain", "--untracked-files=all", "--ignored")
	if err != nil {
		return nil, err
	}
	for _, ln := range lines {
		if len(ln) < 4 {
			continue
		}
		code := ln[:2]
		path := strings.Trim(strings.TrimSpace(ln[3:]), `"`)
		switch code {
		case "??":
			out[path] = StatusUntracked
		case "!!":
			out[path] = StatusIgnored
		}
	}
	return out, nil
}

// LoadReserved reads .harness/reserved.json. Extra fields (e.g. _comment) are
// ignored.
func LoadReserved(path string) (Reserved, error) {
	var r Reserved
	data, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(data, &r)
}

func gitLines(workdir string, args ...string) ([]string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workdir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	s := strings.TrimRight(string(out), "\n")
	if s == "" {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}
