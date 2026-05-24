package worker

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/gophish/gophish/config"
	"github.com/gophish/gophish/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTrashTTLJob_Defaults(t *testing.T) {
	job := NewTrashTTLJob(TrashTTLConfig{})

	assert.Equal(t, 90, job.retentionDays, "Default retention should be 90 days")
	assert.Equal(t, 1*time.Hour, job.interval, "Default interval should be 1 hour")
	assert.Equal(t, 100, job.batchSize, "Default batch size should be 100")
	assert.False(t, job.enabled, "Default enabled should be false")
}

func TestNewTrashTTLJob_CustomConfig(t *testing.T) {
	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 30,
		Interval:      30 * time.Minute,
		BatchSize:     50,
		Enabled:       true,
	})

	assert.Equal(t, 30, job.retentionDays)
	assert.Equal(t, 30*time.Minute, job.interval)
	assert.Equal(t, 50, job.batchSize)
	assert.True(t, job.enabled)
}

func TestStart_DisabledJob(t *testing.T) {
	job := NewTrashTTLJob(TrashTTLConfig{
		Enabled: false,
	})

	// Should not panic when disabled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	job.Start(ctx)
	time.Sleep(200 * time.Millisecond)

	// No assertions needed - just verify it doesn't crash
}

func TestGetMetrics(t *testing.T) {
	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 30,
		Interval:      15 * time.Minute,
		BatchSize:     75,
		Enabled:       true,
	})

	metrics := job.GetMetrics()
	assert.Equal(t, 30, metrics["retention_days"])
	assert.Equal(t, "15m0s", metrics["interval"])
	assert.Equal(t, 75, metrics["batch_size"])
	assert.Equal(t, true, metrics["enabled"])
}

func TestRunOnce_ContextCancellation(t *testing.T) {
	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 90,
		Enabled:       true,
	})

	// Create context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// RunOnce should handle canceled context gracefully
	// Since database is not initialized in unit test, this will return an error
	err := job.RunOnce(ctx)
	// Should get database error (not initialized in this test context)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

func TestTrashTTLRunOnceDisabledDoesNotPurgeExpiredCampaign(t *testing.T) {
	db := setupTrashTTLTestDB(t)
	campaign := createTrashTTLTestCampaign(t, "ttl disabled")
	softDeleteAndAgeCampaign(t, db, campaign, 120*24*time.Hour)

	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 30,
		BatchSize:     100,
		Enabled:       false,
	})

	require.NoError(t, job.RunOnce(context.Background()))
	assertCampaignStillTrashed(t, campaign.Id, campaign.UserId)
}

func TestTrashTTLRunOnceKeepsRecentTrashWithinRetention(t *testing.T) {
	db := setupTrashTTLTestDB(t)
	campaign := createTrashTTLTestCampaign(t, "ttl recent trash")
	softDeleteAndAgeCampaign(t, db, campaign, 24*time.Hour)

	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 30,
		BatchSize:     100,
		Enabled:       true,
	})

	require.NoError(t, job.RunOnce(context.Background()))
	assertCampaignStillTrashed(t, campaign.Id, campaign.UserId)
}

func TestTrashTTLRunOncePurgesExpiredTrash(t *testing.T) {
	db := setupTrashTTLTestDB(t)
	campaign := createTrashTTLTestCampaign(t, "ttl expired trash")
	softDeleteAndAgeCampaign(t, db, campaign, 31*24*time.Hour)

	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 30,
		BatchSize:     100,
		Enabled:       true,
	})

	require.NoError(t, job.RunOnce(context.Background()))
	assertCampaignPurged(t, campaign.Id, campaign.UserId)
}

func TestTrashTTLRunOnceDoesNotTouchActiveOldCampaign(t *testing.T) {
	db := setupTrashTTLTestDB(t)
	campaign := createTrashTTLTestCampaign(t, "ttl active old")
	_, err := db.Exec("UPDATE campaigns SET created_date = ? WHERE id = ?", time.Now().UTC().Add(-120*24*time.Hour), campaign.Id)
	require.NoError(t, err)

	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 30,
		BatchSize:     100,
		Enabled:       true,
	})

	require.NoError(t, job.RunOnce(context.Background()))
	_, err = models.GetCampaign(campaign.Id, campaign.UserId)
	require.NoError(t, err)
}

func TestTrashTTLRunOncePurgesExpiredDependencies(t *testing.T) {
	db := setupTrashTTLTestDB(t)
	campaign := createTrashTTLTestCampaign(t, "ttl expired dependencies")
	group := createTrashTTLTestCampaignGroup(t, campaign, "ttl dependency group")
	result := campaign.Results[0]
	require.NoError(t, models.AddEvent(&models.Event{Email: result.Email, Message: models.EventSent}, campaign.Id))
	require.NoError(t, models.SaveCalendarEvent(&models.CalendarEvent{ResultId: result.Id, EventType: "ics_sent", Timestamp: time.Now().UTC()}))
	require.NoError(t, models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "ttl dependency test"))
	require.NoError(t, models.GenerateMailLog(&campaign, &result, time.Now().UTC()))
	ageCampaignDeletedAt(t, db, campaign.Id, 31*24*time.Hour)

	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 30,
		BatchSize:     100,
		Enabled:       true,
	})

	require.NoError(t, job.RunOnce(context.Background()))
	assertCampaignPurged(t, campaign.Id, campaign.UserId)
	assertTableCount(t, db, "mail_logs", "campaign_id = ?", 0, campaign.Id)
	assertTableCount(t, db, "results", "campaign_id = ?", 0, campaign.Id)
	assertTableCount(t, db, "events", "campaign_id = ?", 0, campaign.Id)
	assertTableCount(t, db, "calendar_events", "result_id = ?", 0, result.Id)
	assertTableCount(t, db, "campaign_group_campaigns", "campaign_id = ?", 0, campaign.Id)

	got, err := models.GetCampaignGroup(group.Id, campaign.UserId)
	require.NoError(t, err)
	assert.Len(t, got.Campaigns, 0)
}

func TestTrashTTLRunOnceRespectsBatchSize(t *testing.T) {
	db := setupTrashTTLTestDB(t)
	campaigns := make([]models.Campaign, 0, 5)
	for i := 0; i < 5; i++ {
		campaign := createTrashTTLTestCampaign(t, fmt.Sprintf("ttl batch %d", i))
		softDeleteAndAgeCampaign(t, db, campaign, time.Duration(31+i)*24*time.Hour)
		campaigns = append(campaigns, campaign)
	}

	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 30,
		BatchSize:     2,
		Enabled:       true,
	})

	require.NoError(t, job.RunOnce(context.Background()))

	purged := 0
	trashed := 0
	for _, campaign := range campaigns {
		if _, err := models.GetTrashedCampaignByID(campaign.Id, campaign.UserId); err == nil {
			trashed++
		} else {
			purged++
		}
	}
	assert.Equal(t, 2, purged)
	assert.Equal(t, 3, trashed)
}

func TestTrashTTLRunOnceCreatesSystemAuditLog(t *testing.T) {
	db := setupTrashTTLTestDB(t)
	campaign := createTrashTTLTestCampaign(t, "ttl audit")
	softDeleteAndAgeCampaign(t, db, campaign, 31*24*time.Hour)

	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: 30,
		BatchSize:     100,
		Enabled:       true,
	})

	require.NoError(t, job.RunOnce(context.Background()))
	assertTableCount(t, db, "audit_log", "entity_type = ? AND entity_id = ? AND action = ? AND actor_name = ?", 1, "campaign", campaign.Id, models.AuditCampaignPurged, "system:trash-ttl")
}

func TestNewTrashTTLJobInvalidConfigUsesSafeDefaults(t *testing.T) {
	job := NewTrashTTLJob(TrashTTLConfig{
		RetentionDays: -1,
		Interval:      -1 * time.Minute,
		BatchSize:     -1,
		Enabled:       true,
	})

	assert.Equal(t, 90, job.retentionDays)
	assert.Equal(t, 1*time.Hour, job.interval)
	assert.Equal(t, 100, job.batchSize)
	assert.True(t, job.enabled)
}

func setupTrashTTLTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "ttl-test.db")
	require.NoError(t, models.Setup(&config.Config{
		DBName:         "sqlite3",
		DBPath:         dbPath,
		MigrationsPath: "../db/db_sqlite3/migrations/",
	}))
	sqlDB, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB.Close()
	})
	return sqlDB
}

func createTrashTTLTestCampaign(t *testing.T, name string) models.Campaign {
	t.Helper()
	group := models.Group{Name: name + " group", UserId: 1}
	group.Targets = []models.Target{{BaseRecipient: models.BaseRecipient{Email: fmt.Sprintf("%d@example.com", time.Now().UnixNano())}}}
	require.NoError(t, models.PostGroup(&group))

	template := models.Template{Name: name + " template", Subject: "Test subject", Text: "Text", HTML: "<html>Test</html>", UserId: 1}
	require.NoError(t, models.PostTemplate(&template))

	page := models.Page{Name: name + " page", HTML: "<html>Test</html>", UserId: 1}
	require.NoError(t, models.PostPage(&page))

	smtp := models.SMTP{Name: name + " smtp", UserId: 1, Host: "example.com", FromAddress: "test@test.com"}
	require.NoError(t, models.PostSMTP(&smtp))

	campaign := models.Campaign{
		Name:     name,
		UserId:   1,
		Template: template,
		Page:     page,
		SMTP:     smtp,
		Groups:   []models.Group{group},
	}
	require.NoError(t, models.PostCampaign(&campaign, campaign.UserId))
	return campaign
}

func createTrashTTLTestCampaignGroup(t *testing.T, campaign models.Campaign, name string) models.CampaignGroup {
	t.Helper()
	group := models.CampaignGroup{
		Name:   name,
		UserId: campaign.UserId,
		Campaigns: []models.CampaignGroupCampaign{
			{CampaignId: campaign.Id, OrderIndex: 0},
		},
	}
	require.NoError(t, models.PostCampaignGroup(&group, campaign.UserId))
	return group
}

func softDeleteAndAgeCampaign(t *testing.T, db *sql.DB, campaign models.Campaign, age time.Duration) {
	t.Helper()
	require.NoError(t, models.SoftDeleteCampaign(campaign.Id, campaign.UserId, "ttl test"))
	ageCampaignDeletedAt(t, db, campaign.Id, age)
}

func ageCampaignDeletedAt(t *testing.T, db *sql.DB, campaignID int64, age time.Duration) {
	t.Helper()
	deletedAt := time.Now().UTC().Add(-age)
	_, err := db.Exec("UPDATE campaigns SET deleted_at = ? WHERE id = ?", deletedAt, campaignID)
	require.NoError(t, err)
}

func assertCampaignStillTrashed(t *testing.T, campaignID int64, userID int64) {
	t.Helper()
	_, err := models.GetTrashedCampaignByID(campaignID, userID)
	require.NoError(t, err)
}

func assertCampaignPurged(t *testing.T, campaignID int64, userID int64) {
	t.Helper()
	if _, err := models.GetCampaign(campaignID, userID); err == nil {
		t.Fatalf("expected campaign %d to be purged", campaignID)
	}
}

func assertTableCount(t *testing.T, db *sql.DB, table string, where string, expected int, args ...interface{}) {
	t.Helper()
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+where, args...).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, expected, count)
}
