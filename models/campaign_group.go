package models

import (
	"encoding/json"
	"errors"
	"time"

	log "github.com/gophish/gophish/logger"
	"github.com/jinzhu/gorm"
)

// CampaignGroup represents a group of linked campaigns for combined telemetry
type CampaignGroup struct {
	Id          int64                   `json:"id"`
	Name        string                  `json:"name" sql:"not null"`
	CreatedDate time.Time               `json:"created_date"`
	Archived    bool                    `json:"archived" sql:"default:false"`
	UserId      int64                   `json:"-"`
	Campaigns   []CampaignGroupCampaign `json:"campaigns,omitempty" gorm:"foreignkey:GroupId"`
	TrashableModel
}

// CampaignGroupCampaign represents the many-to-many relationship between
// campaign groups and campaigns with ordering information
type CampaignGroupCampaign struct {
	Id         int64     `json:"id"`
	GroupId    int64     `json:"group_id"`
	CampaignId int64     `json:"campaign_id"`
	OrderIndex int       `json:"order_index" sql:"default:0"`
	AddedDate  time.Time `json:"added_date"`
	Campaign   Campaign  `json:"campaign,omitempty" gorm:"foreignkey:CampaignId"`
}

// CampaignGroupSummary is a struct representing the overview of a single campaign group
type CampaignGroupSummary struct {
	Id            int64     `json:"id"`
	Name          string    `json:"name"`
	CreatedDate   time.Time `json:"created_date"`
	Archived      bool      `json:"archived"`
	CampaignCount int       `json:"campaign_count"`
}

// CampaignGroupStats represents aggregated telemetry for a campaign group
type CampaignGroupStats struct {
	TotalRecipients int64 `json:"total_recipients"`
	EmailsSent      int64 `json:"sent"`
	OpenedEmail     int64 `json:"opened"`
	ClickedLink     int64 `json:"clicked"`
	SubmittedData   int64 `json:"submitted_data"`
	EmailReported   int64 `json:"email_reported"`
	Error           int64 `json:"error"`
	// Calendar-specific events (if applicable)
	CalendarOpened  int64 `json:"calendar_opened,omitempty"`
	CalendarClicked int64 `json:"calendar_clicked,omitempty"`
	// Per-recipient journey tracking
	RecipientJourneys []RecipientJourney `json:"recipient_journeys,omitempty"`
}

// RecipientJourney tracks the progress of a single recipient across multiple campaigns in a group
type RecipientJourney struct {
	Email           string                    `json:"email"`
	CampaignResults []CampaignRecipientResult `json:"campaign_results"`
}

// EventSummary is a lightweight event entry for the journey comparison table
type EventSummary struct {
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

// CampaignRecipientResult tracks a recipient's full event history and captured
// form data for a specific campaign inside a campaign group
type CampaignRecipientResult struct {
	CampaignId   int64               `json:"campaign_id"`
	CampaignName string              `json:"campaign_name"`
	Status       string              `json:"status"`
	Events       []EventSummary      `json:"events"`
	FormData     map[string][]string `json:"form_data,omitempty"`
}

type CampaignGroupReference struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

// Validation errors
var (
	ErrCampaignGroupNameEmpty    = errors.New("Campaign group name is required")
	ErrCampaignGroupNotFound     = errors.New("Campaign group not found")
	ErrNoCampaignsInGroup        = errors.New("At least one campaign is required")
	ErrDuplicateCampaignInGroup  = errors.New("Campaign already exists in this group")
	ErrInvalidCampaignGroupOwner = errors.New("One or more campaigns do not belong to this user")
)

// GetCampaignGroups returns the active (non-deleted) campaign groups owned by the given user
func GetCampaignGroups(uid int64) ([]CampaignGroup, error) {
	cgs := []CampaignGroup{}
	err := db.Where("user_id = ? AND deleted_at IS NULL", uid).Order("created_date DESC").Find(&cgs).Error
	if err != nil {
		log.Error(err)
		return cgs, err
	}
	// Load campaigns for each group
	for i := range cgs {
		err = loadCampaignGroupCampaigns(&cgs[i], false)
		if err != nil {
			log.Error(err)
		}
	}
	return cgs, nil
}

// GetCampaignGroup returns the campaign group with the given id and user_id
func GetCampaignGroup(id int64, uid int64) (CampaignGroup, error) {
	cg := CampaignGroup{}
	err := db.Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, uid).First(&cg).Error
	if err != nil {
		log.Error(err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return cg, ErrCampaignGroupNotFound
		}
		return cg, err
	}
	// Load campaigns with full campaign details, including soft-deleted
	// campaigns so linked campaign rows never render a zero-value Campaign.
	err = loadCampaignGroupCampaigns(&cg, true)
	if err != nil {
		log.Error(err)
	}
	return cg, nil
}

func loadCampaignGroupCampaigns(cg *CampaignGroup, includeDetails bool) error {
	links := []CampaignGroupCampaign{}
	err := db.Where("group_id = ?", cg.Id).
		Order("order_index ASC").
		Find(&links).Error
	if err != nil {
		return err
	}

	cg.Campaigns = []CampaignGroupCampaign{}
	for i := range links {
		campaign := Campaign{}
		query := db.Unscoped().Where("id = ? AND user_id = ?", links[i].CampaignId, cg.UserId)
		if includeDetails {
			query = query.Preload("Results").Preload("Events")
		}
		if err := query.First(&campaign).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				log.Warnf("Skipping orphaned campaign group link group=%d campaign=%d", cg.Id, links[i].CampaignId)
				continue
			}
			return err
		}
		links[i].Campaign = campaign
		cg.Campaigns = append(cg.Campaigns, links[i])
	}
	return nil
}

func GetCampaignGroupsForCampaign(campaignID int64, uid int64) ([]CampaignGroupReference, error) {
	groups := []CampaignGroupReference{}
	rows, err := db.Table("campaign_group_campaigns").
		Select("campaign_groups.id, campaign_groups.name").
		Joins("JOIN campaign_groups ON campaign_groups.id = campaign_group_campaigns.group_id").
		Where("campaign_group_campaigns.campaign_id = ? AND campaign_groups.user_id = ? AND campaign_groups.deleted_at IS NULL", campaignID, uid).
		Order("campaign_groups.name ASC").
		Rows()
	if err != nil {
		log.Error(err)
		return groups, err
	}
	defer rows.Close()

	for rows.Next() {
		group := CampaignGroupReference{}
		if err := rows.Scan(&group.Id, &group.Name); err != nil {
			log.Error(err)
			return groups, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

// GetCampaignGroupSummaries returns summaries of campaign groups for a user
func GetCampaignGroupSummaries(uid int64) ([]CampaignGroupSummary, error) {
	summaries := []CampaignGroupSummary{}

	rows, err := db.Table("campaign_groups").
		Select("campaign_groups.id, campaign_groups.name, campaign_groups.created_date, campaign_groups.archived, COUNT(campaign_group_campaigns.id) as campaign_count").
		Joins("LEFT JOIN campaign_group_campaigns ON campaign_groups.id = campaign_group_campaigns.group_id").
		Where("campaign_groups.user_id = ? AND campaign_groups.deleted_at IS NULL", uid).
		Group("campaign_groups.id").
		Order("campaign_groups.created_date DESC").
		Rows()

	if err != nil {
		log.Error(err)
		return summaries, err
	}
	defer rows.Close()

	for rows.Next() {
		summary := CampaignGroupSummary{}
		err := rows.Scan(&summary.Id, &summary.Name, &summary.CreatedDate, &summary.Archived, &summary.CampaignCount)
		if err != nil {
			log.Error(err)
			continue
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// PostCampaignGroup creates a new campaign group in the database
func PostCampaignGroup(cg *CampaignGroup, uid int64) error {
	// Validate
	if cg.Name == "" {
		return ErrCampaignGroupNameEmpty
	}
	if len(cg.Campaigns) == 0 {
		return ErrNoCampaignsInGroup
	}

	// Set metadata
	cg.UserId = uid
	cg.CreatedDate = time.Now().UTC()

	// Verify all campaigns belong to the user
	campaignIds := make([]int64, len(cg.Campaigns))
	for i, cgc := range cg.Campaigns {
		campaignIds[i] = cgc.CampaignId
	}

	var count int
	err := db.Table("campaigns").
		Where("id IN (?) AND user_id = ? AND deleted_at IS NULL", campaignIds, uid).
		Count(&count).Error
	if err != nil {
		log.Error(err)
		return err
	}
	if count != len(campaignIds) {
		return ErrInvalidCampaignGroupOwner
	}

	// Start transaction
	tx := db.Begin()

	// Create the group
	err = tx.Save(cg).Error
	if err != nil {
		tx.Rollback()
		log.Error(err)
		return err
	}

	// Create the campaign associations
	for i := range cg.Campaigns {
		cg.Campaigns[i].GroupId = cg.Id
		cg.Campaigns[i].AddedDate = time.Now().UTC()
		err = tx.Save(&cg.Campaigns[i]).Error
		if err != nil {
			tx.Rollback()
			log.Error(err)
			return err
		}
	}

	// Commit
	err = tx.Commit().Error
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

// PutCampaignGroup updates an existing campaign group
func PutCampaignGroup(cg *CampaignGroup, uid int64) error {
	// Verify ownership
	existing, err := GetCampaignGroup(cg.Id, uid)
	if err != nil {
		if err == gorm.ErrRecordNotFound || err == ErrCampaignGroupNotFound {
			return ErrCampaignGroupNotFound
		}
		return err
	}

	// Validate
	if cg.Name == "" {
		return ErrCampaignGroupNameEmpty
	}

	// Start transaction
	tx := db.Begin()

	// Update basic fields
	existing.Name = cg.Name
	existing.Archived = cg.Archived
	err = tx.Save(&existing).Error
	if err != nil {
		tx.Rollback()
		log.Error(err)
		return err
	}

	// Update campaigns if provided
	if len(cg.Campaigns) > 0 {
		// Verify all campaigns belong to the user
		campaignIds := make([]int64, len(cg.Campaigns))
		for i, cgc := range cg.Campaigns {
			campaignIds[i] = cgc.CampaignId
		}

		var count int
		err := tx.Table("campaigns").
			Where("id IN (?) AND user_id = ? AND deleted_at IS NULL", campaignIds, uid).
			Count(&count).Error
		if err != nil {
			tx.Rollback()
			log.Error(err)
			return err
		}
		if count != len(campaignIds) {
			tx.Rollback()
			return ErrInvalidCampaignGroupOwner
		}

		// Delete existing associations
		err = tx.Where("group_id = ?", cg.Id).Delete(&CampaignGroupCampaign{}).Error
		if err != nil {
			tx.Rollback()
			log.Error(err)
			return err
		}

		// Create new associations
		for i := range cg.Campaigns {
			cg.Campaigns[i].GroupId = cg.Id
			cg.Campaigns[i].AddedDate = time.Now().UTC()
			err = tx.Save(&cg.Campaigns[i]).Error
			if err != nil {
				tx.Rollback()
				log.Error(err)
				return err
			}
		}
	}

	// Commit
	err = tx.Commit().Error
	if err != nil {
		log.Error(err)
		return err
	}

	return nil
}

// DeleteCampaignGroup moves a campaign group to the trash (soft delete).
// Kept for backward compatibility; callers should prefer SoftDeleteCampaignGroup.
func DeleteCampaignGroup(id int64, uid int64) error {
	return SoftDeleteCampaignGroup(id, uid, "")
}

// GetCampaignGroupStats returns aggregated telemetry for a campaign group
func GetCampaignGroupStats(id int64, uid int64) (CampaignGroupStats, error) {
	stats := CampaignGroupStats{}

	// Get the campaign group structure (campaigns list + basic info)
	cg, err := GetCampaignGroup(id, uid)
	if err != nil {
		return stats, err
	}
	if len(cg.Campaigns) == 0 {
		return stats, nil
	}

	// Collect campaign IDs and build name lookup
	campaignIds := make([]int64, 0, len(cg.Campaigns))
	campaignNames := make(map[int64]string)
	for _, cgc := range cg.Campaigns {
		if cgc.Campaign.DeletedAt != nil {
			continue
		}
		cid := cgc.Campaign.Id
		campaignIds = append(campaignIds, cid)
		campaignNames[cid] = cgc.Campaign.Name
	}
	if len(campaignIds) == 0 {
		return stats, nil
	}

	// Direct DB queries — more reliable than nested GORM preloads
	var allEvents []Event
	if err = db.Where("campaign_id IN (?) AND email != ''", campaignIds).
		Order("time ASC").Find(&allEvents).Error; err != nil {
		log.Error(err)
		return stats, err
	}

	var allResults []Result
	if err = db.Where("campaign_id IN (?)", campaignIds).
		Find(&allResults).Error; err != nil {
		log.Error(err)
		return stats, err
	}

	// Group events by (email, campaign_id)
	type rcKey struct {
		Email      string
		CampaignId int64
	}
	eventsByRC := make(map[rcKey][]EventSummary)
	formDataByRC := make(map[rcKey]map[string][]string)

	for _, event := range allEvents {
		key := rcKey{Email: event.Email, CampaignId: event.CampaignId}
		eventsByRC[key] = append(eventsByRC[key], EventSummary{
			Message: event.Message,
			Time:    event.Time,
		})
		if event.Message == EventDataSubmit && event.Details != "" {
			var details EventDetails
			if err2 := json.Unmarshal([]byte(event.Details), &details); err2 == nil && details.Payload != nil {
				formDataByRC[key] = map[string][]string(details.Payload)
			}
		}
		switch event.Message {
		case EventOpened:
			stats.OpenedEmail++
		case EventClicked:
			stats.ClickedLink++
		case EventDataSubmit:
			stats.SubmittedData++
		case EventReported:
			stats.EmailReported++
		case "Calendar Opened":
			stats.CalendarOpened++
		case "Calendar Clicked":
			stats.CalendarClicked++
		}
	}

	// Build recipient journey map
	recipientMap := make(map[string]*RecipientJourney)
	for _, result := range allResults {
		email := result.Email
		if email == "" {
			continue
		}
		if _, exists := recipientMap[email]; !exists {
			recipientMap[email] = &RecipientJourney{
				Email:           email,
				CampaignResults: []CampaignRecipientResult{},
			}
		}
		key := rcKey{Email: email, CampaignId: result.CampaignId}
		events := eventsByRC[key]
		if events == nil {
			events = []EventSummary{}
		}
		recipientMap[email].CampaignResults = append(
			recipientMap[email].CampaignResults,
			CampaignRecipientResult{
				CampaignId:   result.CampaignId,
				CampaignName: campaignNames[result.CampaignId],
				Status:       result.Status,
				Events:       events,
				FormData:     formDataByRC[key],
			},
		)
		switch result.Status {
		case StatusSuccess, EventSent:
			stats.EmailsSent++
		case Error:
			stats.Error++
		}
	}

	stats.TotalRecipients = int64(len(recipientMap))
	for _, journey := range recipientMap {
		stats.RecipientJourneys = append(stats.RecipientJourneys, *journey)
	}

	return stats, nil
}
