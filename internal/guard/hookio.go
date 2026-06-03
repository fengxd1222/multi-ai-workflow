package guard

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Runtime hook payload field names differ between Claude and Codex; ParsePreTool
// normalizes both into a ToolCall. The exact field set may need tuning when
// wired to a specific CLI version — kept tolerant on purpose (rev3 §11).

// claudePreTool mirrors Claude's PreToolUse payload.
type claudePreTool struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		Command      string `json:"command"`
	} `json:"tool_input"`
}

// codexPreTool mirrors a best-effort Codex PreToolUse payload.
type codexPreTool struct {
	Tool  string `json:"tool"`
	Input struct {
		Command json.RawMessage `json:"command"` // string or []string
		Patch   string          `json:"patch"`
		Path    string          `json:"path"`
	} `json:"input"`
}

// ParsePreTool parses a runtime hook payload into a normalized ToolCall.
func ParsePreTool(runtime string, input []byte) (ToolCall, error) {
	switch runtime {
	case "claude":
		return parseClaude(input)
	case "codex":
		return parseCodex(input)
	default:
		return ToolCall{}, fmt.Errorf("unknown runtime %q", runtime)
	}
}

func parseClaude(input []byte) (ToolCall, error) {
	var p claudePreTool
	if err := json.Unmarshal(input, &p); err != nil {
		return ToolCall{}, err
	}
	tc := ToolCall{Tool: p.ToolName}
	switch p.ToolName {
	case "Bash":
		tc.Bash = p.ToolInput.Command
	case "NotebookEdit":
		if p.ToolInput.NotebookPath != "" {
			tc.Paths = []string{p.ToolInput.NotebookPath}
		}
	default:
		if p.ToolInput.FilePath != "" {
			tc.Paths = []string{p.ToolInput.FilePath}
		}
	}
	return tc, nil
}

func parseCodex(input []byte) (ToolCall, error) {
	var p codexPreTool
	if err := json.Unmarshal(input, &p); err != nil {
		return ToolCall{}, err
	}
	switch p.Tool {
	case "shell", "bash", "local_shell":
		return ToolCall{Tool: "Bash", Bash: codexCommand(p.Input.Command)}, nil
	case "apply_patch":
		return ToolCall{Tool: "Edit", Paths: patchPaths(p.Input.Patch)}, nil
	default:
		tc := ToolCall{Tool: p.Tool}
		if p.Input.Path != "" {
			tc.Paths = []string{p.Input.Path}
		}
		return tc, nil
	}
}

// codexCommand accepts either a JSON string or a JSON array form for command.
func codexCommand(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		// ["bash","-lc","<cmd>"] -> take the last element, else join
		if n := len(arr); n > 0 {
			if n >= 3 && (arr[0] == "bash" || arr[0] == "sh") {
				return arr[n-1]
			}
			return strings.Join(arr, " ")
		}
	}
	return ""
}

// patchPaths extracts file paths from apply_patch headers
// (*** Add/Update/Delete File: <path>).
func patchPaths(patch string) []string {
	var paths []string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimSpace(line)
		for _, pfx := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: "} {
			if strings.HasPrefix(line, pfx) {
				paths = append(paths, strings.TrimSpace(strings.TrimPrefix(line, pfx)))
			}
		}
	}
	return paths
}

// FormatDecision renders a PreToolUse decision in the runtime's hook output
// shape (rev3 §11). allow|deny|ask; Gate maps to ask (human approval).
func FormatDecision(runtime string, d Decision, reason string) []byte {
	perm := map[Decision]string{Allow: "allow", Deny: "deny", Gate: "ask"}[d]
	switch runtime {
	case "claude":
		out, _ := json.Marshal(map[string]any{
			"hookSpecificOutput": map[string]string{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       perm,
				"permissionDecisionReason": reason,
			},
		})
		return out
	default: // codex / generic
		out, _ := json.Marshal(map[string]string{"decision": perm, "reason": reason})
		return out
	}
}
