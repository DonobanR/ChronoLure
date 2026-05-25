package models

import (
	"time"

	check "gopkg.in/check.v1"
)

func (s *ModelsSuite) TestCampaignGroupActiveMetrics(c *check.C) {
	campaign := s.createCampaign(c)
	result := campaign.Results[0]
	c.Assert(result.HandleClickedLink(EventDetails{}), check.Equals, nil)
	result = campaign.Results[1]
	c.Assert(result.HandleFormSubmit(EventDetails{}), check.Equals, nil)

	group := createCampaignGroupForCampaigns(c, campaign.UserId, "Active metrics group", campaign.Id)

	stats, err := GetCampaignGroupStats(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(stats.TotalRecipients, check.Equals, int64(len(campaign.Results)))
	c.Assert(stats.ClickedLink, check.Equals, int64(1))
	c.Assert(stats.SubmittedData, check.Equals, int64(1))
	c.Assert(len(stats.RecipientJourneys), check.Equals, len(campaign.Results))
}

func (s *ModelsSuite) TestCampaignGroupTrashedCampaignShownButExcludedFromMetrics(c *check.C) {
	campaign := s.createCampaign(c)
	result := campaign.Results[0]
	c.Assert(result.HandleClickedLink(EventDetails{}), check.Equals, nil)
	group := createCampaignGroupForCampaigns(c, campaign.UserId, "Trashed metrics group", campaign.Id)

	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "metrics policy"), check.Equals, nil)

	got, err := GetCampaignGroup(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got.Campaigns), check.Equals, 1)
	c.Assert(got.Campaigns[0].Campaign.Id, check.Equals, campaign.Id)
	c.Assert(got.Campaigns[0].Campaign.Name, check.Not(check.Equals), "")
	c.Assert(got.Campaigns[0].Campaign.DeletedAt, check.NotNil)
	c.Assert(got.Campaigns[0].Campaign.CreatedDate.IsZero(), check.Equals, false)

	stats, err := GetCampaignGroupStats(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(stats.TotalRecipients, check.Equals, int64(0))
	c.Assert(stats.ClickedLink, check.Equals, int64(0))
	c.Assert(len(stats.RecipientJourneys), check.Equals, 0)
}

func (s *ModelsSuite) TestCampaignGroupRestoreReincludesMetrics(c *check.C) {
	campaign := s.createCampaign(c)
	result := campaign.Results[0]
	c.Assert(result.HandleClickedLink(EventDetails{}), check.Equals, nil)
	group := createCampaignGroupForCampaigns(c, campaign.UserId, "Restore metrics group", campaign.Id)

	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "metrics policy"), check.Equals, nil)
	_, err := RestoreCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)

	got, err := GetCampaignGroup(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got.Campaigns), check.Equals, 1)
	c.Assert(got.Campaigns[0].Campaign.DeletedAt, check.IsNil)

	stats, err := GetCampaignGroupStats(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(stats.TotalRecipients, check.Equals, int64(len(campaign.Results)))
	c.Assert(stats.ClickedLink, check.Equals, int64(1))
}

func (s *ModelsSuite) TestCampaignGroupPurgeRemovesLinkAndMetrics(c *check.C) {
	campaign := s.createCampaign(c)
	result := campaign.Results[0]
	c.Assert(result.HandleClickedLink(EventDetails{}), check.Equals, nil)
	group := createCampaignGroupForCampaigns(c, campaign.UserId, "Purge metrics group", campaign.Id)

	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "purge metrics"), check.Equals, nil)
	c.Assert(PurgeCampaign(campaign.Id, campaign.UserId, true), check.Equals, nil)

	assertTableCount(c, "campaign_group_campaigns", "campaign_id = ?", 0, campaign.Id)
	got, err := GetCampaignGroup(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got.Campaigns), check.Equals, 0)

	stats, err := GetCampaignGroupStats(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(stats.TotalRecipients, check.Equals, int64(0))
	c.Assert(stats.ClickedLink, check.Equals, int64(0))
}

func (s *ModelsSuite) TestCampaignGroupOrphanLinkSkippedInStats(c *check.C) {
	group := CampaignGroup{Name: "Orphan metrics group", UserId: 1, CreatedDate: time.Now().UTC()}
	c.Assert(db.Save(&group).Error, check.Equals, nil)
	c.Assert(db.Save(&CampaignGroupCampaign{GroupId: group.Id, CampaignId: 999999, OrderIndex: 0}).Error, check.Equals, nil)

	got, err := GetCampaignGroup(group.Id, group.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got.Campaigns), check.Equals, 0)

	stats, err := GetCampaignGroupStats(group.Id, group.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(stats.TotalRecipients, check.Equals, int64(0))
	c.Assert(len(stats.RecipientJourneys), check.Equals, 0)
}

func (s *ModelsSuite) TestCampaignGroupCalendarCampaignMetrics(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.CampaignType = "calendar"
	campaign.EventStartTime = time.Now().UTC().Add(time.Hour)
	campaign.EventDuration = 30
	campaign.OrganizerName = "Calendar Organizer"
	c.Assert(PostCampaign(&campaign, campaign.UserId), check.Equals, nil)
	result := campaign.Results[0]
	c.Assert(result.HandleClickedLink(EventDetails{}), check.Equals, nil)

	group := createCampaignGroupForCampaigns(c, campaign.UserId, "Calendar metrics group", campaign.Id)
	got, err := GetCampaignGroup(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got.Campaigns), check.Equals, 1)
	c.Assert(got.Campaigns[0].Campaign.CampaignType, check.Equals, "calendar")

	stats, err := GetCampaignGroupStats(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(stats.TotalRecipients, check.Equals, int64(len(campaign.Results)))
	c.Assert(stats.ClickedLink, check.Equals, int64(1))
}

func createCampaignGroupForCampaigns(c *check.C, userID int64, name string, campaignIDs ...int64) CampaignGroup {
	group := CampaignGroup{
		Name:      name,
		UserId:    userID,
		Campaigns: make([]CampaignGroupCampaign, len(campaignIDs)),
	}
	for i, campaignID := range campaignIDs {
		group.Campaigns[i] = CampaignGroupCampaign{CampaignId: campaignID, OrderIndex: i}
	}
	c.Assert(PostCampaignGroup(&group, userID), check.Equals, nil)
	return group
}
