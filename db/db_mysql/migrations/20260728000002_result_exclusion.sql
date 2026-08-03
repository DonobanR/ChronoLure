-- +goose Up
-- CL-102: reversible exclusion of internal validation recipients. An excluded
-- result is kept (auditable, reversible) but dropped from every metric surface
-- (funnel, chart, conclusions, Excel annex, group stats).
ALTER TABLE results ADD COLUMN excluded BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE results ADD COLUMN excluded_reason TEXT NULL;
ALTER TABLE results ADD COLUMN excluded_by BIGINT NULL;
ALTER TABLE results ADD COLUMN excluded_at DATETIME NULL;
CREATE INDEX idx_results_campaign_excluded ON results(campaign_id, excluded);

-- +goose Down
DROP INDEX idx_results_campaign_excluded ON results;
ALTER TABLE results DROP COLUMN excluded_at;
ALTER TABLE results DROP COLUMN excluded_by;
ALTER TABLE results DROP COLUMN excluded_reason;
ALTER TABLE results DROP COLUMN excluded;
