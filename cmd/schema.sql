CREATE TABLE IF NOT EXISTS customers (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  stage VARCHAR(32) NOT NULL DEFAULT 'onboarding',
  health VARCHAR(16) DEFAULT 'green',
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS activities (
  id VARCHAR(64) PRIMARY KEY,
  customer_id VARCHAR(64) NOT NULL,
  kind VARCHAR(32) NOT NULL,
  summary VARCHAR(500) NOT NULL,
  body TEXT,
  links JSON,
  occurred_at TIMESTAMP NOT NULL,
  INDEX (customer_id, occurred_at)
);

CREATE TABLE IF NOT EXISTS items (
  id VARCHAR(64) PRIMARY KEY,
  customer_id VARCHAR(64) NOT NULL,
  type VARCHAR(16) NOT NULL,
  title VARCHAR(255) NOT NULL,
  description TEXT,
  status VARCHAR(16) NOT NULL DEFAULT 'open',
  priority TINYINT DEFAULT 2,
  external_ref VARCHAR(500),
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  resolved_at TIMESTAMP NULL,
  due_at DATE NULL,
  INDEX (customer_id, status)
);

CREATE TABLE IF NOT EXISTS edges (
  from_id VARCHAR(64) NOT NULL,
  to_id VARCHAR(64) NOT NULL,
  rel VARCHAR(24) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (from_id, to_id, rel)
);

CREATE TABLE IF NOT EXISTS stage_events (
  id VARCHAR(64) PRIMARY KEY,
  customer_id VARCHAR(64) NOT NULL,
  from_stage VARCHAR(32),
  to_stage VARCHAR(32) NOT NULL,
  reason VARCHAR(500),
  occurred_at TIMESTAMP NOT NULL,
  INDEX (customer_id, occurred_at)
);

-- Per-customer Claude Code chat session, so `cs board`'s chat pane can resume
-- each account's conversation. Also created on demand by the TUI.
CREATE TABLE IF NOT EXISTS chat_sessions (
  customer_id VARCHAR(64) PRIMARY KEY,
  session_id VARCHAR(64) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
