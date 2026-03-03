package models

// Campaign Group Trash — entity-specific trash operations.
//
// Implements the Trashable interface on CampaignGroup and provides the
// public API (SoftDeleteCampaignGroup, RestoreCampaignGroup, PurgeCampaignGroup)
// by delegating to the generic functions in trash.go.

import (
	"errors"

	log "github.com/gophish/gophish/logger"
	"github.com/jinzhu/gorm"
)

// ─── Trashable interface implementation ──────────────────────────────────────

func (cg *CampaignGroup) GetID() int64         { return cg.Id }
func (cg *CampaignGroup) GetUserID() int64      { return cg.UserId }
func (cg *CampaignGroup) GetName() string       { return cg.Name }
func (cg *CampaignGroup) GetEntityType() string { return "campaign_group" }
func (cg *CampaignGroup) GetTableName() string  { return "campaign_groups" }

// PurgeDependencies deletes the campaign associations for a group before the
// group row itself is hard-deleted. Called by GenericPurge inside the same
// transaction.
func (cg *CampaignGroup) PurgeDependencies(tx *gorm.DB) error {
	if err := tx.Where("group_id = ?", cg.Id).Delete(&CampaignGroupCampaign{}).Error; err != nil {
		log.Errorf("Failed to delete campaign associations for group %d during purge: %v", cg.Id, err)
		return err
	}
	return nil
}

// ─── Scopes ──────────────────────────────────────────────────────────────────

// ScopeCampaignGroupsActive filters active (non-deleted) groups.
func ScopeCampaignGroupsActive(db *gorm.DB) *gorm.DB {
	return db.Where("deleted_at IS NULL")
}

// ScopeCampaignGroupsTrashed filters soft-deleted groups.
func ScopeCampaignGroupsTrashed(db *gorm.DB) *gorm.DB {
	return db.Where("deleted_at IS NOT NULL")
}

// ─── Internal helpers ────────────────────────────────────────────────────────

// getCampaignGroupUnscoped fetches a group including soft-deleted records,
// validating user ownership. Returns ErrCampaignGroupNotFound if not found.
func getCampaignGroupUnscoped(id int64, uid int64) (*CampaignGroup, error) {
	cg := &CampaignGroup{}
	err := db.Unscoped().Where("id = ? AND user_id = ?", id, uid).First(cg).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCampaignGroupNotFound
		}
		return nil, err
	}
	return cg, nil
}

// checkGroupNameConflict returns true if a non-deleted group with the given
// name (case-insensitive) already exists for the user, excluding excludeID.
func checkGroupNameConflict(name string, userID int64, excludeID int64) (bool, error) {
	var count int64
	err := db.Table("campaign_groups").
		Where("user_id = ? AND LOWER(name) = LOWER(?) AND deleted_at IS NULL AND id != ?",
			userID, name, excludeID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ─── Public trash operations ─────────────────────────────────────────────────

// SoftDeleteCampaignGroup moves a campaign group to the trash.
// The group's campaigns are NOT deleted — they remain active.
func SoftDeleteCampaignGroup(groupID int64, userID int64, reason string) error {
	// Fetch active group (validates ownership; deleted groups not found here)
	cg := &CampaignGroup{}
	if err := db.Where("id = ? AND user_id = ? AND deleted_at IS NULL", groupID, userID).
		First(cg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCampaignGroupNotFound
		}
		return err
	}

	return GenericSoftDelete(cg, userID, reason, AuditGroupSoftDeleted)
}

// RestoreCampaignGroup restores a campaign group from the trash.
// If a name conflict exists with an active group, the name is adjusted.
func RestoreCampaignGroup(groupID int64, userID int64) (*TrashRestoreResult, error) {
	// Fetch including soft-deleted records
	cg, err := getCampaignGroupUnscoped(groupID, userID)
	if err != nil {
		return nil, err
	}
	if !cg.IsInTrash() {
		return nil, ErrNotInTrash
	}

	// Build conflict checker scoped to this user/group
	conflictCheck := func(name string) (bool, error) {
		return checkGroupNameConflict(name, userID, groupID)
	}

	return GenericRestore(cg, userID, AuditGroupRestored, conflictCheck)
}

// PurgeCampaignGroup permanently deletes a campaign group from the trash.
// The group must be in the trash first.
// All campaign associations are deleted; the campaigns themselves are preserved.
func PurgeCampaignGroup(groupID int64, userID int64) error {
	cg, err := getCampaignGroupUnscoped(groupID, userID)
	if err != nil {
		return err
	}
	if !cg.IsInTrash() {
		return ErrNotInTrash
	}

	return GenericPurge(cg, userID, AuditGroupPurged)
}

// ─── Query helpers ────────────────────────────────────────────────────────────

// GetTrashedCampaignGroups returns all soft-deleted campaign groups for a user.
func GetTrashedCampaignGroups(userID int64) ([]CampaignGroup, error) {
	groups := []CampaignGroup{}
	err := db.Unscoped().
		Scopes(ScopeCampaignGroupsTrashed).
		Where("user_id = ?", userID).
		Order("deleted_at DESC").
		Find(&groups).Error
	if err != nil {
		log.Errorf("Error retrieving trashed campaign groups: %v", err)
		return nil, err
	}
	return groups, nil
}
