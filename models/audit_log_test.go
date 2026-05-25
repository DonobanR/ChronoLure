package models

import (
	"encoding/json"
	"testing"
	"time"

	check "gopkg.in/check.v1"
)

func (s *ModelsSuite) TestAuditCampaignSoftDelete(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "audit soft delete"), check.Equals, nil)

	audit := assertAuditLog(c, "campaign", campaign.Id, AuditCampaignSoftDeleted)
	assertUserAuditActor(c, audit, campaign.UserId)
	assertAuditTimestamp(c, audit)
	metadata := auditMetadata(c, audit)
	c.Assert(metadata["name"], check.Equals, campaign.Name)
	c.Assert(metadata["reason"], check.Equals, "audit soft delete")
	c.Assert(metadata["status_before_delete"], check.Equals, CampaignInProgress)
	_, hasOldKey := metadata["status_before"]
	c.Assert(hasOldKey, check.Equals, false)
}

func (s *ModelsSuite) TestAuditCampaignRestore(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "audit restore"), check.Equals, nil)
	_, err := RestoreCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)

	audit := assertAuditLog(c, "campaign", campaign.Id, AuditCampaignRestored)
	assertUserAuditActor(c, audit, campaign.UserId)
	assertAuditTimestamp(c, audit)
	metadata := auditMetadata(c, audit)
	c.Assert(metadata["name"], check.Equals, campaign.Name)
	c.Assert(metadata["original_name"], check.Equals, campaign.Name)
	c.Assert(metadata["name_changed"], check.Equals, false)
}

func (s *ModelsSuite) TestAuditCampaignPurge(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "audit purge"), check.Equals, nil)
	c.Assert(PurgeCampaign(campaign.Id, campaign.UserId, true), check.Equals, nil)

	audit := assertAuditLog(c, "campaign", campaign.Id, AuditCampaignPurged)
	assertUserAuditActor(c, audit, campaign.UserId)
	assertAuditTimestamp(c, audit)
	metadata := auditMetadata(c, audit)
	c.Assert(metadata["name"], check.Equals, campaign.Name)
	c.Assert(int64(metadata["user_id"].(float64)), check.Equals, campaign.UserId)

	logs, err := GetAuditLogs("campaign", campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(logs) >= 2, check.Equals, true)
}

func (s *ModelsSuite) TestAuditCampaignSystemPurge(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "audit ttl"), check.Equals, nil)
	c.Assert(PurgeSystemCampaign(campaign.Id), check.Equals, nil)

	audit := assertAuditLog(c, "campaign", campaign.Id, AuditCampaignPurged)
	c.Assert(audit.ActorID, check.IsNil)
	c.Assert(audit.ActorName, check.Equals, "system:trash-ttl")
	assertAuditTimestamp(c, audit)
	metadata := auditMetadata(c, audit)
	c.Assert(metadata["purge_type"], check.Equals, "ttl_job")
	c.Assert(int64(metadata["user_id"].(float64)), check.Equals, campaign.UserId)
}

func (s *ModelsSuite) TestAuditCampaignGroupSoftDelete(c *check.C) {
	group := createAuditCampaignGroup(c)
	c.Assert(SoftDeleteCampaignGroup(group.Id, group.UserId, "audit group soft delete"), check.Equals, nil)

	audit := assertAuditLog(c, "campaign_group", group.Id, AuditGroupSoftDeleted)
	assertUserAuditActor(c, audit, group.UserId)
	assertAuditTimestamp(c, audit)
	metadata := auditMetadata(c, audit)
	c.Assert(metadata["name"], check.Equals, group.Name)
	c.Assert(metadata["reason"], check.Equals, "audit group soft delete")
}

func (s *ModelsSuite) TestAuditCampaignGroupRestore(c *check.C) {
	group := createAuditCampaignGroup(c)
	c.Assert(SoftDeleteCampaignGroup(group.Id, group.UserId, "audit group restore"), check.Equals, nil)
	_, err := RestoreCampaignGroup(group.Id, group.UserId)
	c.Assert(err, check.Equals, nil)

	audit := assertAuditLog(c, "campaign_group", group.Id, AuditGroupRestored)
	assertUserAuditActor(c, audit, group.UserId)
	assertAuditTimestamp(c, audit)
	metadata := auditMetadata(c, audit)
	c.Assert(metadata["name"], check.Equals, group.Name)
	c.Assert(metadata["original_name"], check.Equals, group.Name)
	c.Assert(metadata["name_changed"], check.Equals, false)
}

func (s *ModelsSuite) TestAuditCampaignGroupPurge(c *check.C) {
	group := createAuditCampaignGroup(c)
	c.Assert(SoftDeleteCampaignGroup(group.Id, group.UserId, "audit group purge"), check.Equals, nil)
	c.Assert(PurgeCampaignGroup(group.Id, group.UserId), check.Equals, nil)

	audit := assertAuditLog(c, "campaign_group", group.Id, AuditGroupPurged)
	assertUserAuditActor(c, audit, group.UserId)
	assertAuditTimestamp(c, audit)
	metadata := auditMetadata(c, audit)
	c.Assert(metadata["name"], check.Equals, group.Name)
	c.Assert(int64(metadata["user_id"].(float64)), check.Equals, group.UserId)

	logs, err := GetAuditLogs("campaign_group", group.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(logs) >= 2, check.Equals, true)
}

func createAuditCampaignGroup(c *check.C) CampaignGroup {
	campaign := (&ModelsSuite{}).createCampaign(c)
	group := CampaignGroup{
		Name:   "Audit campaign group",
		UserId: campaign.UserId,
		Campaigns: []CampaignGroupCampaign{
			{CampaignId: campaign.Id, OrderIndex: 0},
		},
	}
	c.Assert(PostCampaignGroup(&group, campaign.UserId), check.Equals, nil)
	return group
}

func assertAuditLog(c *check.C, entityType string, entityID int64, action string) AuditLog {
	logs, err := GetAuditLogs(entityType, entityID)
	c.Assert(err, check.Equals, nil)
	for _, audit := range logs {
		if audit.Action == action {
			c.Assert(audit.EntityType, check.Equals, entityType)
			c.Assert(audit.EntityID, check.Equals, entityID)
			return audit
		}
	}
	c.Fatalf("expected audit log action %s for %s/%d, got %#v", action, entityType, entityID, logs)
	return AuditLog{}
}

func assertUserAuditActor(c *check.C, audit AuditLog, userID int64) {
	c.Assert(audit.ActorID, check.NotNil)
	c.Assert(*audit.ActorID, check.Equals, userID)
	user, err := GetUser(userID)
	c.Assert(err, check.Equals, nil)
	c.Assert(audit.ActorName, check.Equals, user.Username)
}

func assertAuditTimestamp(c *check.C, audit AuditLog) {
	c.Assert(audit.Timestamp.IsZero(), check.Equals, false)
	c.Assert(audit.Timestamp.After(time.Now().UTC().Add(-5*time.Minute)), check.Equals, true)
}

func auditMetadata(c *check.C, audit AuditLog) map[string]interface{} {
	metadata := map[string]interface{}{}
	c.Assert(json.Unmarshal([]byte(audit.Metadata), &metadata), check.Equals, nil)
	return metadata
}

func TestAuditLogMetadataRoundTrip(t *testing.T) {
	audit := AuditLog{}
	if err := audit.SetMetadata(map[string]interface{}{"reason": "test"}); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}
	metadata, err := audit.GetMetadata()
	if err != nil {
		t.Fatalf("GetMetadata: %v", err)
	}
	if metadata["reason"] != "test" {
		t.Fatalf("unexpected metadata: %#v", metadata)
	}
}
