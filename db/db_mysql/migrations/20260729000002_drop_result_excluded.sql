-- +goose Up
-- CL-102R: the `excluded` state is superseded by soft delete; drop it after the
-- conversion migration (20260729000001) has run.
DROP INDEX idx_results_campaign_excluded ON results;
ALTER TABLE results DROP COLUMN excluded_at;
ALTER TABLE results DROP COLUMN excluded_by;
ALTER TABLE results DROP COLUMN excluded_reason;
ALTER TABLE results DROP COLUMN excluded;

-- +goose Down
ALTER TABLE results ADD COLUMN excluded BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE results ADD COLUMN excluded_reason TEXT NULL;
ALTER TABLE results ADD COLUMN excluded_by BIGINT NULL;
ALTER TABLE results ADD COLUMN excluded_at DATETIME NULL;
CREATE INDEX idx_results_campaign_excluded ON results(campaign_id, excluded);
