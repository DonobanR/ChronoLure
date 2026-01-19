package models

import (
	"errors"
	"fmt"
	"time"

	log "github.com/gophish/gophish/logger"
	"github.com/jinzhu/gorm"
)

var (
	ErrAlreadyDeleted   = errors.New("campaign already deleted")
	ErrNotDeleted       = errors.New("campaign not deleted")
	ErrCampaignNotFound = errors.New("campaign not found")
	ErrPermissionDenied = errors.New("permission denied")
	ErrNameConflict     = errors.New("campaign name conflict")
)

// IsDeleted returns true if campaign is in trash
func (c *Campaign) IsDeleted() bool {
	return c.DeletedAt != nil
}

// ScopeCampaignsActive returns query scope for active campaigns only
func ScopeCampaignsActive(db *gorm.DB) *gorm.DB {
	return db.Where("deleted_at IS NULL")
}

// ScopeCampaignsTrashed returns query scope for deleted campaigns only
func ScopeCampaignsTrashed(db *gorm.DB) *gorm.DB {
	return db.Where("deleted_at IS NOT NULL")
}

// lockForUpdate adds FOR UPDATE clause only for databases that support it (MySQL/PostgreSQL)
// SQLite doesn't support FOR UPDATE but has automatic database-level locking
func lockForUpdate(tx *gorm.DB) *gorm.DB {
	// Check database dialect
	if tx.Dialect().GetName() == "sqlite3" {
		// SQLite doesn't support FOR UPDATE, skip it
		return tx
	}
	// MySQL, PostgreSQL support FOR UPDATE
	return tx.Set("gorm:query_option", "FOR UPDATE")
}

// SoftDeleteCampaign moves campaign to trash
func SoftDeleteCampaign(campaignID int64, userID int64, reason string) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Lock row for update
	c := &Campaign{}
	if err := lockForUpdate(tx).First(c, campaignID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCampaignNotFound
		}
		return err
	}

	// Validate ownership (multi-tenant)
	if c.UserId != userID {
		tx.Rollback()
		log.Warnf("User %d attempted to delete campaign %d owned by %d", userID, campaignID, c.UserId)
		return ErrPermissionDenied
	}

	// Idempotency: already deleted
	if c.IsDeleted() {
		tx.Rollback()
		log.Warnf("Campaign %d already deleted, idempotent response", campaignID)
		return nil // Success (idempotent)
	}

	// Stop scheduler if running/queued
	if c.Status == CampaignInProgress || c.Status == CampaignQueued {
		log.Infof("Stopping campaign %d (status: %s) before soft delete", campaignID, c.Status)
		// Mark as completed to stop any pending jobs
		c.StatusBeforeDelete = c.Status
		c.Status = CampaignComplete
	} else {
		c.StatusBeforeDelete = c.Status
	}

	// Set soft delete fields
	now := time.Now().UTC()
	c.DeletedAt = &now
	c.DeletedBy = &userID
	c.DeleteReason = reason
	c.Version++

	// Save
	if err := tx.Save(c).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to soft delete campaign %d: %v", campaignID, err)
		return err
	}

	// Audit log
	audit := &AuditLog{
		ActorID:    &userID,
		Action:     AuditCampaignSoftDeleted,
		EntityType: "campaign",
		EntityID:   campaignID,
	}
	audit.SetMetadata(map[string]interface{}{
		"name":          c.Name,
		"status_before": c.StatusBeforeDelete,
		"reason":        reason,
		"campaign_type": c.CampaignType,
	})

	if err := tx.Create(audit).Error; err != nil {
		log.Errorf("Failed to create audit log (non-blocking): %v", err)
		// Don't fail transaction for audit log
	}

	log.Infof("Campaign %d (%s) soft deleted by user %d", campaignID, c.Name, userID)
	return tx.Commit().Error
}

// RestoreResult contains restore operation result
type RestoreResult struct {
	Success     bool
	Campaign    *Campaign
	NameChanged bool
	OldName     string
	NewName     string
	Warnings    []string
}

// RestoreCampaign recupera campaña de papelera
func RestoreCampaign(campaignID int64, userID int64) (*RestoreResult, error) {
	result := &RestoreResult{Success: false, Warnings: []string{}}

	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Lock campaign - need to use Unscoped() to find deleted records
	c := &Campaign{}
	if err := lockForUpdate(tx).Unscoped().First(c, campaignID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCampaignNotFound
		}
		return nil, err
	}

	// Validate ownership
	if c.UserId != userID {
		tx.Rollback()
		log.Warnf("User %d attempted to restore campaign %d owned by %d", userID, campaignID, c.UserId)
		return nil, ErrPermissionDenied
	}

	// Must be deleted
	if !c.IsDeleted() {
		tx.Rollback()
		return nil, ErrNotDeleted
	}

	// Check name conflicts with active campaigns
	hasConflict, err := checkNameConflict(tx, c.Name, userID, campaignID)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	originalName := c.Name
	if hasConflict {
		// Generate new name with timestamp
		c.Name = fmt.Sprintf("%s (Restored %s)", c.Name,
			time.Now().Format("2006-01-02 15:04"))

		// Re-check if new name also conflicts (unlikely but possible)
		for i := 1; i < 10; i++ {
			conflictCheck, err := checkNameConflict(tx, c.Name, userID, campaignID)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
			if !conflictCheck {
				break
			}
			c.Name = fmt.Sprintf("%s (Restored %s-%d)", originalName,
				time.Now().Format("2006-01-02"), i)
		}

		result.NameChanged = true
		result.OldName = originalName
		result.NewName = c.Name
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("Campaign renamed from '%s' to '%s' due to name conflict", originalName, c.Name))
		log.Infof("Campaign %d renamed during restore: %s -> %s", campaignID, originalName, c.Name)
	}

	// Clear soft delete fields
	c.DeletedAt = nil
	c.DeletedBy = nil
	now := time.Now().UTC()
	c.RestoredAt = &now
	c.RestoredBy = &userID

	// Set safe status (paused/created, not auto-sending)
	c.Status = CampaignCreated
	c.Version++

	// Save using Unscoped() to ensure we can update soft-deleted records
	if err := tx.Unscoped().Save(c).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to restore campaign %d: %v", campaignID, err)
		return nil, err
	}

	// Audit
	audit := &AuditLog{
		ActorID:    &userID,
		Action:     AuditCampaignRestored,
		EntityType: "campaign",
		EntityID:   campaignID,
	}
	audit.SetMetadata(map[string]interface{}{
		"name":          c.Name,
		"original_name": originalName,
		"name_changed":  result.NameChanged,
		"warnings":      result.Warnings,
	})

	if err := tx.Create(audit).Error; err != nil {
		log.Errorf("Failed to create audit log (non-blocking): %v", err)
	}

	result.Success = true
	result.Campaign = c

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	log.Infof("Campaign %d restored by user %d (warnings: %d)", campaignID, userID, len(result.Warnings))
	return result, nil
}

// PurgeCampaign ejecuta hard delete definitivo
func PurgeCampaign(campaignID int64, userID int64, isAdmin bool) error {
	if !isAdmin {
		return errors.New("purge requires admin privileges")
	}

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Lock campaign
	c := &Campaign{}
	if err := lockForUpdate(tx).First(c, campaignID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCampaignNotFound
		}
		return err
	}

	// Must be in trash
	if !c.IsDeleted() {
		tx.Rollback()
		return errors.New("can only purge campaigns in trash")
	}

	// Audit BEFORE delete (critical - must persist even after deletion)
	audit := &AuditLog{
		ActorID:    &userID,
		Action:     AuditCampaignPurged,
		EntityType: "campaign",
		EntityID:   campaignID,
	}
	audit.SetMetadata(map[string]interface{}{
		"name":       c.Name,
		"deleted_at": c.DeletedAt,
		"user_id":    c.UserId,
	})

	if err := tx.Create(audit).Error; err != nil {
		tx.Rollback()
		// Audit log failure for purge is CRITICAL - must succeed
		log.Errorf("CRITICAL: Failed to create audit log for purge: %v", err)
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	// Delete dependencies in correct order
	// 1. Calendar events (if they exist)
	if err := tx.Exec("DELETE FROM calendar_events WHERE result_id IN (SELECT id FROM results WHERE campaign_id = ?)", campaignID).Error; err != nil {
		log.Errorf("Failed to delete calendar events for campaign %d: %v", campaignID, err)
		// Continue - calendar_events may not exist in all setups
	}

	// 2. Events
	if err := tx.Where("campaign_id = ?", campaignID).Delete(&Event{}).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to delete events for campaign %d: %v", campaignID, err)
		return err
	}

	// 3. Results
	if err := tx.Where("campaign_id = ?", campaignID).Delete(&Result{}).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to delete results for campaign %d: %v", campaignID, err)
		return err
	}

	// 4. Campaign-Group associations (many-to-many)
	if err := tx.Exec("DELETE FROM campaign_groups WHERE campaign_id = ?", campaignID).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to delete campaign_groups for campaign %d: %v", campaignID, err)
		return err
	}

	// 5. Campaign itself
	if err := tx.Delete(c).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to delete campaign %d: %v", campaignID, err)
		return err
	}

	log.Infof("Campaign %d (%s) PURGED by user %d", campaignID, c.Name, userID)
	return tx.Commit().Error
}

// checkNameConflict checks if campaign name conflicts with active campaigns
func checkNameConflict(tx *gorm.DB, name string, userID int64, excludeID int64) (bool, error) {
	var count int64
	err := tx.Model(&Campaign{}).
		Where("user_id = ? AND LOWER(name) = LOWER(?) AND deleted_at IS NULL AND id != ?",
			userID, name, excludeID).
		Count(&count).Error

	if err != nil {
		log.Errorf("Error checking name conflict: %v", err)
		return false, err
	}

	return count > 0, nil
}

// GetTrashedCampaigns retrieves deleted campaigns for a user
func GetTrashedCampaigns(userID int64) ([]Campaign, error) {
	campaigns := []Campaign{}
	// Use Unscoped() to include soft-deleted records
	err := db.Unscoped().Scopes(ScopeCampaignsTrashed).
		Where("user_id = ?", userID).
		Order("deleted_at DESC").
		Find(&campaigns).Error

	if err != nil {
		log.Errorf("Error retrieving trashed campaigns: %v", err)
		return nil, err
	}

	return campaigns, nil
}

// GetTrashedCampaignsPaginated retrieves deleted campaigns with pagination
func GetTrashedCampaignsPaginated(userID int64, offset, limit int) ([]Campaign, int64, error) {
	campaigns := []Campaign{}
	var total int64

	// Use Unscoped() to include soft-deleted records
	query := db.Unscoped().Scopes(ScopeCampaignsTrashed).
		Where("user_id = ?", userID)

	// Count total
	if err := query.Model(&Campaign{}).Count(&total).Error; err != nil {
		log.Errorf("Error counting trashed campaigns: %v", err)
		return nil, 0, err
	}

	// Get paginated results
	err := query.Offset(offset).
		Limit(limit).
		Order("deleted_at DESC").
		Find(&campaigns).Error

	if err != nil {
		log.Errorf("Error retrieving trashed campaigns: %v", err)
		return nil, 0, err
	}

	return campaigns, total, nil
}

// ListPurgeCandidates returns campaign IDs eligible for auto-purge
// cutoff: campaigns deleted before this time are eligible
// limit: maximum number of IDs to return (for batch processing)
func ListPurgeCandidates(cutoff time.Time, limit int) ([]int64, error) {
	if db == nil {
		return nil, errors.New("database not initialized")
	}

	var ids []int64

	err := db.Table("campaigns").
		Where("deleted_at IS NOT NULL AND deleted_at < ?", cutoff).
		Order("deleted_at ASC"). // Purge oldest first
		Limit(limit).
		Pluck("id", &ids).Error

	if err != nil {
		log.Errorf("Error listing purge candidates: %v", err)
		return nil, err
	}

	return ids, nil
}

// PurgeSystemCampaign is a system-level purge (bypasses user permission checks)
// Used by TTL job. Still requires campaign to be in trash.
func PurgeSystemCampaign(campaignID int64) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Lock campaign
	c := &Campaign{}
	if err := lockForUpdate(tx).First(c, campaignID).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Already purged - this is OK (idempotent)
			log.Warnf("Campaign %d not found during system purge (already purged?)", campaignID)
			return nil
		}
		return err
	}

	// Must be in trash (race condition check)
	if !c.IsDeleted() {
		tx.Rollback()
		log.Warnf("Campaign %d was restored before system purge, skipping", campaignID)
		return nil // No-op, campaign was restored
	}

	// Audit BEFORE delete
	audit := &AuditLog{
		ActorID:    nil, // System actor
		ActorName:  "system:trash-ttl",
		Action:     AuditCampaignPurged,
		EntityType: "campaign",
		EntityID:   campaignID,
	}
	audit.SetMetadata(map[string]interface{}{
		"name":       c.Name,
		"deleted_at": c.DeletedAt,
		"user_id":    c.UserId,
		"purge_type": "ttl_job",
	})

	if err := tx.Create(audit).Error; err != nil {
		tx.Rollback()
		log.Errorf("CRITICAL: Failed to create audit log for system purge: %v", err)
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	// Delete dependencies in correct order
	// 1. Calendar events (if they exist)
	if err := tx.Exec("DELETE FROM calendar_events WHERE result_id IN (SELECT id FROM results WHERE campaign_id = ?)", campaignID).Error; err != nil {
		log.Warnf("Failed to delete calendar events for campaign %d: %v", campaignID, err)
		// Continue - calendar_events may not exist in all setups
	}

	// 2. Events
	if err := tx.Where("campaign_id = ?", campaignID).Delete(&Event{}).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to delete events for campaign %d: %v", campaignID, err)
		return err
	}

	// 3. Results
	if err := tx.Where("campaign_id = ?", campaignID).Delete(&Result{}).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to delete results for campaign %d: %v", campaignID, err)
		return err
	}

	// 4. Campaign-Group associations (many-to-many)
	if err := tx.Exec("DELETE FROM campaign_groups WHERE campaign_id = ?", campaignID).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to delete campaign_groups for campaign %d: %v", campaignID, err)
		return err
	}

	// 5. Campaign itself
	if err := tx.Delete(c).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to delete campaign %d: %v", campaignID, err)
		return err
	}

	log.Infof("Campaign %d (%s) PURGED by system TTL job", campaignID, c.Name)
	return tx.Commit().Error
}
