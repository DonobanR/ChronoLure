package models

// Generic Trash System
//
// This file provides a reusable soft-delete (trash) mechanism.
// To add trash support to any new model:
//
//   1. Embed TrashableModel in your struct:
//        type MyModel struct {
//            Id   int64
//            Name string
//            TrashableModel
//        }
//
//   2. Implement all Trashable interface methods on your struct.
//
//   3. Add a DB migration to add the trash columns to your table.
//
//   4. Create entity-specific wrapper functions (like SoftDeleteCampaignGroup)
//      that fetch/validate and then call GenericSoftDelete/GenericRestore/GenericPurge.
//
// The generic functions use raw SQL via db.Exec to avoid GORM v1 zero-value
// skipping issues (particularly for clearing deleted_at = NULL on restore).

import (
	"errors"
	"fmt"
	"time"

	log "github.com/gophish/gophish/logger"
	"github.com/jinzhu/gorm"
)

// Generic trash errors
var (
	ErrAlreadyInTrash  = errors.New("entity is already in trash")
	ErrNotInTrash      = errors.New("entity is not in trash")
	ErrTrashPermission = errors.New("permission denied")
)

// TrashableModel is an embeddable struct that adds soft-delete fields to any model.
// When embedded, GORM v1 will automatically add WHERE deleted_at IS NULL to all
// normal queries, and the entity supports Unscoped() to query trash records.
type TrashableModel struct {
	DeletedAt    *time.Time `json:"deleted_at,omitempty" gorm:"index"`
	DeletedBy    *int64     `json:"deleted_by,omitempty"`
	DeleteReason string     `json:"delete_reason,omitempty"`
	RestoredAt   *time.Time `json:"restored_at,omitempty"`
	RestoredBy   *int64     `json:"restored_by,omitempty"`
}

// IsInTrash returns true if the record has been soft-deleted.
func (t *TrashableModel) IsInTrash() bool {
	return t.DeletedAt != nil
}

// Trashable is the interface any entity must implement to use the generic trash system.
type Trashable interface {
	// Identity
	GetID() int64
	GetUserID() int64
	GetName() string

	// Metadata used for audit log entries and raw SQL routing
	GetEntityType() string // e.g. "campaign_group"
	GetTableName() string  // e.g. "campaign_groups"

	// State — provided for free by embedding TrashableModel
	IsInTrash() bool

	// PurgeDependencies is called inside the purge transaction BEFORE the row is deleted.
	// Implementations must delete all child records in the same transaction.
	PurgeDependencies(tx *gorm.DB) error
}

// TrashRestoreResult contains the outcome of a restore operation.
type TrashRestoreResult struct {
	Success     bool
	NameChanged bool
	OldName     string
	NewName     string
	Warnings    []string
}

// GenericSoftDelete moves an entity to the trash.
//
// Prerequisites (caller must guarantee):
//   - entity was fetched from DB with current state
//   - ownership was validated (entity.GetUserID() == callerUserID)
//
// Uses raw SQL to avoid GORM v1 zero-value filtering.
func GenericSoftDelete(entity Trashable, userID int64, reason string, auditAction string) error {
	// Idempotency: already deleted → success (no-op)
	if entity.IsInTrash() {
		log.Warnf("%s %d already in trash, idempotent response", entity.GetEntityType(), entity.GetID())
		return nil
	}

	now := time.Now().UTC()

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Raw SQL: set all three trash fields atomically
	if err := tx.Exec(
		"UPDATE "+entity.GetTableName()+" SET deleted_at=?, deleted_by=?, delete_reason=? WHERE id=?",
		now, userID, reason, entity.GetID(),
	).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to soft delete %s %d: %v", entity.GetEntityType(), entity.GetID(), err)
		return err
	}

	// Audit log (non-blocking: log failure but don't roll back)
	audit := userAuditLog(tx, userID, auditAction, entity.GetEntityType(), entity.GetID())
	audit.SetMetadata(map[string]interface{}{
		"name":   entity.GetName(),
		"reason": reason,
	})
	if err := tx.Create(audit).Error; err != nil {
		log.Errorf("Failed to create audit log (non-blocking): %v", err)
	}

	log.Infof("%s %d (%s) soft deleted by user %d", entity.GetEntityType(), entity.GetID(), entity.GetName(), userID)
	return tx.Commit().Error
}

// GenericRestore restores an entity from the trash.
//
// Prerequisites (caller must guarantee):
//   - entity was fetched using Unscoped() so soft-deleted rows are visible
//   - ownership was validated
//
// nameConflictCheck: optional function returning (hasConflict, error).
// Pass nil to skip name-conflict handling.
func GenericRestore(entity Trashable, userID int64, auditAction string, nameConflictCheck func(name string) (bool, error)) (*TrashRestoreResult, error) {
	result := &TrashRestoreResult{Warnings: []string{}}

	if !entity.IsInTrash() {
		return nil, ErrNotInTrash
	}

	newName := entity.GetName()
	originalName := entity.GetName()

	// Resolve name conflicts BEFORE opening the transaction.
	// With SQLite3 MaxOpenConns(1) the open write-transaction holds the only
	// connection; running a SELECT on the global db inside the tx would block
	// forever waiting for a second connection that never becomes available.
	if nameConflictCheck != nil {
		hasConflict, err := nameConflictCheck(newName)
		if err != nil {
			return nil, err
		}
		if hasConflict {
			newName = fmt.Sprintf("%s (Restaurado %s)", originalName, time.Now().Format("2006-01-02 15:04"))
			// Try up to 9 additional suffixes
			for i := 1; i < 10; i++ {
				conflict, err2 := nameConflictCheck(newName)
				if err2 != nil {
					return nil, err2
				}
				if !conflict {
					break
				}
				newName = fmt.Sprintf("%s (Restaurado %s-%d)", originalName, time.Now().Format("2006-01-02"), i)
			}
			result.NameChanged = true
			result.OldName = originalName
			result.NewName = newName
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"Nombre cambiado de '%s' a '%s' por conflicto", originalName, newName))
			log.Infof("%s %d renamed on restore: %s → %s",
				entity.GetEntityType(), entity.GetID(), originalName, newName)
		}
	}

	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	now := time.Now().UTC()

	// Raw SQL: clear delete fields, set restore metadata.
	// GORM v1 skips nil pointer updates, so we must use Exec.
	if err := tx.Exec(
		"UPDATE "+entity.GetTableName()+
			" SET deleted_at=NULL, deleted_by=NULL, delete_reason='', restored_at=?, restored_by=? WHERE id=?",
		now, userID, entity.GetID(),
	).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to restore %s %d: %v", entity.GetEntityType(), entity.GetID(), err)
		return nil, err
	}

	// Apply renamed name if needed
	if result.NameChanged {
		if err := tx.Exec(
			"UPDATE "+entity.GetTableName()+" SET name=? WHERE id=?",
			newName, entity.GetID(),
		).Error; err != nil {
			tx.Rollback()
			log.Errorf("Failed to rename %s %d on restore: %v", entity.GetEntityType(), entity.GetID(), err)
			return nil, err
		}
	}

	// Audit log
	audit := userAuditLog(tx, userID, auditAction, entity.GetEntityType(), entity.GetID())
	audit.SetMetadata(map[string]interface{}{
		"name":          newName,
		"original_name": originalName,
		"name_changed":  result.NameChanged,
		"warnings":      result.Warnings,
	})
	if err := tx.Create(audit).Error; err != nil {
		log.Errorf("Failed to create audit log (non-blocking): %v", err)
	}

	result.Success = true
	log.Infof("%s %d (%s) restored by user %d", entity.GetEntityType(), entity.GetID(), entity.GetName(), userID)
	return result, tx.Commit().Error
}

// GenericPurge permanently deletes an entity from the trash.
//
// Prerequisites (caller must guarantee):
//   - entity was fetched using Unscoped()
//   - entity.IsInTrash() == true
//
// The audit log is written BEFORE the hard delete so it survives even when
// the row is gone.
func GenericPurge(entity Trashable, userID int64, auditAction string) error {
	if !entity.IsInTrash() {
		return errors.New("can only purge entities that are in trash")
	}

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Audit BEFORE delete — critical, must persist even after row is gone
	audit := userAuditLog(tx, userID, auditAction, entity.GetEntityType(), entity.GetID())
	audit.SetMetadata(map[string]interface{}{
		"name":    entity.GetName(),
		"user_id": entity.GetUserID(),
	})
	if err := tx.Create(audit).Error; err != nil {
		tx.Rollback()
		log.Errorf("CRITICAL: Failed to create audit log before purge of %s %d: %v",
			entity.GetEntityType(), entity.GetID(), err)
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	// Entity-specific dependency cleanup
	if err := entity.PurgeDependencies(tx); err != nil {
		tx.Rollback()
		log.Errorf("Failed to purge dependencies for %s %d: %v",
			entity.GetEntityType(), entity.GetID(), err)
		return err
	}

	// Hard delete
	if err := tx.Exec(
		"DELETE FROM "+entity.GetTableName()+" WHERE id=?", entity.GetID(),
	).Error; err != nil {
		tx.Rollback()
		log.Errorf("Failed to hard delete %s %d: %v", entity.GetEntityType(), entity.GetID(), err)
		return err
	}

	log.Infof("%s %d (%s) PURGED by user %d",
		entity.GetEntityType(), entity.GetID(), entity.GetName(), userID)
	return tx.Commit().Error
}
