package worker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
