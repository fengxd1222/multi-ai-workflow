#!/bin/bash
# Codex CLI wrapper script - Adapted for multi-ai-workflow-cli v2.0
# Purpose: Call Codex CLI for code generation

set -e  # Exit immediately on error

PROMPT=$1
OUTPUT_DIR=$2

# Parameter validation
if [ -z "$PROMPT" ] || [ -z "$OUTPUT_DIR" ]; then
    echo "Error: Missing required parameters"
    echo "Usage: $0 <prompt> <output_directory>"
    exit 1
fi

# Ensure output directory exists
if [ ! -d "$OUTPUT_DIR" ]; then
    echo "Error: Output directory does not exist: $OUTPUT_DIR"
    exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Starting Codex code generation..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Output directory: $OUTPUT_DIR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Save current directory
ORIGINAL_DIR=$(pwd)

# Disable set -e to capture exit code
set +e

# Call Codex CLI
echo "Calling Codex..."
codex exec "$PROMPT" -C "$OUTPUT_DIR" --full-auto

# Capture exit code
EXIT_CODE=$?

# Return to original directory
cd "$ORIGINAL_DIR" || true

set -e

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $EXIT_CODE -eq 0 ]; then
    echo "✓ Codex code generation completed!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit 0
else
    echo "✗ Codex execution failed! Error code: $EXIT_CODE"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Possible causes:"
    echo "1. Codex CLI not installed or not in PATH"
    echo "2. API key not configured"
    echo "3. Invalid prompt format"
    echo "4. Output directory permission issues"
    echo ""
    echo "Suggestions:"
    echo "- Check Codex CLI: which codex"
    echo "- Check configuration: codex --version"
    echo "- Manual test: codex exec 'hello world'"
    echo "- Or use Claude: /workflow-start --ai=claude"
    exit 1
fi
