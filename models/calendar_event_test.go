package models

import (
	"os"
	"strings"
	"time"

	check "gopkg.in/check.v1"
)

func (s *ModelsSuite) TestCalendarCampaignValidationAndEmailRegression(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.CampaignType = "calendar"
	campaign.EventStartTime = time.Date(2026, 6, 1, 15, 30, 0, 0, time.UTC)
	campaign.EventDuration = 45
	campaign.OrganizerName = "Calendar Organizer"
	campaign.EventTitle = "Calendar title {{.FirstName}}"
	campaign.EventDescription = "Calendar description {{.RId}}"
	c.Assert(PostCampaign(&campaign, campaign.UserId), check.Equals, nil)
	c.Assert(campaign.CampaignType, check.Equals, "calendar")
	c.Assert(campaign.EventTitle, check.Equals, "Calendar title {{.FirstName}}")
	c.Assert(campaign.EventDescription, check.Equals, "Calendar description {{.RId}}")
	c.Assert(campaign.PlatformType, check.Equals, "")
	c.Assert(len(campaign.Results), check.Equals, 4)

	emailCampaign := s.createCampaignDependencies(c)
	c.Assert(PostCampaign(&emailCampaign, emailCampaign.UserId), check.Equals, nil)
	c.Assert(emailCampaign.CampaignType, check.Equals, "email")
	c.Assert(len(emailCampaign.Results), check.Equals, 4)
}

func (s *ModelsSuite) TestCalendarCampaignRequiresCalendarFields(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.CampaignType = "calendar"
	c.Assert(campaign.Validate(), check.ErrorMatches, "Event start time is required for calendar campaigns")

	campaign.EventStartTime = time.Now().UTC().Add(time.Hour)
	c.Assert(campaign.Validate(), check.ErrorMatches, "Event duration must be greater than 0 for calendar campaigns")

	campaign.EventDuration = 30
	c.Assert(campaign.Validate(), check.ErrorMatches, "Organizer name is required for calendar campaigns")
}

func (s *ModelsSuite) TestSaveCalendarEventOnce(c *check.C) {
	campaign := s.createCampaign(c)
	result := campaign.Results[0]
	event := &CalendarEvent{
		ResultId:  result.Id,
		EventType: "page_loaded",
		IP:        "127.0.0.1",
		UserAgent: "calendar-agent",
		Details:   `{"event":["page_loaded"]}`,
	}
	c.Assert(SaveCalendarEventOnce(event), check.Equals, nil)
	c.Assert(SaveCalendarEventOnce(&CalendarEvent{
		ResultId:  result.Id,
		EventType: "page_loaded",
		IP:        "127.0.0.2",
		UserAgent: "calendar-agent-2",
		Details:   `{"event":["page_loaded"]}`,
	}), check.Equals, nil)

	events, err := GetCalendarEventsByResult(result.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(events), check.Equals, 1)
	c.Assert(events[0].EventType, check.Equals, "page_loaded")
	c.Assert(events[0].IP, check.Equals, "127.0.0.1")
	c.Assert(events[0].UserAgent, check.Equals, "calendar-agent")
	c.Assert(events[0].Details, check.Equals, `{"event":["page_loaded"]}`)
}

func (s *ModelsSuite) TestCalendarMigrationParity(c *check.C) {
	sqliteMigration, err := os.ReadFile("../db/db_sqlite3/migrations/20251227000000_calendar_phishing.sql")
	c.Assert(err, check.Equals, nil)
	mysqlMigration, err := os.ReadFile("../db/db_mysql/migrations/20251227000000_calendar_phishing.sql")
	c.Assert(err, check.Equals, nil)

	for _, field := range []string{
		"campaign_type",
		"platform_type",
		"event_meeting_url",
		"event_title",
		"event_description",
		"event_start_time",
		"event_duration",
		"organizer_name",
		"organizer_email",
		"calendar_events",
		"result_id",
		"event_type",
		"timestamp",
		"user_agent",
		"details",
	} {
		c.Assert(strings.Contains(string(sqliteMigration), field), check.Equals, true)
		c.Assert(strings.Contains(string(mysqlMigration), field), check.Equals, true)
	}
}
