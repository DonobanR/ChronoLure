package worker

import (
	"context"
	"fmt"
	"time"

	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/models"
)

// TrashTTLJob handles automatic purging of campaigns after retention period
type TrashTTLJob struct {
	retentionDays int
	interval      time.Duration
	batchSize     int
	enabled       bool
	stopChan      chan struct{}
}

// TrashTTLConfig configures the TTL job
type TrashTTLConfig struct {
	RetentionDays int           // Days to keep campaigns in trash
	Interval      time.Duration // How often to check for purgeable campaigns
	BatchSize     int           // Max campaigns to purge per batch
	Enabled       bool          // Whether job is enabled
}

// NewTrashTTLJob creates a new TTL job instance
func NewTrashTTLJob(config TrashTTLConfig) *TrashTTLJob {
	// Defaults
	if config.RetentionDays <= 0 {
		config.RetentionDays = 90
	}
	if config.Interval <= 0 {
		config.Interval = 1 * time.Hour
	}
	if config.BatchSize <= 0 {
		config.BatchSize = 100
	}

	return &TrashTTLJob{
		retentionDays: config.RetentionDays,
		interval:      config.Interval,
		batchSize:     config.BatchSize,
		enabled:       config.Enabled,
		stopChan:      make(chan struct{}),
	}
}

// Start begins the TTL job in a goroutine
func (j *TrashTTLJob) Start(ctx context.Context) {
	if !j.enabled {
		log.Info("Trash TTL job is disabled, not starting")
		return
	}

	log.Infof("Starting Trash TTL job (retention=%d days, interval=%v, batch=%d)",
		j.retentionDays, j.interval, j.batchSize)

	// Run immediately on startup
	go func() {
		// Initial run with timeout
		runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		if err := j.RunOnce(runCtx); err != nil {
			log.Errorf("Initial trash TTL run failed: %v", err)
		}
		cancel()

		ticker := time.NewTicker(j.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Run with timeout to prevent infinite hangs
				runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				if err := j.RunOnce(runCtx); err != nil {
					log.Errorf("Trash TTL run failed: %v", err)
				}
				cancel()

			case <-ctx.Done():
				log.Info("Trash TTL job stopped (context canceled)")
				close(j.stopChan)
				return

			case <-j.stopChan:
				log.Info("Trash TTL job stopped (stop signal)")
				return
			}
		}
	}()
}

// Stop gracefully stops the TTL job
func (j *TrashTTLJob) Stop() {
	log.Info("Stopping Trash TTL job...")
	close(j.stopChan)
}

// RunOnce executes a single purge cycle (useful for testing and manual triggers)
func (j *TrashTTLJob) RunOnce(ctx context.Context) error {
	startTime := time.Now()
	
	// Calculate cutoff time
	cutoff := time.Now().Add(-time.Duration(j.retentionDays) * 24 * time.Hour)
	
	log.Debugf("Trash TTL: Looking for campaigns deleted before %s", cutoff.Format(time.RFC3339))

	// Get candidates for purge
	candidateIDs, err := models.ListPurgeCandidates(cutoff, j.batchSize)
	if err != nil {
		return fmt.Errorf("failed to list purge candidates: %w", err)
	}

	if len(candidateIDs) == 0 {
		log.Debug("Trash TTL: No campaigns to purge")
		return nil
	}

	log.Infof("Trash TTL: Found %d campaign(s) to purge", len(candidateIDs))

	// Track metrics
	successCount := 0
	errorCount := 0
	skippedCount := 0

	// Process each campaign
	for i, campaignID := range candidateIDs {
		// Check for cancellation
		select {
		case <-ctx.Done():
			log.Warnf("Trash TTL: Context canceled after %d/%d campaigns", i, len(candidateIDs))
			return ctx.Err()
		default:
		}

		log.Debugf("Trash TTL: Purging campaign %d (%d/%d)", campaignID, i+1, len(candidateIDs))

		err := models.PurgeSystemCampaign(campaignID)
		if err != nil {
			log.Errorf("Trash TTL: Failed to purge campaign %d: %v", campaignID, err)
			errorCount++
			// Continue to next campaign (don't fail entire batch)
			continue
		}

		// Check if it was a no-op (already purged or restored)
		// PurgeSystemCampaign returns nil for idempotent cases
		successCount++
	}

	// Log summary
	duration := time.Since(startTime)
	log.Infof("Trash TTL: Batch complete - %d succeeded, %d errors, %d skipped in %v",
		successCount, errorCount, skippedCount, duration)

	// Return error if entire batch failed
	if errorCount > 0 && successCount == 0 {
		return fmt.Errorf("all %d purge operations failed", errorCount)
	}

	return nil
}

// GetMetrics returns current job metrics (for observability)
func (j *TrashTTLJob) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"retention_days": j.retentionDays,
		"interval":       j.interval.String(),
		"batch_size":     j.batchSize,
		"enabled":        j.enabled,
	}
}
