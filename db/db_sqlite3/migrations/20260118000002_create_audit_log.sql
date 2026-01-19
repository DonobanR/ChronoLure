-- +goose Up
-- Create audit log table for tracking campaign operations (SQLite version)

CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  actor_id INTEGER NULL,
  actor_name TEXT NULL,
  action TEXT NOT NULL,
  entity_type TEXT NOT NULL,
  entity_id INTEGER NOT NULL,
  metadata TEXT NULL,
  ip_address TEXT NULL,
  user_agent TEXT NULL
);

CREATE INDEX idx_audit_timestamp ON audit_log(timestamp DESC);
CREATE INDEX idx_audit_entity ON audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_actor_date ON audit_log(actor_id, timestamp DESC);
CREATE INDEX idx_audit_action ON audit_log(action, timestamp DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_log;
