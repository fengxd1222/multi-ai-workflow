# harness

A **local, daemonless, database-free CLI harness** for coordinating multi-agent
work across **Codex** and **Claude Code**. One task is decomposed into jobs, each
job runs a real CLI worker in an **isolated git worktree**, and the harness
verifies, integrates, and hands off the result as a branch — **without ever
touching your main working tree**.

> Status: v1 feature-complete. 11 internal packages, all tests green, `go vet`
> clean, `-race` clean, ≥80% coverage per package. Validated end-to-end against
> real `claude` 2.1.156 and `codex` 0.135.0.

---

## Why

Codex and Claude Code both ship mature non-interactive modes (`codex exec`,
`claude -p`), structured JSON output, sandboxes, and lifecycle hooks. So instead
of building a third "brain", harness is a thin **protocol + state + git** layer
that treats both CLIs as interchangeable *runtimes*:

- **Protocol before runtime** — `.harness/` files define session / task / job /
  gate semantics; Codex and Claude are just executors behind one adapter.
- **Events are the single source of truth** — an append-only event log per
  actor; all JSON views are rebuildable. Survives crash / compact / resume.
- **LLM proposes, CLI disposes** — every state-machine, scope, gate, and verify
  decision is made by deterministic Go code, never by a model. Worker
  self-reports (`changed_files`, `verification`, `usage`) are *informational
  only*; the CLI establishes ground truth with `git` and by re-running checks.
- **Write isolation by worktree** — each write job gets its own worktree on a
  `job/<id>` branch. Crash recovery = discard the worktree. The human's working
  tree is never written; results are delivered on `harness/integration/<task>`
  for you to merge.

Design docs: [`docs/harness-v1-spec.md`](docs/harness-v1-spec.md) (rev3),
[`docs/scope-eval.md`](docs/scope-eval.md), [`docs/harness-architecture-opinion.md`](docs/harness-architecture-opinion.md).

---

## Requirements

- Go **1.26+**
- git **2.x**
- macOS or Linux (Windows is out of v1 scope)
- For real runs: `claude` and/or `codex` on `PATH` (override with
  `HARNESS_CLAUDE_BIN` / `HARNESS_CODEX_BIN`)

## Build

```bash
go build -o harness ./cmd/harness
go test ./...        # unit suite (no API calls)
```

---

## Quickstart

The full loop, exactly as validated end-to-end with a real `claude` worker:

```bash
# 0. a git repo with at least one commit
cd your-project && git init -q && git add -A && git commit -m init

# 1. initialize + start a session (captures a repo baseline)
harness init
harness session start

# 2. create a task
harness task create --title "add a hello file" --accept "src/hello.txt exists"
#  -> created task T-20260603-...

# 3. delegate a write job to a runtime (claude or codex)
harness delegate \
  --task T-20260603-... \
  --role implementation \
  --runtime claude \
  --goal 'Create src/hello.txt containing exactly: hello harness' \
  --allow 'src/**' \
  --verify 'test -f src/hello.txt'
#  -> delegated J-20260603-... (claude/implementation)

# 4. run it — claude does the work in an isolated worktree, the CLI extracts
#    the result, commits it to job/J-..., re-runs scope review + verify itself
harness run --job J-20260603-...
#  -> job J-20260603-... -> completed (ok)

# 5. task-level verify, integrate, handoff
harness verify   --task T-20260603-... --cmd 'test -f src/hello.txt'
harness integrate --task T-20260603-...   # merges job branches -> harness/integration/<task>
harness handoff  --task T-20260603-...     # renders tasks/<task>/handoff.md

# 6. review and merge the delivery branch into your branch
git log --oneline harness/integration/T-20260603-...
git merge harness/integration/T-20260603-...
```

Your main working tree stays clean the whole time — the change lives only on
`job/J-...` and `harness/integration/T-...` until *you* merge it.

---

## Command reference

| Command | Purpose |
|---|---|
| `harness init` | Create `.harness/` (schemas, reserved.json, contract); refuses outside a git repo or if `.harness` is tracked |
| `harness session start` | Start a session, capture the repo baseline |
| `harness task create --title <t> [--accept <a,...>] [--budget <n>]` | Create a task |
| `harness task phase --task <id> --to <phase>` | Advance task phase (CLI-checked legality) |
| `harness delegate --task <id> --role <r> --runtime <codex\|claude> [--goal <g>] [--allow <g,...>] [--deny <g,...>] [--verify <cmd,...>] [--depth <n>]` | Create a job from a task |
| `harness run --job <id> [--session <sid>]` | Run a created job to a terminal state |
| `harness verify --task <id> --cmd <c> [--cmd ...] [--workdir <d>]` | CLI-run task-level verification → `verification.json` |
| `harness integrate --task <id>` | Merge completed write-job branches → `harness/integration/<id>` |
| `harness handoff --task <id>` | Render `handoff.md` (jobs, verification, delivery branch) |
| `harness gate list \| show --gate <g> \| approve --gate <g> [--option <o>] [--files <f,...>] \| reject --gate <g>` | Human-gate management |
| `harness recover [--session <sid>]` | Rebuild views; reconcile stale jobs & orphan worktrees |
| `harness guard pretool --runtime <r> --job <id>` | PreToolUse hook (reads payload on stdin) |
| `harness guard posttool --job <id>` | PostToolUse diff review |
| `harness hook task-stop --role <worker\|orchestrator> [--job <id>] [--task <id>]` | Stop/TaskCompleted gate |
| `harness version` | Print version |

`--session` defaults to the latest session when omitted.

**Roles:** `analysis` · `implementation` · `test` · `review` · `verification` ·
`integration`. Write roles (implementation/test/integration) run in a worktree;
read-only roles run in the shared main tree.

**Exit codes:** `0` ok · `10` blocked-policy · `12` needs-human · `20`
verify-failed · `22` result-invalid · `30` state-corrupt · `31` lock-timeout ·
`32` cas-retry · `40` runtime-exec-failed · `41` budget-exceeded · `42`
delegation-loop · `64` usage / not-a-git-repo.

---

## Exceptions: gates

When a worker writes out of scope, a commit fails, verify fails, or integrate
hits a denied change / conflict, the job goes **needs-human** and an actionable
**gate** is opened:

```bash
harness gate list
harness gate show --gate G-...
# approve: extend scope and re-run the job
harness gate approve --gate G-... --option approve_extra_files --files 'config/**'
# reject: abandon the job's work (cancelled) and discard its worktree
harness gate reject --gate G-...
```

Resolving a gate moves the job out of `needs-human` (reset to `created` for a
re-run, or `cancelled`) so it stops blocking task completion. Each resolution
prints the concrete next command.

## Crash recovery

```bash
harness recover
```

Rebuilds all views from the event log, then: discards worktrees of stale jobs
(dead worker, detected via pid + boot-id) and resets/escalates them, and prunes
orphan worktrees. `recover` is mutually exclusive (recover.lock) and idempotent.

---

## Hooks (optional, advanced)

The runtime path (worker invocation, result extraction) is validated. Wiring the
in-session **PreToolUse / PostToolUse / Stop** guards into a runtime's settings
(e.g. Claude Code `.claude/settings.json`) is supported by `harness guard …` /
`harness hook …`, which normalize each runtime's hook payload. The exact payload
field names are constructed per each CLI's documentation; calibrate against a
live hook event before relying on them.

---

## Testing

```bash
go test ./...                 # unit suite, no API calls, deterministic
go test -race ./...           # race detector
HARNESS_E2E=1 go test ./internal/runtime/ -run E2E   # real CLI calls (costs tokens)
```

The protocol layer is fully testable via a **mock runtime** that injects crash /
scope-violation / zombie / bad-schema / non-zero-exit scenarios — no API calls.

---

## Architecture

```
cmd/harness            CLI entry
internal/store         atomic write + flock + path layout
internal/event         append-only event log (ULID, fsync, torn-tail tolerant)
internal/state         CAS transition protocol + view rebuild + gates
internal/model         data contracts (mirror schemas/*.json)
internal/scope         deterministic scope evaluation (normalize/glob/classify/changed-set)
internal/runtime       Runtime interface + Mock + real codex/claude adapters
internal/worktree      per-write-job worktree lifecycle
internal/adapter       push orchestration: spawn → ground-truth → CAS terminal
internal/guard         PreToolUse path guard + dangerous-command policy + hook I/O
internal/verify        CLI-run verification
internal/integrate     integration-worktree merge → delivery branch
internal/cli           subcommand implementations
schemas/               embedded JSON Schemas + reserved.json (written by `harness init`)
```

## v1 scope (intentionally not done)

Autonomous orchestrator loop (CLI is human-driven), Windows, MCP server, cross-repo
coordination, team mailbox, UI, file leases / shared-write mode (worktree isolation
replaces them).
