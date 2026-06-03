package runtime

import (
	"context"
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
		KilledByWatchdog: pr.Killed,
	}, nil
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

func (c Claude) Run(ctx context.Context, req Request) (Result, error) {
	schemaContent := ""
	if req.SchemaPath != "" {
		if b, err := os.ReadFile(req.SchemaPath); err == nil {
			schemaContent = string(b)
		}
	}
	pr := runProcess(ctx, req.Workdir, c.bin(), claudeArgs(req, schemaContent))
	return Result{
		ExitCode:         pr.ExitCode,
		Stdout:           pr.Stdout,
		Stderr:           pr.Stderr,
		FinalJSON:        pr.Stdout,
		FinalJSONOK:      completeJSON(pr.Stdout),
		KilledByWatchdog: pr.Killed,
	}, nil
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
