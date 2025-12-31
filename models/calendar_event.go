package models

import (
	"time"

	log "github.com/gophish/gophish/logger"
)

// CalendarEvent represents an event in a calendar phishing campaign
type CalendarEvent struct {
	Id        int64     `json:"id"`
	ResultId  int64     `json:"result_id"`
	EventType string    `json:"event_type"` // ics_sent, link_opened, credentials_submitted, reported
	Timestamp time.Time `json:"timestamp"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	Details   string    `json:"details,omitempty"` // JSON field for additional metadata
}

// SaveCalendarEvent saves a calendar event to the database
func SaveCalendarEvent(ce *CalendarEvent) error {
	if ce.Timestamp.IsZero() {
		ce.Timestamp = time.Now().UTC()
	}
	err := db.Save(ce).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// GetCalendarEventsByResult returns all calendar events for a given result ID
func GetCalendarEventsByResult(resultId int64) ([]CalendarEvent, error) {
	events := []CalendarEvent{}
	err := db.Where("result_id = ?", resultId).Order("timestamp desc").Find(&events).Error
	if err != nil {
		log.Error(err)
	}
	return events, err
}

// GetCalendarEventsByCampaign returns all calendar events for a given campaign
func GetCalendarEventsByCampaign(campaignId int64) ([]CalendarEvent, error) {
	events := []CalendarEvent{}
	err := db.Table("calendar_events").
		Joins("JOIN results ON calendar_events.result_id = results.id").
		Where("results.campaign_id = ?", campaignId).
		Order("calendar_events.timestamp desc").
		Select("calendar_events.*").
		Find(&events).Error
	if err != nil {
		log.Error(err)
	}
	return events, err
}
