package model

// Materialized-view structs. Mirror schemas/{session,task,job,gate}.schema.json
// (rev3 §3). Rebuildable from events; never the authoritative source.

const ProtocolVersion = "1.0"

// Runtime / role / status enums.
const (
	RuntimeCodex  = "codex"
	RuntimeClaude = "claude"

	RoleAnalysis       = "analysis"
	RoleImplementation = "implementation"
	RoleTest           = "test"
	RoleReview         = "review"
	RoleVerification   = "verification"
	RoleIntegration    = "integration"

	JobCreated    = "created"
	JobRunning    = "running"
	JobCompleted  = "completed"
	JobFailed     = "failed"
	JobNeedsHuman = "needs-human"
	JobTimeout    = "timeout"
	JobCancelled  = "cancelled" // gate-resolved/abandoned; ignored by completion

	ModeWorktree = "worktree"
	ModeShared   = "shared"

	TaskActive       = "active"
	TaskBlockedHuman = "blocked-human"
	TaskDone         = "done"
	TaskFailed       = "failed"
)

// writeRoles produce file changes and therefore run in a worktree.
var writeRoles = map[string]bool{
	RoleImplementation: true,
	RoleTest:           true,
	RoleIntegration:    true,
}

// RoleWrites reports whether a role mutates files (→ worktree) vs read-only
// (→ shared main tree). rev3 §7.
func RoleWrites(role string) bool { return writeRoles[role] }

// Session — schemas/session.schema.json.
type Session struct {
	SessionID       string        `json:"session_id"`
	ProtocolVersion string        `json:"protocol_version"`
	RepoRoot        string        `json:"repo_root"`
	StateRoot       string        `json:"state_root"`
	CreatedAt       string        `json:"created_at"`
	Status          string        `json:"status"` // active | ended
	Orchestrator    *Orchestrator `json:"orchestrator,omitempty"`
	ActiveTaskID    *string       `json:"active_task_id"`
}

type Orchestrator struct {
	AgentID string `json:"agent_id"`
	Runtime string `json:"runtime"`
}

// Task — schemas/task.schema.json.
type Task struct {
	TaskID            string     `json:"task_id"`
	Rev               int        `json:"rev"`
	Title             string     `json:"title"`
	Goal              string     `json:"goal,omitempty"`
	Status            string     `json:"status"`
	Phase             string     `json:"phase"`
	Acceptance        []string   `json:"acceptance"`
	JobIDs            []string   `json:"job_ids"`
	IntegrationBranch *string    `json:"integration_branch"`
	Budget            *Budget    `json:"budget,omitempty"`
	Completion        Completion `json:"completion"`
}

type Completion struct {
	AllJobsDone      bool `json:"all_jobs_done"`
	TaskVerifyPassed bool `json:"task_verify_passed"`
	HandoffWritten   bool `json:"handoff_written"`
	OpenGates        int  `json:"open_gates"`
}

// Job — schemas/job.schema.json.
type Job struct {
	JobID                    string      `json:"job_id"`
	TaskID                   string      `json:"task_id"`
	Rev                      int         `json:"rev"`
	CreatedBy                string      `json:"created_by"`
	TargetRuntime            string      `json:"target_runtime"`
	Role                     string      `json:"role"`
	Goal                     string      `json:"goal,omitempty"`
	TrellisTask              string      `json:"trellis_task,omitempty"` // co-located Trellis task slug, for write-back
	Writes                   bool        `json:"writes"`
	Status                   string      `json:"status"`
	Mode                     string      `json:"mode"`
	StateRoot                string      `json:"state_root"`
	RepoRoot                 string      `json:"repo_root"`
	Workdir                  string      `json:"workdir"`
	Branch                   *string     `json:"branch"`
	BaseCommit               *string     `json:"base_commit"`
	Worker                   *Worker     `json:"worker,omitempty"`
	Scope                    Scope       `json:"scope"`
	Brief                    string      `json:"brief,omitempty"`
	Constraints              []string    `json:"constraints,omitempty"`
	ContextRefs              []string    `json:"context_refs,omitempty"` // repo-relative files the worker must Read first
	VerificationRequirements []string    `json:"verification_requirements,omitempty"`
	Delegation               Delegation  `json:"delegation"`
	Budget                   JobBudget   `json:"budget"`
	RecoverCount             int         `json:"recover_count"`
	ResultContract           string      `json:"result_contract"`
}

type Worker struct {
	PID          int    `json:"pid"`
	BootID       string `json:"boot_id"`
	RunningSince string `json:"running_since,omitempty"`
}

type Scope struct {
	Allowed []string `json:"allowed"`
	Denied  []string `json:"denied"`
}

type Delegation struct {
	Depth             int      `json:"depth"`
	ChainFingerprints []string `json:"chain_fingerprints"`
}

type Budget struct {
	MaxTokens int `json:"max_tokens"`
}

type JobBudget struct {
	MaxTokens int `json:"max_tokens"`
	TimeoutS  int `json:"timeout_s"`
}

// Gate — schemas/gate.schema.json.
type Gate struct {
	GateID        string      `json:"gate_id"`
	TaskID        string      `json:"task_id"`
	JobID         string      `json:"job_id"`
	Reason        string      `json:"reason"`
	AffectedFiles []string    `json:"affected_files,omitempty"`
	Options       []string    `json:"options"`
	Recommended   string      `json:"recommended,omitempty"`
	Status        string      `json:"status"`
	CreatedAt     string      `json:"created_at"`
	Resolution    *Resolution `json:"resolution,omitempty"`
	RetryCount    int         `json:"retry_count,omitempty"`
}

type Resolution struct {
	Option     string `json:"option"`
	ResolvedAt string `json:"resolved_at"`
	By         string `json:"by,omitempty"`
	Note       string `json:"note,omitempty"`
}
