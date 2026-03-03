-- +goose Up
-- Create campaign_groups table (SQLite version)

CREATE TABLE campaign_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    created_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived BOOLEAN NOT NULL DEFAULT 0,
    user_id INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_campaign_groups_user_id ON campaign_groups(user_id);
CREATE INDEX idx_campaign_groups_archived ON campaign_groups(archived);

-- Create campaign_group_campaigns table (junction table)
CREATE TABLE campaign_group_campaigns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_id INTEGER NOT NULL,
    campaign_id INTEGER NOT NULL,
    order_index INTEGER NOT NULL DEFAULT 0,
    added_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (group_id) REFERENCES campaign_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    UNIQUE(group_id, campaign_id)
);

CREATE INDEX idx_campaign_group_campaigns_group_id ON campaign_group_campaigns(group_id);
CREATE INDEX idx_campaign_group_campaigns_campaign_id ON campaign_group_campaigns(campaign_id);
CREATE INDEX idx_campaign_group_campaigns_order ON campaign_group_campaigns(group_id, order_index);

-- +goose Down
DROP TABLE IF EXISTS campaign_group_campaigns;
DROP TABLE IF EXISTS campaign_groups;
