package ics

import (
	"fmt"
	"strings"
	"time"
)

// CalendarEvent represents the data needed to generate an .ICS file
type CalendarEvent struct {
	UID             string
	Title           string
	Description     string
	Location        string
	StartTime       time.Time
	EndTime         time.Time
	OrganizerName   string
	OrganizerEmail  string
	AttendeeName    string
	AttendeeEmail   string
	ReminderMinutes int
	MeetingURL      string
}

// Generate creates an .ICS file content from the CalendarEvent
func (e *CalendarEvent) Generate() string {
	// Format times in UTC for .ICS (YYYYMMDDTHHmmssZ)
	startTime := e.StartTime.UTC().Format("20060102T150405Z")
	endTime := e.EndTime.UTC().Format("20060102T150405Z")
	timestamp := time.Now().UTC().Format("20060102T150405Z")

	// Build description with meeting URL
	description := e.Description
	if e.MeetingURL != "" {
		description = fmt.Sprintf("%s\n\nÚnete a la reunión:\n%s", e.Description, e.MeetingURL)
	}

	// Escape special characters for .ICS format
	description = escapeICSText(description)
	title := escapeICSText(e.Title)
	location := escapeICSText(e.Location)

	var ics strings.Builder

	// VCALENDAR header
	ics.WriteString("BEGIN:VCALENDAR\r\n")
	ics.WriteString("VERSION:2.0\r\n")
	ics.WriteString("PRODID:-//Gophish//Calendar Phishing//EN\r\n")
	ics.WriteString("METHOD:REQUEST\r\n")
	ics.WriteString("CALSCALE:GREGORIAN\r\n")

	// VEVENT
	ics.WriteString("BEGIN:VEVENT\r\n")
	ics.WriteString(foldLine(fmt.Sprintf("UID:%s", e.UID)))
	ics.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", timestamp))
	ics.WriteString(fmt.Sprintf("DTSTART:%s\r\n", startTime))
	ics.WriteString(fmt.Sprintf("DTEND:%s\r\n", endTime))
	ics.WriteString(foldLine(fmt.Sprintf("SUMMARY:%s", title)))

	if description != "" {
		ics.WriteString(foldLine(fmt.Sprintf("DESCRIPTION:%s", description)))
	}

	if location != "" {
		ics.WriteString(foldLine(fmt.Sprintf("LOCATION:%s", location)))
	}

	// Organizer
	if e.OrganizerEmail != "" {
		organizerName := escapeICSText(e.OrganizerName)
		ics.WriteString(foldLine(fmt.Sprintf("ORGANIZER;CN=%s:mailto:%s", organizerName, e.OrganizerEmail)))
	}

	// Attendee
	if e.AttendeeEmail != "" {
		attendeeName := escapeICSText(e.AttendeeName)
		ics.WriteString(foldLine(fmt.Sprintf("ATTENDEE;CN=%s;RSVP=TRUE:mailto:%s", attendeeName, e.AttendeeEmail)))
	}

	ics.WriteString("STATUS:CONFIRMED\r\n")
	ics.WriteString("SEQUENCE:0\r\n")

	// Reminder (VALARM)
	if e.ReminderMinutes > 0 {
		ics.WriteString("BEGIN:VALARM\r\n")
		ics.WriteString(fmt.Sprintf("TRIGGER:-PT%dM\r\n", e.ReminderMinutes))
		ics.WriteString("ACTION:DISPLAY\r\n")
		ics.WriteString(fmt.Sprintf("DESCRIPTION:Recordatorio: Reunión en %d minutos\r\n", e.ReminderMinutes))
		ics.WriteString("END:VALARM\r\n")
	}

	ics.WriteString("END:VEVENT\r\n")
	ics.WriteString("END:VCALENDAR\r\n")

	return ics.String()
}

// escapeICSText escapes special characters for .ICS format
func escapeICSText(text string) string {
	// First escape backslashes, then replace newlines
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "\n", "\\n")
	text = strings.ReplaceAll(text, "\r", "")
	// Escape commas and semicolons
	text = strings.ReplaceAll(text, ",", "\\,")
	text = strings.ReplaceAll(text, ";", "\\;")
	return text
}

// foldLine implements RFC 5545 line folding (max 75 chars per line)
func foldLine(line string) string {
	const maxLineLength = 75
	if len(line) <= maxLineLength {
		return line + "\r\n"
	}

	var result strings.Builder
	remaining := line

	// First line
	result.WriteString(remaining[:maxLineLength])
	result.WriteString("\r\n")
	remaining = remaining[maxLineLength:]

	// Continuation lines (start with space)
	for len(remaining) > 0 {
		if len(remaining) <= maxLineLength-1 {
			result.WriteString(" ")
			result.WriteString(remaining)
			result.WriteString("\r\n")
			break
		} else {
			result.WriteString(" ")
			result.WriteString(remaining[:maxLineLength-1])
			result.WriteString("\r\n")
			remaining = remaining[maxLineLength-1:]
		}
	}

	return result.String()
}
