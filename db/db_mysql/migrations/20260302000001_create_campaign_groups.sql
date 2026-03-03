-- +goose Up
-- Create campaign_groups table (MySQL version)

CREATE TABLE campaign_groups (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    archived BOOLEAN NOT NULL DEFAULT 0,
    user_id BIGINT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_campaign_groups_user_id (user_id),
    INDEX idx_campaign_groups_archived (archived)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Create campaign_group_campaigns table (junction table)
CREATE TABLE campaign_group_campaigns (
    id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    group_id INT NOT NULL,
    campaign_id BIGINT NOT NULL,
    order_index INT NOT NULL DEFAULT 0,
    added_date DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (group_id) REFERENCES campaign_groups(id) ON DELETE CASCADE,
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    UNIQUE KEY unique_group_campaign (group_id, campaign_id),
    INDEX idx_campaign_group_campaigns_group_id (group_id),
    INDEX idx_campaign_group_campaigns_campaign_id (campaign_id),
    INDEX idx_campaign_group_campaigns_order (group_id, order_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS campaign_group_campaigns;
DROP TABLE IF EXISTS campaign_groups;
