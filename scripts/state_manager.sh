#!/bin/bash
# state_manager.sh - Workflow state management for pause/resume functionality

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB_PATH="${DB_PATH:-$HOME/.claude/data/workflow-cli.db}"

# Source db_manager for database operations
source "$SCRIPT_DIR/db_manager.sh"

# Save workflow state for pause/resume
# Usage: save_state <workflow_id> <context_json>
# context_json should include: current_step, user_choices, file_paths, etc.
save_state() {
    local workflow_id="$1"
    local context="$2"

    if [ -z "$workflow_id" ] || [ -z "$context" ]; then
        error "save_state requires workflow_id and context"
        return 1
    fi

    # Escape single quotes in context
    context="${context//\'/\'\'}"

    sqlite3 "$DB_PATH" <<EOF
INSERT INTO workflow_state (workflow_id, context)
VALUES ('$workflow_id', '$context')
ON CONFLICT(workflow_id) DO UPDATE SET
    context='$context',
    updated_at=datetime('now');
EOF

    if [ $? -eq 0 ]; then
        echo "State saved for workflow: $workflow_id"
    else
        error "Failed to save state"
        return 1
    fi
}

# Restore workflow state
# Usage: restore_state <workflow_id>
# Returns: JSON context object
restore_state() {
    local workflow_id="$1"

    if [ -z "$workflow_id" ]; then
        error "restore_state requires workflow_id"
        return 1
    fi

    local context=$(sqlite3 "$DB_PATH" "SELECT context FROM workflow_state WHERE workflow_id='$workflow_id';")

    if [ -z "$context" ]; then
        error "No saved state found for workflow: $workflow_id"
        return 1
    fi

    echo "$context"
}

# Get current workflow context with all relevant information
# Usage: get_workflow_context <workflow_id>
# Returns: Complete JSON object with workflow, project, and state info
get_workflow_context() {
    local workflow_id="$1"

    if [ -z "$workflow_id" ]; then
        error "get_workflow_context requires workflow_id"
        return 1
    fi

    # Get workflow info
    local workflow_info=$(get_workflow "$workflow_id")

    if [ -z "$workflow_info" ] || [ "$workflow_info" = "null" ]; then
        error "Workflow not found: $workflow_id"
        return 1
    fi

    # Get steps info
    local steps_info=$(get_steps "$workflow_id")

    # Get saved state (if exists)
    local saved_state=$(sqlite3 "$DB_PATH" "SELECT context FROM workflow_state WHERE workflow_id='$workflow_id';" || echo "{}")

    # Combine all information
    echo "$workflow_info" | jq --argjson steps "$steps_info" --argjson state "$saved_state" \
        '. + {steps: $steps, saved_state: $state}'
}

# Check if workflow is resumable (paused or running)
# Usage: is_resumable <workflow_id>
# Returns: 0 if resumable, 1 otherwise
is_resumable() {
    local workflow_id="$1"

    local status=$(sqlite3 "$DB_PATH" "SELECT status FROM workflows WHERE id='$workflow_id';")

    if [ "$status" = "paused" ] || [ "$status" = "running" ]; then
        return 0
    else
        return 1
    fi
}

# Find the last active workflow (paused or running)
# Usage: find_last_active_workflow
# Returns: workflow_id or empty string
find_last_active_workflow() {
    sqlite3 "$DB_PATH" <<EOF
SELECT id FROM workflows
WHERE status IN ('paused', 'running')
ORDER BY updated_at DESC
LIMIT 1;
EOF
}

# Pause workflow
# Usage: pause_workflow <workflow_id> [context_json]
pause_workflow() {
    local workflow_id="$1"
    local context="${2:-{}}"

    if [ -z "$workflow_id" ]; then
        error "pause_workflow requires workflow_id"
        return 1
    fi

    # Update workflow status to paused
    update_workflow_status "$workflow_id" "paused"

    # Save state if context provided
    if [ "$context" != "{}" ]; then
        save_state "$workflow_id" "$context"
    fi

    success "Workflow paused: $workflow_id"
}

# Resume workflow
# Usage: resume_workflow <workflow_id>
# Returns: JSON context for resuming
resume_workflow() {
    local workflow_id="$1"

    if [ -z "$workflow_id" ]; then
        error "resume_workflow requires workflow_id"
        return 1
    fi

    # Check if workflow is resumable
    if ! is_resumable "$workflow_id"; then
        error "Workflow is not resumable (status must be 'paused' or 'running')"
        return 1
    fi

    # Update status to running
    update_workflow_status "$workflow_id" "running"

    # Get full context
    get_workflow_context "$workflow_id"
}

# Create a snapshot of current workflow state
# Usage: create_snapshot <workflow_id> <step_number> <step_name> <data>
create_snapshot() {
    local workflow_id="$1"
    local step_number="$2"
    local step_name="$3"
    local data="$4"

    local context=$(cat <<EOF
{
    "step_number": $step_number,
    "step_name": "$step_name",
    "data": $data,
    "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
}
EOF
)

    save_state "$workflow_id" "$context"
}

# Get workflow summary for display
# Usage: get_workflow_summary <workflow_id>
# Returns: Human-readable summary
get_workflow_summary() {
    local workflow_id="$1"

    local context=$(get_workflow_context "$workflow_id")

    if [ -z "$context" ] || [ "$context" = "null" ]; then
        echo "Workflow not found"
        return 1
    fi

    echo "$context" | jq -r '
        "Workflow ID: " + .id + "\n" +
        "Project: " + .project_name + "\n" +
        "Path: " + .project_path + "\n" +
        "Status: " + .status + "\n" +
        "Current Step: " + (.current_step | tostring) + "\n" +
        "Created: " + .created_at + "\n" +
        "Updated: " + .updated_at
    '
}

# List all resumable workflows
# Usage: list_resumable_workflows
# Returns: JSON array of resumable workflows
list_resumable_workflows() {
    sqlite3 "$DB_PATH" <<EOF
SELECT json_group_array(
    json_object(
        'id', w.id,
        'project_name', p.name,
        'project_path', p.path,
        'status', w.status,
        'current_step', w.current_step,
        'updated_at', w.updated_at
    )
) FROM workflows w
JOIN projects p ON w.project_id = p.id
WHERE w.status IN ('paused', 'running')
ORDER BY w.updated_at DESC;
EOF
}

# Main command dispatcher
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    case "${1}" in
        save)
            save_state "${2}" "${3}"
            ;;
        restore)
            restore_state "${2}"
            ;;
        context)
            get_workflow_context "${2}"
            ;;
        pause)
            pause_workflow "${2}" "${3}"
            ;;
        resume)
            resume_workflow "${2}"
            ;;
        summary)
            get_workflow_summary "${2}"
            ;;
        find_active)
            find_last_active_workflow
            ;;
        list_resumable)
            list_resumable_workflows
            ;;
        snapshot)
            create_snapshot "${2}" "${3}" "${4}" "${5}"
            ;;
        *)
            echo "Usage: $0 {command} [args...]"
            echo "Commands:"
            echo "  save <workflow_id> <context_json>"
            echo "  restore <workflow_id>"
            echo "  context <workflow_id>"
            echo "  pause <workflow_id> [context_json]"
            echo "  resume <workflow_id>"
            echo "  summary <workflow_id>"
            echo "  find_active"
            echo "  list_resumable"
            echo "  snapshot <workflow_id> <step_number> <step_name> <data_json>"
            exit 1
            ;;
    esac
fi
