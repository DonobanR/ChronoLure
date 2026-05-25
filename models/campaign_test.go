package models

import (
	"fmt"
	"testing"
	"time"

	check "gopkg.in/check.v1"
)

func (s *ModelsSuite) TestGenerateSendDate(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	// Test that if no launch date is provided, the campaign's creation date
	// is used.
	err := PostCampaign(&campaign, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(campaign.LaunchDate, check.Equals, campaign.CreatedDate)

	// For comparing the dates, we need to fetch the campaign again. This is
	// to solve an issue where the campaign object right now has time down to
	// the microsecond, while in MySQL it's rounded down to the second.
	campaign, _ = GetCampaign(campaign.Id, campaign.UserId)

	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	for _, m := range ms {
		c.Assert(m.SendDate, check.Equals, campaign.CreatedDate)
	}

	// Test that if no send date is provided, all the emails are sent at the
	// campaign's launch date
	campaign = s.createCampaignDependencies(c)
	campaign.LaunchDate = time.Now().UTC()
	err = PostCampaign(&campaign, campaign.UserId)
	c.Assert(err, check.Equals, nil)

	campaign, _ = GetCampaign(campaign.Id, campaign.UserId)

	ms, err = GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	for _, m := range ms {
		c.Assert(m.SendDate, check.Equals, campaign.LaunchDate)
	}

	// Finally, test that if a send date is provided, the emails are staggered
	// correctly.
	campaign = s.createCampaignDependencies(c)
	campaign.LaunchDate = time.Now().UTC()
	campaign.SendByDate = campaign.LaunchDate.Add(2 * time.Minute)
	err = PostCampaign(&campaign, campaign.UserId)
	c.Assert(err, check.Equals, nil)

	campaign, _ = GetCampaign(campaign.Id, campaign.UserId)

	ms, err = GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	sendingOffset := 2 / float64(len(ms))
	for i, m := range ms {
		expectedOffset := int(sendingOffset * float64(i))
		expectedDate := campaign.LaunchDate.Add(time.Duration(expectedOffset) * time.Minute)
		c.Assert(m.SendDate, check.Equals, expectedDate)
	}
}

func (s *ModelsSuite) TestCampaignDateValidation(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	// If both are zero, then the campaign should start immediately with no
	// send by date
	err := campaign.Validate()
	c.Assert(err, check.Equals, nil)

	// If the launch date is specified, then the send date is optional
	campaign = s.createCampaignDependencies(c)
	campaign.LaunchDate = time.Now().UTC()
	err = campaign.Validate()
	c.Assert(err, check.Equals, nil)

	// If the send date is greater than the launch date, then there's no
	//problem
	campaign = s.createCampaignDependencies(c)
	campaign.LaunchDate = time.Now().UTC()
	campaign.SendByDate = campaign.LaunchDate.Add(1 * time.Minute)
	err = campaign.Validate()
	c.Assert(err, check.Equals, nil)

	// If the send date is less than the launch date, then there's an issue
	campaign = s.createCampaignDependencies(c)
	campaign.LaunchDate = time.Now().UTC()
	campaign.SendByDate = campaign.LaunchDate.Add(-1 * time.Minute)
	err = campaign.Validate()
	c.Assert(err, check.Equals, ErrInvalidSendByDate)
}

func (s *ModelsSuite) TestLaunchCampaignMaillogStatus(c *check.C) {
	// For the first test, ensure that campaigns created with the zero date
	// (and therefore are set to launch immediately) have maillogs that are
	// locked to prevent race conditions.
	campaign := s.createCampaign(c)
	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)

	for _, m := range ms {
		c.Assert(m.Processing, check.Equals, true)
	}

	// Next, verify that campaigns scheduled in the future do not lock the
	// maillogs so that they can be picked up by the background worker.
	campaign = s.createCampaignDependencies(c)
	campaign.Name = "New Campaign"
	campaign.LaunchDate = time.Now().Add(1 * time.Hour)
	c.Assert(PostCampaign(&campaign, campaign.UserId), check.Equals, nil)
	ms, err = GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)

	for _, m := range ms {
		c.Assert(m.Processing, check.Equals, false)
	}
}

func (s *ModelsSuite) TestDeleteCampaignAlsoDeletesMailLogs(c *check.C) {
	campaign := s.createCampaign(c)
	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, len(campaign.Results))

	err = DeleteCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)

	ms, err = GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, 0)
}

func (s *ModelsSuite) TestCompleteCampaignAlsoDeletesMailLogs(c *check.C) {
	campaign := s.createCampaign(c)
	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, len(campaign.Results))

	err = CompleteCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)

	ms, err = GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, 0)
}

func (s *ModelsSuite) TestCampaignGetResults(c *check.C) {
	campaign := s.createCampaign(c)
	got, err := GetCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(campaign.Results), check.Equals, len(got.Results))
}

func (s *ModelsSuite) TestSoftDeleteCampaignRemovesPendingMailLogsFromQueue(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.Name = "Queued soft delete campaign"
	campaign.LaunchDate = time.Now().UTC().Add(1 * time.Hour)
	c.Assert(PostCampaign(&campaign, campaign.UserId), check.Equals, nil)

	past := time.Now().UTC().Add(-1 * time.Minute)
	c.Assert(db.Model(&MailLog{}).Where("campaign_id = ?", campaign.Id).
		Updates(map[string]interface{}{"send_date": past, "processing": false}).Error, check.Equals, nil)

	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"), check.Equals, nil)

	queued, err := GetQueuedMailLogs(time.Now().UTC())
	c.Assert(err, check.Equals, nil)
	for _, m := range queued {
		c.Assert(m.CampaignId, check.Not(check.Equals), campaign.Id)
	}
}

func (s *ModelsSuite) TestGetCampaignMailContextRejectsSoftDeletedCampaign(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.Name = "Deleted mail context campaign"
	c.Assert(PostCampaign(&campaign, campaign.UserId), check.Equals, nil)

	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"), check.Equals, nil)

	_, err := GetCampaignMailContext(campaign.Id, campaign.UserId)
	c.Assert(err, check.NotNil)
}

func (s *ModelsSuite) TestWorkerResidualDoesNotProcessTrashedCampaign(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.Name = "Worker residual trashed campaign"
	campaign.LaunchDate = time.Now().UTC().Add(1 * time.Hour)
	c.Assert(PostCampaign(&campaign, campaign.UserId), check.Equals, nil)

	past := time.Now().UTC().Add(-1 * time.Minute)
	c.Assert(db.Save(&MailLog{UserId: campaign.UserId, CampaignId: campaign.Id, RId: "worker-residual", SendDate: past}).Error, check.Equals, nil)
	c.Assert(db.Model(&Campaign{}).Where("id = ?", campaign.Id).Update("deleted_at", past).Error, check.Equals, nil)

	queued, err := GetQueuedMailLogs(time.Now().UTC())
	c.Assert(err, check.Equals, nil)
	for _, m := range queued {
		c.Assert(m.CampaignId, check.Not(check.Equals), campaign.Id)
	}

	_, err = GetCampaignMailContext(campaign.Id, campaign.UserId)
	c.Assert(err, check.NotNil)
}

func (s *ModelsSuite) TestPurgeCampaignDeletesMailLogs(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.Name = "Purge campaign mail logs"
	c.Assert(PostCampaign(&campaign, campaign.UserId), check.Equals, nil)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"), check.Equals, nil)
	c.Assert(db.Save(&MailLog{UserId: campaign.UserId, CampaignId: campaign.Id, RId: "purge1", SendDate: time.Now().UTC()}).Error, check.Equals, nil)

	c.Assert(PurgeCampaign(campaign.Id, campaign.UserId, true), check.Equals, nil)

	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, 0)
}

func (s *ModelsSuite) TestPurgeCampaignDeletesDependentRows(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.Name = "Purge campaign dependencies"
	c.Assert(PostCampaign(&campaign, campaign.UserId), check.Equals, nil)

	result := Result{CampaignId: campaign.Id, UserId: campaign.UserId, RId: "purge-result", Status: EventSent, BaseRecipient: BaseRecipient{Email: "purge-result@example.com"}}
	c.Assert(db.Save(&result).Error, check.Equals, nil)
	event := Event{CampaignId: campaign.Id, Email: result.Email, Time: time.Now().UTC(), Message: EventSent}
	c.Assert(db.Save(&event).Error, check.Equals, nil)
	calendarEvent := CalendarEvent{ResultId: result.Id, EventType: "ics_sent", Timestamp: time.Now().UTC()}
	c.Assert(db.Save(&calendarEvent).Error, check.Equals, nil)
	c.Assert(db.Exec("INSERT INTO campaign_groups (id, name, user_id) VALUES (?, ?, ?)", 98765, "purge dependency group", campaign.UserId).Error, check.Equals, nil)
	c.Assert(db.Save(&CampaignGroupCampaign{GroupId: 98765, CampaignId: campaign.Id}).Error, check.Equals, nil)
	c.Assert(db.Save(&MailLog{UserId: campaign.UserId, CampaignId: campaign.Id, RId: "purge-dependency-log", SendDate: time.Now().UTC()}).Error, check.Equals, nil)

	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"), check.Equals, nil)
	c.Assert(PurgeCampaign(campaign.Id, campaign.UserId, true), check.Equals, nil)

	assertTableCount(c, "campaigns", "id = ?", 0, campaign.Id)
	assertTableCount(c, "mail_logs", "campaign_id = ?", 0, campaign.Id)
	assertTableCount(c, "results", "campaign_id = ?", 0, campaign.Id)
	assertTableCount(c, "events", "campaign_id = ?", 0, campaign.Id)
	assertTableCount(c, "calendar_events", "result_id = ?", 0, result.Id)
	assertTableCount(c, "campaign_group_campaigns", "campaign_id = ?", 0, campaign.Id)
	assertTableCount(c, "audit_log", "entity_type = ? AND entity_id = ? AND action = ?", 1, "campaign", campaign.Id, AuditCampaignPurged)
}

func (s *ModelsSuite) TestPurgeCampaignRejectsDifferentUser(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "ownership test"), check.Equals, nil)

	err := PurgeCampaign(campaign.Id, campaign.UserId+1, true)
	c.Assert(err, check.Equals, ErrPermissionDenied)

	_, err = GetTrashedCampaignByID(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	assertTableCount(c, "audit_log", "entity_type = ? AND entity_id = ? AND action = ?", 0, "campaign", campaign.Id, AuditCampaignPurged)
}

func (s *ModelsSuite) TestPurgeCampaignAllowsOwnerWithAdminFlag(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "owner admin purge"), check.Equals, nil)

	c.Assert(PurgeCampaign(campaign.Id, campaign.UserId, true), check.Equals, nil)

	assertTableCount(c, "campaigns", "id = ?", 0, campaign.Id)
	assertTableCount(c, "audit_log", "entity_type = ? AND entity_id = ? AND action = ?", 1, "campaign", campaign.Id, AuditCampaignPurged)
}

func (s *ModelsSuite) TestPurgeCampaignRejectsOwnerWithoutAdminFlag(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "owner non-admin purge"), check.Equals, nil)

	err := PurgeCampaign(campaign.Id, campaign.UserId, false)
	c.Assert(err, check.ErrorMatches, "purge requires admin privileges")

	_, err = GetTrashedCampaignByID(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	assertTableCount(c, "audit_log", "entity_type = ? AND entity_id = ? AND action = ?", 0, "campaign", campaign.Id, AuditCampaignPurged)
}

func (s *ModelsSuite) TestCampaignGroupLinkedCampaignActiveHasCompleteData(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(CompleteCampaign(campaign.Id, campaign.UserId), check.Equals, nil)

	group := CampaignGroup{
		Name:   "Linked active campaign group",
		UserId: campaign.UserId,
		Campaigns: []CampaignGroupCampaign{
			{CampaignId: campaign.Id, OrderIndex: 0},
		},
	}
	c.Assert(PostCampaignGroup(&group, campaign.UserId), check.Equals, nil)

	got, err := GetCampaignGroup(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got.Campaigns), check.Equals, 1)
	linked := got.Campaigns[0].Campaign
	c.Assert(linked.Id, check.Equals, campaign.Id)
	c.Assert(linked.Name, check.Not(check.Equals), "")
	c.Assert(linked.CampaignType, check.Not(check.Equals), "")
	c.Assert(linked.Status, check.Not(check.Equals), "")
	c.Assert(linked.CreatedDate.IsZero(), check.Equals, false)
	c.Assert(linked.CompletedDate.IsZero(), check.Equals, false)
}

func (s *ModelsSuite) TestCampaignGroupLinkedCampaignSoftDeletedHasCompleteTrashData(c *check.C) {
	campaign := s.createCampaign(c)
	group := CampaignGroup{
		Name:   "Linked trashed campaign group",
		UserId: campaign.UserId,
		Campaigns: []CampaignGroupCampaign{
			{CampaignId: campaign.Id, OrderIndex: 0},
		},
	}
	c.Assert(PostCampaignGroup(&group, campaign.UserId), check.Equals, nil)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"), check.Equals, nil)

	got, err := GetCampaignGroup(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got.Campaigns), check.Equals, 1)
	linked := got.Campaigns[0].Campaign
	c.Assert(linked.Id, check.Equals, campaign.Id)
	c.Assert(linked.Name, check.Not(check.Equals), "")
	c.Assert(linked.CampaignType, check.Not(check.Equals), "")
	c.Assert(linked.Status, check.Not(check.Equals), "")
	c.Assert(linked.CreatedDate.IsZero(), check.Equals, false)
	c.Assert(linked.DeletedAt, check.NotNil)
}

func (s *ModelsSuite) TestCampaignGroupLinkedCampaignRestoreHasCompleteData(c *check.C) {
	campaign := s.createCampaign(c)
	group := CampaignGroup{
		Name:   "Linked restored campaign group",
		UserId: campaign.UserId,
		Campaigns: []CampaignGroupCampaign{
			{CampaignId: campaign.Id, OrderIndex: 0},
		},
	}
	c.Assert(PostCampaignGroup(&group, campaign.UserId), check.Equals, nil)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"), check.Equals, nil)
	_, err := RestoreCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)

	got, err := GetCampaignGroup(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got.Campaigns), check.Equals, 1)
	linked := got.Campaigns[0].Campaign
	c.Assert(linked.Id, check.Equals, campaign.Id)
	c.Assert(linked.Name, check.Not(check.Equals), "")
	c.Assert(linked.Status, check.Not(check.Equals), "")
	c.Assert(linked.CreatedDate.IsZero(), check.Equals, false)
	c.Assert(linked.DeletedAt, check.IsNil)
}

func (s *ModelsSuite) TestCampaignGroupSkipsOrphanedCampaignLink(c *check.C) {
	group := CampaignGroup{
		Name: "Orphaned campaign link group",
	}
	group.UserId = 1
	group.CreatedDate = time.Now().UTC()
	c.Assert(db.Save(&group).Error, check.Equals, nil)
	c.Assert(db.Save(&CampaignGroupCampaign{GroupId: group.Id, CampaignId: 999999, OrderIndex: 0}).Error, check.Equals, nil)

	got, err := GetCampaignGroup(group.Id, group.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got.Campaigns), check.Equals, 0)
}

func (s *ModelsSuite) TestPurgeCampaignLinkedToCampaignGroupDeletesLink(c *check.C) {
	campaign := s.createCampaign(c)
	group := CampaignGroup{
		Name:   "Linked purge campaign group",
		UserId: campaign.UserId,
		Campaigns: []CampaignGroupCampaign{
			{CampaignId: campaign.Id, OrderIndex: 0},
		},
	}
	c.Assert(PostCampaignGroup(&group, campaign.UserId), check.Equals, nil)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"), check.Equals, nil)
	c.Assert(PurgeCampaign(campaign.Id, campaign.UserId, true), check.Equals, nil)

	assertTableCount(c, "campaign_group_campaigns", "campaign_id = ?", 0, campaign.Id)
}

func (s *ModelsSuite) TestPurgeSystemCampaignDeletesMailLogs(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.Name = "System purge campaign mail logs"
	c.Assert(PostCampaign(&campaign, campaign.UserId), check.Equals, nil)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"), check.Equals, nil)
	c.Assert(db.Save(&MailLog{UserId: campaign.UserId, CampaignId: campaign.Id, RId: "purge-system1", SendDate: time.Now().UTC()}).Error, check.Equals, nil)

	c.Assert(PurgeSystemCampaign(campaign.Id), check.Equals, nil)

	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, 0)
}

func assertTableCount(c *check.C, table string, query string, expected int, args ...interface{}) {
	var count int
	c.Assert(db.Table(table).Where(query, args...).Count(&count).Error, check.Equals, nil)
	c.Assert(count, check.Equals, expected)
}

func setupCampaignDependencies(b *testing.B, size int) {
	group := Group{Name: "Test Group"}
	// Create a large group of 5000 members
	for i := 0; i < size; i++ {
		group.Targets = append(group.Targets, Target{BaseRecipient: BaseRecipient{Email: fmt.Sprintf("test%d@example.com", i), FirstName: "User", LastName: fmt.Sprintf("%d", i)}})
	}
	group.UserId = 1
	err := PostGroup(&group)
	if err != nil {
		b.Fatalf("error posting group: %v", err)
	}

	// Add a template
	template := Template{Name: "Test Template"}
	template.Subject = "{{.RId}} - Subject"
	template.Text = "{{.RId}} - Text"
	template.HTML = "{{.RId}} - HTML"
	template.UserId = 1
	err = PostTemplate(&template)
	if err != nil {
		b.Fatalf("error posting template: %v", err)
	}

	// Add a landing page
	p := Page{Name: "Test Page"}
	p.HTML = "<html>Test</html>"
	p.UserId = 1
	err = PostPage(&p)
	if err != nil {
		b.Fatalf("error posting page: %v", err)
	}

	// Add a sending profile
	smtp := SMTP{Name: "Test Page"}
	smtp.UserId = 1
	smtp.Host = "example.com"
	smtp.FromAddress = "test@test.com"
	err = PostSMTP(&smtp)
	if err != nil {
		b.Fatalf("error posting smtp: %v", err)
	}
}

// setupCampaign sets up the campaign dependencies as well as posting the
// actual campaign
func setupCampaign(b *testing.B, size int) Campaign {
	setupCampaignDependencies(b, size)
	campaign := Campaign{Name: "Test campaign"}
	campaign.UserId = 1
	campaign.Template = Template{Name: "Test Template"}
	campaign.Page = Page{Name: "Test Page"}
	campaign.SMTP = SMTP{Name: "Test Page"}
	campaign.Groups = []Group{Group{Name: "Test Group"}}
	PostCampaign(&campaign, 1)
	return campaign
}

func BenchmarkCampaign100(b *testing.B) {
	setupBenchmark(b)
	setupCampaignDependencies(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		campaign := Campaign{Name: "Test campaign"}
		campaign.UserId = 1
		campaign.Template = Template{Name: "Test Template"}
		campaign.Page = Page{Name: "Test Page"}
		campaign.SMTP = SMTP{Name: "Test Page"}
		campaign.Groups = []Group{Group{Name: "Test Group"}}

		b.StartTimer()
		err := PostCampaign(&campaign, 1)
		if err != nil {
			b.Fatalf("error posting campaign: %v", err)
		}
		b.StopTimer()
		db.Delete(Result{})
		db.Delete(MailLog{})
		db.Delete(Campaign{})
	}
	tearDownBenchmark(b)
}

func BenchmarkCampaign1000(b *testing.B) {
	setupBenchmark(b)
	setupCampaignDependencies(b, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		campaign := Campaign{Name: "Test campaign"}
		campaign.UserId = 1
		campaign.Template = Template{Name: "Test Template"}
		campaign.Page = Page{Name: "Test Page"}
		campaign.SMTP = SMTP{Name: "Test Page"}
		campaign.Groups = []Group{Group{Name: "Test Group"}}

		b.StartTimer()
		err := PostCampaign(&campaign, 1)
		if err != nil {
			b.Fatalf("error posting campaign: %v", err)
		}
		b.StopTimer()
		db.Delete(Result{})
		db.Delete(MailLog{})
		db.Delete(Campaign{})
	}
	tearDownBenchmark(b)
}

func BenchmarkCampaign10000(b *testing.B) {
	setupBenchmark(b)
	setupCampaignDependencies(b, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		campaign := Campaign{Name: "Test campaign"}
		campaign.UserId = 1
		campaign.Template = Template{Name: "Test Template"}
		campaign.Page = Page{Name: "Test Page"}
		campaign.SMTP = SMTP{Name: "Test Page"}
		campaign.Groups = []Group{Group{Name: "Test Group"}}

		b.StartTimer()
		err := PostCampaign(&campaign, 1)
		if err != nil {
			b.Fatalf("error posting campaign: %v", err)
		}
		b.StopTimer()
		db.Delete(Result{})
		db.Delete(MailLog{})
		db.Delete(Campaign{})
	}
	tearDownBenchmark(b)
}

func BenchmarkGetCampaign100(b *testing.B) {
	setupBenchmark(b)
	campaign := setupCampaign(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetCampaign(campaign.Id, campaign.UserId)
		if err != nil {
			b.Fatalf("error getting campaign: %v", err)
		}
	}
	tearDownBenchmark(b)
}

func BenchmarkGetCampaign1000(b *testing.B) {
	setupBenchmark(b)
	campaign := setupCampaign(b, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetCampaign(campaign.Id, campaign.UserId)
		if err != nil {
			b.Fatalf("error getting campaign: %v", err)
		}
	}
	tearDownBenchmark(b)
}

func BenchmarkGetCampaign5000(b *testing.B) {
	setupBenchmark(b)
	campaign := setupCampaign(b, 5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetCampaign(campaign.Id, campaign.UserId)
		if err != nil {
			b.Fatalf("error getting campaign: %v", err)
		}
	}
	tearDownBenchmark(b)
}

func BenchmarkGetCampaign10000(b *testing.B) {
	setupBenchmark(b)
	campaign := setupCampaign(b, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetCampaign(campaign.Id, campaign.UserId)
		if err != nil {
			b.Fatalf("error getting campaign: %v", err)
		}
	}
	tearDownBenchmark(b)
}
