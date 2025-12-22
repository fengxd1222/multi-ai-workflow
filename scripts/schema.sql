-- Multi-AI Workflow CLI Database Schema
-- SQLite3 database for managing workflow state and configuration

-- Projects table: tracks all initialized projects
CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY,
  name TEXT UNIQUE NOT NULL,
  path TEXT NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Workflows table: tracks workflow executions
CREATE TABLE IF NOT EXISTS workflows (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('pending', 'running', 'paused', 'completed', 'failed')),
  current_step INTEGER DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Workflow steps table: tracks individual step executions
CREATE TABLE IF NOT EXISTS workflow_steps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workflow_id TEXT NOT NULL,
  step_number INTEGER NOT NULL,
  step_name TEXT NOT NULL CHECK(step_name IN ('init', 'requirements', 'code', 'review', 'optimize')),
  ai_type TEXT NOT NULL CHECK(ai_type IN ('claude', 'codex', 'gemini')),
  status TEXT NOT NULL CHECK(status IN ('pending', 'running', 'completed', 'failed')),
  started_at TIMESTAMP,
  completed_at TIMESTAMP,
  result TEXT,
  output_files TEXT,  -- JSON array of generated file paths
  error_message TEXT,
  FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

-- Workflow config table: global configuration
CREATE TABLE IF NOT EXISTS workflow_config (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Workflow state table: stores execution context for pause/resume
CREATE TABLE IF NOT EXISTS workflow_state (
  workflow_id TEXT PRIMARY KEY,
  context TEXT NOT NULL,  -- JSON object with execution context
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_workflows_project ON workflows(project_id);
CREATE INDEX IF NOT EXISTS idx_workflows_status ON workflows(status);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_workflow ON workflow_steps(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_status ON workflow_steps(status);

-- Insert default configuration
INSERT OR IGNORE INTO workflow_config (key, value) VALUES
  ('step1_ai', 'claude'),
  ('step2_ai', 'codex'),
  ('step3_ai', 'gemini'),
  ('enable_auto_resume', 'true'),
  ('max_retry_attempts', '3'),
  ('timeout_minutes', '30');

-- Create a trigger to update updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_projects_timestamp
  AFTER UPDATE ON projects
BEGIN
  UPDATE projects SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_workflows_timestamp
  AFTER UPDATE ON workflows
BEGIN
  UPDATE workflows SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_config_timestamp
  AFTER UPDATE ON workflow_config
BEGIN
  UPDATE workflow_config SET updated_at = CURRENT_TIMESTAMP WHERE key = NEW.key;
END;
