#!/bin/bash
# db_manager.sh - Database management functions for multi-ai-workflow-cli
# Provides CRUD operations for projects, workflows, and steps

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DB_PATH="${DB_PATH:-$HOME/.claude/data/workflow-cli.db}"
SCHEMA_PATH="$SCRIPT_DIR/schema.sql"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Helper functions
error() {
    echo -e "${RED}Error: $1${NC}" >&2
    return 1
}

success() {
    echo -e "${GREEN}$1${NC}"
}

# Initialize database
init_db() {
    if [ ! -f "$DB_PATH" ]; then
        mkdir -p "$(dirname "$DB_PATH")"
        sqlite3 "$DB_PATH" < "$SCHEMA_PATH"
        success "Database initialized at $DB_PATH"
    fi
}

# Ensure database is initialized
ensure_db() {
    if [ ! -f "$DB_PATH" ]; then
        init_db
    fi
}

# Generate UUID (cross-platform)
generate_uuid() {
    if command -v uuidgen &> /dev/null; then
        uuidgen | tr '[:upper:]' '[:lower:]'
    else
        # Fallback: use Python
        python3 -c "import uuid; print(str(uuid.uuid4()))"
    fi
}

#######################
# Project Operations
#######################

# Create a new project
# Usage: create_project <name> <path>
# Returns: project_id
create_project() {
    ensure_db
    local name="$1"
    local path="$2"

    if [ -z "$name" ] || [ -z "$path" ]; then
        error "create_project requires name and path"
        return 1
    fi

    local project_id=$(generate_uuid)

    sqlite3 "$DB_PATH" <<EOF
INSERT INTO projects (id, name, path) VALUES ('$project_id', '$name', '$path');
EOF

    if [ $? -eq 0 ]; then
        echo "$project_id"
    else
        error "Failed to create project"
        return 1
    fi
}

# Get project by name
# Usage: get_project <name>
# Returns: JSON object
get_project() {
    ensure_db
    local name="$1"

    sqlite3 "$DB_PATH" <<EOF
SELECT json_object(
    'id', id,
    'name', name,
    'path', path,
    'created_at', created_at,
    'updated_at', updated_at
) FROM projects WHERE name='$name';
EOF
}

# Get project by ID
# Usage: get_project_by_id <id>
# Returns: JSON object
get_project_by_id() {
    ensure_db
    local id="$1"

    sqlite3 "$DB_PATH" <<EOF
SELECT json_object(
    'id', id,
    'name', name,
    'path', path,
    'created_at', created_at,
    'updated_at', updated_at
) FROM projects WHERE id='$id';
EOF
}

# List all projects
# Usage: list_projects
# Returns: JSON array
list_projects() {
    ensure_db

    sqlite3 "$DB_PATH" <<EOF
SELECT json_group_array(
    json_object(
        'id', id,
        'name', name,
        'path', path,
        'created_at', created_at
    )
) FROM projects ORDER BY created_at DESC;
EOF
}

#######################
# Workflow Operations
#######################

# Create a new workflow
# Usage: create_workflow <project_id>
# Returns: workflow_id
create_workflow() {
    ensure_db
    local project_id="$1"

    if [ -z "$project_id" ]; then
        error "create_workflow requires project_id"
        return 1
    fi

    local workflow_id=$(generate_uuid)

    sqlite3 "$DB_PATH" <<EOF
INSERT INTO workflows (id, project_id, status, current_step)
VALUES ('$workflow_id', '$project_id', 'pending', 0);
EOF

    if [ $? -eq 0 ]; then
        echo "$workflow_id"
    else
        error "Failed to create workflow"
        return 1
    fi
}

# Get workflow by ID
# Usage: get_workflow <workflow_id>
# Returns: JSON object
get_workflow() {
    ensure_db
    local workflow_id="$1"

    sqlite3 "$DB_PATH" <<EOF
SELECT json_object(
    'id', w.id,
    'project_id', w.project_id,
    'project_name', p.name,
    'project_path', p.path,
    'status', w.status,
    'current_step', w.current_step,
    'created_at', w.created_at,
    'updated_at', w.updated_at
) FROM workflows w
JOIN projects p ON w.project_id = p.id
WHERE w.id='$workflow_id';
EOF
}

# Update workflow status
# Usage: update_workflow_status <workflow_id> <status> [current_step]
update_workflow_status() {
    ensure_db
    local workflow_id="$1"
    local status="$2"
    local current_step="$3"

    if [ -z "$workflow_id" ] || [ -z "$status" ]; then
        error "update_workflow_status requires workflow_id and status"
        return 1
    fi

    if [ -n "$current_step" ]; then
        sqlite3 "$DB_PATH" "UPDATE workflows SET status='$status', current_step=$current_step WHERE id='$workflow_id';"
    else
        sqlite3 "$DB_PATH" "UPDATE workflows SET status='$status' WHERE id='$workflow_id';"
    fi
}

# Get active workflows (running or paused)
# Usage: get_active_workflows
# Returns: JSON array
get_active_workflows() {
    ensure_db

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
WHERE w.status IN ('running', 'paused')
ORDER BY w.updated_at DESC;
EOF
}

# List workflows for a project
# Usage: list_workflows <project_id>
# Returns: JSON array
list_workflows() {
    ensure_db
    local project_id="$1"

    local where_clause=""
    if [ -n "$project_id" ]; then
        where_clause="WHERE w.project_id='$project_id'"
    fi

    sqlite3 "$DB_PATH" <<EOF
SELECT json_group_array(
    json_object(
        'id', w.id,
        'project_name', p.name,
        'status', w.status,
        'current_step', w.current_step,
        'created_at', w.created_at,
        'updated_at', w.updated_at
    )
) FROM workflows w
JOIN projects p ON w.project_id = p.id
$where_clause
ORDER BY w.created_at DESC;
EOF
}

#######################
# Step Operations
#######################

# Create a workflow step
# Usage: create_step <workflow_id> <step_number> <step_name> <ai_type>
# Returns: step_id
create_step() {
    ensure_db
    local workflow_id="$1"
    local step_number="$2"
    local step_name="$3"
    local ai_type="$4"

    if [ -z "$workflow_id" ] || [ -z "$step_number" ] || [ -z "$step_name" ] || [ -z "$ai_type" ]; then
        error "create_step requires workflow_id, step_number, step_name, and ai_type"
        return 1
    fi

    sqlite3 "$DB_PATH" <<EOF
INSERT INTO workflow_steps (workflow_id, step_number, step_name, ai_type, status, started_at)
VALUES ('$workflow_id', $step_number, '$step_name', '$ai_type', 'running', datetime('now'));
SELECT last_insert_rowid();
EOF
}

# Update step status
# Usage: update_step <step_id> <status> [result] [output_files] [error_message]
update_step() {
    ensure_db
    local step_id="$1"
    local status="$2"
    local result="$3"
    local output_files="$4"
    local error_message="$5"

    if [ -z "$step_id" ] || [ -z "$status" ]; then
        error "update_step requires step_id and status"
        return 1
    fi

    local sql="UPDATE workflow_steps SET status='$status'"

    if [ "$status" = "completed" ] || [ "$status" = "failed" ]; then
        sql="$sql, completed_at=datetime('now')"
    fi

    if [ -n "$result" ]; then
        # Escape single quotes in result
        result="${result//\'/\'\'}"
        sql="$sql, result='$result'"
    fi

    if [ -n "$output_files" ]; then
        output_files="${output_files//\'/\'\'}"
        sql="$sql, output_files='$output_files'"
    fi

    if [ -n "$error_message" ]; then
        error_message="${error_message//\'/\'\'}"
        sql="$sql, error_message='$error_message'"
    fi

    sql="$sql WHERE id=$step_id;"

    sqlite3 "$DB_PATH" "$sql"
}

# Get steps for a workflow
# Usage: get_steps <workflow_id>
# Returns: JSON array
get_steps() {
    ensure_db
    local workflow_id="$1"

    sqlite3 "$DB_PATH" <<EOF
SELECT json_group_array(
    json_object(
        'id', id,
        'step_number', step_number,
        'step_name', step_name,
        'ai_type', ai_type,
        'status', status,
        'started_at', started_at,
        'completed_at', completed_at,
        'result', result
    )
) FROM workflow_steps WHERE workflow_id='$workflow_id' ORDER BY step_number;
EOF
}

#######################
# Config Operations
#######################

# Get config value
# Usage: get_config <key>
# Returns: value
get_config() {
    ensure_db
    local key="$1"

    sqlite3 "$DB_PATH" "SELECT value FROM workflow_config WHERE key='$key';"
}

# Set config value
# Usage: set_config <key> <value>
set_config() {
    ensure_db
    local key="$1"
    local value="$2"

    if [ -z "$key" ] || [ -z "$value" ]; then
        error "set_config requires key and value"
        return 1
    fi

    sqlite3 "$DB_PATH" <<EOF
INSERT INTO workflow_config (key, value) VALUES ('$key', '$value')
ON CONFLICT(key) DO UPDATE SET value='$value', updated_at=datetime('now');
EOF
}

# List all config
# Usage: list_config
# Returns: JSON array
list_config() {
    ensure_db

    sqlite3 "$DB_PATH" <<EOF
SELECT json_group_array(
    json_object('key', key, 'value', value)
) FROM workflow_config ORDER BY key;
EOF
}

#######################
# Cleanup Operations
#######################

# Delete old workflows
# Usage: cleanup_workflows <days>
cleanup_workflows() {
    ensure_db
    local days="${1:-7}"

    sqlite3 "$DB_PATH" <<EOF
DELETE FROM workflows
WHERE status='completed'
  AND datetime(updated_at) < datetime('now', '-$days days');
EOF

    echo "Cleaned up workflows older than $days days"
}

# Main command dispatcher
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    case "${1}" in
        init)
            init_db
            ;;
        create_project)
            create_project "${2}" "${3}"
            ;;
        get_project)
            get_project "${2}"
            ;;
        list_projects)
            list_projects
            ;;
        create_workflow)
            create_workflow "${2}"
            ;;
        get_workflow)
            get_workflow "${2}"
            ;;
        update_workflow_status)
            update_workflow_status "${2}" "${3}" "${4}"
            ;;
        get_active_workflows)
            get_active_workflows
            ;;
        list_workflows)
            list_workflows "${2}"
            ;;
        create_step)
            create_step "${2}" "${3}" "${4}" "${5}"
            ;;
        update_step)
            update_step "${2}" "${3}" "${4}" "${5}" "${6}"
            ;;
        get_steps)
            get_steps "${2}"
            ;;
        get_config)
            get_config "${2}"
            ;;
        set_config)
            set_config "${2}" "${3}"
            ;;
        list_config)
            list_config
            ;;
        cleanup_workflows)
            cleanup_workflows "${2}"
            ;;
        *)
            echo "Usage: $0 {command} [args...]"
            echo "Commands:"
            echo "  init"
            echo "  create_project <name> <path>"
            echo "  get_project <name>"
            echo "  list_projects"
            echo "  create_workflow <project_id>"
            echo "  get_workflow <workflow_id>"
            echo "  update_workflow_status <workflow_id> <status> [current_step]"
            echo "  get_active_workflows"
            echo "  list_workflows [project_id]"
            echo "  create_step <workflow_id> <step_number> <step_name> <ai_type>"
            echo "  update_step <step_id> <status> [result] [output_files] [error]"
            echo "  get_steps <workflow_id>"
            echo "  get_config <key>"
            echo "  set_config <key> <value>"
            echo "  list_config"
            echo "  cleanup_workflows [days]"
            exit 1
            ;;
    esac
fi
