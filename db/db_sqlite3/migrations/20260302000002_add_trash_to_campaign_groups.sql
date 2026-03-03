-- +goose Up
-- Add soft-delete (trash) columns to campaign_groups table
ALTER TABLE campaign_groups ADD COLUMN deleted_at DATETIME;
ALTER TABLE campaign_groups ADD COLUMN deleted_by INTEGER;
ALTER TABLE campaign_groups ADD COLUMN delete_reason TEXT DEFAULT '';
ALTER TABLE campaign_groups ADD COLUMN restored_at DATETIME;
ALTER TABLE campaign_groups ADD COLUMN restored_by INTEGER;
CREATE INDEX idx_campaign_groups_deleted_at ON campaign_groups(deleted_at);

-- +goose Down
-- SQLite does not support DROP COLUMN natively.
-- To roll back, the campaign_groups table would need to be recreated without these columns.
DROP INDEX IF EXISTS idx_campaign_groups_deleted_at;
