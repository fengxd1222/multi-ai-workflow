package model

import (
	"encoding/json"
	"fmt"
)

// JobResult mirrors schemas/job-result.schema.json. Every field except the
// status/summary identity is INFORMATIONAL-ONLY: the CLI establishes ground
// truth via git + re-run verification and never trusts these (rev3 §3.2 C3).
type JobResult struct {
	JobID         string            `json:"job_id"`
	Status        string            `json:"status"`
	Summary       string            `json:"summary"`
	Informational bool              `json:"_informational,omitempty"`
	ChangedFiles  []string          `json:"changed_files,omitempty"`
	Verification  map[string]string `json:"verification,omitempty"`
	Usage         *ResultUsage      `json:"usage,omitempty"`
	NeedsHuman    bool              `json:"needs_human,omitempty"`
	Followups     []string          `json:"followups,omitempty"`
}

type ResultUsage struct {
	Tokens int `json:"tokens"`
}

// Verification mirrors schemas/verification.schema.json. Checks are run by the
// CLI (rev3 §13); `required` defaults to true when absent (fail-safe, N23).
type Verification struct {
	Level  string              `json:"level,omitempty"`
	Checks []VerificationCheck `json:"checks"`
}

type VerificationCheck struct {
	Level      string `json:"level,omitempty"`
	Command    string `json:"command"`
	Result     string `json:"result"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	DurationMS *int   `json:"duration_ms,omitempty"`
	LogRef     string `json:"log_ref,omitempty"`
	Required   *bool  `json:"required,omitempty"`
}

// IsRequired reports whether a check is required; absent means true (N23).
func (c VerificationCheck) IsRequired() bool { return c.Required == nil || *c.Required }

// Passed reports whether the task-level verification satisfies completion
// contract condition 2: at least one required check, and all required passed.
// An empty/required-less set is NOT vacuously true (rev3 §6 N23).
func (v Verification) Passed() bool {
	hasRequired := false
	for _, c := range v.Checks {
		if c.IsRequired() {
			hasRequired = true
			if c.Result != JobCompleted && c.Result != "passed" {
				return false
			}
		}
	}
	return hasRequired
}

var resultStatuses = map[string]bool{
	JobCompleted: true, JobFailed: true, JobNeedsHuman: true,
}

// ParseJobResult validates a worker result candidate against the contract: it
// must parse and carry job_id, a known status, and a non-empty summary. This is
// the boundary check that gates the schema-repair loop (rev3 §8.3).
func ParseJobResult(data []byte) (JobResult, error) {
	var r JobResult
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("job-result not valid JSON: %w", err)
	}
	if r.JobID == "" {
		return r, fmt.Errorf("job-result missing job_id")
	}
	if !resultStatuses[r.Status] {
		return r, fmt.Errorf("job-result invalid status %q", r.Status)
	}
	if r.Summary == "" {
		return r, fmt.Errorf("job-result missing summary")
	}
	return r, nil
}
