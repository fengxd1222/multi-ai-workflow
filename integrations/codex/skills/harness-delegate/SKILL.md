---
name: harness-delegate
description: Delegate a coding task to a harness worker in an isolated git worktree, with ground-truth verify and a reviewable delivery branch — never writing the user's working tree. The first word of the argument is the worker runtime (claude|codex); the rest is the task goal. Use when the user wants to hand a scoped change to harness.
argument-hint: "<runtime: claude|codex> <task goal…>"
allowed-tools: Bash, Read
---

You are the **orchestrator**. Drive the `harness` CLI (via shell) to delegate a
coding task to a worker runtime in its own isolated git worktree, then deliver a
reviewable branch — never writing the user's working tree directly.

Request: `$ARGUMENTS`
(first word is the worker runtime `claude` or `codex`; the rest is the task goal.
If the first word isn't a runtime, default the worker to `claude`.)

Steps — narrate each briefly; stop and ask the user if a gate opens:

1. Preflight: ensure a git repo with a commit (`git rev-parse HEAD`); run
   `harness init` if `.harness/` is missing; then `harness session start`.
2. If `.trellis/` exists, prefer `harness delegate … --trellis-task <slug>` (or
   auto-link via `task.py current`) so the goal/grounding come from Trellis;
   otherwise use the goal text.
3. `harness task create --title "<short title>" --accept "<acceptance>"` → capture `T-…`.
4. `harness delegate --task T-… --role implementation --runtime <runtime>
   --goal "<goal>" --allow '<scope globs>' --verify '<acceptance command>'` → capture `J-…`.
5. `harness run --job J-…` — the worker edits in an isolated worktree; the CLI
   extracts the result, commits it to `job/J-…`, and establishes ground truth
   (git diff scope review + re-running your verify command).
6. Outcome: `completed` → continue. `needs-human` → `harness gate list` /
   `harness gate show --gate G-…`, explain the violation, and ask the user whether
   to `approve` (extend scope + re-run) or `reject`. Never decide scope/secrets for them.
7. `harness verify --task T-… --cmd '<acceptance command>'`,
   `harness integrate --task T-…` (reclaims merged worktrees → `harness/integration/T-…`),
   `harness handoff --task T-…`.
8. Report the delivery branch `harness/integration/T-…`; the user reviews and
   `git merge`s it. Their working tree was never touched.

The deterministic CLI makes every scope/verify/gate decision — you propose, it disposes.
