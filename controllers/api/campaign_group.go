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

// CampaignGroups handles requests for campaign groups
// GET: Returns list of campaign groups
// POST: Creates a new campaign group
func (as *Server) CampaignGroups(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		cgs, err := models.GetCampaignGroups(ctx.Get(r, "user_id").(int64))
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, cgs, http.StatusOK)
	case r.Method == "POST":
		cg := models.CampaignGroup{}
		err := json.NewDecoder(r.Body).Decode(&cg)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		err = models.PostCampaignGroup(&cg, ctx.Get(r, "user_id").(int64))
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		JSONResponse(w, cg, http.StatusCreated)
	}
}

// CampaignGroupsSummary returns summary information for campaign groups
func (as *Server) CampaignGroupsSummary(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		summaries, err := models.GetCampaignGroupSummaries(ctx.Get(r, "user_id").(int64))
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, summaries, http.StatusOK)
	}
}

// CampaignGroup handles requests for a specific campaign group
// GET: Returns campaign group details with campaigns
// PUT: Updates campaign group
// DELETE: Deletes campaign group
func (as *Server) CampaignGroup(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	
	switch {
	case r.Method == "GET":
		cg, err := models.GetCampaignGroup(id, ctx.Get(r, "user_id").(int64))
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: "Campaign group not found"}, http.StatusNotFound)
			return
		}
		JSONResponse(w, cg, http.StatusOK)
	case r.Method == "PUT":
		cg := models.CampaignGroup{}
		err := json.NewDecoder(r.Body).Decode(&cg)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		cg.Id = id
		err = models.PutCampaignGroup(&cg, ctx.Get(r, "user_id").(int64))
		if err != nil {
			if err == models.ErrCampaignGroupNotFound {
				JSONResponse(w, models.Response{Success: false, Message: "Campaign group not found"}, http.StatusNotFound)
				return
			}
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		JSONResponse(w, cg, http.StatusOK)
	case r.Method == "DELETE":
		var req struct {
			Reason string `json:"reason"`
		}
		// Reason is optional — ignore decode errors
		json.NewDecoder(r.Body).Decode(&req)

		err := models.SoftDeleteCampaignGroup(id, ctx.Get(r, "user_id").(int64), req.Reason)
		if err != nil {
			if err == models.ErrCampaignGroupNotFound {
				JSONResponse(w, models.Response{Success: false, Message: "Campaign group not found"}, http.StatusNotFound)
				return
			}
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: "Error moving campaign group to trash"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Campaign group moved to trash"}, http.StatusOK)
	}
}

// CampaignGroupStats returns aggregated telemetry for a campaign group
func (as *Server) CampaignGroupStats(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	
	switch {
	case r.Method == "GET":
		stats, err := models.GetCampaignGroupStats(id, ctx.Get(r, "user_id").(int64))
		if err != nil {
			log.Error(err)
			if err == models.ErrCampaignGroupNotFound {
				JSONResponse(w, models.Response{Success: false, Message: "Campaign group not found"}, http.StatusNotFound)
				return
			}
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, stats, http.StatusOK)
	}
}

// CampaignGroupTrashList returns all trashed campaign groups for the current user
func (as *Server) CampaignGroupTrashList(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		JSONResponse(w, models.Response{Success: false, Message: "Method Not Allowed"}, http.StatusMethodNotAllowed)
		return
	}
	uid := ctx.Get(r, "user_id").(int64)
	groups, err := models.GetTrashedCampaignGroups(uid)
	if err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, map[string]interface{}{"groups": groups}, http.StatusOK)
}

// CampaignGroupRestore restores a campaign group from the trash
func (as *Server) CampaignGroupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		JSONResponse(w, models.Response{Success: false, Message: "Method Not Allowed"}, http.StatusMethodNotAllowed)
		return
	}
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	uid := ctx.Get(r, "user_id").(int64)

	result, err := models.RestoreCampaignGroup(id, uid)
	if err != nil {
		log.Error(err)
		switch err {
		case models.ErrCampaignGroupNotFound:
			JSONResponse(w, models.Response{Success: false, Message: "Campaign group not found"}, http.StatusNotFound)
		case models.ErrNotInTrash:
			JSONResponse(w, models.Response{Success: false, Message: "Campaign group is not in trash"}, http.StatusBadRequest)
		default:
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
		}
		return
	}
	JSONResponse(w, map[string]interface{}{
		"success":      result.Success,
		"name_changed": result.NameChanged,
		"old_name":     result.OldName,
		"new_name":     result.NewName,
		"warnings":     result.Warnings,
	}, http.StatusOK)
}

// CampaignGroupPurge permanently deletes a campaign group from the trash
func (as *Server) CampaignGroupPurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		JSONResponse(w, models.Response{Success: false, Message: "Method Not Allowed"}, http.StatusMethodNotAllowed)
		return
	}
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	uid := ctx.Get(r, "user_id").(int64)

	if err := models.PurgeCampaignGroup(id, uid); err != nil {
		log.Error(err)
		switch err {
		case models.ErrCampaignGroupNotFound:
			JSONResponse(w, models.Response{Success: false, Message: "Campaign group not found"}, http.StatusNotFound)
		case models.ErrNotInTrash:
			JSONResponse(w, models.Response{Success: false, Message: "Campaign group is not in trash"}, http.StatusBadRequest)
		default:
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
		}
		return
	}
	JSONResponse(w, models.Response{Success: true, Message: "Campaign group permanently deleted"}, http.StatusOK)
}

// CampaignGroupArchive archives or unarchives a campaign group
func (as *Server) CampaignGroupArchive(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	
	switch {
	case r.Method == "POST":
		var req struct {
			Archived bool `json:"archived"`
		}
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		
		// Get the campaign group
		cg, err := models.GetCampaignGroup(id, ctx.Get(r, "user_id").(int64))
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: "Campaign group not found"}, http.StatusNotFound)
			return
		}
		
		// Update archived status
		cg.Archived = req.Archived
		err = models.PutCampaignGroup(&cg, ctx.Get(r, "user_id").(int64))
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		
		message := "Campaign group archived successfully"
		if !req.Archived {
			message = "Campaign group unarchived successfully"
		}
		JSONResponse(w, models.Response{Success: true, Message: message}, http.StatusOK)
	}
}
