package models

// Global Trash Aggregator
//
// This file provides the single-source-of-truth for the unified trash UI.
// It collects soft-deleted items from every registered model and presents
// them as a flat []TrashItem list.
//
// ─── How to add a new model to the global trash ────────────────────────────
//
//  1. Make sure the model has soft-delete support (embed TrashableModel or
//     add the five columns manually).
//
//  2. Add a GetTrashedXxx(userID int64) ([]Xxx, error) function.
//
//  3. Add a constant to TrashItemType section below.
//
//  4. In GetTrashItems, add a new branch under // ── collectors ──.
//
//  5. In RestoreTrashItem and PurgeTrashItem, add the matching switch cases.
//
// That's it.  No template or JS changes are required — the unified trash
// page renders the Type badge automatically.

import (
	"errors"
	"sort"
	"time"

	log "github.com/gophish/gophish/logger"
)

// ─── Types ────────────────────────────────────────────────────────────────────

// TrashItem is the unified representation of any soft-deleted entity.
// All fields are safe to send to the browser.
type TrashItem struct {
	Type         string    `json:"type"`
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	DeletedAt    time.Time `json:"deleted_at"`
	DeletedBy    *int64    `json:"deleted_by,omitempty"`
	DeleteReason string    `json:"delete_reason,omitempty"`
}

// TrashItemType constants — use these in switch statements throughout.
const (
	TrashTypeCampaign      = "campaign"
	TrashTypeCampaignGroup = "campaign_group"
)

// ─── Aggregator ───────────────────────────────────────────────────────────────

// GetTrashItems returns trashed items for a user, optionally filtered by type.
// filterType: "all" | "campaign" | "campaign_group" (empty == "all").
// Results are ordered by deleted_at DESC.
func GetTrashItems(userID int64, filterType string) ([]TrashItem, error) {
	if filterType == "" {
		filterType = "all"
	}

	// Reject unknown filter types up front
	if filterType != "all" &&
		filterType != TrashTypeCampaign &&
		filterType != TrashTypeCampaignGroup {
		return nil, errors.New("unknown trash type: " + filterType)
	}

	items := []TrashItem{}

	// ── collectors ───────────────────────────────────────────────────────────

	if filterType == TrashTypeCampaign || filterType == "all" {
		campaigns, err := GetTrashedCampaigns(userID)
		if err != nil {
			log.Errorf("GetTrashItems: campaigns: %v", err)
			return nil, err
		}
		for _, c := range campaigns {
			var at time.Time
			if c.DeletedAt != nil {
				at = *c.DeletedAt
			}
			items = append(items, TrashItem{
				Type:         TrashTypeCampaign,
				ID:           c.Id,
				Name:         c.Name,
				DeletedAt:    at,
				DeletedBy:    c.DeletedBy,
				DeleteReason: c.DeleteReason,
			})
		}
	}

	if filterType == TrashTypeCampaignGroup || filterType == "all" {
		groups, err := GetTrashedCampaignGroups(userID)
		if err != nil {
			log.Errorf("GetTrashItems: campaign_groups: %v", err)
			return nil, err
		}
		for _, g := range groups {
			var at time.Time
			if g.DeletedAt != nil {
				at = *g.DeletedAt
			}
			items = append(items, TrashItem{
				Type:         TrashTypeCampaignGroup,
				ID:           g.Id,
				Name:         g.Name,
				DeletedAt:    at,
				DeletedBy:    g.DeletedBy,
				DeleteReason: g.DeleteReason,
			})
		}
	}

	// Sort by deleted_at DESC — most recently deleted first
	sort.Slice(items, func(i, j int) bool {
		return items[i].DeletedAt.After(items[j].DeletedAt)
	})

	return items, nil
}

// ─── Dispatcher: Restore ──────────────────────────────────────────────────────

// RestoreTrashItem dispatches a restore to the correct model handler.
// Returns: nameChanged, newName, warnings, error.
func RestoreTrashItem(userID int64, itemType string, itemID int64) (bool, string, []string, error) {
	switch itemType {

	case TrashTypeCampaign:
		result, err := RestoreCampaign(itemID, userID)
		if err != nil {
			return false, "", nil, err
		}
		return result.NameChanged, result.NewName, result.Warnings, nil

	case TrashTypeCampaignGroup:
		result, err := RestoreCampaignGroup(itemID, userID)
		if err != nil {
			return false, "", nil, err
		}
		return result.NameChanged, result.NewName, result.Warnings, nil

	default:
		return false, "", nil, errors.New("unknown trash type: " + itemType)
	}
}

// ─── Dispatcher: Purge ────────────────────────────────────────────────────────

// PurgeTrashItem dispatches a permanent purge to the correct model handler.
// isAdmin is forwarded because campaign purges require admin privileges.
func PurgeTrashItem(userID int64, isAdmin bool, itemType string, itemID int64) error {
	switch itemType {

	case TrashTypeCampaign:
		// Campaign purge enforces admin via the isAdmin flag
		return PurgeCampaign(itemID, userID, isAdmin)

	case TrashTypeCampaignGroup:
		// Group purge is allowed for any owner — no admin required
		return PurgeCampaignGroup(itemID, userID)

	default:
		return errors.New("unknown trash type: " + itemType)
	}
}
