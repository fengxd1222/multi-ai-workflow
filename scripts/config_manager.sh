#!/bin/bash
# config_manager.sh - Configuration management with 4-tier priority
# Priority (high to low): CLI args > Project config > Global DB > Defaults

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB_PATH="${DB_PATH:-$HOME/.claude/data/workflow-cli.db}"

# Source db_manager for database operations
source "$SCRIPT_DIR/db_manager.sh"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Default AI configuration
DEFAULT_STEP1_AI="claude"
DEFAULT_STEP2_AI="codex"
DEFAULT_STEP3_AI="gemini"

#######################
# Path Resolution
#######################

# Get document path based on convention and config
# Usage: get_docs_path <project_dir> <doc_type>
# doc_type: requirements | design | code | reviews
get_docs_path() {
    local project_dir="$1"
    local doc_type="$2"

    # Check project config first
    if [ -f "$project_dir/.workflow-config.yaml" ]; then
        local custom_path=$(grep -A10 "^paths:" "$project_dir/.workflow-config.yaml" 2>/dev/null | grep "  $doc_type:" | awk '{print $2}' || echo "")
        if [ -n "$custom_path" ] && [ "$custom_path" != "null" ]; then
            echo "$project_dir/$custom_path"
            return
        fi
    fi

    # Use convention-based paths
    case $doc_type in
        requirements)
            echo "$project_dir/docs/requirements.md"
            ;;
        design)
            echo "$project_dir/docs/design.md"
            ;;
        code)
            echo "$project_dir/code"
            ;;
        reviews)
            echo "$project_dir/docs/reviews"
            ;;
        *)
            error "Unknown doc type: $doc_type"
            return 1
            ;;
    esac
}

#######################
# AI Configuration
#######################

# Get AI type for a specific step with 4-tier priority
# Usage: get_ai_for_step <step_number> [project_dir] [cli_override]
# Returns: AI type (claude/codex/gemini)
get_ai_for_step() {
    local step_number="$1"
    local project_dir="$2"
    local cli_override="$3"

    # Priority 1: CLI argument override
    if [ -n "$cli_override" ]; then
        echo "$cli_override"
        return
    fi

    # Priority 2: Project configuration file
    if [ -n "$project_dir" ] && [ -f "$project_dir/.workflow-config.yaml" ]; then
        local project_ai=$(grep -A5 "^ai:" "$project_dir/.workflow-config.yaml" 2>/dev/null | grep "  step$step_number:" | awk '{print $2}' || echo "")
        if [ -n "$project_ai" ] && [ "$project_ai" != "null" ]; then
            echo "$project_ai"
            return
        fi
    fi

    # Priority 3: Global database config
    local db_ai=$(get_config "step${step_number}_ai" 2>/dev/null || echo "")
    if [ -n "$db_ai" ]; then
        echo "$db_ai"
        return
    fi

    # Priority 4: System defaults
    case $step_number in
        1)
            echo "$DEFAULT_STEP1_AI"
            ;;
        2)
            echo "$DEFAULT_STEP2_AI"
            ;;
        3)
            echo "$DEFAULT_STEP3_AI"
            ;;
        *)
            echo "claude"  # Default fallback
            ;;
    esac
}

# Get all AI configuration for a project
# Usage: get_all_ai_config [project_dir]
# Returns: JSON object with step1, step2, step3 AI types
get_all_ai_config() {
    local project_dir="$1"

    local step1_ai=$(get_ai_for_step 1 "$project_dir" "")
    local step2_ai=$(get_ai_for_step 2 "$project_dir" "")
    local step3_ai=$(get_ai_for_step 3 "$project_dir" "")

    cat <<EOF
{
    "step1": "$step1_ai",
    "step2": "$step2_ai",
    "step3": "$step3_ai"
}
EOF
}

#######################
# Project Config
#######################

# Read project configuration
# Usage: read_project_config <project_dir> <key>
# Returns: value or empty string
read_project_config() {
    local project_dir="$1"
    local key="$2"

    if [ ! -f "$project_dir/.workflow-config.yaml" ]; then
        return
    fi

    # Simple YAML parsing (works for basic key-value pairs)
    grep "^$key:" "$project_dir/.workflow-config.yaml" | awk '{print $2}' || echo ""
}

# Check if auto-resume is enabled
# Usage: is_auto_resume_enabled [project_dir]
# Returns: 0 if enabled, 1 otherwise
is_auto_resume_enabled() {
    local project_dir="$1"

    # Check project config
    if [ -n "$project_dir" ] && [ -f "$project_dir/.workflow-config.yaml" ]; then
        local project_setting=$(grep -A5 "^workflow:" "$project_dir/.workflow-config.yaml" 2>/dev/null | grep "  enable_auto_resume:" | awk '{print $2}' || echo "")
        if [ -n "$project_setting" ]; then
            [ "$project_setting" = "true" ] && return 0 || return 1
        fi
    fi

    # Check global config
    local global_setting=$(get_config "enable_auto_resume" 2>/dev/null || echo "true")
    [ "$global_setting" = "true" ] && return 0 || return 1
}

# Check if auto git commit is enabled
# Usage: is_auto_git_enabled [project_dir]
# Returns: 0 if enabled, 1 otherwise
is_auto_git_enabled() {
    local project_dir="$1"

    # Check project config
    if [ -n "$project_dir" ] && [ -f "$project_dir/.workflow-config.yaml" ]; then
        local project_setting=$(grep -A5 "^workflow:" "$project_dir/.workflow-config.yaml" 2>/dev/null | grep "  auto_git_commit:" | awk '{print $2}' || echo "")
        if [ -n "$project_setting" ]; then
            [ "$project_setting" = "true" ] && return 0 || return 1
        fi
    fi

    # Default: enabled
    return 0
}

#######################
# Global Config
#######################

# Set global AI configuration
# Usage: set_global_ai <step_number> <ai_type>
set_global_ai() {
    local step_number="$1"
    local ai_type="$2"

    if [ -z "$step_number" ] || [ -z "$ai_type" ]; then
        error "set_global_ai requires step_number and ai_type"
        return 1
    fi

    # Validate AI type
    if [[ ! "$ai_type" =~ ^(claude|codex|gemini)$ ]]; then
        error "Invalid AI type: $ai_type (must be claude, codex, or gemini)"
        return 1
    fi

    set_config "step${step_number}_ai" "$ai_type"
    success "Global AI config updated: step$step_number = $ai_type"
}

# Show global configuration
# Usage: show_global_config
show_global_config() {
    echo "Global AI Configuration:"
    echo "  Step 1 (Requirements): $(get_config step1_ai || echo $DEFAULT_STEP1_AI)"
    echo "  Step 2 (Code): $(get_config step2_ai || echo $DEFAULT_STEP2_AI)"
    echo "  Step 3 (Review): $(get_config step3_ai || echo $DEFAULT_STEP3_AI)"
    echo ""
    echo "Other Settings:"
    echo "  Auto Resume: $(get_config enable_auto_resume || echo true)"
    echo "  Max Retry: $(get_config max_retry_attempts || echo 3)"
    echo "  Timeout: $(get_config timeout_minutes || echo 30) minutes"
}

# Show project configuration
# Usage: show_project_config <project_dir>
show_project_config() {
    local project_dir="$1"

    if [ ! -f "$project_dir/.workflow-config.yaml" ]; then
        echo "No project configuration found at: $project_dir"
        return 1
    fi

    echo "Project Configuration ($project_dir/.workflow-config.yaml):"
    echo ""
    cat "$project_dir/.workflow-config.yaml"
}

# Show effective configuration (after merging all priorities)
# Usage: show_effective_config [project_dir]
show_effective_config() {
    local project_dir="$1"

    echo "Effective Configuration:"
    echo "========================"
    echo ""
    echo "AI Configuration:"
    echo "  Step 1 (Requirements): $(get_ai_for_step 1 "$project_dir" "")"
    echo "  Step 2 (Code): $(get_ai_for_step 2 "$project_dir" "")"
    echo "  Step 3 (Review): $(get_ai_for_step 3 "$project_dir" "")"
    echo ""
    echo "Paths:"
    if [ -n "$project_dir" ]; then
        echo "  Requirements: $(get_docs_path "$project_dir" requirements)"
        echo "  Design: $(get_docs_path "$project_dir" design)"
        echo "  Code: $(get_docs_path "$project_dir" code)"
        echo "  Reviews: $(get_docs_path "$project_dir" reviews)"
    fi
    echo ""
    echo "Options:"
    echo "  Auto Resume: $(is_auto_resume_enabled "$project_dir" && echo "enabled" || echo "disabled")"
    echo "  Auto Git Commit: $(is_auto_git_enabled "$project_dir" && echo "enabled" || echo "disabled")"
}

# Parse CLI arguments for AI overrides
# Usage: parse_ai_args "$@"
# Sets environment variables: STEP1_AI_OVERRIDE, STEP2_AI_OVERRIDE, STEP3_AI_OVERRIDE
parse_ai_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --step1=*)
                export STEP1_AI_OVERRIDE="${1#*=}"
                ;;
            --step2=*)
                export STEP2_AI_OVERRIDE="${1#*=}"
                ;;
            --step3=*)
                export STEP3_AI_OVERRIDE="${1#*=}"
                ;;
            --ai=*)
                # Set all steps to the same AI
                local ai="${1#*=}"
                export STEP1_AI_OVERRIDE="$ai"
                export STEP2_AI_OVERRIDE="$ai"
                export STEP3_AI_OVERRIDE="$ai"
                ;;
        esac
        shift
    done
}

# Main command dispatcher
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    case "${1}" in
        get_ai)
            get_ai_for_step "${2}" "${3}" "${4}"
            ;;
        get_all_ai)
            get_all_ai_config "${2}"
            ;;
        get_path)
            get_docs_path "${2}" "${3}"
            ;;
        set_global_ai)
            set_global_ai "${2}" "${3}"
            ;;
        show_global)
            show_global_config
            ;;
        show_project)
            show_project_config "${2}"
            ;;
        show_effective)
            show_effective_config "${2}"
            ;;
        read_project)
            read_project_config "${2}" "${3}"
            ;;
        *)
            echo "Usage: $0 {command} [args...]"
            echo "Commands:"
            echo "  get_ai <step_number> [project_dir] [cli_override]"
            echo "  get_all_ai [project_dir]"
            echo "  get_path <project_dir> <doc_type>"
            echo "  set_global_ai <step_number> <ai_type>"
            echo "  show_global"
            echo "  show_project <project_dir>"
            echo "  show_effective [project_dir]"
            echo "  read_project <project_dir> <key>"
            exit 1
            ;;
    esac
fi
