-- +goose Up
-- Create audit log table for tracking campaign operations

CREATE TABLE audit_log (
  id BIGINT PRIMARY KEY AUTO_INCREMENT,
  timestamp DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  actor_id BIGINT NULL,
  actor_name VARCHAR(255) NULL,
  action VARCHAR(100) NOT NULL,
  entity_type VARCHAR(50) NOT NULL,
  entity_id BIGINT NOT NULL,
  metadata JSON NULL,
  ip_address VARCHAR(45) NULL,
  user_agent TEXT NULL,
  
  INDEX idx_audit_timestamp (timestamp DESC),
  INDEX idx_audit_entity (entity_type, entity_id),
  INDEX idx_audit_actor_date (actor_id, timestamp DESC),
  INDEX idx_audit_action (action, timestamp DESC),
  
  CONSTRAINT fk_audit_actor 
    FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
DROP TABLE IF EXISTS audit_log;
