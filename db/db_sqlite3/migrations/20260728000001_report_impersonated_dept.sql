-- +goose Up
-- CL-103: two manual metadata fields for the S3 Ejecución fixed paragraph —
-- who was impersonated ({{SUPLANTANDO}}) and which communiqué was faked
-- ({{COMUNICADO}}). {{EMPRESA}} is already tokenized from company_name.
ALTER TABLE reports ADD COLUMN impersonated_as TEXT;
ALTER TABLE reports ADD COLUMN communique TEXT;

-- +goose Down
-- SQLite (pre-3.35) does not support DROP COLUMN; columns remain unused on
-- rollback.
SELECT 1;
