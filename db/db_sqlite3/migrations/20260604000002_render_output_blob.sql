-- +goose Up
-- C1: the final DOCX is now stored as a content-addressed blob (key =
-- output_sha256, already present). content_fingerprint is a normalized,
-- toolchain-independent hash for durable reproducibility audits; output_size is
-- the byte length of the stored DOCX.
ALTER TABLE report_renders ADD COLUMN content_fingerprint CHAR(64);
ALTER TABLE report_renders ADD COLUMN output_size BIGINT;

-- +goose Down
-- SQLite (pre-3.35) does not support DROP COLUMN; columns remain but unused on
-- rollback. Modern SQLite would allow:
--   ALTER TABLE report_renders DROP COLUMN output_size;
--   ALTER TABLE report_renders DROP COLUMN content_fingerprint;
SELECT 1;
