---
description: Delegate a coding task to a harness worker (isolated worktree + ground-truth verify), end to end.
argument-hint: <runtime: claude|codex> <task goal…>
allowed-tools: Bash(harness *), Bash(git *), Bash(ls *), Bash(cat *)
---

You are the **orchestrator**. Drive the `harness` CLI to delegate this task to a
worker runtime, in its own isolated git worktree, and deliver a reviewable branch
— never writing the user's working tree directly.

Request: `$ARGUMENTS`
(first word is the runtime `claude` or `codex`; the rest is the task goal. Default
runtime to `codex` if the first word isn't a runtime.)

Do this, narrating each step briefly and stopping to ask the user if a gate opens:

1. **Preflight.** Confirm we're in a git repo with at least one commit
   (`git rev-parse HEAD`). If `.harness/` is absent, run `harness init`.
   Run `harness session start` (idempotent enough; a fresh session is fine).
2. **Trellis (optional).** If `.trellis/` exists, prefer letting harness pull the
   goal/grounding from it — you can pass `--trellis-task <slug>` to `harness
   delegate`, or rely on auto-link if a task is active. Otherwise use the goal text.
3. **Create the task.** `harness task create --title "<short title>" --accept "<acceptance>"`.
   Capture the `T-…` id from the output.
4. **Delegate a write job.** `harness delegate --task T-… --role implementation
   --runtime <runtime> --goal "<goal>" --allow '<scope globs>' --verify '<a real
   acceptance command>'`. Choose scope globs and a verify command from the task.
   Capture the `J-…` id.
5. **Run it.** `harness run --job J-…`. The worker edits in an isolated worktree;
   the CLI extracts the result, commits to `job/J-…`, then establishes ground truth
   (git diff scope review + re-running your verify command).
6. **Handle the outcome.**
   - `completed` → continue.
   - `needs-human` → run `harness gate list` and `harness gate show --gate G-…`,
     explain the violation to the user, and ask whether to `approve` (extend scope,
     re-run) or `reject` (abandon). Do **not** decide for them on scope/secrets.
7. **Verify + integrate + handoff.**
   `harness verify --task T-… --cmd '<acceptance command>'`,
   then `harness integrate --task T-…` (merges job branches →
   `harness/integration/T-…`), then `harness handoff --task T-…`.
8. **Report.** Print the delivery branch (`harness/integration/T-…`) and remind the
   user that their working tree is untouched — they review and `git merge` it.

Keep commands explicit and let the deterministic CLI make every scope/verify/gate
decision. You propose; the CLI disposes.
