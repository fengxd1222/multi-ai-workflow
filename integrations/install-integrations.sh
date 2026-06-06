#!/usr/bin/env bash
# Install the harness slash command / custom prompt into Claude Code and/or Codex.
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
  dest="$HOME/.codex/prompts"
  mkdir -p "$dest"
  cp codex/prompts/harness-delegate.md "$dest/"
  echo "✓ Codex: $dest/harness-delegate.md   →  use  /harness-delegate claude <goal>"
fi

echo "done. (harness itself must be on PATH — see install.sh / install-remote.sh)"
