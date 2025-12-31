
-- +goose Up
-- SQL in this section is executed when the migration is applied.

-- Add calendar phishing fields to campaigns table
ALTER TABLE `campaigns` ADD COLUMN campaign_type VARCHAR(255) DEFAULT 'email';
ALTER TABLE `campaigns` ADD COLUMN event_title VARCHAR(255);
ALTER TABLE `campaigns` ADD COLUMN event_description TEXT;
ALTER TABLE `campaigns` ADD COLUMN event_start_time DATETIME;
ALTER TABLE `campaigns` ADD COLUMN event_duration INTEGER;
ALTER TABLE `campaigns` ADD COLUMN organizer_name VARCHAR(255);
ALTER TABLE `campaigns` ADD COLUMN organizer_email VARCHAR(255);

-- Create calendar_events table
CREATE TABLE IF NOT EXISTS `calendar_events` (
    id INT AUTO_INCREMENT PRIMARY KEY,
    result_id BIGINT,
    event_type VARCHAR(255),
    timestamp DATETIME,
    ip VARCHAR(255),
    user_agent TEXT,
    details TEXT,
    FOREIGN KEY(result_id) REFERENCES results(id) ON DELETE CASCADE
);

-- Create index on result_id for faster lookups
CREATE INDEX idx_calendar_events_result_id ON calendar_events(result_id);

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.

DROP INDEX idx_calendar_events_result_id ON calendar_events;
DROP TABLE IF EXISTS `calendar_events`;

ALTER TABLE `campaigns` DROP COLUMN campaign_type;
ALTER TABLE `campaigns` DROP COLUMN event_title;
ALTER TABLE `campaigns` DROP COLUMN event_description;
ALTER TABLE `campaigns` DROP COLUMN event_start_time;
ALTER TABLE `campaigns` DROP COLUMN event_duration;
ALTER TABLE `campaigns` DROP COLUMN organizer_name;
ALTER TABLE `campaigns` DROP COLUMN organizer_email;

