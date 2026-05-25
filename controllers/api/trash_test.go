package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gophish/gophish/models"
)

func createTrashAPITestCampaign(t *testing.T, name string) models.Campaign {
	return createTrashAPITestCampaignForUser(t, name, 1)
}

func createTrashAPITestCampaignForUser(t *testing.T, name string, userID int64) models.Campaign {
	group := models.Group{Name: fmt.Sprintf("%s group", name)}
	group.Targets = []models.Target{
		{BaseRecipient: models.BaseRecipient{Email: fmt.Sprintf("%d@example.com", time.Now().UnixNano())}},
	}
	group.UserId = userID
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("PostGroup: %v", err)
	}

	template := models.Template{
		Name:    fmt.Sprintf("%s template", name),
		Subject: "Test subject",
		Text:    "Text",
		HTML:    "<html>Test</html>",
		UserId:  userID,
	}
	if err := models.PostTemplate(&template); err != nil {
		t.Fatalf("PostTemplate: %v", err)
	}

	page := models.Page{
		Name:   fmt.Sprintf("%s page", name),
		HTML:   "<html>Test</html>",
		UserId: userID,
	}
	if err := models.PostPage(&page); err != nil {
		t.Fatalf("PostPage: %v", err)
	}

	smtp := models.SMTP{
		Name:        fmt.Sprintf("%s smtp", name),
		UserId:      userID,
		Host:        "example.com",
		FromAddress: "test@test.com",
	}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatalf("PostSMTP: %v", err)
	}

	campaign := models.Campaign{
		Name:     name,
		UserId:   userID,
		Template: template,
		Page:     page,
		SMTP:     smtp,
		Groups:   []models.Group{group},
	}
	if err := models.PostCampaign(&campaign, campaign.UserId); err != nil {
		t.Fatalf("PostCampaign: %v", err)
	}
	return campaign
}

func createTrashAPIUser(t *testing.T, prefix string) models.User {
	role, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatalf("GetRoleBySlug: %v", err)
	}
	user := models.User{
		Username: fmt.Sprintf("%s-user-%d", prefix, time.Now().UnixNano()),
		Hash:     "hash",
		ApiKey:   fmt.Sprintf("%s-api-key-%d", prefix, time.Now().UnixNano()),
		Role:     role,
		RoleID:   role.ID,
	}
	if err := models.PutUser(&user); err != nil {
		t.Fatalf("PutUser: %v", err)
	}
	return user
}

func makeTrashPurgeRequest(t *testing.T, testCtx *testContext, campaignID int64, confirmation string) *httptest.ResponseRecorder {
	body, err := json.Marshal(map[string]string{"confirmation": confirmation})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return makeTrashPurgeRawRequest(t, testCtx, "campaign", campaignID, body)
}

func makeTrashPurgeRawRequest(t *testing.T, testCtx *testContext, itemType string, campaignID int64, body []byte) *httptest.ResponseRecorder {
	return makeTrashPurgeRawRequestWithAPIKey(t, testCtx, itemType, campaignID, body, testCtx.apiKey)
}

func makeTrashPurgeRawRequestWithAPIKey(t *testing.T, testCtx *testContext, itemType string, campaignID int64, body []byte, apiKey string) *httptest.ResponseRecorder {
	url := fmt.Sprintf("/api/trash/%s/%d/purge", itemType, campaignID)
	req := httptest.NewRequest(http.MethodDelete, url, bytes.NewReader(body))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, req)
	return w
}

func makeTrashRestoreRequest(t *testing.T, testCtx *testContext, itemType string, campaignID int64, apiKey string) *httptest.ResponseRecorder {
	url := fmt.Sprintf("/api/trash/%s/%d/restore", itemType, campaignID)
	req := httptest.NewRequest(http.MethodPost, url, nil)
	if apiKey == "" {
		apiKey = testCtx.apiKey
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, req)
	return w
}

func makeTrashListRequest(t *testing.T, testCtx *testContext, filter string) *httptest.ResponseRecorder {
	return makeTrashListRequestWithAPIKey(t, testCtx, filter, testCtx.apiKey)
}

func makeTrashListRequestWithAPIKey(t *testing.T, testCtx *testContext, filter string, apiKey string) *httptest.ResponseRecorder {
	url := "/api/trash"
	if filter != "" {
		url += "?type=" + filter
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, req)
	return w
}

func TestGlobalTrashPurgeRejectsIncorrectConfirmation(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "trash incorrect confirmation")
	if err := models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	w := makeTrashPurgeRequest(t, testCtx, campaign.Id, "wrong name")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected campaign to remain in trash, got err=%v", err)
	}
}

func TestGlobalTrashPurgeRejectsEmptyConfirmation(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "trash empty confirmation")
	if err := models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	w := makeTrashPurgeRequest(t, testCtx, campaign.Id, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected campaign to remain in trash, got err=%v", err)
	}
}

func TestGlobalTrashPurgeRejectsInvalidJSON(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "trash invalid json")
	if err := models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	w := makeTrashPurgeRawRequest(t, testCtx, "campaign", campaign.Id, []byte("{invalid"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected campaign to remain in trash, got err=%v", err)
	}
}

func TestGlobalTrashPurgeRejectsInvalidTrashType(t *testing.T) {
	testCtx := setupTest(t)

	w := makeTrashPurgeRawRequest(t, testCtx, "unknown", 1, []byte(`{"confirmation":"anything"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

func TestGlobalTrashPurgeRejectsActiveCampaign(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "trash active campaign")

	w := makeTrashPurgeRequest(t, testCtx, campaign.Id, campaign.Name)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if _, err := models.GetCampaign(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected active campaign to remain active, got err=%v", err)
	}
}

func TestGlobalTrashPurgeDeletesSoftDeletedCampaignWithCorrectConfirmation(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "trash correct confirmation")
	if err := models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	w := makeTrashPurgeRequest(t, testCtx, campaign.Id, campaign.Name)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	var response models.Response
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !response.Success {
		t.Fatalf("expected success response got %#v", response)
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err != models.ErrCampaignNotFound {
		t.Fatalf("expected campaign to be purged, got err=%v", err)
	}
	ms, err := models.GetMailLogsByCampaign(campaign.Id)
	if err != nil {
		t.Fatalf("GetMailLogsByCampaign: %v", err)
	}
	if len(ms) != 0 {
		t.Fatalf("expected purge to delete mail logs, got %d", len(ms))
	}
}

func TestGlobalTrashPurgeReturnsNotFoundForMissingCampaign(t *testing.T) {
	testCtx := setupTest(t)

	w := makeTrashPurgeRequest(t, testCtx, 999999, "missing")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestGlobalTrashRestoreBasicCampaignCycle(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "restore basic campaign")
	if err := campaign.UpdateStatus(models.CampaignEmailsSent); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	if err := models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "restore test"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	trashed, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId)
	if err != nil {
		t.Fatalf("GetTrashedCampaignByID: %v", err)
	}
	if trashed.DeletedAt == nil || trashed.StatusBeforeDelete != models.CampaignEmailsSent {
		t.Fatalf("expected campaign in trash with saved status, got %#v", trashed)
	}
	assertTrashListContains(t, testCtx, campaign.Id)

	w := makeTrashRestoreRequest(t, testCtx, "campaign", campaign.Id, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	restored, err := models.GetCampaign(campaign.Id, campaign.UserId)
	if err != nil {
		t.Fatalf("expected campaign to appear in campaigns after restore, got err=%v", err)
	}
	if restored.DeletedAt != nil || restored.DeletedBy != nil {
		t.Fatalf("expected deleted fields to be cleared, got deleted_at=%v deleted_by=%v", restored.DeletedAt, restored.DeletedBy)
	}
	if restored.RestoredAt == nil || restored.RestoredBy == nil || *restored.RestoredBy != campaign.UserId {
		t.Fatalf("expected restore metadata, got restored_at=%v restored_by=%v", restored.RestoredAt, restored.RestoredBy)
	}
	if restored.Status != models.CampaignEmailsSent {
		t.Fatalf("expected status %q got %q", models.CampaignEmailsSent, restored.Status)
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err != models.ErrCampaignNotFound {
		t.Fatalf("expected campaign to leave trash, got err=%v", err)
	}
	assertTrashListMissing(t, testCtx, campaign.Id)
}

func TestGlobalTrashRestoreLinkedCampaignPreservesGroupView(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "restore linked campaign")
	if err := campaign.UpdateStatus(models.CampaignEmailsSent); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	group := createTrashAPITestCampaignGroup(t, campaign, "restore linked group")
	if err := models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "restore linked test"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	got, err := models.GetCampaignGroup(group.Id, campaign.UserId)
	if err != nil {
		t.Fatalf("GetCampaignGroup before restore: %v", err)
	}
	if len(got.Campaigns) != 1 {
		t.Fatalf("expected linked campaign while trashed, got %d", len(got.Campaigns))
	}
	trashedLinked := got.Campaigns[0].Campaign
	if trashedLinked.DeletedAt == nil || trashedLinked.Name == "" || trashedLinked.Status == "" || trashedLinked.CreatedDate.IsZero() {
		t.Fatalf("expected trashed linked campaign to render complete data, got %#v", trashedLinked)
	}

	w := makeTrashRestoreRequest(t, testCtx, "campaign", campaign.Id, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	got, err = models.GetCampaignGroup(group.Id, campaign.UserId)
	if err != nil {
		t.Fatalf("GetCampaignGroup after restore: %v", err)
	}
	if len(got.Campaigns) != 1 {
		t.Fatalf("expected linked campaign after restore, got %d", len(got.Campaigns))
	}
	restoredLinked := got.Campaigns[0].Campaign
	if restoredLinked.DeletedAt != nil {
		t.Fatalf("expected restored linked campaign without trash badge data, got deleted_at=%v", restoredLinked.DeletedAt)
	}
	if restoredLinked.Name != campaign.Name || restoredLinked.Status != models.CampaignEmailsSent || restoredLinked.CreatedDate.IsZero() {
		t.Fatalf("expected real restored campaign data, got %#v", restoredLinked)
	}
	assertCampaignGroupLinkExists(t, campaign.Id, group.Id)
}

func TestGlobalTrashPurgeLinkedCampaignRemovesGroupLink(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "purge linked campaign")
	group := createTrashAPITestCampaignGroup(t, campaign, "purge linked group")
	if err := models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "purge linked test"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	w := makeTrashPurgeRequest(t, testCtx, campaign.Id, campaign.Name)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	if _, err := models.GetCampaign(campaign.Id, campaign.UserId); err == nil {
		t.Fatalf("expected campaign to be purged, got err=%v", err)
	}
	ms, err := models.GetMailLogsByCampaign(campaign.Id)
	if err != nil {
		t.Fatalf("GetMailLogsByCampaign: %v", err)
	}
	if len(ms) != 0 {
		t.Fatalf("expected no mail logs, got %d", len(ms))
	}
	got, err := models.GetCampaignGroup(group.Id, campaign.UserId)
	if err != nil {
		t.Fatalf("GetCampaignGroup: %v", err)
	}
	if len(got.Campaigns) != 0 {
		t.Fatalf("expected campaign group to skip purged campaign link, got %#v", got.Campaigns)
	}
	assertCampaignGroupLinkMissing(t, campaign.Id, group.Id)
}

func TestGlobalTrashRestoreReturnsNotFoundForMissingCampaign(t *testing.T) {
	testCtx := setupTest(t)

	w := makeTrashRestoreRequest(t, testCtx, "campaign", 999999, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestGlobalTrashRestoreRejectsActiveCampaign(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "restore active campaign")

	w := makeTrashRestoreRequest(t, testCtx, "campaign", campaign.Id, "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if _, err := models.GetCampaign(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected active campaign to remain active, got err=%v", err)
	}
}

func TestGlobalTrashRestoreRejectsCampaignOwnedByAnotherUser(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "restore forbidden campaign")
	if err := models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "restore forbidden test"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}
	role, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatalf("GetRoleBySlug: %v", err)
	}
	otherUser := &models.User{
		Username: fmt.Sprintf("restore-user-%d", time.Now().UnixNano()),
		Hash:     "bar",
		ApiKey:   fmt.Sprintf("restore-api-key-%d", time.Now().UnixNano()),
		Role:     role,
		RoleID:   role.ID,
	}
	if err := models.PutUser(otherUser); err != nil {
		t.Fatalf("PutUser: %v", err)
	}

	w := makeTrashRestoreRequest(t, testCtx, "campaign", campaign.Id, otherUser.ApiKey)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d got %d body=%s", http.StatusForbidden, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected campaign to remain in trash, got err=%v", err)
	}
}

func TestUserCannotSoftDeleteAnotherUsersCampaign(t *testing.T) {
	testCtx := setupTest(t)
	owner := createTrashAPIUser(t, "campaign-owner")
	campaign := createTrashAPITestCampaignForUser(t, "foreign soft delete campaign", owner.Id)

	w := makeCampaignDeleteRequest(t, testCtx, campaign.Id, []byte(`{"reason":"forbidden"}`))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
	if _, err := models.GetCampaign(campaign.Id, owner.Id); err != nil {
		t.Fatalf("expected campaign to remain active for owner, got err=%v", err)
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, owner.Id); err != models.ErrCampaignNotFound {
		t.Fatalf("expected campaign not to move to trash, got err=%v", err)
	}
}

func TestUserCannotRestoreAnotherUsersCampaign(t *testing.T) {
	testCtx := setupTest(t)
	owner := createTrashAPIUser(t, "campaign-owner")
	campaign := createTrashAPITestCampaignForUser(t, "foreign restore campaign", owner.Id)
	if err := models.SoftDeleteCampaign(campaign.Id, owner.Id, "owner delete"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	w := makeTrashRestoreRequest(t, testCtx, "campaign", campaign.Id, testCtx.apiKey)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d got %d body=%s", http.StatusForbidden, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, owner.Id); err != nil {
		t.Fatalf("expected campaign to remain in trash, got err=%v", err)
	}
}

func TestUserCannotPurgeAnotherUsersCampaign(t *testing.T) {
	testCtx := setupTest(t)
	owner := createTrashAPIUser(t, "campaign-owner")
	campaign := createTrashAPITestCampaignForUser(t, "foreign purge campaign", owner.Id)
	if err := models.SoftDeleteCampaign(campaign.Id, owner.Id, "owner delete"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}
	body, err := json.Marshal(map[string]string{"confirmation": campaign.Name})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	w := makeTrashPurgeRawRequestWithAPIKey(t, testCtx, "campaign", campaign.Id, body, testCtx.apiKey)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, owner.Id); err != nil {
		t.Fatalf("expected campaign to remain in trash, got err=%v", err)
	}
}

func TestNonAdminOwnerCannotPurgeCampaign(t *testing.T) {
	testCtx := setupTest(t)
	owner := createTrashAPIUser(t, "non-admin-owner")
	campaign := createTrashAPITestCampaignForUser(t, "non admin owner purge campaign", owner.Id)
	if err := models.SoftDeleteCampaign(campaign.Id, owner.Id, "owner delete"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}
	body, err := json.Marshal(map[string]string{"confirmation": campaign.Name})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	w := makeTrashPurgeRawRequestWithAPIKey(t, testCtx, "campaign", campaign.Id, body, owner.ApiKey)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status %d got %d body=%s", http.StatusForbidden, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, owner.Id); err != nil {
		t.Fatalf("expected campaign to remain in trash, got err=%v", err)
	}
}

func TestUserCannotSeeAnotherUsersTrashItem(t *testing.T) {
	testCtx := setupTest(t)
	owner := createTrashAPIUser(t, "trash-owner")
	campaign := createTrashAPITestCampaignForUser(t, "foreign trash list campaign", owner.Id)
	if err := models.SoftDeleteCampaign(campaign.Id, owner.Id, "owner delete"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	items := decodeTrashItems(t, makeTrashListRequestWithAPIKey(t, testCtx, "campaign", testCtx.apiKey))
	for _, item := range items {
		if int64(item["id"].(float64)) == campaign.Id && item["type"] == models.TrashTypeCampaign {
			t.Fatalf("unexpected foreign campaign in trash list: %#v", item)
		}
	}
}

func createTrashAPITestCampaignGroup(t *testing.T, campaign models.Campaign, name string) models.CampaignGroup {
	group := models.CampaignGroup{
		Name:   name,
		UserId: campaign.UserId,
		Campaigns: []models.CampaignGroupCampaign{
			{CampaignId: campaign.Id, OrderIndex: 0},
		},
	}
	if err := models.PostCampaignGroup(&group, campaign.UserId); err != nil {
		t.Fatalf("PostCampaignGroup: %v", err)
	}
	return group
}

func TestUserCannotGetAnotherUsersCampaignGroup(t *testing.T) {
	testCtx := setupTest(t)
	owner := createTrashAPIUser(t, "group-owner")
	campaign := createTrashAPITestCampaignForUser(t, "foreign group campaign", owner.Id)
	group := createTrashAPITestCampaignGroup(t, campaign, "foreign group")

	w := makeAPIRequest(t, testCtx, http.MethodGet, fmt.Sprintf("/api/campaign-groups/%d", group.Id), nil, testCtx.apiKey)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestUserCannotGetAnotherUsersCampaignGroupStats(t *testing.T) {
	testCtx := setupTest(t)
	owner := createTrashAPIUser(t, "group-owner")
	campaign := createTrashAPITestCampaignForUser(t, "foreign group stats campaign", owner.Id)
	group := createTrashAPITestCampaignGroup(t, campaign, "foreign stats group")

	w := makeAPIRequest(t, testCtx, http.MethodGet, fmt.Sprintf("/api/campaign-groups/%d/stats", group.Id), nil, testCtx.apiKey)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d got %d body=%s", http.StatusNotFound, w.Code, w.Body.String())
	}
}

func TestUserCannotRestoreOrPurgeAnotherUsersCampaignGroup(t *testing.T) {
	testCtx := setupTest(t)
	owner := createTrashAPIUser(t, "group-owner")
	campaign := createTrashAPITestCampaignForUser(t, "foreign trashed group campaign", owner.Id)
	group := createTrashAPITestCampaignGroup(t, campaign, "foreign trashed group")
	if err := models.SoftDeleteCampaignGroup(group.Id, owner.Id, "owner delete"); err != nil {
		t.Fatalf("SoftDeleteCampaignGroup: %v", err)
	}

	restore := makeTrashRestoreRequest(t, testCtx, models.TrashTypeCampaignGroup, group.Id, testCtx.apiKey)
	if restore.Code != http.StatusNotFound {
		t.Fatalf("expected restore status %d got %d body=%s", http.StatusNotFound, restore.Code, restore.Body.String())
	}
	if _, err := models.RestoreCampaignGroup(group.Id, owner.Id); err != nil {
		t.Fatalf("owner should still be able to restore group, got err=%v", err)
	}
	if err := models.SoftDeleteCampaignGroup(group.Id, owner.Id, "owner delete again"); err != nil {
		t.Fatalf("SoftDeleteCampaignGroup second: %v", err)
	}

	purge := makeTrashPurgeRawRequestWithAPIKey(t, testCtx, models.TrashTypeCampaignGroup, group.Id, []byte(`{}`), testCtx.apiKey)
	if purge.Code != http.StatusNotFound {
		t.Fatalf("expected purge status %d got %d body=%s", http.StatusNotFound, purge.Code, purge.Body.String())
	}
	if _, err := models.RestoreCampaignGroup(group.Id, owner.Id); err != nil {
		t.Fatalf("expected group to remain in trash for owner, got err=%v", err)
	}
}

func makeCampaignDeleteRequest(t *testing.T, testCtx *testContext, campaignID int64, body []byte) *httptest.ResponseRecorder {
	return makeCampaignDeleteRequestWithAPIKey(t, testCtx, campaignID, body, testCtx.apiKey)
}

func makeCampaignDeleteRequestWithAPIKey(t *testing.T, testCtx *testContext, campaignID int64, body []byte, apiKey string) *httptest.ResponseRecorder {
	url := fmt.Sprintf("/api/campaigns/%d", campaignID)
	req := httptest.NewRequest(http.MethodDelete, url, bytes.NewReader(body))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, req)
	return w
}

func makeAPIRequest(t *testing.T, testCtx *testContext, method string, path string, body []byte, apiKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, req)
	return w
}

func TestCampaignDeleteLinkedToCampaignGroupRequiresAcknowledgement(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "linked campaign requires ack")
	group := createTrashAPITestCampaignGroup(t, campaign, "linked ack group")

	w := makeCampaignDeleteRequest(t, testCtx, campaign.Id, []byte(`{}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("expected status %d got %d body=%s", http.StatusConflict, w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["requires_acknowledgement"] != true {
		t.Fatalf("expected requires_acknowledgement=true got %#v", response)
	}
	if int(response["campaign_groups_count"].(float64)) != 1 {
		t.Fatalf("expected campaign_groups_count=1 got %#v", response)
	}

	if _, err := models.GetCampaign(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected campaign to remain active, got err=%v", err)
	}
	assertCampaignGroupLinkExists(t, campaign.Id, group.Id)
}

func TestCampaignDeleteLinkedToCampaignGroupWithAcknowledgementMovesToTrash(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "linked campaign with ack")
	group := createTrashAPITestCampaignGroup(t, campaign, "linked acknowledged group")

	w := makeCampaignDeleteRequest(t, testCtx, campaign.Id, []byte(`{"reason":"test","acknowledge_campaign_groups":true}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected campaign to be in trash, got err=%v", err)
	}
	assertCampaignGroupLinkExists(t, campaign.Id, group.Id)

	got, err := models.GetCampaignGroup(group.Id, campaign.UserId)
	if err != nil {
		t.Fatalf("GetCampaignGroup: %v", err)
	}
	if len(got.Campaigns) != 1 {
		t.Fatalf("expected linked campaign to remain visible, got %d", len(got.Campaigns))
	}
	linked := got.Campaigns[0].Campaign
	if linked.Name == "" || linked.Status == "" || linked.CreatedDate.IsZero() || linked.DeletedAt == nil {
		t.Fatalf("expected complete trashed campaign data, got %#v", linked)
	}
}

func TestCampaignDeleteUnlinkedCampaignStillMovesToTrash(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "unlinked campaign delete")

	w := makeCampaignDeleteRequest(t, testCtx, campaign.Id, []byte(`{}`))
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected campaign to be in trash, got err=%v", err)
	}
}

func TestCampaignDeleteInvalidJSONReturnsBadRequest(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "invalid json campaign delete")

	w := makeCampaignDeleteRequest(t, testCtx, campaign.Id, []byte(`{invalid`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d got %d body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}
	if _, err := models.GetCampaign(campaign.Id, campaign.UserId); err != nil {
		t.Fatalf("expected campaign to remain active, got err=%v", err)
	}
}

func assertCampaignGroupLinkExists(t *testing.T, campaignID int64, groupID int64) {
	t.Helper()
	got, err := models.GetCampaignGroup(groupID, 1)
	if err != nil {
		t.Fatalf("GetCampaignGroup: %v", err)
	}
	for _, cgc := range got.Campaigns {
		if cgc.CampaignId == campaignID {
			return
		}
	}
	t.Fatalf("expected campaign_group_campaigns link for campaign %d in group %d", campaignID, groupID)
}

func assertCampaignGroupLinkMissing(t *testing.T, campaignID int64, groupID int64) {
	t.Helper()
	got, err := models.GetCampaignGroupsForCampaign(campaignID, 1)
	if err != nil {
		t.Fatalf("GetCampaignGroupsForCampaign: %v", err)
	}
	for _, group := range got {
		if group.Id == groupID {
			t.Fatalf("expected no campaign_group_campaigns link for campaign %d in group %d", campaignID, groupID)
		}
	}
}

func assertTrashListContains(t *testing.T, testCtx *testContext, campaignID int64) {
	t.Helper()
	items := decodeTrashItems(t, makeTrashListRequest(t, testCtx, "campaign"))
	for _, item := range items {
		if int64(item["id"].(float64)) == campaignID {
			if item["type"] != "campaign" || item["name"] == "" || item["deleted_at"] == "" {
				t.Fatalf("expected complete trash item for campaign %d, got %#v", campaignID, item)
			}
			return
		}
	}
	t.Fatalf("expected campaign %d in trash list, got %#v", campaignID, items)
}

func assertTrashListMissing(t *testing.T, testCtx *testContext, campaignID int64) {
	t.Helper()
	items := decodeTrashItems(t, makeTrashListRequest(t, testCtx, "campaign"))
	for _, item := range items {
		if int64(item["id"].(float64)) == campaignID {
			t.Fatalf("expected campaign %d to be absent from trash list, got %#v", campaignID, item)
		}
	}
}

func decodeTrashItems(t *testing.T, w *httptest.ResponseRecorder) []map[string]interface{} {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("expected trash list status %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	var response struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode trash list: %v", err)
	}
	return response.Items
}
