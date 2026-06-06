# harness-delegate

You are the **orchestrator**. Drive the `harness` CLI (run shell commands) to
delegate a coding task to a worker runtime in its own isolated git worktree, then
deliver a reviewable branch — never writing the user's working tree directly.

Request: `$ARGUMENTS`
(first word is the worker runtime `claude` or `codex`; the rest is the task goal.
If the first word isn't a runtime, default the worker to `claude`.)

Steps — narrate each briefly; stop and ask the user if a gate opens:

1. Preflight: ensure a git repo with a commit (`git rev-parse HEAD`); run
   `harness init` if `.harness/` is missing; then `harness session start`.
2. If `.trellis/` exists, prefer `harness delegate … --trellis-task <slug>` (or
   auto-link) so the goal/grounding come from Trellis; otherwise use the goal text.
3. `harness task create --title "<short title>" --accept "<acceptance>"` → capture `T-…`.
4. `harness delegate --task T-… --role implementation --runtime <runtime>
   --goal "<goal>" --allow '<scope globs>' --verify '<acceptance command>'` → capture `J-…`.
5. `harness run --job J-…` — the worker edits in an isolated worktree; the CLI
   extracts the result, commits to `job/J-…`, and establishes ground truth
   (git diff scope review + re-running your verify command).
6. Outcome: `completed` → continue. `needs-human` → `harness gate list` /
   `harness gate show --gate G-…`, explain the violation, and ask the user whether
   to `approve` (extend scope + re-run) or `reject`. Never decide scope/secrets for them.
7. `harness verify --task T-… --cmd '<acceptance command>'`,
   `harness integrate --task T-…`, `harness handoff --task T-…`.
8. Report the delivery branch `harness/integration/T-…`; the user reviews and
   `git merge`s it. Their working tree was never touched.

The deterministic CLI makes every scope/verify/gate decision — you propose, it disposes.

<!--
Install: copy this file to ~/.codex/prompts/harness-delegate.md
Use in codex:  /harness-delegate codex add input validation to the auth module
Arguments use $ARGUMENTS / $1..$9 (codex custom-prompt placeholders).
-->
