-- +goose Up
-- C1: the final DOCX is now stored as a content-addressed blob (key =
-- output_sha256, already present). content_fingerprint is a normalized,
-- toolchain-independent hash for durable reproducibility audits; output_size is
-- the byte length of the stored DOCX.
ALTER TABLE report_renders ADD COLUMN content_fingerprint CHAR(64) NULL;
ALTER TABLE report_renders ADD COLUMN output_size BIGINT NULL;

-- +goose Down
ALTER TABLE report_renders DROP COLUMN output_size;
ALTER TABLE report_renders DROP COLUMN content_fingerprint;
