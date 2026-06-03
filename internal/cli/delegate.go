package cli

import (
	"fmt"

	"github.com/fengxudong/harness/internal/model"
	"github.com/fengxudong/harness/internal/state"
	"github.com/fengxudong/harness/internal/store"
)

// DelegateSpec describes a job to create from a task (rev3 §8, §16). In the
// CLI+hooks model the orchestrator (a human-driven session) supplies these.
type DelegateSpec struct {
	TaskID  string
	Role    string
	Runtime string
	Allowed []string
	Denied  []string
	Verify  []string
	Depth   int
}

// Delegate creates a job (status created) the adapter can then run. Mode/writes
// derive from the role (write roles → worktree).
func Delegate(dir, sid string, spec DelegateSpec) (string, error) {
	root, err := repoRoot(dir)
	if err != nil {
		return "", coded(ExitUsage, "%s is not inside a git repository", dir)
	}
	l := store.NewLayout(root)
	if sid == "" {
		if sid, err = latestSession(l); err != nil {
			return "", err
		}
	}
	if spec.Role == "" || spec.Runtime == "" || spec.TaskID == "" {
		return "", coded(ExitUsage, "delegate needs --task, --role and --runtime")
	}
	if spec.Depth >= maxDelegationDepth {
		return "", coded(ExitDelegationLoop, "delegation depth %d exceeds max %d", spec.Depth, maxDelegationDepth)
	}

	writes := model.RoleWrites(spec.Role)
	mode := model.ModeShared
	if writes {
		mode = model.ModeWorktree
	}
	jobID := newJobID()
	job := model.Job{
		JobID: jobID, TaskID: spec.TaskID, CreatedBy: "orchestrator", TargetRuntime: spec.Runtime,
		Role: spec.Role, Writes: writes, Mode: mode,
		StateRoot: l.StateRoot, RepoRoot: root, Workdir: root,
		Scope:                    model.Scope{Allowed: spec.Allowed, Denied: spec.Denied},
		VerificationRequirements: spec.Verify,
		Delegation:               model.Delegation{Depth: spec.Depth, ChainFingerprints: []string{spec.TaskID + ":" + spec.Role}},
		Budget:                   model.JobBudget{MaxTokens: 200000, TimeoutS: 1800},
		ResultContract:           "job-result.schema.json",
	}
	if err := state.New(l, sid).CreateJob("orchestrator", job); err != nil {
		return "", err
	}
	fmt.Printf("delegated %s (%s/%s) for task %s\n", jobID, spec.Runtime, spec.Role, spec.TaskID)
	return jobID, nil
}

const maxDelegationDepth = 3
