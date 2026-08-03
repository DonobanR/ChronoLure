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

// TestGlobalTrashPurgeAcceptsWhitespaceNormalizedConfirmation reproduces CL-101:
// campaign names carry trailing/double spaces (e.g. "Toledano ") that the browser
// collapses when rendering the confirm modal, so the user can only ever type the
// visible, normalized name. The backend must accept that normalized confirmation.
func TestGlobalTrashPurgeAcceptsWhitespaceNormalizedConfirmation(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "Toledano ")
	if err := models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "test"); err != nil {
		t.Fatalf("SoftDeleteCampaign: %v", err)
	}

	// User types the visible name without the trailing space it can't see.
	w := makeTrashPurgeRequest(t, testCtx, campaign.Id, "Toledano")
	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d got %d body=%s", http.StatusOK, w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err != models.ErrCampaignNotFound {
		t.Fatalf("expected campaign to be purged, got err=%v", err)
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

// TestGlobalTrashPurgeGroupRequiresConfirmation covers CL-101b: purging a
// campaign group via the global trash must also require a matching confirmation,
// so it cannot be bypassed by an empty-body API call.
func TestGlobalTrashPurgeGroupRequiresConfirmation(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "group purge campaign")
	group := createTrashAPITestCampaignGroup(t, campaign, "Grupo Interno  QA ") // messy name on purpose
	if err := models.SoftDeleteCampaignGroup(group.Id, group.UserId, "test"); err != nil {
		t.Fatalf("SoftDeleteCampaignGroup: %v", err)
	}

	// Empty confirmation → 400, group stays.
	empty := makeTrashPurgeRawRequest(t, testCtx, models.TrashTypeCampaignGroup, group.Id, []byte(`{}`))
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty confirmation: expected 400 got %d body=%s", empty.Code, empty.Body.String())
	}

	// Wrong confirmation → 400, group stays.
	wrong := makeTrashPurgeRawRequest(t, testCtx, models.TrashTypeCampaignGroup, group.Id, []byte(`{"confirmation":"otro nombre"}`))
	if wrong.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmation: expected 400 got %d body=%s", wrong.Code, wrong.Body.String())
	}

	// Normalized (visible) name → 200, group purged. Stored has double + trailing spaces.
	ok := makeTrashPurgeRawRequest(t, testCtx, models.TrashTypeCampaignGroup, group.Id, []byte(`{"confirmation":"Grupo Interno QA"}`))
	if ok.Code != http.StatusOK {
		t.Fatalf("correct confirmation: expected 200 got %d body=%s", ok.Code, ok.Body.String())
	}
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

	// Groups now require a confirmation too; a non-owner must still get 404 because
	// the group is not in their trash (the confirmation never resolves for them).
	purge := makeTrashPurgeRawRequestWithAPIKey(t, testCtx, models.TrashTypeCampaignGroup, group.Id, []byte(`{"confirmation":"foreign trashed group"}`), testCtx.apiKey)
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

// TestNormalizeConfirmation locks the purge-confirmation gate against the messy
// real-world campaign names that broke CL-101: trailing/leading and double
// spaces (collapsed by the browser) plus zero-width / formatting characters
// that survive plain whitespace normalization. The user must always be able to
// type the visible name and match.

// TestNormalizeConfirmation locks the purge-confirmation gate against the messy
// real-world campaign names that broke CL-101: trailing/leading and double
// spaces (collapsed by the browser) plus zero-width / formatting characters
// that survive plain whitespace normalization. The user must always be able to
// type the visible name and match. Invisible chars are written as \u escapes so
// the test source stays ASCII.
func TestNormalizeConfirmation(t *testing.T) {
	cases := []struct {
		name      string
		stored    string
		typed     string
		wantEqual bool
	}{
		{"double space in middle (real prod case id=1626)",
			"Copy of Copy of TEST - Seguros Universales -  TESTING",
			"Copy of Copy of TEST - Seguros Universales - TESTING", true},
		{"trailing space", "Toledano ", "Toledano", true},
		{"leading and double space", " campana unipago  2022", "campana unipago 2022", true},
		{"tab and newline collapse", "A\tB\nC", "A B C", true},
		{"nbsp between words", "Campana\u00A0Phishing", "Campana Phishing", true},
		{"zero-width space", "Banco\u200BTest", "BancoTest", true},
		{"BOM prefix", "\uFEFFNordestana", "Nordestana", true},
		{"soft hyphen inside", "Inter\u00ADbanco", "Interbanco", true},
		{"word joiner", "Red\u2060Team", "RedTeam", true},
		{"genuinely different names still differ", "Alpha", "Beta", false},
		{"different only by real letters", "TEST A", "TEST B", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeConfirmation(tc.stored) == normalizeConfirmation(tc.typed)
			if got != tc.wantEqual {
				t.Fatalf("match=%v want=%v\n stored=%q -> %q\n typed =%q -> %q",
					got, tc.wantEqual,
					tc.stored, normalizeConfirmation(tc.stored),
					tc.typed, normalizeConfirmation(tc.typed))
			}
		})
	}
}

// makeRecipientPurgeRequest hits POST /api/trash/recipient/{id}/purge with a raw body.
func makeRecipientPurgeRequest(t *testing.T, testCtx *testContext, resultID int64, body []byte) *httptest.ResponseRecorder {
	url := fmt.Sprintf("/api/trash/recipient/%d/purge", resultID)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, req)
	return w
}

// TestRecipientPurgeRequiresBackendConfirmation (ticket §10, API): purging a
// recipient can never be done by an empty/absent/wrong confirmation — the check
// lives in the backend, not only in the dialog.
func TestRecipientPurgeRequiresBackendConfirmation(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "recipient purge confirmation")
	cr, err := models.GetCampaignResults(campaign.Id, campaign.UserId)
	if err != nil {
		t.Fatalf("GetCampaignResults: %v", err)
	}
	if len(cr.Results) == 0 {
		t.Fatalf("expected at least one recipient")
	}
	victim := cr.Results[0]
	if _, _, err := models.SoftDeleteResults(campaign.Id, []string{victim.RId}, campaign.UserId, "", models.DeleteScopeCampaign); err != nil {
		t.Fatalf("SoftDeleteResults: %v", err)
	}

	// No body → 400, recipient survives.
	if w := makeRecipientPurgeRequest(t, testCtx, victim.Id, []byte(`{}`)); w.Code != http.StatusBadRequest {
		t.Fatalf("empty confirmation: expected 400 got %d body=%s", w.Code, w.Body.String())
	}
	// Wrong email → 400.
	if w := makeRecipientPurgeRequest(t, testCtx, victim.Id, []byte(`{"confirm":"otro@correo.com"}`)); w.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmation: expected 400 got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedRecipientByID(campaign.UserId, victim.Id); err != nil {
		t.Fatalf("recipient should still be in trash after rejected purges, got err=%v", err)
	}

	// Exact email → 200 and gone.
	w := makeRecipientPurgeRequest(t, testCtx, victim.Id, []byte(fmt.Sprintf(`{"confirm":%q}`, victim.Email)))
	if w.Code != http.StatusOK {
		t.Fatalf("correct confirmation: expected 200 got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := models.GetTrashedRecipientByID(campaign.UserId, victim.Id); err != models.ErrResultNotFound {
		t.Fatalf("recipient should be purged and gone from trash, got err=%v", err)
	}
}

// createTrashAPITestCampaignWithEmail creates a campaign whose target list holds
// exactly the given email, so several campaigns can share one recipient (needed to
// exercise scope="group").
func createTrashAPITestCampaignWithEmail(t *testing.T, name, email string) models.Campaign {
	userID := int64(1)
	group := models.Group{Name: fmt.Sprintf("%s group", name), UserId: userID}
	group.Targets = []models.Target{{BaseRecipient: models.BaseRecipient{Email: email}}}
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("PostGroup: %v", err)
	}
	template := models.Template{Name: fmt.Sprintf("%s template", name), Subject: "s", Text: "t",
		HTML: "<html>h</html>", UserId: userID}
	if err := models.PostTemplate(&template); err != nil {
		t.Fatalf("PostTemplate: %v", err)
	}
	page := models.Page{Name: fmt.Sprintf("%s page", name), HTML: "<html>h</html>", UserId: userID}
	if err := models.PostPage(&page); err != nil {
		t.Fatalf("PostPage: %v", err)
	}
	smtp := models.SMTP{Name: fmt.Sprintf("%s smtp", name), UserId: userID, Host: "example.com",
		FromAddress: "test@test.com"}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatalf("PostSMTP: %v", err)
	}
	campaign := models.Campaign{Name: name, UserId: userID, Template: template, Page: page,
		SMTP: smtp, Groups: []models.Group{group}}
	if err := models.PostCampaign(&campaign, userID); err != nil {
		t.Fatalf("PostCampaign: %v", err)
	}
	return campaign
}

// TestBulkDeleteAPIWithGroupScope (102R-b contract) — the bulk-delete endpoint
// honours scope="group": one email selected in one campaign removes it from every
// campaign of the group, under a single batch id the UI can undo.
func TestBulkDeleteAPIWithGroupScope(t *testing.T) {
	testCtx := setupTest(t)
	shared := fmt.Sprintf("interno-%d@example.com", time.Now().UnixNano())
	c1 := createTrashAPITestCampaignWithEmail(t, "bulk group scope A", shared)
	c2 := createTrashAPITestCampaignWithEmail(t, "bulk group scope B", shared)
	group := models.CampaignGroup{Name: "bulk scope group", UserId: c1.UserId,
		Campaigns: []models.CampaignGroupCampaign{
			{CampaignId: c1.Id, OrderIndex: 0}, {CampaignId: c2.Id, OrderIndex: 1},
		}}
	if err := models.PostCampaignGroup(&group, c1.UserId); err != nil {
		t.Fatalf("PostCampaignGroup: %v", err)
	}

	cr1, err := models.GetCampaignResults(c1.Id, c1.UserId)
	if err != nil {
		t.Fatalf("GetCampaignResults: %v", err)
	}
	if len(cr1.Results) != 1 {
		t.Fatalf("expected 1 recipient in c1, got %d", len(cr1.Results))
	}
	body := fmt.Sprintf(`{"result_ids":[%q],"reason":"internos","scope":"group"}`, cr1.Results[0].RId)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/campaigns/%d/results/bulk-delete", c1.Id), bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+testCtx.apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Affected int    `json:"affected"`
		BatchID  string `json:"batch_id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Affected != 2 {
		t.Fatalf("group scope should affect both campaigns, affected=%d", resp.Affected)
	}
	// The whole batch is undoable in one call.
	n, err := models.RestoreResultBatch(c1.UserId, resp.BatchID)
	if err != nil {
		t.Fatalf("RestoreResultBatch: %v", err)
	}
	if n != 2 {
		t.Fatalf("undo restored %d, want 2", n)
	}
}

// TestTrashCountsAfterEachMutation (102R-b sync contract) — the badges endpoint
// must be truthful after EVERY mutation: delete → undo → delete → restore. A stale
// badge destroys trust in the number as much as a wrong computation.
func TestTrashCountsAfterEachMutation(t *testing.T) {
	testCtx := setupTest(t)
	campaign := createTrashAPITestCampaign(t, "counts after mutation")
	cr, err := models.GetCampaignResults(campaign.Id, campaign.UserId)
	if err != nil {
		t.Fatalf("GetCampaignResults: %v", err)
	}
	if len(cr.Results) == 0 {
		t.Fatalf("no recipients")
	}
	victim := cr.Results[0]

	counts := func() models.TrashCounts {
		req := httptest.NewRequest(http.MethodGet, "/api/trash/counts", nil)
		req.Header.Set("Authorization", "Bearer "+testCtx.apiKey)
		w := httptest.NewRecorder()
		testCtx.apiServer.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("counts: expected 200 got %d body=%s", w.Code, w.Body.String())
		}
		var c models.TrashCounts
		if err := json.NewDecoder(w.Body).Decode(&c); err != nil {
			t.Fatalf("decode counts: %v", err)
		}
		return c
	}

	base := counts()

	// delete → recipients +1, batches +1, all +1
	batch, _, err := models.SoftDeleteResults(campaign.Id, []string{victim.RId}, campaign.UserId, "", models.DeleteScopeCampaign)
	if err != nil {
		t.Fatalf("SoftDeleteResults: %v", err)
	}
	afterDelete := counts()
	if afterDelete.Recipients != base.Recipients+1 {
		t.Fatalf("after delete recipients=%d want %d", afterDelete.Recipients, base.Recipients+1)
	}
	if afterDelete.RecipientBatches != base.RecipientBatches+1 {
		t.Fatalf("after delete batches=%d want %d", afterDelete.RecipientBatches, base.RecipientBatches+1)
	}
	if afterDelete.All != base.All+1 {
		t.Fatalf("after delete all=%d want %d", afterDelete.All, base.All+1)
	}

	// undo → back to baseline
	if _, err := models.RestoreResultBatch(campaign.UserId, batch); err != nil {
		t.Fatalf("RestoreResultBatch: %v", err)
	}
	afterUndo := counts()
	if afterUndo.Recipients != base.Recipients || afterUndo.All != base.All {
		t.Fatalf("after undo counts did not return to baseline: %+v vs %+v", afterUndo, base)
	}

	// delete again → restore individually → baseline again
	if _, _, err := models.SoftDeleteResults(campaign.Id, []string{victim.RId}, campaign.UserId, "", models.DeleteScopeCampaign); err != nil {
		t.Fatalf("second SoftDeleteResults: %v", err)
	}
	if counts().Recipients != base.Recipients+1 {
		t.Fatalf("second delete not reflected")
	}
	if err := models.RestoreResultByID(campaign.UserId, victim.Id); err != nil {
		t.Fatalf("RestoreResultByID: %v", err)
	}
	final := counts()
	if final.Recipients != base.Recipients || final.RecipientBatches != base.RecipientBatches || final.All != base.All {
		t.Fatalf("after individual restore counts stale: %+v vs %+v", final, base)
	}
}

// TestDeleteReportAssetRemovesOnlyThatSlot (CL-109) — deleting one slot's image
// leaves every other slot untouched, and the blob is NOT removed (other reports and
// frozen renders may reference the same bytes).
func TestDeleteReportAssetRemovesOnlyThatSlot(t *testing.T) {
	setupTest(t)
	tpl := &models.ReportTemplate{UserId: 1, Name: fmt.Sprintf("tpl-del-%d", time.Now().UnixNano())}
	if err := models.CreateReportTemplate(tpl); err != nil {
		t.Fatalf("CreateReportTemplate: %v", err)
	}
	rep := &models.Report{UserId: 1, SubjectKind: "campaign", SubjectId: 1, TemplateId: tpl.Id, Status: "draft"}
	if err := models.CreateReport(rep); err != nil {
		t.Fatalf("CreateReport: %v", err)
	}
	store := models.NewDBBlobStore()
	sha, err := store.Put([]byte("bytes-compartidos"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	for _, slot := range []string{"figura_1", "figura_2"} {
		if err := models.CreateReportAsset(&models.ReportAsset{
			ReportId: rep.Id, Slot: slot, ContentSha256: sha, Mime: "image/png"}); err != nil {
			t.Fatalf("CreateReportAsset %s: %v", slot, err)
		}
	}

	if err := models.DeleteReportAsset(rep.Id, "figura_1"); err != nil {
		t.Fatalf("DeleteReportAsset: %v", err)
	}
	assets, err := models.GetReportAssets(rep.Id)
	if err != nil {
		t.Fatalf("GetReportAssets: %v", err)
	}
	if len(assets) != 1 || assets[0].Slot != "figura_2" {
		t.Fatalf("solo debía borrarse figura_1, quedó: %+v", assets)
	}
	// El blob sobrevive: otros informes/renders pueden referenciarlo.
	if _, err := store.Get(sha); err != nil {
		t.Fatalf("el blob no debe borrarse: %v", err)
	}
}
