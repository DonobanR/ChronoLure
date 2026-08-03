-- +goose Up
-- CL-102R: reversible DELETION of recipients via soft delete on results,
-- integrated with the existing Trash (replaces the CL-102 `excluded` state).
ALTER TABLE results ADD COLUMN deleted_at      DATETIME;
ALTER TABLE results ADD COLUMN deleted_by      BIGINT;
ALTER TABLE results ADD COLUMN delete_reason   TEXT;
ALTER TABLE results ADD COLUMN restored_at     DATETIME;
ALTER TABLE results ADD COLUMN restored_by     BIGINT;
ALTER TABLE results ADD COLUMN delete_scope    VARCHAR(16);
ALTER TABLE results ADD COLUMN delete_batch_id VARCHAR(36);

CREATE INDEX idx_results_campaign_deleted ON results(campaign_id, deleted_at);
CREATE INDEX idx_results_deleted_at ON results(deleted_at);

-- Non-destructive conversion of any existing CL-102 exclusions to soft-deletes.
-- One audit row per migrated recipient so the history is not mute.
INSERT INTO audit_log (timestamp, actor_id, actor_name, action, entity_type, entity_id, metadata)
SELECT CURRENT_TIMESTAMP, excluded_by, 'system:cl-102r-migration', 'recipient_soft_deleted', 'result', id,
       json_object('migrated_from', 'excluded', 'ticket', 'CL-102R',
                   'campaign_id', campaign_id, 'email', email,
                   'reason', COALESCE(excluded_reason, ''))
FROM results WHERE excluded = 1;

UPDATE results
   SET deleted_at    = COALESCE(excluded_at, CURRENT_TIMESTAMP),
       deleted_by    = excluded_by,
       delete_reason = COALESCE(excluded_reason, 'Migrado desde exclusión (CL-102R)'),
       delete_scope  = 'campaign'
 WHERE excluded = 1;

-- +goose Down
DROP INDEX IF EXISTS idx_results_deleted_at;
DROP INDEX IF EXISTS idx_results_campaign_deleted;
ALTER TABLE results DROP COLUMN delete_batch_id;
ALTER TABLE results DROP COLUMN delete_scope;
ALTER TABLE results DROP COLUMN restored_by;
ALTER TABLE results DROP COLUMN restored_at;
ALTER TABLE results DROP COLUMN delete_reason;
ALTER TABLE results DROP COLUMN deleted_by;
ALTER TABLE results DROP COLUMN deleted_at;
