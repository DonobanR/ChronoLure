-- +goose Up
-- The report generation now also produces a per-recipient Excel (No. | Correo |
-- Estado), stored as a content-addressed blob keyed by this column.
ALTER TABLE report_renders ADD COLUMN output_xlsx_sha256 CHAR(64);

-- +goose Down
-- SQLite (pre-3.35) does not support DROP COLUMN; column remains unused on
-- rollback.
SELECT 1;
