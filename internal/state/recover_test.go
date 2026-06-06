package state

import (
	"testing"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
)

// TestRecoverJob_ResetsThenEscalates verifies a stale job is reset to created up
// to the retry cap, then escalated to needs-human (rev3 §15 N3).
func TestRecoverJob_ResetsThenEscalates(t *testing.T) {
	e := newEngine(t)
	if err := e.CreateJob("c", minimalJob("J-1")); err != nil {
		t.Fatal(err)
	}
	toRunning := func(rev int) int {
		j, err := e.TransitionJobRunning("a", "J-1", rev, &model.Worker{PID: 1, BootID: "b"}, "/wd", "job/J-1", "abc1234")
		if err != nil {
			t.Fatal(err)
		}
		return j.Rev
	}

	rev := toRunning(1)
	// recover #1 -> created, count 1
	j, err := e.RecoverJob("recover", "J-1", 2)
	if err != nil || j.Status != model.JobCreated || j.RecoverCount != 1 {
		t.Fatalf("recover#1: %+v err=%v", j, err)
	}
	rev = toRunning(j.Rev)
	_ = rev
	// recover #2 -> created, count 2
	j, _ = e.RecoverJob("recover", "J-1", 2)
	if j.Status != model.JobCreated || j.RecoverCount != 2 {
		t.Fatalf("recover#2: %+v", j)
	}
	toRunning(j.Rev)
	// recover #3 -> count 3 > 2 -> needs-human
	j, _ = e.RecoverJob("recover", "J-1", 2)
	if j.Status != model.JobNeedsHuman || j.RecoverCount != 3 {
		t.Fatalf("recover#3 should escalate: %+v", j)
	}
}

func TestRecoverJob_NoopWhenNotRunning(t *testing.T) {
	e := newEngine(t)
	if err := e.CreateJob("c", minimalJob("J-1")); err != nil {
		t.Fatal(err)
	}
	// job is created, not running -> recover is a no-op
	j, err := e.RecoverJob("recover", "J-1", 2)
	if err != nil || j.Status != model.JobCreated || j.RecoverCount != 0 {
		t.Fatalf("noop expected: %+v err=%v", j, err)
	}
}
