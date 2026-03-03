-- +goose Up
-- Add soft-delete (trash) columns to campaign_groups table
ALTER TABLE campaign_groups ADD COLUMN deleted_at DATETIME NULL;
ALTER TABLE campaign_groups ADD COLUMN deleted_by INT NULL;
ALTER TABLE campaign_groups ADD COLUMN delete_reason TEXT;
ALTER TABLE campaign_groups ADD COLUMN restored_at DATETIME NULL;
ALTER TABLE campaign_groups ADD COLUMN restored_by INT NULL;
CREATE INDEX idx_campaign_groups_deleted_at ON campaign_groups(deleted_at);

-- +goose Down
ALTER TABLE campaign_groups DROP INDEX idx_campaign_groups_deleted_at;
ALTER TABLE campaign_groups DROP COLUMN restored_by;
ALTER TABLE campaign_groups DROP COLUMN restored_at;
ALTER TABLE campaign_groups DROP COLUMN delete_reason;
ALTER TABLE campaign_groups DROP COLUMN deleted_by;
ALTER TABLE campaign_groups DROP COLUMN deleted_at;
