package models

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gophish/gomail"
	"github.com/gophish/gophish/config"
	"github.com/gophish/gophish/ics"
	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/mailer"
)

// MaxSendAttempts set to 8 since we exponentially backoff after each failed send
// attempt. This will give us a maximum send delay of 256 minutes, or about 4.2 hours.
var MaxSendAttempts = 8

// ErrMaxSendAttempts is thrown when the maximum number of sending attempts for a given
// MailLog is exceeded.
var ErrMaxSendAttempts = errors.New("max send attempts exceeded")

// Attachments with these file extensions have inline disposition
var embeddedFileExtensions = []string{".jpg", ".jpeg", ".png", ".gif"}

// MailLog is a struct that holds information about an email that is to be
// sent out.
type MailLog struct {
	Id          int64     `json:"-"`
	UserId      int64     `json:"-"`
	CampaignId  int64     `json:"campaign_id"`
	RId         string    `json:"id"`
	SendDate    time.Time `json:"send_date"`
	SendAttempt int       `json:"send_attempt"`
	Processing  bool      `json:"-"`

	cachedCampaign *Campaign
}

// GenerateMailLog creates a new maillog for the given campaign and
// result. It sets the initial send date to match the campaign's launch date.
func GenerateMailLog(c *Campaign, r *Result, sendDate time.Time) error {
	m := &MailLog{
		UserId:     c.UserId,
		CampaignId: c.Id,
		RId:        r.RId,
		SendDate:   sendDate,
	}
	return db.Save(m).Error
}

// Backoff sets the MailLog SendDate to be the next entry in an exponential
// backoff. ErrMaxRetriesExceeded is thrown if this maillog has been retried
// too many times. Backoff also unlocks the maillog so that it can be processed
// again in the future.
func (m *MailLog) Backoff(reason error) error {
	r, err := GetResult(m.RId)
	if err != nil {
		return err
	}
	if m.SendAttempt == MaxSendAttempts {
		r.HandleEmailError(ErrMaxSendAttempts)
		return ErrMaxSendAttempts
	}
	// Add an error, since we had to backoff because of a
	// temporary error of some sort during the SMTP transaction
	m.SendAttempt++
	backoffDuration := math.Pow(2, float64(m.SendAttempt))
	m.SendDate = m.SendDate.Add(time.Minute * time.Duration(backoffDuration))
	err = db.Save(m).Error
	if err != nil {
		return err
	}
	err = r.HandleEmailBackoff(reason, m.SendDate)
	if err != nil {
		return err
	}
	err = m.Unlock()
	return err
}

// Unlock removes the processing flag so the maillog can be processed again
func (m *MailLog) Unlock() error {
	m.Processing = false
	return db.Save(&m).Error
}

// Lock sets the processing flag so that other processes cannot modify the maillog
func (m *MailLog) Lock() error {
	m.Processing = true
	return db.Save(&m).Error
}

// Error sets the error status on the models.Result that the
// maillog refers to. Since MailLog errors are permanent,
// this action also deletes the maillog.
func (m *MailLog) Error(e error) error {
	r, err := GetResult(m.RId)
	if err != nil {
		log.Warn(err)
		return err
	}
	err = r.HandleEmailError(e)
	if err != nil {
		log.Warn(err)
		return err
	}
	err = db.Delete(m).Error
	return err
}

// Success deletes the maillog from the database and updates the underlying
// campaign result.
func (m *MailLog) Success() error {
	r, err := GetResult(m.RId)
	if err != nil {
		return err
	}
	err = r.HandleEmailSent()
	if err != nil {
		return err
	}
	err = db.Delete(m).Error
	return err
}

// GetDialer returns a dialer based on the maillog campaign's SMTP configuration
func (m *MailLog) GetDialer() (mailer.Dialer, error) {
	c := m.cachedCampaign
	if c == nil {
		campaign, err := GetCampaignMailContext(m.CampaignId, m.UserId)
		if err != nil {
			return nil, err
		}
		c = &campaign
	}
	return c.SMTP.GetDialer()
}

// CacheCampaign allows bulk-mail workers to cache the otherwise expensive
// campaign lookup operation by providing a pointer to the campaign here.
func (m *MailLog) CacheCampaign(campaign *Campaign) error {
	if campaign.Id != m.CampaignId {
		return fmt.Errorf("incorrect campaign provided for caching. expected %d got %d", m.CampaignId, campaign.Id)
	}
	m.cachedCampaign = campaign
	return nil
}

func (m *MailLog) GetSmtpFrom() (string, error) {
	c, err := GetCampaign(m.CampaignId, m.UserId)
	if err != nil {
		return "", err
	}

	f, err := mail.ParseAddress(c.SMTP.FromAddress)
	return f.Address, err
}

// Generate fills in the details of a gomail.Message instance with
// the correct headers and body from the campaign and recipient listed in
// the maillog. We accept the gomail.Message as an argument so that the caller
// can choose to re-use the message across recipients.
func (m *MailLog) Generate(msg *gomail.Message) error {
	r, err := GetResult(m.RId)
	if err != nil {
		return err
	}
	c := m.cachedCampaign
	if c == nil {
		campaign, err := GetCampaignMailContext(m.CampaignId, m.UserId)
		if err != nil {
			return err
		}
		c = &campaign
	}

	// Check if this is a calendar campaign
	if c.CampaignType == "calendar" {
		return m.generateCalendarEmail(msg, &r, c)
	}

	// Regular email campaign generation
	f, err := mail.ParseAddress(c.Template.EnvelopeSender)
	if err != nil {
		f, err = mail.ParseAddress(c.SMTP.FromAddress)
		if err != nil {
			return err
		}
	}
	msg.SetAddressHeader("From", f.Address, f.Name)

	ptx, err := NewPhishingTemplateContext(c, r.BaseRecipient, r.RId)
	if err != nil {
		return err
	}

	// Add the transparency headers
	msg.SetHeader("X-Mailer", config.ServerName)
	if conf.ContactAddress != "" {
		msg.SetHeader("X-Gophish-Contact", conf.ContactAddress)
	}

	// Add Message-Id header as described in RFC 2822.
	messageID, err := m.generateMessageID()
	if err != nil {
		return err
	}
	msg.SetHeader("Message-Id", messageID)

	// Parse the customHeader templates
	for _, header := range c.SMTP.Headers {
		key, err := ExecuteTemplate(header.Key, ptx)
		if err != nil {
			log.Warn(err)
		}

		value, err := ExecuteTemplate(header.Value, ptx)
		if err != nil {
			log.Warn(err)
		}

		// Add our header immediately
		msg.SetHeader(key, value)
	}

	// Parse remaining templates
	subject, err := ExecuteTemplate(c.Template.Subject, ptx)

	if err != nil {
		log.Warn(err)
	}
	// don't set Subject header if the subject is empty
	if subject != "" {
		msg.SetHeader("Subject", subject)
	}

	msg.SetHeader("To", r.FormatAddress())
	if c.Template.Text != "" {
		text, err := ExecuteTemplate(c.Template.Text, ptx)
		if err != nil {
			log.Warn(err)
		}
		msg.SetBody("text/plain", text)
	}
	if c.Template.HTML != "" {
		html, err := ExecuteTemplate(c.Template.HTML, ptx)
		if err != nil {
			log.Warn(err)
		}
		if c.Template.Text == "" {
			msg.SetBody("text/html", html)
		} else {
			msg.AddAlternative("text/html", html)
		}
	}
	// Attach the files
	for _, a := range c.Template.Attachments {
		addAttachment(msg, a, ptx)
	}

	return nil
}

// generateCalendarEmail generates an email using the template and attaches .ICS calendar invitation
func (m *MailLog) generateCalendarEmail(msg *gomail.Message, r *Result, c *Campaign) error {
	// Set From address (prefer template envelope sender, fallback to SMTP)
	f, err := mail.ParseAddress(c.Template.EnvelopeSender)
	if err != nil {
		f, err = mail.ParseAddress(c.SMTP.FromAddress)
		if err != nil {
			return err
		}
	}
	msg.SetAddressHeader("From", f.Address, f.Name)

	ptx, err := NewPhishingTemplateContext(c, r.BaseRecipient, r.RId)
	if err != nil {
		return err
	}

	// Use Template Subject as Event Title
	processedEventTitle, err := ExecuteTemplate(c.Template.Subject, ptx)
	if err != nil {
		log.Warn(err)
		processedEventTitle = c.Template.Subject
	}
	
	// Use Template Text as Event Description
	processedEventDescription, err := ExecuteTemplate(c.Template.Text, ptx)
	if err != nil {
		log.Warn(err)
		processedEventDescription = c.Template.Text
	}

	// Populate calendar-specific fields in the template context with processed values
	ptx.EventTitle = processedEventTitle
	ptx.EventDescription = processedEventDescription
	ptx.EventStartTime = c.EventStartTime.Format("02 Jan 2006, 03:04 PM MST")
	ptx.EventDuration = c.EventDuration
	ptx.OrganizerName = c.OrganizerName
	ptx.OrganizerEmail = c.OrganizerEmail

	// Add transparency headers
	msg.SetHeader("X-Mailer", config.ServerName)
	if conf.ContactAddress != "" {
		msg.SetHeader("X-Gophish-Contact", conf.ContactAddress)
	}

	// Add Message-Id header
	messageID, err := m.generateMessageID()
	if err != nil {
		return err
	}
	msg.SetHeader("Message-Id", messageID)

	// Parse custom headers
	for _, header := range c.SMTP.Headers {
		key, err := ExecuteTemplate(header.Key, ptx)
		if err != nil {
			log.Warn(err)
		}
		value, err := ExecuteTemplate(header.Value, ptx)
		if err != nil {
			log.Warn(err)
		}
		msg.SetHeader(key, value)
	}

	// Parse template subject
	subject, err := ExecuteTemplate(c.Template.Subject, ptx)
	if err != nil {
		log.Warn(err)
	}
	if subject != "" {
		msg.SetHeader("Subject", subject)
	}
	msg.SetHeader("To", r.FormatAddress())

	// Parse template text and HTML
	if c.Template.Text != "" {
		text, err := ExecuteTemplate(c.Template.Text, ptx)
		if err != nil {
			log.Warn(err)
		}
		msg.SetBody("text/plain", text)
	}
	if c.Template.HTML != "" {
		html, err := ExecuteTemplate(c.Template.HTML, ptx)
		if err != nil {
			log.Warn(err)
		}
		// If we have text, use AddAlternative, else SetBody
		if c.Template.Text != "" {
			msg.AddAlternative("text/html", html)
		} else {
			msg.SetBody("text/html", html)
		}
	}

	// Attach template attachments
	for _, a := range c.Template.Attachments {
		msg.Attach(func(a Attachment) (string, gomail.FileSetting, gomail.FileSetting) {
			h := map[string][]string{
				"Content-ID": {fmt.Sprintf("<%s>", a.Name)},
			}
			return a.Name, gomail.SetCopyFunc(func(w io.Writer) error {
				decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(a.Content))
				_, err = io.Copy(w, decoder)
				return err
			}), gomail.SetHeader(h)
		}(a))
	}

	// Generate calendar meeting URL
	meetingURL := fmt.Sprintf("%s/calendar?rid=%s", c.URL, r.RId)

	// Use the already processed event title and description from above
	eventTitle := processedEventTitle
	eventDescription := processedEventDescription

	// Calculate end time
	endTime := c.EventStartTime.Add(time.Duration(c.EventDuration) * time.Minute)

	// Generate .ICS file content
	// IMPORTANT: Location field uses tracking URL (/calendar?rid=...) for proper tracking
	// The MeetingURL field in description will also show the same URL
	calEvent := ics.CalendarEvent{
		UID:             fmt.Sprintf("gophish-%s@%s", r.RId, "gophish.local"),
		Title:           eventTitle,
		Description:     eventDescription,
		Location:        meetingURL,
		StartTime:       c.EventStartTime,
		EndTime:         endTime,
		OrganizerName:   c.OrganizerName,
		OrganizerEmail:  c.OrganizerEmail,
		AttendeeName:    r.FormatName(),
		AttendeeEmail:   r.Email,
		ReminderMinutes: 15,
		MeetingURL:      meetingURL,
	}
	icsContent := calEvent.Generate()

	// Attach .ICS file with proper MIME type for Outlook
	msg.Attach(fmt.Sprintf("meeting_%s.ics", r.RId),
		gomail.SetCopyFunc(func(w io.Writer) error {
			_, err := w.Write([]byte(icsContent))
			return err
		}),
		gomail.SetHeader(map[string][]string{
			"Content-Type":        {"text/calendar; charset=utf-8; method=REQUEST; name=\"meeting.ics\""},
			"Content-Disposition": {"attachment; filename=\"meeting.ics\""},
		}),
	)

	// Log calendar event
	calEventLog := &CalendarEvent{
		ResultId:  r.Id,
		EventType: "ics_sent",
		Timestamp: time.Now().UTC(),
		Details:   fmt.Sprintf(`{"event_title":"%s","start_time":"%s"}`, processedEventTitle, c.EventStartTime.Format(time.RFC3339)),
	}
	SaveCalendarEvent(calEventLog)

	return nil
}

// GetQueuedMailLogs returns the mail logs that are queued up for the given minute.
func GetQueuedMailLogs(t time.Time) ([]*MailLog, error) {
	ms := []*MailLog{}
	err := db.Where("send_date <= ? AND processing = ?", t, false).
		Find(&ms).Error
	if err != nil {
		log.Warn(err)
	}
	return ms, err
}

// GetMailLogsByCampaign returns all of the mail logs for a given campaign.
func GetMailLogsByCampaign(cid int64) ([]*MailLog, error) {
	ms := []*MailLog{}
	err := db.Where("campaign_id = ?", cid).Find(&ms).Error
	return ms, err
}

// LockMailLogs locks or unlocks a slice of maillogs for processing.
func LockMailLogs(ms []*MailLog, lock bool) error {
	tx := db.Begin()
	for i := range ms {
		ms[i].Processing = lock
		err := tx.Save(ms[i]).Error
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	tx.Commit()
	return nil
}

// UnlockAllMailLogs removes the processing lock for all maillogs
// in the database. This is intended to be called when Gophish is started
// so that any previously locked maillogs can resume processing.
func UnlockAllMailLogs() error {
	return db.Model(&MailLog{}).Update("processing", false).Error
}

var maxBigInt = big.NewInt(math.MaxInt64)

// generateMessageID generates and returns a string suitable for an RFC 2822
// compliant Message-ID, e.g.:
// <1444789264909237300.3464.1819418242800517193@DESKTOP01>
//
// The following parameters are used to generate a Message-ID:
// - The nanoseconds since Epoch
// - The calling PID
// - A cryptographically random int64
// - The sending hostname
func (m *MailLog) generateMessageID() (string, error) {
	t := time.Now().UnixNano()
	pid := os.Getpid()
	rint, err := rand.Int(rand.Reader, maxBigInt)
	if err != nil {
		return "", err
	}
	h, err := os.Hostname()
	// If we can't get the hostname, we'll use localhost
	if err != nil {
		h = "localhost.localdomain"
	}
	msgid := fmt.Sprintf("<%d.%d.%d@%s>", t, pid, rint, h)
	return msgid, nil
}

// Check if an attachment should have inline disposition based on
// its file extension.
func shouldEmbedAttachment(name string) bool {
	ext := filepath.Ext(name)
	for _, v := range embeddedFileExtensions {
		if strings.EqualFold(ext, v) {
			return true
		}
	}
	return false
}

// Add an attachment to a gomail message, with the Content-Disposition
// header set to inline or attachment depending on its file extension.
func addAttachment(msg *gomail.Message, a Attachment, ptx PhishingTemplateContext) {
	copyFunc := gomail.SetCopyFunc(func(c Attachment) func(w io.Writer) error {
		return func(w io.Writer) error {
			reader, err := a.ApplyTemplate(ptx)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, reader)
			return err
		}
	}(a))
	if shouldEmbedAttachment(a.Name) {
		msg.Embed(a.Name, copyFunc)
	} else {
		msg.Attach(a.Name, copyFunc)
	}
}

// GenerateICSForResult generates an ICS file for a given result and campaign
func GenerateICSForResult(r *Result, c *Campaign) (string, error) {
	ptx, err := NewPhishingTemplateContext(c, r.BaseRecipient, r.RId)
	if err != nil {
		return "", err
	}

	// Use Template Subject as Event Title
	processedEventTitle, err := ExecuteTemplate(c.Template.Subject, ptx)
	if err != nil {
		log.Warn(err)
		processedEventTitle = c.Template.Subject
	}

	// Use Template Text as Event Description
	processedEventDescription, err := ExecuteTemplate(c.Template.Text, ptx)
	if err != nil {
		log.Warn(err)
		processedEventDescription = c.Template.Text
	}

	// Generate calendar meeting URL
	meetingURL := fmt.Sprintf("%s/calendar?rid=%s", c.URL, r.RId)

	// Use the already processed event title and description from above
	eventTitle := processedEventTitle
	eventDescription := processedEventDescription

	// Calculate end time
	endTime := c.EventStartTime.Add(time.Duration(c.EventDuration) * time.Minute)

	// Generate .ICS file content
	// IMPORTANT: Location field uses tracking URL (/calendar?rid=...) for proper tracking
	// The MeetingURL field in description will also show the same URL
	calEvent := ics.CalendarEvent{
		UID:             fmt.Sprintf("gophish-%s@%s", r.RId, "gophish.local"),
		Title:           eventTitle,
		Description:     eventDescription,
		Location:        meetingURL,
		StartTime:       c.EventStartTime,
		EndTime:         endTime,
		OrganizerName:   c.OrganizerName,
		OrganizerEmail:  c.OrganizerEmail,
		AttendeeName:    r.FormatName(),
		AttendeeEmail:   r.Email,
		ReminderMinutes: 15,
		MeetingURL:      meetingURL,
	}

	return calEvent.Generate(), nil
}
