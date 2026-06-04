// Package runtime is the injectable seam between the adapter layer and the real
// codex/claude CLIs (rev3 §8, §16). M2 ships the interface plus a scripted Mock
// for crash / scope-violation / zombie / bad-schema injection; the real
// adapters land in M3. Keeping this an interface is what makes the protocol
// layer testable without burning API calls (rev3 §18).
package runtime

import "context"

// Request is everything an adapter hands a runtime to run one job.
type Request struct {
	JobID         string
	Runtime       string // "codex" | "claude"
	Workdir       string // cwd: a worktree for write jobs, repo_root for read-only
	Prompt        string
	SchemaPath    string
	FinalJSONPath string   // codex: --output-last-message destination
	AllowedTools  []string // claude --allowedTools whitelist
	Sandbox       string   // codex --sandbox mode
	HookSettings  string   // claude --settings JSON: wires the in-session PreToolUse path guard
	TimeoutS      int
	RepairOf      string // non-empty on a schema-repair retry: the prior error (rev3 §8.3)
}

// Result separates the process layer (exit/stdout/stderr/usage) from the
// business result; the business job-result is carried in FinalJSON and stays
// informational until the CLI verifies it (rev3 §3.2, §8.2 C3).
type Result struct {
	ExitCode         int
	Stdout           []byte
	Stderr           []byte
	FinalJSON        []byte // codex: --output-last-message file; claude: stdout JSON
	FinalJSONOK      bool   // FinalJSON is complete & parseable (false => torn, N36)
	ReportedTokens   *int   // nil => runtime reported no usage (conservative-estimate path, N28)
	KilledByWatchdog bool   // adapter SIGKILL'd it past TimeoutS (N3)
}

// Runtime runs one job to completion and returns its process-layer result.
type Runtime interface {
	Run(ctx context.Context, req Request) (Result, error)
}
