-- +goose Up
-- Corrective migration: campaign_groups.deleted_by / restored_by were created as INT
-- in 20260302000002_add_trash_to_campaign_groups.sql, but they hold user IDs and
-- users.id is BIGINT. This widens them to BIGINT for type consistency with users(id)
-- and parity with campaigns.deleted_by / campaigns.restored_by.
--
-- MODIFY widens INT -> BIGINT without data loss (BIGINT is a strict superset of INT).
-- Columns stay NULLable so existing rows (NULL) are preserved unchanged.
ALTER TABLE campaign_groups MODIFY COLUMN deleted_by BIGINT NULL;
ALTER TABLE campaign_groups MODIFY COLUMN restored_by BIGINT NULL;

-- +goose Down
-- Revert to the original INT definition. Note: narrowing BIGINT -> INT can truncate
-- values above 2147483647; user IDs are not expected to reach that range in practice.
ALTER TABLE campaign_groups MODIFY COLUMN deleted_by INT NULL;
ALTER TABLE campaign_groups MODIFY COLUMN restored_by INT NULL;
