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
	// The fields below carry the extra context the recipient type needs
	// (CL-102R addendum §4). Context is the human-readable parent (campaign name);
	// BatchID groups a bulk deletion so the UI can roll it up and restore/purge it
	// as one unit. ParentTrashed lets the UI DISABLE (never hide) "Restaurar" when
	// the recipient's campaign is itself in the Trash.
	Context       string `json:"context,omitempty"`
	BatchID       string `json:"batch_id,omitempty"`
	CampaignID    int64  `json:"campaign_id,omitempty"`
	CampaignName  string `json:"campaign_name,omitempty"`
	GroupID       *int64 `json:"group_id,omitempty"`
	GroupName     string `json:"group_name,omitempty"`
	Scope         string `json:"scope,omitempty"`
	DeletedByName string `json:"deleted_by_name,omitempty"`
	ParentTrashed bool   `json:"parent_campaign_trashed,omitempty"`
}

// TrashItemType constants — use these in switch statements throughout.
const (
	TrashTypeCampaign      = "campaign"
	TrashTypeCampaignGroup = "campaign_group"
	TrashTypeRecipient     = "recipient"
)

// ─── Aggregator ───────────────────────────────────────────────────────────────

// GetTrashItems returns trashed items for a user, optionally filtered by type,
// without pagination. Kept for callers that need the whole list.
// filterType: "all" | "campaign" | "campaign_group" | "recipient" (empty == "all").
// Results are ordered by deleted_at DESC.
func GetTrashItems(userID int64, filterType string) ([]TrashItem, error) {
	items, _, err := GetTrashItemsFiltered(userID, filterType, RecipientTrashQuery{})
	return items, err
}

// GetTrashItemsFiltered is GetTrashItems plus the recipient filters/pagination
// of addendum §4 (campaign_id, group_id, q, offset/limit). It also returns the
// TOTAL number of matching recipients — needed for the pager and the tab badge,
// which must reflect all matches, not just the current page. The filters and
// pagination apply to recipients only; campaigns and groups are never paginated
// (they were fine without it and adding it would change their contract).
func GetTrashItemsFiltered(userID int64, filterType string, rq RecipientTrashQuery) ([]TrashItem, int64, error) {
	if filterType == "" {
		filterType = "all"
	}

	// Reject unknown filter types up front
	if filterType != "all" &&
		filterType != TrashTypeCampaign &&
		filterType != TrashTypeCampaignGroup &&
		filterType != TrashTypeRecipient {
		return nil, 0, errors.New("unknown trash type: " + filterType)
	}

	items := []TrashItem{}

	// ── collectors ───────────────────────────────────────────────────────────

	if filterType == TrashTypeCampaign || filterType == "all" {
		campaigns, err := GetTrashedCampaigns(userID)
		if err != nil {
			log.Errorf("GetTrashItems: campaigns: %v", err)
			return nil, 0, err
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
			return nil, 0, err
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

	var recipientTotal int64
	if filterType == TrashTypeRecipient || filterType == "all" {
		recips, total, err := GetTrashedRecipients(userID, rq)
		if err != nil {
			log.Errorf("GetTrashItems: recipients: %v", err)
			return nil, 0, err
		}
		recipientTotal = total
		for _, rr := range recips {
			var at time.Time
			if rr.DeletedAt != nil {
				at = *rr.DeletedAt
			}
			items = append(items, TrashItem{
				Type:          TrashTypeRecipient,
				ID:            rr.ResultId,
				Name:          rr.Email,
				DeletedAt:     at,
				DeletedBy:     rr.DeletedBy,
				DeleteReason:  rr.Reason,
				Context:       rr.CampaignName,
				BatchID:       rr.BatchId,
				CampaignID:    rr.CampaignId,
				CampaignName:  rr.CampaignName,
				GroupID:       rr.GroupId,
				GroupName:     rr.GroupName,
				Scope:         rr.Scope,
				DeletedByName: rr.DeletedByName,
				ParentTrashed: rr.ParentCampaignTrashed,
			})
		}
	}

	// Sort by deleted_at DESC — most recently deleted first
	sort.Slice(items, func(i, j int) bool {
		return items[i].DeletedAt.After(items[j].DeletedAt)
	})

	return items, recipientTotal, nil
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

	case TrashTypeRecipient:
		// Recipients never get renamed on restore (no name-conflict rule); the
		// nested-trash and duplicate-email guards live in RestoreResultByID.
		return false, "", nil, RestoreResultByID(userID, itemID)

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
