package runtime

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// These tests invoke the real codex/claude CLIs and cost tokens, so they are
// skipped unless HARNESS_E2E=1. They calibrate result + usage extraction
// against the actual CLI output (rev3 §8.2).

func e2eGuard(t *testing.T, bin string) {
	t.Helper()
	if os.Getenv("HARNESS_E2E") == "" {
		t.Skip("set HARNESS_E2E=1 to run real-CLI e2e tests")
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("%s not in PATH", bin)
	}
}

const triviaPrompt = `Reply with exactly this JSON and nothing else: {"job_id":"J-e2e","status":"completed","summary":"ok"}`

func assertJobResult(t *testing.T, r Result) {
	t.Helper()
	if !r.FinalJSONOK {
		t.Fatalf("FinalJSON not ok; final=%q stderr=%q", r.FinalJSON, r.Stderr)
	}
	var m map[string]any
	if err := json.Unmarshal(r.FinalJSON, &m); err != nil {
		t.Fatalf("final not JSON: %v (%q)", err, r.FinalJSON)
	}
	if m["job_id"] == nil || m["status"] == nil {
		t.Fatalf("extracted result missing fields: %v", m)
	}
	if r.ReportedTokens == nil || *r.ReportedTokens == 0 {
		t.Fatalf("usage not extracted: %v", r.ReportedTokens)
	}
}

func TestE2E_Claude_Extracts(t *testing.T) {
	e2eGuard(t, "claude")
	r, err := Claude{}.Run(context.Background(), Request{
		Workdir: t.TempDir(), Prompt: triviaPrompt, TimeoutS: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJobResult(t, r)
}

func TestE2E_Codex_Extracts(t *testing.T) {
	e2eGuard(t, "codex")
	dir := t.TempDir()
	for _, a := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t.t"}, {"config", "user.name", "t"}} {
		c := exec.Command("git", a...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	// codex --output-schema is OpenAI-strict: additionalProperties:false and all
	// properties required (matches schemas/codex-output.schema.json).
	schema := filepath.Join(dir, "jr.schema.json")
	_ = os.WriteFile(schema, []byte(`{"type":"object","additionalProperties":false,"required":["job_id","status","summary"],"properties":{"job_id":{"type":"string"},"status":{"type":"string"},"summary":{"type":"string"}}}`), 0o644)

	r, err := Codex{}.Run(context.Background(), Request{
		Workdir: dir, Prompt: triviaPrompt, SchemaPath: schema, Sandbox: "read-only", TimeoutS: 180,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertJobResult(t, r)
}
