package guard

import (
	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/scope"
)

// Decision is the PreToolUse verdict. Ordered Allow < Gate < Deny so the worst
// path/command verdict wins.
type Decision int

const (
	Allow Decision = iota
	Gate
	Deny
)

func (d Decision) String() string {
	switch d {
	case Deny:
		return "deny"
	case Gate:
		return "gate"
	default:
		return "allow"
	}
}

// ToolCall is the runtime-agnostic view of a tool invocation (rev3 §9/§10).
type ToolCall struct {
	Tool string   // Edit / Write / Bash / Read / ...
	Paths []string // target write paths (resolved against workdir)
	Bash string   // command, when Tool == Bash
}

// Result is a PreToolUse evaluation.
type Result struct {
	Decision Decision
	Reason   string
	Verdicts []scope.Verdict
}

var readOnlyTools = set("Read", "Grep", "Glob", "LS", "NotebookRead", "WebFetch", "WebSearch", "Task", "TodoWrite")
var writeTools = set("Edit", "Write", "MultiEdit", "NotebookEdit")

// EvaluatePreTool decides allow/gate/deny for one tool call: dangerous commands
// (rev3 §12), path-level scope enforcement for write tools (rev3 §9/§10), and
// allow for read-only tools.
func EvaluatePreTool(tc ToolCall, workdir string, sc model.Scope, rsv scope.Reserved, ignorecase bool) Result {
	switch {
	case tc.Tool == "Bash":
		d, r := ClassifyCommand(tc.Bash)
		switch d {
		case CmdDeny:
			return Result{Decision: Deny, Reason: "dangerous-command:" + r}
		case CmdGate:
			return Result{Decision: Gate, Reason: "dangerous-command:" + r}
		default:
			return Result{Decision: Allow} // PostToolUse diff review is the backstop
		}
	case readOnlyTools[tc.Tool]:
		return Result{Decision: Allow}
	case writeTools[tc.Tool] || len(tc.Paths) > 0:
		return evalPaths(tc.Paths, workdir, sc, rsv, ignorecase)
	default:
		return Result{Decision: Allow}
	}
}

func evalPaths(paths []string, workdir string, sc model.Scope, rsv scope.Reserved, ignorecase bool) Result {
	res := Result{Decision: Allow}
	for _, p := range paths {
		rel, err := scope.Normalize(p, workdir)
		if err != nil { // symlink-in-path / escapes-base-root (N25)
			res.Verdicts = append(res.Verdicts, scope.Verdict{Path: p, Decision: scope.Reject})
			worsen(&res, Deny, err.Error())
			continue
		}
		v := scope.Classify(rel, sc, rsv, ignorecase)
		res.Verdicts = append(res.Verdicts, v)
		switch v.Decision {
		case scope.DenyReserved, scope.DenyScope:
			worsen(&res, Deny, "scope:"+v.Decision.String()+":"+v.Rule)
		case scope.Gate:
			worsen(&res, Gate, "default-deny:"+rel)
		}
	}
	return res
}

func worsen(r *Result, d Decision, reason string) {
	if d > r.Decision {
		r.Decision = d
		r.Reason = reason
	}
}

// PostToolReview is the PostToolUse / migration-point diff review: it recomputes
// the real change set in a worktree and returns every non-Allow verdict. Worker
// self-reports are never consulted (rev3 §10 C3).
func PostToolReview(workdir, baseCommit string, sc model.Scope, rsv scope.Reserved, ignorecase bool) ([]scope.Verdict, error) {
	changed, err := scope.ChangedSet(workdir, baseCommit)
	if err != nil {
		return nil, err
	}
	var violations []scope.Verdict
	for path := range changed {
		if v := scope.Classify(path, sc, rsv, ignorecase); v.Decision != scope.Allow {
			violations = append(violations, v)
		}
	}
	return violations, nil
}
