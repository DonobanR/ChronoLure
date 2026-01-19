-- +goose Up
-- Add soft delete columns to campaigns table (SQLite version)

ALTER TABLE campaigns ADD COLUMN deleted_at DATETIME DEFAULT NULL;
ALTER TABLE campaigns ADD COLUMN deleted_by INTEGER DEFAULT NULL;
ALTER TABLE campaigns ADD COLUMN restored_at DATETIME DEFAULT NULL;
ALTER TABLE campaigns ADD COLUMN restored_by INTEGER DEFAULT NULL;
ALTER TABLE campaigns ADD COLUMN status_before_delete VARCHAR(50) DEFAULT NULL;
ALTER TABLE campaigns ADD COLUMN delete_reason TEXT DEFAULT NULL;
ALTER TABLE campaigns ADD COLUMN version INTEGER NOT NULL DEFAULT 0;

-- Indexes for performance
CREATE INDEX idx_campaigns_deleted_at ON campaigns(deleted_at);
CREATE INDEX idx_campaigns_user_deleted ON campaigns(user_id, deleted_at);
CREATE INDEX idx_campaigns_deleted_by_date ON campaigns(deleted_by, deleted_at);

-- SQLite supports partial indexes (only active campaigns)
CREATE UNIQUE INDEX idx_campaigns_name_active 
  ON campaigns(LOWER(name), user_id) 
  WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_campaigns_name_active;
DROP INDEX IF EXISTS idx_campaigns_deleted_by_date;
DROP INDEX IF EXISTS idx_campaigns_user_deleted;
DROP INDEX IF EXISTS idx_campaigns_deleted_at;

-- Note: SQLite doesn't support DROP COLUMN easily
-- Columns will remain but be unused in rollback
