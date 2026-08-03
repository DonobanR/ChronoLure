-- +goose Up
-- CL-103: two manual metadata fields for the S3 Ejecución fixed paragraph —
-- who was impersonated ({{SUPLANTANDO}}) and which communiqué was faked
-- ({{COMUNICADO}}). {{EMPRESA}} is already tokenized from company_name.
ALTER TABLE reports ADD COLUMN impersonated_as TEXT NULL;
ALTER TABLE reports ADD COLUMN communique TEXT NULL;

-- +goose Down
ALTER TABLE reports DROP COLUMN communique;
ALTER TABLE reports DROP COLUMN impersonated_as;
