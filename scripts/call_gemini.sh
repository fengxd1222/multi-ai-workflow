#!/bin/zsh
# Gemini CLI wrapper script - Adapted for multi-ai-workflow-cli v2.0
# Purpose: Call Gemini CLI for code review and generate review report
#
# Notes:
# - Uses zsh to load user's gemini function configuration
# - Automatically detects if gemini is a function or command:
#   * If it's a function (defined in ~/.zshrc): uses function call (includes user's proxy config, etc.)
#   * If it's a command (system-installed CLI): uses command call

set -e  # Exit immediately on error

PROMPT=$1
PROJECT_DIR=$2

# Parameter validation
if [ -z "$PROMPT" ] || [ -z "$PROJECT_DIR" ]; then
    echo "Error: Missing required parameters"
    echo "Usage: $0 <prompt> <project_directory>"
    exit 1
fi

# Check if project directory exists
if [ ! -d "$PROJECT_DIR" ]; then
    echo "Error: Project directory does not exist: $PROJECT_DIR"
    exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Starting Gemini code review..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Project directory: $PROJECT_DIR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Save current directory
ORIGINAL_DIR=$(pwd)

# Change to project directory
cd "$PROJECT_DIR" || exit 1

# Disable set -e to capture exit code
set +e

# Try to load ~/.zshrc (if user defined gemini function there)
if [ -f ~/.zshrc ]; then
    source ~/.zshrc 2>/dev/null || true
fi

# Check if gemini is a zsh function
if typeset -f gemini > /dev/null 2>&1; then
    # gemini is a function (user defined in ~/.zshrc, may include proxy config)
    echo "Calling Gemini (using function from ~/.zshrc)..."
    gemini --yolo --output-format text "$PROMPT"
else
    # gemini is not a function, call as regular command
    echo "Calling Gemini (using system command)..."
    command gemini --yolo --output-format text "$PROMPT"
fi

# Capture exit code
EXIT_CODE=$?

# Return to original directory
cd "$ORIGINAL_DIR" || true

set -e

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $EXIT_CODE -eq 0 ]; then
    echo "✓ Gemini review completed!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit 0
else
    echo "✗ Gemini execution failed! Error code: $EXIT_CODE"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "Possible causes:"
    echo "1. Gemini CLI not installed or not in PATH"
    echo "2. Network connection issues (may need proxy or VPN)"
    echo "3. API key not configured"
    echo "4. Invalid prompt format"
    echo ""
    echo "Suggestions:"
    echo "- Check if Gemini is available:"
    echo "  * If it's a function: check gemini function definition in ~/.zshrc"
    echo "  * If it's a command: which gemini"
    echo "- Check proxy settings: echo \$HTTP_PROXY"
    echo "- Check network connection: curl -I https://cloudcode-pa.googleapis.com"
    echo "- Manual test: gemini --yolo 'hello'"
    echo "- Or use Claude: /workflow-start --step3=claude"
    exit 1
fi
