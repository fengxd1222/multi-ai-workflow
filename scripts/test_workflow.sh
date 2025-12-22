#!/bin/bash
# test_workflow.sh - Quick test script for multi-ai-workflow-cli
# This script tests basic functionality of the workflow system

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILLS_DIR="$HOME/.claude/skills/multi-ai-workflow-cli"
DB_PATH="$HOME/.claude/data/workflow-cli.db"
TEST_PROJECT_DIR="/tmp/test-workflow-project-$$"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test results
TESTS_PASSED=0
TESTS_FAILED=0

print_header() {
    echo ""
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

print_test() {
    echo -e "${YELLOW}TEST: $1${NC}"
}

pass() {
    echo -e "${GREEN}✓ PASS${NC}: $1"
    ((TESTS_PASSED++))
}

fail() {
    echo -e "${RED}✗ FAIL${NC}: $1"
    ((TESTS_FAILED++))
}

cleanup() {
    echo ""
    echo "Cleaning up test artifacts..."
    rm -rf "$TEST_PROJECT_DIR" 2>/dev/null || true
}

trap cleanup EXIT

# ======================================
# Test 1: Database Initialization
# ======================================
print_header "Test 1: Database Initialization"

print_test "Initialize database"
if "$SKILLS_DIR/scripts/db_manager.sh" init &>/dev/null; then
    pass "Database initialized"
else
    fail "Database initialization failed"
fi

print_test "Check if database file exists"
if [ -f "$DB_PATH" ]; then
    pass "Database file exists at $DB_PATH"
else
    fail "Database file not found"
fi

# ======================================
# Test 2: Database Operations
# ======================================
print_header "Test 2: Database Operations"

print_test "Create test project"
PROJECT_ID=$("$SKILLS_DIR/scripts/db_manager.sh" create_project "test-project" "/tmp/test" 2>/dev/null)
if [ -n "$PROJECT_ID" ]; then
    pass "Project created with ID: ${PROJECT_ID:0:8}..."
else
    fail "Failed to create project"
fi

print_test "Get project by name"
PROJECT_JSON=$("$SKILLS_DIR/scripts/db_manager.sh" get_project "test-project" 2>/dev/null)
if echo "$PROJECT_JSON" | jq -e '.id' &>/dev/null; then
    pass "Retrieved project successfully"
else
    fail "Failed to retrieve project"
fi

print_test "Create workflow"
WORKFLOW_ID=$("$SKILLS_DIR/scripts/db_manager.sh" create_workflow "$PROJECT_ID" 2>/dev/null)
if [ -n "$WORKFLOW_ID" ]; then
    pass "Workflow created with ID: ${WORKFLOW_ID:0:8}..."
else
    fail "Failed to create workflow"
fi

print_test "Get workflow"
WORKFLOW_JSON=$("$SKILLS_DIR/scripts/db_manager.sh" get_workflow "$WORKFLOW_ID" 2>/dev/null)
if echo "$WORKFLOW_JSON" | jq -e '.id' &>/dev/null; then
    pass "Retrieved workflow successfully"
else
    fail "Failed to retrieve workflow"
fi

print_test "Create workflow step"
STEP_ID=$("$SKILLS_DIR/scripts/db_manager.sh" create_step "$WORKFLOW_ID" 1 "requirements" "claude" 2>/dev/null)
if [ -n "$STEP_ID" ]; then
    pass "Step created with ID: $STEP_ID"
else
    fail "Failed to create step"
fi

print_test "Update step status"
if "$SKILLS_DIR/scripts/db_manager.sh" update_step "$STEP_ID" "completed" "Test result" "[]" "" &>/dev/null; then
    pass "Step status updated"
else
    fail "Failed to update step status"
fi

# ======================================
# Test 3: Configuration Management
# ======================================
print_header "Test 3: Configuration Management"

print_test "Set global config"
if "$SKILLS_DIR/scripts/db_manager.sh" set_config "test_key" "test_value" &>/dev/null; then
    pass "Global config set"
else
    fail "Failed to set global config"
fi

print_test "Get global config"
CONFIG_VALUE=$("$SKILLS_DIR/scripts/db_manager.sh" get_config "test_key" 2>/dev/null)
if [ "$CONFIG_VALUE" = "test_value" ]; then
    pass "Retrieved correct config value"
else
    fail "Config value mismatch: expected 'test_value', got '$CONFIG_VALUE'"
fi

print_test "Get AI configuration (default)"
AI_TYPE=$("$SKILLS_DIR/scripts/config_manager.sh" get_ai 1 "" "" 2>/dev/null)
if [ -n "$AI_TYPE" ]; then
    pass "Got AI type for step 1: $AI_TYPE"
else
    fail "Failed to get AI type"
fi

# ======================================
# Test 4: State Management
# ======================================
print_header "Test 4: State Management"

print_test "Save workflow state"
TEST_STATE='{"step": 1, "data": "test"}'
if "$SKILLS_DIR/scripts/state_manager.sh" save "$WORKFLOW_ID" "$TEST_STATE" &>/dev/null; then
    pass "State saved"
else
    fail "Failed to save state"
fi

print_test "Restore workflow state"
RESTORED_STATE=$("$SKILLS_DIR/scripts/state_manager.sh" restore "$WORKFLOW_ID" 2>/dev/null)
if echo "$RESTORED_STATE" | jq -e '.step' &>/dev/null; then
    pass "State restored successfully"
else
    fail "Failed to restore state"
fi

print_test "Pause workflow"
if "$SKILLS_DIR/scripts/state_manager.sh" pause "$WORKFLOW_ID" '{}' &>/dev/null; then
    pass "Workflow paused"
else
    fail "Failed to pause workflow"
fi

print_test "Find active workflow"
ACTIVE_ID=$("$SKILLS_DIR/scripts/state_manager.sh" find_active 2>/dev/null)
if [ "$ACTIVE_ID" = "$WORKFLOW_ID" ]; then
    pass "Found active workflow"
else
    fail "Failed to find active workflow"
fi

# ======================================
# Test 5: Project Initialization
# ======================================
print_header "Test 5: Project Initialization"

print_test "Initialize test project"
PROJECT_PATH=$("$SKILLS_DIR/scripts/init_project.sh" "test-workflow-project-$$" "/tmp" 2>&1 | tail -1)
if [ -d "$PROJECT_PATH" ]; then
    pass "Project directory created: $PROJECT_PATH"
else
    fail "Failed to create project directory"
fi

print_test "Check project structure"
if [ -d "$PROJECT_PATH/docs" ] && [ -d "$PROJECT_PATH/code" ] && [ -d "$PROJECT_PATH/.workflow" ]; then
    pass "Project structure is correct"
else
    fail "Project structure is incomplete"
fi

print_test "Check template files"
if [ -f "$PROJECT_PATH/README.md" ] && [ -f "$PROJECT_PATH/.gitignore" ] && [ -f "$PROJECT_PATH/.workflow-config.yaml" ]; then
    pass "Template files created"
else
    fail "Template files missing"
fi

print_test "Check git initialization"
if [ -d "$PROJECT_PATH/.git" ]; then
    pass "Git repository initialized"
else
    fail "Git repository not initialized"
fi

# ======================================
# Test 6: File Existence
# ======================================
print_header "Test 6: File Existence"

print_test "Check SKILL.md"
if [ -f "$SKILLS_DIR/SKILL.md" ]; then
    pass "SKILL.md exists"
else
    fail "SKILL.md not found"
fi

print_test "Check README.md"
if [ -f "$SKILLS_DIR/README.md" ]; then
    pass "README.md exists"
else
    fail "README.md not found"
fi

print_test "Check all scripts"
SCRIPTS=("db_manager.sh" "state_manager.sh" "config_manager.sh" "init_project.sh")
ALL_SCRIPTS_EXIST=true
for script in "${SCRIPTS[@]}"; do
    if [ ! -f "$SKILLS_DIR/scripts/$script" ]; then
        ALL_SCRIPTS_EXIST=false
        fail "Missing script: $script"
        break
    fi
done
if $ALL_SCRIPTS_EXIST; then
    pass "All scripts exist"
fi

print_test "Check all commands"
COMMANDS=("workflow-init" "workflow-start" "workflow-pause" "workflow-resume" "workflow-status" "workflow-config" "workflow-rollback" "workflow-list" "workflow-clean")
ALL_COMMANDS_EXIST=true
for cmd in "${COMMANDS[@]}"; do
    if [ ! -f "$HOME/.claude/commands/$cmd.md" ]; then
        ALL_COMMANDS_EXIST=false
        fail "Missing command: $cmd"
        break
    fi
done
if $ALL_COMMANDS_EXIST; then
    pass "All commands exist"
fi

# ======================================
# Test Summary
# ======================================
print_header "Test Summary"

TOTAL_TESTS=$((TESTS_PASSED + TESTS_FAILED))
echo ""
echo "Total Tests: $TOTAL_TESTS"
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
if [ $TESTS_FAILED -gt 0 ]; then
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
else
    echo -e "${GREEN}Failed: $TESTS_FAILED${NC}"
fi
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  ALL TESTS PASSED! ✓${NC}"
    echo -e "${GREEN}========================================${NC}"
    exit 0
else
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}  SOME TESTS FAILED! ✗${NC}"
    echo -e "${RED}========================================${NC}"
    exit 1
fi
