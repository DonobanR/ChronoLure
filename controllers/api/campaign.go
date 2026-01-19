package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	ctx "github.com/gophish/gophish/context"
	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/models"
	"github.com/gorilla/mux"
	"github.com/jinzhu/gorm"
)

// Campaigns returns a list of campaigns if requested via GET.
// If requested via POST, APICampaigns creates a new campaign and returns a reference to it.
func (as *Server) Campaigns(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		cs, err := models.GetCampaigns(ctx.Get(r, "user_id").(int64))
		if err != nil {
			log.Error(err)
		}
		JSONResponse(w, cs, http.StatusOK)
	//POST: Create a new campaign and return it as JSON
	case r.Method == "POST":
		c := models.Campaign{}
		// Put the request into a campaign
		err := json.NewDecoder(r.Body).Decode(&c)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		err = models.PostCampaign(&c, ctx.Get(r, "user_id").(int64))
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		// If the campaign is scheduled to launch immediately, send it to the worker.
		// Otherwise, the worker will pick it up at the scheduled time
		if c.Status == models.CampaignInProgress {
			go as.worker.LaunchCampaign(c)
		}
		JSONResponse(w, c, http.StatusCreated)
	}
}

// CampaignsSummary returns the summary for the current user's campaigns
func (as *Server) CampaignsSummary(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		cs, err := models.GetCampaignSummaries(ctx.Get(r, "user_id").(int64))
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, cs, http.StatusOK)
	}
}

// Campaign returns details about the requested campaign. If the campaign is not
// valid, APICampaign returns null.
func (as *Server) Campaign(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	c, err := models.GetCampaign(id, ctx.Get(r, "user_id").(int64))
	if err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
		return
	}
	switch {
	case r.Method == "GET":
		JSONResponse(w, c, http.StatusOK)
	case r.Method == "DELETE":
		// Soft delete - move to trash
		// Read optional reason from body
		var req struct {
			Reason string `json:"reason"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		err = models.SoftDeleteCampaign(id, ctx.Get(r, "user_id").(int64), req.Reason)
		if err != nil {
			if err == models.ErrCampaignNotFound {
				JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
				return
			}
			if err == models.ErrPermissionDenied {
				JSONResponse(w, models.Response{Success: false, Message: "Permission denied"}, http.StatusForbidden)
				return
			}
			log.Errorf("Error soft deleting campaign %d: %v", id, err)
			JSONResponse(w, models.Response{Success: false, Message: "Error moving campaign to trash"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Campaign moved to trash"}, http.StatusOK)
	}
}

// CampaignResults returns just the results for a given campaign to
// significantly reduce the information returned.
func (as *Server) CampaignResults(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	cr, err := models.GetCampaignResults(id, ctx.Get(r, "user_id").(int64))
	if err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
		return
	}
	if r.Method == "GET" {
		JSONResponse(w, cr, http.StatusOK)
		return
	}
}

// CampaignSummary returns the summary for a given campaign.
func (as *Server) CampaignSummary(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	switch {
	case r.Method == "GET":
		cs, err := models.GetCampaignSummary(id, ctx.Get(r, "user_id").(int64))
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
			} else {
				JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			}
			log.Error(err)
			return
		}
		JSONResponse(w, cs, http.StatusOK)
	}
}

// CampaignComplete effectively "ends" a campaign.
// Future phishing emails clicked will return a simple "404" page.
func (as *Server) CampaignComplete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	switch {
	case r.Method == "GET":
		err := models.CompleteCampaign(id, ctx.Get(r, "user_id").(int64))
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Error completing campaign"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Campaign completed successfully!"}, http.StatusOK)
	}
}

// CampaignsTrash returns campaigns in trash (soft deleted)
func (as *Server) CampaignsTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusMethodNotAllowed)
		return
	}

	userID := ctx.Get(r, "user_id").(int64)

	// Parse pagination params
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	offset := (page - 1) * perPage

	campaigns, total, err := models.GetTrashedCampaignsPaginated(userID, offset, perPage)
	if err != nil {
		log.Errorf("Error retrieving trashed campaigns: %v", err)
		JSONResponse(w, models.Response{Success: false, Message: "Error retrieving trash"}, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"campaigns": campaigns,
		"total":     total,
		"page":      page,
		"per_page":  perPage,
	}

	JSONResponse(w, response, http.StatusOK)
}

// CampaignRestore restores a campaign from trash
func (as *Server) CampaignRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	userID := ctx.Get(r, "user_id").(int64)

	result, err := models.RestoreCampaign(id, userID)
	if err != nil {
		if err == models.ErrCampaignNotFound {
			JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
			return
		}
		if err == models.ErrNotDeleted {
			JSONResponse(w, models.Response{Success: false, Message: "Campaign is not in trash"}, http.StatusBadRequest)
			return
		}
		if err == models.ErrPermissionDenied {
			JSONResponse(w, models.Response{Success: false, Message: "Permission denied"}, http.StatusForbidden)
			return
		}
		log.Errorf("Error restoring campaign %d: %v", id, err)
		JSONResponse(w, models.Response{Success: false, Message: "Error restoring campaign"}, http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":      true,
		"message":      "Campaign restored successfully",
		"campaign":     result.Campaign,
		"warnings":     result.Warnings,
		"name_changed": result.NameChanged,
	}

	JSONResponse(w, response, http.StatusOK)
}

// CampaignPurge permanently deletes a campaign (hard delete)
func (as *Server) CampaignPurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusMethodNotAllowed)
		return
	}

	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	userID := ctx.Get(r, "user_id").(int64)

	// Check if user is admin
	user, err := models.GetUser(userID)
	if err != nil {
		log.Errorf("Error getting user %d: %v", userID, err)
		JSONResponse(w, models.Response{Success: false, Message: "Error verifying permissions"}, http.StatusInternalServerError)
		return
	}

	isAdmin := user.Role.Slug == "admin"
	if !isAdmin {
		JSONResponse(w, models.Response{Success: false, Message: "Admin privileges required"}, http.StatusForbidden)
		return
	}

	// Read confirmation from body
	var req struct {
		Confirmation string `json:"confirmation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid request body"}, http.StatusBadRequest)
		return
	}

	// Get campaign to validate confirmation
	c, err := models.GetCampaign(id, userID)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Campaign not found"}, http.StatusNotFound)
		return
	}

	// Validate confirmation (must match campaign name or "DELETE")
	if req.Confirmation != c.Name && req.Confirmation != "DELETE" {
		JSONResponse(w, models.Response{Success: false, Message: "Confirmation does not match"}, http.StatusBadRequest)
		return
	}

	// Purge
	err = models.PurgeCampaign(id, userID, true)
	if err != nil {
		log.Errorf("Error purging campaign %d: %v", id, err)
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
		return
	}

	JSONResponse(w, models.Response{Success: true, Message: "Campaign permanently deleted"}, http.StatusOK)
}
