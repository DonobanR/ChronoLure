package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	ctx "github.com/gophish/gophish/context"
	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/models"
	"github.com/gorilla/mux"
)

// normalizeConfirmation makes a campaign/group name typeable for the purge
// confirmation gate. Names imported from spreadsheets carry characters that
// render invisibly and can never be reproduced by the user:
//   - trailing/leading and double spaces (e.g. "Toledano ", "... -  TESTING"),
//     which the browser collapses when it renders the modal;
//   - zero-width and formatting characters (BOM, zero-width space/joiners, bidi
//     marks, soft hyphen) that survive plain whitespace normalization.
// It strips the invisible formatting chars, then collapses whitespace runs to a
// single space and trims. Client and backend normalize identically so the gate
// stays a UX safeguard, not a blocker. Purge is still scoped by id + ownership +
// in-trash, so relaxing the exact-string match here has no security impact.
func normalizeConfirmation(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == 0xFEFF, // BOM / zero-width no-break space
			r == 0x00AD,                  // soft hyphen
			r == 0x2060,                  // word joiner
			r >= 0x200B && r <= 0x200F,   // zero-width space/joiners, bidi marks
			r >= 0x202A && r <= 0x202E:   // bidi embedding/override
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// trashedGroupName returns the name of a soft-deleted campaign group owned by
// the user, for the purge confirmation gate. There is no single-group trashed
// getter in models, so we resolve it from the user's trashed group list.
// Returns models.ErrCampaignGroupNotFound if the group is not in the user's trash.
func trashedGroupName(userID, groupID int64) (string, error) {
	items, err := models.GetTrashItems(userID, models.TrashTypeCampaignGroup)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.ID == groupID {
			return it.Name, nil
		}
	}
	return "", models.ErrCampaignGroupNotFound
}

// GlobalTrash lists all (or filtered) trashed items for the current user.
//
//	GET /api/trash?type=all|campaign|campaign_group
func (as *Server) GlobalTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		JSONResponse(w, models.Response{Success: false, Message: "Method Not Allowed"}, http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	filterType := q.Get("type")
	if filterType == "" {
		filterType = "all"
	}

	// Recipient filters + pagination (addendum §4). With 40+ trashed recipients a
	// pager is required; campaigns/groups never needed one and keep their contract.
	rq := models.RecipientTrashQuery{Q: q.Get("q")}
	rq.CampaignID, _ = strconv.ParseInt(q.Get("campaign_id"), 10, 64)
	rq.GroupID, _ = strconv.ParseInt(q.Get("group_id"), 10, 64)
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage > 0 {
		if page < 1 {
			page = 1
		}
		if perPage > 200 {
			perPage = 200
		}
		rq.Limit = perPage
		rq.Offset = (page - 1) * perPage
	}

	userID := ctx.Get(r, "user_id").(int64)

	// group_by=batch rolls recipients up by deletion event and paginates over
	// BATCHES (addendum §2) — this is what the "All" tab renders so 40 deletions
	// don't bury the campaigns.
	if q.Get("group_by") == "batch" {
		batches, total, err := models.GetTrashedRecipientBatches(userID, rq)
		if err != nil {
			log.Errorf("GlobalTrash batches: %v", err)
			JSONResponse(w, models.Response{Success: false, Message: "No se pudo cargar la papelera."}, http.StatusInternalServerError)
			return
		}
		resp := map[string]interface{}{"batches": batches, "total": total}
		if rq.Limit > 0 {
			resp["page"] = page
			resp["per_page"] = perPage
		}
		JSONResponse(w, resp, http.StatusOK)
		return
	}

	items, recipientTotal, err := models.GetTrashItemsFiltered(userID, filterType, rq)
	if err != nil {
		log.Errorf("GlobalTrash: %v", err)
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
		return
	}

	resp := map[string]interface{}{"items": items}
	// recipient_total is the count of ALL matching recipients (not just this page),
	// so the tab badge and the pager stay truthful.
	if filterType == models.TrashTypeRecipient || filterType == "all" {
		resp["recipient_total"] = recipientTotal
	}
	if rq.Limit > 0 {
		resp["page"] = page
		resp["per_page"] = perPage
		resp["total"] = recipientTotal
	}
	JSONResponse(w, resp, http.StatusOK)
}

// TrashCounts returns the UNFILTERED per-type totals for the tab badges in one
// call (addendum §7), so the UI does not fire four requests. Note `all` counts
// recipient BATCHES (not rows), matching what the "All" tab renders.
//
//	GET /api/trash/counts
func (as *Server) TrashCounts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	counts, err := models.GetTrashCounts(ctx.Get(r, "user_id").(int64))
	if err != nil {
		log.Errorf("TrashCounts: %v", err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudieron cargar los contadores de la papelera."}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, counts, http.StatusOK)
}

// RecipientBatchDetail returns every trashed recipient of one deletion batch —
// the expanded detail behind a rolled-up row (addendum §2/§3).
//
//	GET /api/trash/recipient/batch/{batch_id}
func (as *Server) RecipientBatchDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	batchID := mux.Vars(r)["batch_id"]
	items, err := models.GetTrashedRecipientsByBatch(ctx.Get(r, "user_id").(int64), batchID)
	if err != nil {
		if err == models.ErrResultNotFound {
			JSONResponse(w, models.Response{Success: false, Message: "No se encontró ese grupo de destinatarios en la papelera."}, http.StatusNotFound)
			return
		}
		log.Errorf("RecipientBatchDetail: %v", err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo cargar el detalle."}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, map[string]interface{}{"items": items, "total": len(items)}, http.StatusOK)
}

// GlobalTrashRestore restores a single item from the global trash.
//
//	POST /api/trash/{type}/{id}/restore
func (as *Server) GlobalTrashRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		JSONResponse(w, models.Response{Success: false, Message: "Method Not Allowed"}, http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r)
	itemType := vars["type"]
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid ID"}, http.StatusBadRequest)
		return
	}

	userID := ctx.Get(r, "user_id").(int64)
	nameChanged, newName, warnings, err := models.RestoreTrashItem(userID, itemType, id)
	if err != nil {
		log.Errorf("GlobalTrashRestore %s/%d: %v", itemType, id, err)
		status := http.StatusInternalServerError
		if err == models.ErrCampaignNotFound || err == models.ErrCampaignGroupNotFound {
			status = http.StatusNotFound
		} else if err == models.ErrNotDeleted || err == models.ErrNotInTrash {
			status = http.StatusBadRequest
		} else if err == models.ErrPermissionDenied {
			status = http.StatusForbidden
		}
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, status)
		return
	}

	JSONResponse(w, map[string]interface{}{
		"success":      true,
		"name_changed": nameChanged,
		"new_name":     newName,
		"warnings":     warnings,
	}, http.StatusOK)
}

// GlobalTrashPurge permanently deletes a single item from the global trash.
//
//	DELETE /api/trash/{type}/{id}/purge
func (as *Server) GlobalTrashPurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		JSONResponse(w, models.Response{Success: false, Message: "Method Not Allowed"}, http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r)
	itemType := vars["type"]
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid ID"}, http.StatusBadRequest)
		return
	}

	userID := ctx.Get(r, "user_id").(int64)

	if itemType != models.TrashTypeCampaign && itemType != models.TrashTypeCampaignGroup {
		JSONResponse(w, models.Response{Success: false, Message: "unknown trash type: " + itemType}, http.StatusBadRequest)
		return
	}

	// Determine admin status (required for campaign purge permission check)
	user, err := models.GetUser(userID)
	if err != nil {
		log.Errorf("GlobalTrashPurge: GetUser %d: %v", userID, err)
		JSONResponse(w, models.Response{Success: false, Message: "Error verifying permissions"}, http.StatusInternalServerError)
		return
	}
	isAdmin := user.Role.Slug == "admin"

	// Confirmation gate is required for BOTH campaigns and campaign groups so a
	// purge cannot be bypassed via the API. The client sends the typed name; we
	// compare it (whitespace/invisible-char normalized) against the stored name.
	var req struct {
		Confirmation string `json:"confirmation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid request body"}, http.StatusBadRequest)
		return
	}
	if req.Confirmation == "" {
		JSONResponse(w, models.Response{Success: false, Message: "Confirmation is required"}, http.StatusBadRequest)
		return
	}

	var expectedName string
	if itemType == models.TrashTypeCampaign {
		campaign, err := models.GetTrashedCampaignByID(id, userID)
		if err != nil {
			if err == models.ErrCampaignNotFound {
				if _, activeErr := models.GetCampaign(id, userID); activeErr == nil {
					JSONResponse(w, models.Response{Success: false, Message: "Campaign is not in trash"}, http.StatusBadRequest)
					return
				}
				JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
				return
			}
			log.Errorf("GlobalTrashPurge: GetTrashedCampaignByID %d: %v", id, err)
			JSONResponse(w, models.Response{Success: false, Message: "Error validating campaign"}, http.StatusInternalServerError)
			return
		}
		expectedName = campaign.Name
	} else { // campaign_group
		name, err := trashedGroupName(userID, id)
		if err != nil {
			if err == models.ErrCampaignGroupNotFound {
				JSONResponse(w, models.Response{Success: false, Message: "Campaign group not found"}, http.StatusNotFound)
				return
			}
			log.Errorf("GlobalTrashPurge: trashedGroupName %d: %v", id, err)
			JSONResponse(w, models.Response{Success: false, Message: "Error validating campaign group"}, http.StatusInternalServerError)
			return
		}
		expectedName = name
	}

	if normalizeConfirmation(req.Confirmation) != normalizeConfirmation(expectedName) {
		JSONResponse(w, models.Response{Success: false, Message: "Confirmation does not match"}, http.StatusBadRequest)
		return
	}

	if err := models.PurgeTrashItem(userID, isAdmin, itemType, id); err != nil {
		log.Errorf("GlobalTrashPurge %s/%d: %v", itemType, id, err)
		status := http.StatusInternalServerError
		switch err {
		case models.ErrCampaignNotFound, models.ErrCampaignGroupNotFound:
			status = http.StatusNotFound
		case models.ErrNotInTrash, models.ErrNotDeleted:
			status = http.StatusBadRequest
		}
		if err.Error() == "purge requires admin privileges" {
			status = http.StatusForbidden
		} else if err.Error() == "can only purge campaigns in trash" {
			status = http.StatusBadRequest
		}
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, status)
		return
	}

	JSONResponse(w, models.Response{Success: true, Message: "Item permanently deleted"}, http.StatusOK)
}
