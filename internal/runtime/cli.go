package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
)

// Codex runs jobs via the codex CLI. The business result is taken from the
// --output-last-message file (rev3 §8.2).
type Codex struct{ Bin string }

func (c Codex) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return "codex"
}

// codexArgs builds the argv. finalPath is where codex writes the last message.
func codexArgs(req Request, finalPath string) []string {
	sandbox := req.Sandbox
	if sandbox == "" {
		sandbox = "workspace-write"
	}
	args := []string{"exec", "--json", "--sandbox", sandbox}
	if req.SchemaPath != "" {
		args = append(args, "--output-schema", req.SchemaPath)
	}
	args = append(args, "--output-last-message", finalPath, "-C", req.Workdir, req.Prompt)
	return args
}

func (c Codex) Run(ctx context.Context, req Request) (Result, error) {
	finalPath := req.FinalJSONPath
	if finalPath == "" {
		f, err := os.CreateTemp("", "codex-final-*.json")
		if err != nil {
			return Result{}, err
		}
		finalPath = f.Name()
		f.Close()
		defer os.Remove(finalPath)
	}
	pr := runProcess(ctx, req.Workdir, c.bin(), codexArgs(req, finalPath))

	final, _ := os.ReadFile(finalPath)
	return Result{
		ExitCode:         pr.ExitCode,
		Stdout:           pr.Stdout,
		Stderr:           pr.Stderr,
		FinalJSON:        final,
		FinalJSONOK:      completeJSON(final),
		ReportedTokens:   codexUsage(pr.Stdout),
		KilledByWatchdog: pr.Killed,
	}, nil
}

// codexUsage extracts token usage from the last turn.completed event in the
// codex JSONL event stream (calibrated against codex-cli 0.135.0).
func codexUsage(stdout []byte) *int {
	var tokens *int
	for _, line := range bytes.Split(stdout, []byte("\n")) {
		var ev struct {
			Type  string `json:"type"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(line, &ev) == nil && ev.Type == "turn.completed" && ev.Usage != nil {
			t := ev.Usage.InputTokens + ev.Usage.OutputTokens
			tokens = &t
		}
	}
	return tokens
}

// Claude runs jobs via the claude CLI. The business result is the stdout JSON
// object (rev3 §8.2).
type Claude struct{ Bin string }

func (c Claude) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return "claude"
}

// claudeArgs builds the argv. schemaContent is the literal json-schema string.
func claudeArgs(req Request, schemaContent string) []string {
	args := []string{"-p", req.Prompt, "--output-format", "json"}
	if schemaContent != "" {
		args = append(args, "--json-schema", schemaContent)
	}
	if len(req.AllowedTools) > 0 {
		args = append(args, "--allowedTools", joinCSV(req.AllowedTools))
	}
	args = append(args, "--permission-mode", "acceptEdits")
	return args
}

// claudeEnvelope is the real `claude -p --output-format json` wrapper. The
// model's structured output is in .result (a JSON string), not at top level.
type claudeEnvelope struct {
	Type    string `json:"type"`
	Subtype string `json:"subtype"`
	IsError bool   `json:"is_error"`
	Result  string `json:"result"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (c Claude) Run(ctx context.Context, req Request) (Result, error) {
	schemaContent := ""
	if req.SchemaPath != "" {
		if b, err := os.ReadFile(req.SchemaPath); err == nil {
			schemaContent = string(b)
		}
	}
	pr := runProcess(ctx, req.Workdir, c.bin(), claudeArgs(req, schemaContent))

	res := Result{ExitCode: pr.ExitCode, Stdout: pr.Stdout, Stderr: pr.Stderr, KilledByWatchdog: pr.Killed}
	var env claudeEnvelope
	if json.Unmarshal(pr.Stdout, &env) == nil && env.Type == "result" {
		// unwrap: the business result is env.Result (a JSON string)
		res.FinalJSON = []byte(env.Result)
		res.FinalJSONOK = !env.IsError && completeJSON(res.FinalJSON)
		t := env.Usage.InputTokens + env.Usage.OutputTokens
		res.ReportedTokens = &t
	} else {
		// not the expected envelope (e.g. process failed before producing it)
		res.FinalJSON = pr.Stdout
		res.FinalJSONOK = completeJSON(pr.Stdout)
	}
	return res, nil
}

func joinCSV(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

var (
	_ Runtime = Codex{}
	_ Runtime = Claude{}
)
