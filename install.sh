#!/usr/bin/env bash
# Install harness on a fresh machine: build the single binary, place it on PATH,
# and report which optional runtime dependencies are present.
#
#   ./install.sh            # install to $(go env GOPATH)/bin
#   ./install.sh /usr/local/bin   # install to a chosen dir (may need sudo)
set -euo pipefail
cd "$(dirname "$0")"

say()  { printf '%s\n' "$*"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$*"; }
miss() { printf '  \033[33m–\033[0m %s\n' "$*"; }

# --- hard prerequisites ------------------------------------------------------
command -v go  >/dev/null || { say "error: Go 1.26+ is required — https://go.dev/dl/"; exit 1; }
command -v git >/dev/null || { say "error: git is required (harness's storage substrate)"; exit 1; }

# --- build + install ---------------------------------------------------------
DEST="${1:-$(go env GOPATH)/bin}"
mkdir -p "$DEST"
say "building harness (zero external Go deps) ..."
go build -o "$DEST/harness" ./cmd/harness
ok "installed: $DEST/harness  ($("$DEST/harness" version 2>/dev/null || echo harness))"

# --- PATH hint ---------------------------------------------------------------
case ":$PATH:" in
  *":$DEST:"*) : ;;
  *) say ""; say "note: $DEST is not on PATH — add to your shell rc:"; say "      export PATH=\"$DEST:\$PATH\"" ;;
esac

# --- optional runtime dependencies ------------------------------------------
say ""
say "runtime dependencies (needed when you USE harness, not to install it):"
command -v git     >/dev/null && ok "git      (required — repos, worktrees, diffs)"   || miss "git"
if command -v claude >/dev/null || command -v codex >/dev/null; then
  command -v claude >/dev/null && ok "claude   (worker runtime)" || miss "claude   (worker runtime — optional)"
  command -v codex  >/dev/null && ok "codex    (worker runtime)" || miss "codex    (worker runtime — optional)"
else
  miss "claude / codex  (need at least one to run jobs)"
fi
command -v python3 >/dev/null && ok "python3  (only for Trellis write-back)" || miss "python3  (only for Trellis write-back — optional)"
command -v trellis >/dev/null && ok "trellis  (optional knowledge/spec layer)" || miss "trellis  (optional — npm i -g @mindfoldhq/trellis)"

say ""
say "done. try:  harness version   then   harness init   in a git repo."
