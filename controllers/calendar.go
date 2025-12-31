package controllers

import (
	"encoding/json"
	"net/http"
	"time"

	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/models"
)

// CalendarPhish handles calendar phishing requests
func (ps *PhishingServer) CalendarPhish(w http.ResponseWriter, r *http.Request) {
	rid := r.URL.Query().Get("rid")
	log.Infof("CalendarPhish called: RID=%s, Method=%s, URL=%s", rid, r.Method, r.URL.String())
	
	if rid == "" {
		log.Warn("CalendarPhish: No RID provided")
		http.NotFound(w, r)
		return
	}

	// Get the result for this RID
	rs, err := models.GetResult(rid)
	if err != nil {
		log.Errorf("CalendarPhish: Error getting result for RID=%s: %v", rid, err)
		http.NotFound(w, r)
		return
	}

	log.Infof("CalendarPhish: Found result - RID=%s, Status=%s, CampaignId=%d", rs.RId, rs.Status, rs.CampaignId)

	// Get the campaign
	c, err := models.GetCampaign(rs.CampaignId, rs.UserId)
	if err != nil {
		log.Errorf("CalendarPhish: Error getting campaign ID=%d: %v", rs.CampaignId, err)
		http.NotFound(w, r)
		return
	}

	log.Infof("CalendarPhish: Campaign type=%s, name=%s", c.CampaignType, c.Name)

	// Check if it's a calendar campaign
	if c.CampaignType != "calendar" {
		log.Warnf("CalendarPhish: Not a calendar campaign (type=%s)", c.CampaignType)
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case "GET":
		ps.handleCalendarPhishGET(w, r, &rs, &c)
	case "POST":
		ps.handleCalendarPhishPOST(w, r, &rs, &c)
	default:
		http.NotFound(w, r)
	}
}

func (ps *PhishingServer) handleCalendarPhishGET(w http.ResponseWriter, r *http.Request, rs *models.Result, c *models.Campaign) {
	// Track that the link was opened
	details := models.EventDetails{
		Payload: r.Form,
		Browser: make(map[string]string),
	}
	err := rs.HandleClickedLink(details)
	if err != nil {
		log.Error(err)
	}

	// Log calendar event
	calEvent := &models.CalendarEvent{
		ResultId:  rs.Id,
		EventType: "link_opened",
		Timestamp: time.Now().UTC(),
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
	}
	err = models.SaveCalendarEvent(calEvent)
	if err != nil {
		log.Error(err)
	}

	log.Infof("Calendar link opened: RID=%s, IP=%s", rs.RId, r.RemoteAddr)

	// Create template context
	ptx, err := models.NewPhishingTemplateContext(c, rs.BaseRecipient, rs.RId)
	if err != nil {
		log.Error(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Use the campaign's landing page HTML
	html, err := models.ExecuteTemplate(c.Page.HTML, ptx)
	if err != nil {
		log.Error(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Write the HTML response
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

func (ps *PhishingServer) handleCalendarPhishPOST(w http.ResponseWriter, r *http.Request, rs *models.Result, c *models.Campaign) {
	err := r.ParseForm()
	if err != nil {
		log.Error(err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")

	// Update result status to submitted
	details := models.EventDetails{
		Payload: r.Form,
		Browser: make(map[string]string),
	}
	err = rs.HandleFormSubmit(details)
	if err != nil {
		log.Error(err)
	}

	// Create event for submitted data
	err = models.AddEvent(&models.Event{
		Email:      rs.Email,
		Message:    models.EventDataSubmit,
		CampaignId: c.Id,
		Details:    "",
	}, c.Id)
	if err != nil {
		log.Error(err)
	}

	// Log calendar event with credentials
	calEventDetails := map[string]string{
		"email": email,
	}
	calDetailsJSON, _ := json.Marshal(calEventDetails)

	calEvent := &models.CalendarEvent{
		ResultId:  rs.Id,
		EventType: "credentials_submitted",
		Timestamp: time.Now().UTC(),
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
		Details:   string(calDetailsJSON),
	}
	err = models.SaveCalendarEvent(calEvent)
	if err != nil {
		log.Error(err)
	}

	log.Infof("Calendar credentials captured: RID=%s, Email=%s", rs.RId, email)

	// Send JSON response with redirect
	w.Header().Set("Content-Type", "application/json")
	redirectURL := c.Page.RedirectURL
	if redirectURL == "" {
		redirectURL = "/"
	}

	response := map[string]string{
		"redirect": redirectURL,
		"message":  "success",
	}

	json.NewEncoder(w).Encode(response)
}

// CalendarTrack handles tracking requests for calendar phishing
func (ps *PhishingServer) CalendarTrack(w http.ResponseWriter, r *http.Request) {
	rid := r.URL.Query().Get("rid")
	eventType := r.URL.Query().Get("event")

	if rid == "" || eventType == "" {
		http.NotFound(w, r)
		return
	}

	// Get the result
	_, err := models.GetResult(rid)
	if err != nil {
		log.Error(err)
		http.NotFound(w, r)
		return
	}

	// Log the tracking event
	log.Infof("Calendar tracking event: RID=%s, Event=%s, IP=%s", rid, eventType, r.RemoteAddr)

	// Return 1x1 transparent pixel
	w.Header().Set("Content-Type", "image/gif")
	w.Write([]byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b})
}

// CalendarDownloadICS serves the .ics file for download (for testing)
func (ps *PhishingServer) CalendarDownloadICS(w http.ResponseWriter, r *http.Request) {
	rid := r.URL.Query().Get("rid")
	if rid == "" {
		http.NotFound(w, r)
		return
	}

	// Get the result
	rs, err := models.GetResult(rid)
	if err != nil {
		log.Error(err)
		http.NotFound(w, r)
		return
	}

	// Get the campaign
	c, err := models.GetCampaign(rs.CampaignId, rs.UserId)
	if err != nil {
		log.Error(err)
		http.NotFound(w, r)
		return
	}

	// Generate the .ics file
	icsContent, err := models.GenerateICSForResult(&rs, &c)
	if err != nil {
		log.Error(err)
		http.Error(w, "Error generating ICS", http.StatusInternalServerError)
		return
	}

	// Serve as downloadable file
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=meeting.ics")
	w.Write([]byte(icsContent))
}
