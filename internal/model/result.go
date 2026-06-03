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
