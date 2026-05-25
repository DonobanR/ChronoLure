package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gophish/gophish/models"
)

func createCalendarCampaign(t *testing.T, pageHTML string) models.Campaign {
	group := models.Group{Name: fmt.Sprintf("Calendar Group %d", time.Now().UnixNano())}
	group.Targets = []models.Target{
		{BaseRecipient: models.BaseRecipient{Email: "calendar@example.com", FirstName: "Calendar", LastName: "User"}},
	}
	group.UserId = 1
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("error creating calendar group: %v", err)
	}

	template := models.Template{Name: fmt.Sprintf("Calendar Template %d", time.Now().UnixNano())}
	template.Subject = "Quarterly Planning"
	template.Text = "Planning discussion at {{.URL}}"
	template.HTML = "<html>Calendar email {{.URL}}</html>"
	template.UserId = 1
	if err := models.PostTemplate(&template); err != nil {
		t.Fatalf("error creating calendar template: %v", err)
	}

	page := models.Page{Name: fmt.Sprintf("Calendar Page %d", time.Now().UnixNano())}
	page.HTML = pageHTML
	page.RedirectURL = "https://teams.microsoft.com/l/meetup-join/test"
	page.UserId = 1
	if err := models.PostPage(&page); err != nil {
		t.Fatalf("error creating calendar page: %v", err)
	}

	smtp := models.SMTP{Name: fmt.Sprintf("Calendar SMTP %d", time.Now().UnixNano())}
	smtp.UserId = 1
	smtp.Host = "example.com"
	smtp.FromAddress = "organizer@example.com"
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatalf("error creating calendar smtp: %v", err)
	}

	campaign := models.Campaign{
		Name:             fmt.Sprintf("Calendar campaign %d", time.Now().UnixNano()),
		UserId:           1,
		Template:         template,
		Page:             page,
		SMTP:             smtp,
		Groups:           []models.Group{group},
		URL:              "http://example.com",
		CampaignType:     "calendar",
		PlatformType:     "teams",
		EventMeetingURL:  "https://teams.microsoft.com/l/meetup-join/test",
		EventTitle:       "Board Sync {{.FirstName}}",
		EventDescription: "Calendar override at {{.URL}}",
		EventStartTime:   time.Date(2026, 6, 1, 15, 30, 0, 0, time.UTC),
		EventDuration:    45,
		OrganizerName:    "Calendar Organizer",
		OrganizerEmail:   "organizer@example.com",
	}
	if err := models.PostCampaign(&campaign, campaign.UserId); err != nil {
		t.Fatalf("error creating calendar campaign: %v", err)
	}
	if err := campaign.UpdateStatus(models.CampaignEmailsSent); err != nil {
		t.Fatalf("error updating calendar campaign status: %v", err)
	}
	return campaign
}

func calendarResult(t *testing.T, campaign models.Campaign) models.Result {
	got, err := models.GetCampaign(campaign.Id, campaign.UserId)
	if err != nil {
		t.Fatalf("error reloading calendar campaign: %v", err)
	}
	if len(got.Results) != 1 {
		t.Fatalf("expected one calendar result, got %d", len(got.Results))
	}
	return got.Results[0]
}

func calendarEventsByType(t *testing.T, resultID int64, eventType string) []models.CalendarEvent {
	events, err := models.GetCalendarEventsByResult(resultID)
	if err != nil {
		t.Fatalf("error getting calendar events: %v", err)
	}
	matches := []models.CalendarEvent{}
	for _, event := range events {
		if event.EventType == eventType {
			matches = append(matches, event)
		}
	}
	return matches
}

func countCampaignEvents(t *testing.T, campaignID int64, message string) int {
	campaign, err := models.GetCampaign(campaignID, 1)
	if err != nil {
		t.Fatalf("error getting campaign events: %v", err)
	}
	count := 0
	for _, event := range campaign.Events {
		if event.Message == message {
			count++
		}
	}
	return count
}

func TestCalendarLandingRendersAndTracksClick(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	html, err := ioutil.ReadFile("templates/calendar_landing.html")
	if err != nil {
		t.Fatalf("error reading calendar landing template: %v", err)
	}
	campaign := createCalendarCampaign(t, string(html))
	result := calendarResult(t, campaign)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/calendar?rid=%s", ctx.phishServer.URL, result.RId), nil)
	if err != nil {
		t.Fatalf("error creating request: %v", err)
	}
	req.Header.Set("User-Agent", "calendar-test-agent")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error requesting calendar landing: %v", err)
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected calendar landing status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "Board Sync Calendar") || !strings.Contains(string(body), result.RId) {
		t.Fatalf("calendar landing did not render expected template data: %s", body)
	}
	if countCampaignEvents(t, campaign.Id, models.EventClicked) != 1 {
		t.Fatalf("expected one clicked-link event for calendar landing")
	}
	if got := calendarEventsByType(t, result.Id, "link_opened"); len(got) != 1 {
		t.Fatalf("expected one calendar link_opened event, got %d", len(got))
	}
}

func TestCalendarLandingInvalidRIDIsNotFound(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	resp, err := http.Get(fmt.Sprintf("%s/calendar?rid=invalid-rid", ctx.phishServer.URL))
	if err != nil {
		t.Fatalf("error requesting invalid calendar landing: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected invalid calendar landing to return 404, got %d", resp.StatusCode)
	}
}

func TestCalendarTrackPersistsEventOnce(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	campaign := createCalendarCampaign(t, "<html>{{.RId}}</html>")
	result := calendarResult(t, campaign)

	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/calendar/track?rid=%s&event=page_loaded", ctx.phishServer.URL, result.RId), nil)
		if err != nil {
			t.Fatalf("error creating track request: %v", err)
		}
		req.Header.Set("User-Agent", "calendar-track-agent")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("error requesting calendar track: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected calendar track status: %d", resp.StatusCode)
		}
	}

	events := calendarEventsByType(t, result.Id, "page_loaded")
	if len(events) != 1 {
		t.Fatalf("expected one page_loaded calendar event, got %d", len(events))
	}
	if events[0].IP == "" || events[0].UserAgent != "calendar-track-agent" {
		t.Fatalf("expected IP and User-Agent on track event, got %#v", events[0])
	}
}

func TestCalendarDownloadICSPersistsEvent(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	campaign := createCalendarCampaign(t, "<html>{{.RId}}</html>")
	result := calendarResult(t, campaign)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/calendar/download?rid=%s", ctx.phishServer.URL, result.RId), nil)
	if err != nil {
		t.Fatalf("error creating download request: %v", err)
	}
	req.Header.Set("User-Agent", "calendar-download-agent")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error requesting calendar ICS: %v", err)
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected calendar ICS status %d body %s", resp.StatusCode, body)
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/calendar") {
		t.Fatalf("unexpected calendar ICS content type: %s", resp.Header.Get("Content-Type"))
	}
	content := string(body)
	for _, expected := range []string{"BEGIN:VCALENDAR", "SUMMARY:Board Sync Calendar", "Calendar override", "ORGANIZER;CN=Calendar Organizer:mailto:organizer@example.com", "DTSTART:20260601T153000Z", "DTEND:20260601T161500Z", fmt.Sprintf("/calendar?rid=%s", result.RId)} {
		if !strings.Contains(content, expected) {
			t.Fatalf("ICS missing %q in:\n%s", expected, content)
		}
	}
	events := calendarEventsByType(t, result.Id, "ics_downloaded")
	if len(events) != 1 {
		t.Fatalf("expected one ics_downloaded event, got %d", len(events))
	}
}

func TestCalendarSubmitDoesNotDuplicateSubmittedData(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	html, err := ioutil.ReadFile("templates/teams_login.html")
	if err != nil {
		t.Fatalf("error reading teams landing template: %v", err)
	}
	campaign := createCalendarCampaign(t, string(html))
	result := calendarResult(t, campaign)
	form := url.Values{"email": {"calendar@example.com"}, "password": {"secret-pass"}}

	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/calendar?rid=%s", ctx.phishServer.URL, result.RId), strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("error creating submit request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", "calendar-submit-agent")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("error posting calendar credentials: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("unexpected submit status %d body %s", resp.StatusCode, body)
		}
		var response map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			resp.Body.Close()
			t.Fatalf("error decoding submit response: %v", err)
		}
		resp.Body.Close()
		if response["redirect"] != campaign.EventMeetingURL {
			t.Fatalf("unexpected submit redirect: %s", response["redirect"])
		}
	}

	if got := countCampaignEvents(t, campaign.Id, models.EventDataSubmit); got != 1 {
		t.Fatalf("expected one Submitted Data event after duplicate posts, got %d", got)
	}
	events := calendarEventsByType(t, result.Id, "credentials_submitted")
	if len(events) != 1 {
		t.Fatalf("expected one credentials_submitted calendar event, got %d", len(events))
	}
	var details map[string]string
	if err := json.Unmarshal([]byte(events[0].Details), &details); err != nil {
		t.Fatalf("error decoding calendar credentials details: %v", err)
	}
	if details["email"] != "calendar@example.com" || details["password"] != "secret-pass" {
		t.Fatalf("unexpected calendar credential details: %#v", details)
	}
}
