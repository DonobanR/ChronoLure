package models

import (
	"encoding/json"
	"time"

	log "github.com/gophish/gophish/logger"
)

// AuditAction constants
const (
	AuditCampaignSoftDeleted = "CAMPAIGN_SOFT_DELETED"
	AuditCampaignRestored    = "CAMPAIGN_RESTORED"
	AuditCampaignPurged      = "CAMPAIGN_PURGED"
)

// AuditLog represents an audit trail entry
type AuditLog struct {
	ID         int64                  `json:"id" gorm:"primaryKey"`
	Timestamp  time.Time              `json:"timestamp" gorm:"default:CURRENT_TIMESTAMP"`
	ActorID    *int64                 `json:"actor_id,omitempty"`
	ActorName  string                 `json:"actor_name,omitempty"`
	Action     string                 `json:"action" gorm:"not null"`
	EntityType string                 `json:"entity_type" gorm:"not null"`
	EntityID   int64                  `json:"entity_id" gorm:"not null"`
	Metadata   string                 `json:"metadata,omitempty" gorm:"type:text"` // JSON as string for compatibility
	IPAddress  string                 `json:"ip_address,omitempty"`
	UserAgent  string                 `json:"user_agent,omitempty" gorm:"type:text"`
}

// TableName specifies the table name for AuditLog
func (AuditLog) TableName() string {
	return "audit_log"
}

// SetMetadata serializes metadata map to JSON string
func (a *AuditLog) SetMetadata(data map[string]interface{}) error {
	if data == nil {
		a.Metadata = ""
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	a.Metadata = string(jsonData)
	return nil
}

// GetMetadata deserializes metadata JSON string to map
func (a *AuditLog) GetMetadata() (map[string]interface{}, error) {
	if a.Metadata == "" {
		return nil, nil
	}
	var data map[string]interface{}
	err := json.Unmarshal([]byte(a.Metadata), &data)
	return data, err
}

// SaveAuditLog persists audit entry
func SaveAuditLog(entry *AuditLog) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	err := db.Create(entry).Error
	if err != nil {
		log.Errorf("Failed to save audit log: %v", err)
	}
	return err
}

// GetAuditLogs retrieves audit history for entity
func GetAuditLogs(entityType string, entityID int64) ([]AuditLog, error) {
	logs := []AuditLog{}
	err := db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Order("timestamp DESC").
		Find(&logs).Error
	if err != nil {
		log.Error(err)
	}
	return logs, err
}

// GetAuditLogsByActor retrieves audit logs for specific actor
func GetAuditLogsByActor(actorID int64, limit int) ([]AuditLog, error) {
	logs := []AuditLog{}
	query := db.Where("actor_id = ?", actorID).
		Order("timestamp DESC")
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	err := query.Find(&logs).Error
	if err != nil {
		log.Error(err)
	}
	return logs, err
}
