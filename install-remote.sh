#!/usr/bin/env bash
# One-line remote install:
#   curl -fsSL https://raw.githubusercontent.com/fengxd1222/multi-ai-workflow/main/install-remote.sh | bash
#
# Clones the repo to a cache dir (or updates it), builds the single binary, and
# installs it to $(go env GOPATH)/bin. Override the install dir:
#   ... | bash -s -- /usr/local/bin
set -euo pipefail

REPO="https://github.com/fengxd1222/multi-ai-workflow"
SRC="${HARNESS_SRC:-$HOME/.cache/harness-src}"
DEST="${1:-$(go env GOPATH 2>/dev/null)/bin}"

command -v go  >/dev/null || { echo "error: Go 1.26+ is required — https://go.dev/dl/"; exit 1; }
command -v git >/dev/null || { echo "error: git is required"; exit 1; }

if [ -d "$SRC/.git" ]; then
  echo "updating $SRC ..."
  git -C "$SRC" fetch -q origin main && git -C "$SRC" reset -q --hard origin/main
else
  echo "cloning $REPO ..."
  rm -rf "$SRC"
  git clone -q --depth 1 "$REPO" "$SRC"
fi

mkdir -p "$DEST"
echo "building ..."
( cd "$SRC" && go build -o "$DEST/harness" ./cmd/harness )
echo "✓ installed: $DEST/harness  ($("$DEST/harness" version 2>/dev/null || echo harness))"

case ":$PATH:" in
  *":$DEST:"*) : ;;
  *) echo "note: add to your shell rc →  export PATH=\"$DEST:\$PATH\"" ;;
esac
echo "next: harness version   then   harness init   in a git repo"
