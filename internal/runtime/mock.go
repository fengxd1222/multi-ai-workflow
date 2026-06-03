package runtime

import (
	"context"
	"os"
	"path/filepath"
)

// Mock is a scripted Runtime for deterministic protocol tests (rev3 §18). Its
// Fn defines the behavior; Calls records every request for assertions.
type Mock struct {
	Fn    func(ctx context.Context, req Request) (Result, error)
	Calls []Request
}

// Run records the request and delegates to Fn.
func (m *Mock) Run(ctx context.Context, req Request) (Result, error) {
	m.Calls = append(m.Calls, req)
	return m.Fn(ctx, req)
}

func intp(v int) *int { return &v }

// Normal returns a clean completed result carrying finalJSON and a usage count.
func Normal(finalJSON []byte, tokens int) *Mock {
	return &Mock{Fn: func(_ context.Context, _ Request) (Result, error) {
		return Result{ExitCode: 0, FinalJSON: finalJSON, FinalJSONOK: true, ReportedTokens: intp(tokens)}, nil
	}}
}

// BadSchema returns a structurally-complete final JSON that will fail
// job-result schema validation (the schema-repair path, rev3 §8.3).
func BadSchema() *Mock {
	return &Mock{Fn: func(_ context.Context, _ Request) (Result, error) {
		return Result{ExitCode: 0, FinalJSON: []byte(`{"not":"a job result"}`), FinalJSONOK: true, ReportedTokens: intp(10)}, nil
	}}
}

// TornFinalJSON simulates a crash mid-write of final.json: complete=false, which
// the adapter must treat as runtime-exec-failed(40), not result-invalid (N36).
func TornFinalJSON() *Mock {
	return &Mock{Fn: func(_ context.Context, _ Request) (Result, error) {
		return Result{ExitCode: 0, FinalJSON: []byte(`{"job_id":"J-1","sta`), FinalJSONOK: false}, nil
	}}
}

// NonZeroExit simulates a process-layer failure (auth/network/CLI args).
func NonZeroExit(code int) *Mock {
	return &Mock{Fn: func(_ context.Context, _ Request) (Result, error) {
		return Result{ExitCode: code, Stderr: []byte("boom")}, nil
	}}
}

// NoUsage returns a completed result with no reported usage, exercising the
// conservative-estimate + gate path (rev3 §8.4 N28).
func NoUsage(finalJSON []byte) *Mock {
	return &Mock{Fn: func(_ context.Context, _ Request) (Result, error) {
		return Result{ExitCode: 0, FinalJSON: finalJSON, FinalJSONOK: true, ReportedTokens: nil}, nil
	}}
}

// Zombie blocks until the context is cancelled (the adapter watchdog firing),
// then reports it was killed (rev3 §5 N3).
func Zombie() *Mock {
	return &Mock{Fn: func(ctx context.Context, _ Request) (Result, error) {
		<-ctx.Done()
		return Result{ExitCode: -1, KilledByWatchdog: true}, nil
	}}
}

// ScopeViolation writes the given files (relpath -> content) into the request
// workdir, simulating a worker that wrote outside its scope, then returns a
// clean-looking result. Used by M4 to verify diff review catches it.
func ScopeViolation(finalJSON []byte, files map[string]string) *Mock {
	return &Mock{Fn: func(_ context.Context, req Request) (Result, error) {
		for rel, content := range files {
			p := filepath.Join(req.Workdir, rel)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return Result{}, err
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				return Result{}, err
			}
		}
		return Result{ExitCode: 0, FinalJSON: finalJSON, FinalJSONOK: true, ReportedTokens: intp(5)}, nil
	}}
}
