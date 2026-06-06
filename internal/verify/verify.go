// Package verify runs verification commands itself and records the results
// (rev3 §13). The CLI — not the worker — establishes whether checks pass (C3);
// every check defaults to required (N23).
package verify

import (
	"os/exec"
	"time"

	"github.com/fengxd1222/multi-ai-workflow/internal/model"
)

// Run executes each command in workdir and returns a Verification record plus
// whether all required checks passed. shell defaults to "sh".
func Run(workdir, shell, level string, cmds []string) (model.Verification, bool) {
	if shell == "" {
		shell = "sh"
	}
	required := true
	v := model.Verification{Level: level}
	allPassed := true

	for _, c := range cmds {
		start := time.Now()
		cmd := exec.Command(shell, "-c", c)
		cmd.Dir = workdir
		err := cmd.Run()
		code := exitCode(err)
		dur := int(time.Since(start).Milliseconds())

		result := "passed"
		if err != nil {
			result = "failed"
			allPassed = false
		}
		v.Checks = append(v.Checks, model.VerificationCheck{
			Level: level, Command: c, Result: result,
			ExitCode: &code, DurationMS: &dur, Required: &required,
		})
	}
	// An empty requirement set does not pass (rev3 N23): nothing was verified.
	if len(cmds) == 0 {
		return v, false
	}
	return v, allPassed
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	return -1
}
