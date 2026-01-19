-- +goose Up
-- Add soft delete columns to campaigns table

ALTER TABLE campaigns 
  ADD COLUMN deleted_at DATETIME(6) NULL DEFAULT NULL,
  ADD COLUMN deleted_by BIGINT NULL DEFAULT NULL,
  ADD COLUMN restored_at DATETIME(6) NULL DEFAULT NULL,
  ADD COLUMN restored_by BIGINT NULL DEFAULT NULL,
  ADD COLUMN status_before_delete VARCHAR(50) NULL DEFAULT NULL,
  ADD COLUMN delete_reason TEXT NULL DEFAULT NULL,
  ADD COLUMN version INT NOT NULL DEFAULT 0;

-- Add constraint for version
ALTER TABLE campaigns
  ADD CONSTRAINT chk_campaigns_version_positive CHECK (version >= 0);

-- Indexes for performance
CREATE INDEX idx_campaigns_deleted_at ON campaigns(deleted_at);
CREATE INDEX idx_campaigns_user_deleted ON campaigns(user_id, deleted_at);
CREATE INDEX idx_campaigns_deleted_by_date ON campaigns(deleted_by, deleted_at);
CREATE INDEX idx_campaigns_active_lookup ON campaigns(user_id, deleted_at, created_at);

-- Foreign keys for deleted_by and restored_by
ALTER TABLE campaigns
  ADD CONSTRAINT fk_campaigns_deleted_by 
    FOREIGN KEY (deleted_by) REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE campaigns
  ADD CONSTRAINT fk_campaigns_restored_by 
    FOREIGN KEY (restored_by) REFERENCES users(id) ON DELETE SET NULL;

-- Trigger to enforce unique names for active campaigns only
DELIMITER //

CREATE TRIGGER trg_campaigns_name_unique_active_insert
BEFORE INSERT ON campaigns
FOR EACH ROW
BEGIN
  IF NEW.deleted_at IS NULL THEN
    IF EXISTS (
      SELECT 1 FROM campaigns 
      WHERE user_id = NEW.user_id 
        AND LOWER(name) = LOWER(NEW.name)
        AND deleted_at IS NULL
        AND id != COALESCE(NEW.id, 0)
    ) THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Campaign name already exists for this user';
    END IF;
  END IF;
END//

CREATE TRIGGER trg_campaigns_name_unique_active_update
BEFORE UPDATE ON campaigns
FOR EACH ROW
BEGIN
  IF NEW.deleted_at IS NULL THEN
    IF EXISTS (
      SELECT 1 FROM campaigns 
      WHERE user_id = NEW.user_id 
        AND LOWER(name) = LOWER(NEW.name)
        AND deleted_at IS NULL
        AND id != NEW.id
    ) THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'Campaign name already exists for this user';
    END IF;
  END IF;
END//

DELIMITER ;

-- +goose Down
-- Remove triggers
DROP TRIGGER IF EXISTS trg_campaigns_name_unique_active_update;
DROP TRIGGER IF EXISTS trg_campaigns_name_unique_active_insert;

-- Remove foreign keys
ALTER TABLE campaigns
  DROP FOREIGN KEY IF EXISTS fk_campaigns_restored_by;

ALTER TABLE campaigns
  DROP FOREIGN KEY IF EXISTS fk_campaigns_deleted_by;

-- Remove indexes
DROP INDEX IF EXISTS idx_campaigns_active_lookup ON campaigns;
DROP INDEX IF EXISTS idx_campaigns_deleted_by_date ON campaigns;
DROP INDEX IF EXISTS idx_campaigns_user_deleted ON campaigns;
DROP INDEX IF EXISTS idx_campaigns_deleted_at ON campaigns;

-- Remove constraint
ALTER TABLE campaigns
  DROP CHECK IF EXISTS chk_campaigns_version_positive;

-- Remove columns
ALTER TABLE campaigns
  DROP COLUMN IF EXISTS version,
  DROP COLUMN IF EXISTS delete_reason,
  DROP COLUMN IF EXISTS status_before_delete,
  DROP COLUMN IF EXISTS restored_by,
  DROP COLUMN IF EXISTS restored_at,
  DROP COLUMN IF EXISTS deleted_by,
  DROP COLUMN IF EXISTS deleted_at;
