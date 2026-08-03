-- +goose Up
-- CL-102: reversible exclusion of internal validation recipients. An excluded
-- result is kept (auditable, reversible) but dropped from every metric surface
-- (funnel, chart, conclusions, Excel annex, group stats).
ALTER TABLE results ADD COLUMN excluded BOOLEAN NOT NULL DEFAULT 0;
ALTER TABLE results ADD COLUMN excluded_reason TEXT;
ALTER TABLE results ADD COLUMN excluded_by INTEGER;
ALTER TABLE results ADD COLUMN excluded_at DATETIME;
CREATE INDEX idx_results_campaign_excluded ON results(campaign_id, excluded);

-- +goose Down
-- SQLite (pre-3.35) does not support DROP COLUMN; drop the index only.
DROP INDEX IF EXISTS idx_results_campaign_excluded;
