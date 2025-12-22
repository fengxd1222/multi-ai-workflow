# Multi-AI Workflow CLI

> Version 2.0.0 - Enhanced multi-AI development workflow with CLI control and state management

A powerful skill for Claude Code that orchestrates software development workflows using multiple AI systems (Claude, Codex, Gemini). Features include persistent state management, flexible configuration, and full workflow control through slash commands.

English | [简体中文](README_CN.md)

## Features

- **Standard Project Structure** - Convention-based directory layout (docs/, code/)
- **Multi-AI Orchestration** - Delegate tasks to Claude, Codex, or Gemini based on configuration
- **State Management** - SQLite-based persistence for pause/resume functionality
- **Flexible Configuration** - 4-tier priority system (CLI > Project > Global > Default)
- **Workflow Control** - 9 slash commands for complete workflow management
- **Auto-Resume** - Automatically detect and recover interrupted workflows
- **Error Recovery** - Graceful error handling with multiple recovery options

## Installation

### Prerequisites

- Claude Code CLI
- SQLite3 (pre-installed on macOS)
- jq (for JSON processing)

```bash
# Install jq if needed
brew install jq

# Optional: Install yq for YAML processing
brew install yq
```

### Setup

1. Clone or copy the skill to your Claude skills directory:

```bash
# The skill should be at:
~/.claude/skills/multi-ai-workflow-cli/
```

2. Initialize the database:

```bash
~/.claude/skills/multi-ai-workflow-cli/scripts/db_manager.sh init
```

3. Verify installation:

```bash
# Check if skill is available
ls ~/.claude/skills/multi-ai-workflow-cli/

# Check if commands are available
ls ~/.claude/commands/workflow-*
```

## Quick Start

### 1. Initialize a New Project

```bash
/workflow-init myproject
cd myproject
```

This creates:
```
myproject/
├── docs/
│   ├── requirements.md
│   ├── design.md
│   └── reviews/
├── code/
├── .workflow/
├── README.md
├── .gitignore
└── .workflow-config.yaml
```

### 2. Start a Workflow

```bash
/workflow-start
```

The workflow will guide you through:
1. **Requirements Analysis** - Generate requirements.md and design.md
2. **Code Implementation** - Generate complete code in code/ directory
3. **Code Review** - Analyze code quality and generate review report
4. **Optimization** - Apply improvements based on review (optional)

### 3. Control the Workflow

```bash
# Pause at any time
/workflow-pause

# Resume later
/workflow-resume

# Check status
/workflow-status

# View all workflows
/workflow-list
```

## Usage Guide

### Project Structure

Every workflow project follows this standard structure:

```
<project-name>/
├── docs/                        # Documentation (convention)
│   ├── requirements.md          # Requirements specification
│   ├── design.md                # Detailed design document
│   └── reviews/                 # Code review reports
│       └── review-YYYYMMDD-HHMMSS.md
├── code/                        # Source code (convention)
│   └── ...                      # Structure varies by tech stack
├── .workflow/                   # Workflow metadata (auto-generated)
│   ├── config.yaml              # Project-specific settings
│   └── workflow-id.txt          # Current workflow ID
├── README.md                    # Project documentation
├── .gitignore                   # Git ignore rules
└── .workflow-config.yaml        # Workflow configuration
```

### Configuration

#### 4-Tier Priority System

Configuration is resolved with the following priority (highest to lowest):

1. **CLI Arguments** (highest priority)
   ```bash
   /workflow-start --step1=claude --step2=codex --step3=gemini
   ```

2. **Project Configuration** (.workflow-config.yaml)
   ```yaml
   ai:
     step1: claude
     step2: codex
     step3: gemini
   ```

3. **Global Database Configuration**
   ```bash
   /workflow-config set step1_ai claude
   ```

4. **System Defaults** (lowest priority)
   - Step 1: claude
   - Step 2: codex
   - Step 3: gemini

#### Configure AI for Each Step

**Global configuration (applies to all projects):**
```bash
/workflow-config set step1_ai claude
/workflow-config set step2_ai codex
/workflow-config set step3_ai gemini
```

**Project configuration (edit .workflow-config.yaml):**
```yaml
ai:
  step1: claude    # Requirements analysis
  step2: codex     # Code implementation
  step3: gemini    # Code review
```

**Per-execution override:**
```bash
/workflow-start --step1=claude --step2=claude --step3=codex
```

#### View Configuration

```bash
# Show all configuration
/workflow-config show

# Show effective configuration (after merging all priorities)
/workflow-config show effective

# Show global configuration only
/workflow-config show global

# Show project configuration only
/workflow-config show project
```

### Workflow Commands

#### Initialize Project

```bash
# Create project in current directory
/workflow-init myproject

# Create project in specific location
/workflow-init myproject --path=/Users/me/projects
```

#### Start Workflow

```bash
# Start with default configuration
/workflow-start

# Start with custom AI assignments
/workflow-start --step1=claude --step2=codex --step3=gemini

# Use same AI for all steps
/workflow-start --ai=claude

# Start in specific project
/workflow-start --project=/path/to/project
```

#### Pause and Resume

```bash
# Pause current workflow
/workflow-pause

# Pause with a note
/workflow-pause Need to review requirements

# Resume most recent paused workflow
/workflow-resume

# Resume specific workflow
/workflow-resume a1b2c3d4-e5f6-7890-abcd-ef1234567890

# Resume workflow for specific project
/workflow-resume myproject
```

#### Check Status

```bash
# Status of current workflow
/workflow-status

# Status of specific workflow
/workflow-status a1b2c3d4-e5f6-7890-abcd-ef1234567890

# Status for project
/workflow-status myproject
```

#### List Workflows

```bash
# List all workflows
/workflow-list

# Filter by project
/workflow-list --project=myproject

# Filter by status
/workflow-list --status=paused

# Limit results
/workflow-list --limit=10
```

#### Rollback

```bash
# Rollback current workflow to step 2
/workflow-rollback 2

# Rollback specific workflow
/workflow-rollback 1 a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

#### Clean Up

```bash
# Clean completed workflows older than 7 days
/workflow-clean

# Clean workflows older than 30 days
/workflow-clean --older-than=30

# Clean failed workflows
/workflow-clean --status=failed

# Preview what would be deleted
/workflow-clean --dry-run

# Clean specific project
/workflow-clean --project=old-project --older-than=14
```

## Workflow Steps

### Step 1: Requirements Analysis

**Default AI:** Claude
**Input:** User description of requirements
**Output:**
- `docs/requirements.md` - Comprehensive requirements specification
- `docs/design.md` - Detailed technical design

**Tasks:**
- Analyze user requirements
- Create structured requirements document
- Design system architecture
- Define technical approach
- Identify constraints and dependencies

### Step 2: Code Implementation

**Default AI:** Codex
**Input:** Requirements and design documents
**Output:** Complete, production-ready code in `code/` directory

**Tasks:**
- Generate project structure appropriate for technology stack
- Implement all functionality per requirements
- Add comprehensive comments
- Include error handling
- Create tests
- Generate README with usage instructions

### Step 3: Code Review

**Default AI:** Gemini
**Input:** Generated code
**Output:** `docs/reviews/review-{timestamp}.md`

**Tasks:**
- Analyze code quality
- Check security vulnerabilities
- Evaluate performance
- Review test coverage
- Assess maintainability
- Provide improvement suggestions

### Step 4: Optimization (Optional)

**AI:** Same as Step 2
**Input:** Code + review feedback
**Output:** Optimized code

**Tasks:**
- Apply review suggestions
- Fix identified issues
- Improve code quality
- Enhance performance

## Advanced Usage

### Custom Project Templates

Modify templates in `~/.claude/skills/multi-ai-workflow-cli/templates/`:
- `README.template.md` - Project README template
- `gitignore.template` - Git ignore rules
- `workflow-config.template.yaml` - Default project configuration

### Auto-Resume Feature

Enable/disable auto-resume in configuration:

```yaml
# In .workflow-config.yaml
workflow:
  enable_auto_resume: true
```

Or globally:
```bash
/workflow-config set enable_auto_resume true
```

When enabled, the skill automatically detects unfinished workflows and prompts you to resume.

### Manual Edits During Workflow

You can pause a workflow, make manual edits, and resume:

```bash
# Start workflow
/workflow-start

# After Step 1, pause to make manual edits
/workflow-pause

# Edit docs/requirements.md manually
# ... make your changes ...

# Resume workflow
/workflow-resume
```

### Working with Multiple Projects

The CLI supports managing multiple projects simultaneously:

```bash
# List all projects' workflows
/workflow-list

# Check specific project status
/workflow-status myproject

# Resume workflow for specific project
/workflow-resume myproject
```

## Troubleshooting

### Database Issues

```bash
# Reinitialize database (WARNING: loses all workflow history)
rm ~/.claude/data/workflow-cli.db
~/.claude/skills/multi-ai-workflow-cli/scripts/db_manager.sh init
```

### Check Database State

```bash
# List all projects
~/.claude/skills/multi-ai-workflow-cli/scripts/db_manager.sh list_projects

# List all workflows
~/.claude/skills/multi-ai-workflow-cli/scripts/db_manager.sh list_workflows

# Get active workflows
~/.claude/skills/multi-ai-workflow-cli/scripts/db_manager.sh get_active_workflows
```

### External CLI Tools

If Codex or Gemini CLI tools fail:

1. **Verify installation:**
   ```bash
   which codex
   which gemini
   ```

2. **Test manually:**
   ```bash
   codex --version
   gemini --version
   ```

3. **Check configuration:**
   - Ensure tools are in PATH
   - Verify API keys are configured
   - Check proxy settings if needed

4. **Use alternative AI:**
   ```bash
   # Use Claude for all steps if external tools unavailable
   /workflow-start --ai=claude
   ```

### Workflow Stuck or Corrupted

```bash
# Check workflow status
/workflow-status

# If corrupted, you can:
# 1. Rollback to earlier step
/workflow-rollback 1

# 2. Or start fresh workflow
/workflow-start

# 3. Or manually fix database
sqlite3 ~/.claude/data/workflow-cli.db
# UPDATE workflows SET status='failed' WHERE id='<workflow-id>';
```

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────┐
│                     Claude (You)                        │
│                   Coordinator/Orchestrator               │
└────────────┬──────────────────────────────┬─────────────┘
             │                              │
   ┌─────────▼─────────┐        ┌──────────▼──────────┐
   │   Configuration   │        │   State Management  │
   │    Manager        │        │                     │
   │  4-tier priority  │        │  SQLite Database    │
   └───────────────────┘        └─────────────────────┘
             │                              │
   ┌─────────▼──────────────────────────────▼─────────┐
   │              Workflow Engine                      │
   │  Step 1 → Step 2 → Step 3 → Step 4               │
   └──┬─────────┬────────────┬──────────────────────┬─┘
      │         │            │                      │
   ┌──▼──┐  ┌──▼───┐    ┌───▼────┐            ┌───▼────┐
   │Claude│  │Codex │    │ Gemini │            │Scripts │
   │      │  │ CLI  │    │  CLI   │            │        │
   └──────┘  └──────┘    └────────┘            └────────┘
```

### Database Schema

```sql
projects          -- Project registry
workflows         -- Workflow instances
workflow_steps    -- Step execution records
workflow_config   -- Global configuration
workflow_state    -- Execution context for pause/resume
```

### File Structure

```
~/.claude/
├── skills/multi-ai-workflow-cli/
│   ├── SKILL.md                      # Main workflow logic
│   ├── README.md                     # This file
│   ├── templates/                    # Project templates
│   │   ├── README.template.md
│   │   ├── gitignore.template
│   │   └── workflow-config.template.yaml
│   ├── scripts/                      # Core scripts
│   │   ├── init_project.sh           # Project initialization
│   │   ├── db_manager.sh             # Database operations
│   │   ├── state_manager.sh          # State management
│   │   ├── config_manager.sh         # Configuration management
│   │   ├── call_codex.sh             # Codex CLI wrapper
│   │   ├── call_gemini.sh            # Gemini CLI wrapper
│   │   └── schema.sql                # Database schema
│   └── references/                   # Documentation
│       ├── document_templates.md
│       └── error_troubleshooting.md
│
├── commands/                         # Slash commands
│   ├── workflow-init.md
│   ├── workflow-start.md
│   ├── workflow-pause.md
│   ├── workflow-resume.md
│   ├── workflow-status.md
│   ├── workflow-config.md
│   ├── workflow-rollback.md
│   ├── workflow-list.md
│   └── workflow-clean.md
│
└── data/
    └── workflow-cli.db               # SQLite database
```

## Examples

### Example 1: Simple Python Project

```bash
# Initialize project
/workflow-init python-calculator --path=~/projects
cd ~/projects/python-calculator

# Edit requirements
# (Edit docs/requirements.md to describe calculator requirements)

# Start workflow
/workflow-start

# Review generated requirements
# Approve to proceed

# Review generated code
# Approve to proceed to review

# View review report
# Choose to optimize or accept

# Done!
```

### Example 2: Web Application with Custom AI

```bash
# Initialize
/workflow-init my-webapp

# Start with Claude for everything (for faster iteration)
/workflow-start --ai=claude

# After seeing requirements, decide to switch
/workflow-pause

# Restart with proper AI allocation
/workflow-start --step1=claude --step2=codex --step3=gemini
```

### Example 3: Resume After Interruption

```bash
# Start workflow
/workflow-start

# ... Computer crashes or Claude session ends ...

# Later, restart Claude and navigate to project
cd myproject

# Skill auto-detects paused workflow
# "Found paused workflow from 2025-01-15. Resume?"

# Yes -> Continue from where you left off
```

## Contributing

This is a custom skill. To modify:

1. Edit `SKILL.md` for workflow logic changes
2. Edit scripts in `scripts/` for functionality changes
3. Edit commands in `~/.claude/commands/` for command behavior

## License

Apache 2.0

## Version History

### 2.0.0 (2025-01-19)
- Complete rewrite with CLI control
- Added SQLite state management
- Implemented 4-tier configuration system
- Added 9 slash commands
- Added pause/resume functionality
- Added rollback capability
- Added auto-resume detection

### 1.0.0 (Previous)
- Basic multi-AI workflow
- File-based state tracking
- Manual coordination

## Related Resources

- [Original multi-ai-workflow](https://github.com/fengxd1222/multi-ai-workflow)
- [Claude Code Documentation](https://docs.claude.com/claude-code)
- [SQLite Documentation](https://sqlite.org/docs.html)

---

**Need Help?**

1. Check workflow status: `/workflow-status`
2. View configuration: `/workflow-config show effective`
3. List all workflows: `/workflow-list`
4. Read troubleshooting section above
5. Check logs in workflow step records

**Happy Coding! 🚀**
