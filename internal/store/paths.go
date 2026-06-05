// Package store provides the on-disk layout, atomic writes, and advisory file
// locks that back the harness state substrate (rev3 §2, §4, §9).
package store

import "path/filepath"

// Layout computes every harness path from the repo root. state_root is always
// <repo>/.harness (absolute); it never moves with a job's workdir (rev3 N4).
type Layout struct {
	RepoRoot  string
	StateRoot string
}

// NewLayout returns the layout rooted at an absolute repo path.
func NewLayout(repoRoot string) Layout {
	return Layout{RepoRoot: repoRoot, StateRoot: filepath.Join(repoRoot, ".harness")}
}

func (l Layout) Schemas() string     { return filepath.Join(l.StateRoot, "schemas") }
func (l Layout) Reserved() string    { return filepath.Join(l.StateRoot, "reserved.json") }
func (l Layout) Contract() string    { return filepath.Join(l.StateRoot, "workflow-contract.md") }
func (l Layout) Current() string     { return filepath.Join(l.StateRoot, "current") }
func (l Layout) StateLock() string   { return filepath.Join(l.Current(), "state.lock") }
func (l Layout) RecoverLock() string { return filepath.Join(l.Current(), "recover.lock") }
func (l Layout) StateSummary() string {
	return filepath.Join(l.Current(), "state-summary.md")
}
func (l Layout) Sessions() string { return filepath.Join(l.StateRoot, "sessions") }

func (l Layout) Session(sid string) string {
	return filepath.Join(l.Sessions(), sid)
}
func (l Layout) SessionFile(sid string) string {
	return filepath.Join(l.Session(sid), "session.json")
}
func (l Layout) SessionBaseline(sid string) string {
	return filepath.Join(l.Session(sid), "session-baseline.json")
}
func (l Layout) ActiveTask(sid string) string {
	return filepath.Join(l.Session(sid), "active-task.json")
}
func (l Layout) Events(sid string) string {
	return filepath.Join(l.Session(sid), "events")
}
func (l Layout) EventFile(sid, actor string) string {
	return filepath.Join(l.Events(sid), actor+".jsonl")
}
func (l Layout) Views(sid string) string {
	return filepath.Join(l.Session(sid), "views")
}
func (l Layout) TaskView(sid, tid string) string {
	return filepath.Join(l.Views(sid), "tasks", tid+".json")
}
func (l Layout) JobView(sid, jid string) string {
	return filepath.Join(l.Views(sid), "jobs", jid+".json")
}
func (l Layout) GateView(sid, gid string) string {
	return filepath.Join(l.Views(sid), "gates", gid+".json")
}
func (l Layout) TaskDir(sid, tid string) string {
	return filepath.Join(l.Session(sid), "tasks", tid)
}
func (l Layout) Verification(sid, tid string) string {
	return filepath.Join(l.TaskDir(sid, tid), "verification.json")
}
func (l Layout) Handoff(sid, tid string) string {
	return filepath.Join(l.TaskDir(sid, tid), "handoff.md")
}
func (l Layout) JobsDir(sid string) string {
	return filepath.Join(l.Views(sid), "jobs")
}
func (l Layout) GatesDir(sid string) string {
	return filepath.Join(l.Views(sid), "gates")
}
func (l Layout) Artifacts(sid, jid string) string {
	return filepath.Join(l.Session(sid), "artifacts", jid)
}
func (l Layout) FinalJSON(sid, jid string) string {
	return filepath.Join(l.Artifacts(sid, jid), "final.json")
}

// Findings is the persisted, human+worker-readable output of a read-only
// (analysis/research) job, referenced by downstream implement jobs (rev3
// research→plan→delegate). Path is repo-relative under .harness so a worker in
// any worktree can Read it via repo_root.
func (l Layout) Findings(sid, jid string) string {
	return filepath.Join(l.Session(sid), "findings", jid+".md")
}
func (l Layout) Trash(jid string) string {
	return filepath.Join(l.StateRoot, ".trash", jid)
}
