#!/usr/bin/env bash
# Install the harness command into Claude Code (slash command) and/or Codex (skill).
#   ./integrations/install-integrations.sh           # both, user-level
#   ./integrations/install-integrations.sh claude    # only Claude Code
#   ./integrations/install-integrations.sh codex     # only Codex
set -euo pipefail
cd "$(dirname "$0")"

want="${1:-both}"

if [ "$want" = "both" ] || [ "$want" = "claude" ]; then
  dest="$HOME/.claude/commands"
  mkdir -p "$dest"
  cp claude-code/commands/harness-delegate.md "$dest/"
  echo "✓ Claude Code: $dest/harness-delegate.md   →  use  /harness-delegate codex <goal>"
fi

if [ "$want" = "both" ] || [ "$want" = "codex" ]; then
  # Codex 0.137+ uses skills: ~/.codex/skills/<name>/SKILL.md (custom prompts are deprecated).
  dest="$HOME/.codex/skills/harness-delegate"
  mkdir -p "$dest"
  cp codex/skills/harness-delegate/SKILL.md "$dest/"
  echo "✓ Codex: $dest/SKILL.md   →  use  /harness-delegate claude <goal>  (or describe the task)"
fi

echo "done. (harness itself must be on PATH — see install.sh / install-remote.sh)"
