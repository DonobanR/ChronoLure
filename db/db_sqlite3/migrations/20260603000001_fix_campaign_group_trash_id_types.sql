-- +goose Up
-- Parity placeholder for the MySQL corrective migration of the same version.
--
-- In SQLite the campaign_groups.deleted_by / restored_by columns were declared as
-- INTEGER, and SQLite's INTEGER storage class is already a 64-bit signed integer
-- (equivalent to BIGINT). There is therefore no type inconsistency to fix here and
-- no schema change is required. This file exists only to keep the migration version
-- history aligned across the SQLite and MySQL dialects.
SELECT 1;

-- +goose Down
SELECT 1;
