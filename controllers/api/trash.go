package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	ctx "github.com/gophish/gophish/context"
	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/models"
	"github.com/gorilla/mux"
)

// GlobalTrash lists all (or filtered) trashed items for the current user.
//
//	GET /api/trash?type=all|campaign|campaign_group
func (as *Server) GlobalTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		JSONResponse(w, models.Response{Success: false, Message: "Method Not Allowed"}, http.StatusMethodNotAllowed)
		return
	}

	filterType := r.URL.Query().Get("type")
	if filterType == "" {
		filterType = "all"
	}

	userID := ctx.Get(r, "user_id").(int64)
	items, err := models.GetTrashItems(userID, filterType)
	if err != nil {
		log.Errorf("GlobalTrash: %v", err)
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
		return
	}

	JSONResponse(w, map[string]interface{}{"items": items}, http.StatusOK)
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

	if itemType == models.TrashTypeCampaign {
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
		if req.Confirmation != campaign.Name {
			JSONResponse(w, models.Response{Success: false, Message: "Confirmation does not match"}, http.StatusBadRequest)
			return
		}
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
