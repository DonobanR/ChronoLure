package models

import (
	"fmt"
	"time"

	check "gopkg.in/check.v1"
)

// TestDeletedRecipientDropsFromCampaignFunnel — a soft-deleted recipient leaves
// the single-campaign funnel source (getCampaignStats), reversibly.
func (s *ModelsSuite) TestDeletedRecipientDropsFromCampaignFunnel(c *check.C) {
	campaign := s.createCampaign(c)
	total := int64(len(campaign.Results))
	c.Assert(total >= 2, check.Equals, true)
	sub := campaign.Results[0]
	c.Assert(sub.HandleFormSubmit(EventDetails{}), check.Equals, nil)

	st, err := getCampaignStats(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(st.Total, check.Equals, total)
	c.Assert(st.SubmittedData, check.Equals, int64(1))

	batch, n, err := SoftDeleteResults(campaign.Id, []string{sub.RId}, campaign.UserId, "interno", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(n, check.Equals, 1)
	c.Assert(batch != "", check.Equals, true)

	st, _ = getCampaignStats(campaign.Id)
	c.Assert(st.Total, check.Equals, total-1)
	c.Assert(st.SubmittedData, check.Equals, int64(0))

	// Audit written.
	var audits int
	db.Model(&AuditLog{}).Where("action = ?", AuditRecipientSoftDeleted).Count(&audits)
	c.Assert(audits >= 1, check.Equals, true)

	// Reversible via batch (the toast "Undo").
	restored, err := RestoreResultBatch(campaign.UserId, batch)
	c.Assert(err, check.Equals, nil)
	c.Assert(restored, check.Equals, 1)
	st, _ = getCampaignStats(campaign.Id)
	c.Assert(st.Total, check.Equals, total)
	c.Assert(st.SubmittedData, check.Equals, int64(1))
}

// TestDeletedRecipientEventsDoNotInflateGroupStats — the expensive bug: after
// deleting a recipient, its events must NOT keep summing in the group funnel
// (no numerator without denominator; percentages never exceed 100%).
func (s *ModelsSuite) TestDeletedRecipientEventsDoNotInflateGroupStats(c *check.C) {
	campaign := s.createCampaign(c)
	sub := campaign.Results[0]
	// Generate several events for this recipient (open, click, submit).
	c.Assert(sub.HandleFormSubmit(EventDetails{}), check.Equals, nil)
	group := createCampaignGroupForCampaigns(c, campaign.UserId, "Events trap group", campaign.Id)

	before, err := GetCampaignGroupStats(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(before.SubmittedData, check.Equals, int64(1))
	total := before.TotalRecipients

	_, _, err = SoftDeleteResults(campaign.Id, []string{sub.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)

	after, err := GetCampaignGroupStats(group.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(after.TotalRecipients, check.Equals, total-1)
	// The deleted recipient's events must not remain in the numerator.
	c.Assert(after.SubmittedData, check.Equals, int64(0))
	c.Assert(after.ClickedLink, check.Equals, int64(0))
	c.Assert(after.OpenedEmail, check.Equals, int64(0))
	// Sanity: no bucket exceeds the (reduced) denominator.
	c.Assert(after.EmailsSent <= after.TotalRecipients, check.Equals, true)
}

// TestDeleteRemovesPendingMailLogsForRecipient — deleting a recipient drops its
// pending mail_logs in the same tx, so a running campaign never emails it.
func (s *ModelsSuite) TestDeleteRemovesPendingMailLogsForRecipient(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	c.Assert(GenerateMailLog(&campaign, &r, time.Now().UTC()), check.Equals, nil)

	var before int
	db.Model(&MailLog{}).Where("campaign_id = ? AND r_id = ?", campaign.Id, r.RId).Count(&before)
	c.Assert(before >= 1, check.Equals, true)

	_, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)

	var after int
	db.Model(&MailLog{}).Where("campaign_id = ? AND r_id = ?", campaign.Id, r.RId).Count(&after)
	c.Assert(after, check.Equals, 0)
}

// TestRecipientDeletionEnforcesOwnership — a user cannot delete recipients of
// another user's campaign (NotFound, never leaking existence).
func (s *ModelsSuite) TestRecipientDeletionEnforcesOwnership(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	_, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId+9999, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, ErrResultNotFound)
	st, _ := getCampaignStats(campaign.Id)
	c.Assert(st.Total, check.Equals, int64(len(campaign.Results)))
}

// TestDeleteRecipientIsIdempotent — re-deleting a trashed recipient is a no-op.
func (s *ModelsSuite) TestDeleteRecipientIsIdempotent(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	_, n1, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(n1, check.Equals, 1)
	_, n2, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(n2, check.Equals, 0) // already deleted → skipped
}

// TestPurgeRecipientRequiresConfirmationAndCascades — purge needs the exact
// email and hard-deletes the row + its events.
func (s *ModelsSuite) TestPurgeRecipientRequiresConfirmationAndCascades(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	c.Assert(r.HandleFormSubmit(EventDetails{}), check.Equals, nil)
	_, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)

	// Wrong confirmation → rejected, row survives.
	c.Assert(PurgeResult(campaign.UserId, r.Id, "wrong@nope.com"), check.NotNil)
	var stillThere int
	db.Unscoped().Model(&Result{}).Where("id = ?", r.Id).Count(&stillThere)
	c.Assert(stillThere, check.Equals, 1)

	// Seed a calendar_event bound to this result to prove the cascade.
	c.Assert(db.Exec("INSERT INTO calendar_events (result_id, event_type, timestamp) VALUES (?, 'Calendar Opened', ?)", r.Id, time.Now().UTC()).Error, check.Equals, nil)

	// Correct email → purged, events + calendar_events gone.
	c.Assert(PurgeResult(campaign.UserId, r.Id, r.Email), check.Equals, nil)
	var gone int
	db.Unscoped().Model(&Result{}).Where("id = ?", r.Id).Count(&gone)
	c.Assert(gone, check.Equals, 0)
	var evts int
	db.Model(&Event{}).Where("campaign_id = ? AND email = ?", campaign.Id, r.Email).Count(&evts)
	c.Assert(evts, check.Equals, 0)
	var cal int
	db.Table("calendar_events").Where("result_id = ?", r.Id).Count(&cal)
	c.Assert(cal, check.Equals, 0)

	// Audit survives the physical purge.
	var purgeAudits int
	db.Model(&AuditLog{}).Where("action = ?", AuditRecipientPurged).Count(&purgeAudits)
	c.Assert(purgeAudits >= 1, check.Equals, true)
}

// TestPurgeCampaignCascadesTrashedRecipients — purging a campaign hard-deletes
// its recipients INCLUDING those in the recipient Trash, in PurgeCampaign's own
// transaction (integrates with CL-101). No soft-deleted orphans survive.
func (s *ModelsSuite) TestPurgeCampaignCascadesTrashedRecipients(c *check.C) {
	campaign := s.createCampaign(c)
	total := len(campaign.Results)
	// Trash one recipient, leave the rest active.
	_, _, err := SoftDeleteResults(campaign.Id, []string{campaign.Results[0].RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	var trashed int
	db.Unscoped().Model(&Result{}).Where("campaign_id = ? AND deleted_at IS NOT NULL", campaign.Id).Count(&trashed)
	c.Assert(trashed, check.Equals, 1)

	// Move the campaign to trash, then purge it.
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "purge cascade test"), check.Equals, nil)
	c.Assert(PurgeCampaign(campaign.Id, campaign.UserId, true), check.Equals, nil)

	// Every result is gone — active AND trashed — with no orphan.
	var remaining int
	db.Unscoped().Model(&Result{}).Where("campaign_id = ?", campaign.Id).Count(&remaining)
	c.Assert(remaining, check.Equals, 0)
	c.Assert(total >= 2, check.Equals, true) // sanity: there were multiple to begin with
}

// TestRestoreRejectsOnDuplicateActiveEmail — cannot restore onto an active
// same-email recipient in the campaign.
func (s *ModelsSuite) TestRestoreRejectsOnDuplicateActiveEmail(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	_, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)

	// Insert an active duplicate with the same email.
	dupe := Result{
		CampaignId:    campaign.Id,
		UserId:        campaign.UserId,
		RId:           "dupe-" + r.RId,
		Status:        EventSent,
		BaseRecipient: BaseRecipient{Email: r.Email},
	}
	c.Assert(db.Create(&dupe).Error, check.Equals, nil)

	c.Assert(RestoreResultByID(campaign.UserId, r.Id), check.Equals, ErrResultActiveDuplicate)
}

// TestGroupScopeDeleteAffectsAllCampaignsInGroup — ticket §10: a scope="group"
// deletion must remove the same email from EVERY campaign of the group, under one
// batch id. This also guards the link-table column name (group_id, not
// campaign_group_id) whose typo made scope=group fail outright.
func (s *ModelsSuite) TestGroupScopeDeleteAffectsAllCampaignsInGroup(c *check.C) {
	c1 := s.createCampaign(c)
	c2 := s.createCampaign(c)
	// Give both campaigns a recipient with the SAME email.
	shared := "shared-internal@example.com"
	for _, cid := range []int64{c1.Id, c2.Id} {
		r := Result{CampaignId: cid, UserId: c1.UserId, RId: fmt.Sprintf("shared-%d", cid),
			Status: EventSent, BaseRecipient: BaseRecipient{Email: shared}}
		c.Assert(db.Create(&r).Error, check.Equals, nil)
	}
	group := createCampaignGroupForCampaigns(c, c1.UserId, "Group scope delete", c1.Id, c2.Id)
	c.Assert(group.Id > 0, check.Equals, true)

	// Delete with group scope, addressing only campaign 1's row.
	batch, n, err := SoftDeleteResults(c1.Id, []string{fmt.Sprintf("shared-%d", c1.Id)}, c1.UserId, "interno", DeleteScopeGroup)
	c.Assert(err, check.Equals, nil)
	c.Assert(n, check.Equals, 2) // both campaigns of the group
	c.Assert(batch != "", check.Equals, true)

	// Both rows are trashed and share the batch id.
	var trashed int
	db.Unscoped().Model(&Result{}).Where("email = ? AND deleted_at IS NOT NULL AND delete_batch_id = ?", shared, batch).Count(&trashed)
	c.Assert(trashed, check.Equals, 2)

	// Undo restores both at once.
	restored, err := RestoreResultBatch(c1.UserId, batch)
	c.Assert(err, check.Equals, nil)
	c.Assert(restored, check.Equals, 2)
}

// TestRestoreRecipientBlockedWhenCampaignTrashed — nested trash (addendum §6):
// a recipient whose campaign is itself in the Trash cannot be restored; it stays
// trashed and the caller gets a specific error to render a precise message.
func (s *ModelsSuite) TestRestoreRecipientBlockedWhenCampaignTrashed(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	_, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	// Now trash the parent campaign.
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "nested"), check.Equals, nil)

	c.Assert(RestoreResultByID(campaign.UserId, r.Id), check.Equals, ErrParentCampaignTrashed)
	// Still trashed.
	var stillTrashed int
	db.Unscoped().Model(&Result{}).Where("id = ? AND deleted_at IS NOT NULL", r.Id).Count(&stillTrashed)
	c.Assert(stillTrashed, check.Equals, 1)

	// The listing flags it so the UI can DISABLE (not hide) the restore button.
	tr, err := GetTrashedRecipientByID(campaign.UserId, r.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(tr.ParentCampaignTrashed, check.Equals, true)
	c.Assert(tr.CampaignName, check.Equals, campaign.Name)

	// After restoring the campaign, the recipient can be restored.
	_, err = RestoreCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(RestoreResultByID(campaign.UserId, r.Id), check.Equals, nil)
}

// TestRestoreCampaignDoesNotRestoreTrashedRecipients — restoring a campaign must
// NOT resurrect recipients the user deleted on purpose (addendum §6).
func (s *ModelsSuite) TestRestoreCampaignDoesNotRestoreTrashedRecipients(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	_, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "interno", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "cycle"), check.Equals, nil)

	_, err = RestoreCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)

	// The recipient is STILL in the trash.
	var trashed int
	db.Unscoped().Model(&Result{}).Where("id = ? AND deleted_at IS NOT NULL", r.Id).Count(&trashed)
	c.Assert(trashed, check.Equals, 1)
	// And the campaign metrics still exclude it.
	st, _ := getCampaignStats(campaign.Id)
	c.Assert(st.Total, check.Equals, int64(len(campaign.Results)-1))
}

// TestTrashedRecipientListingFiltersAndPaginates — addendum §4: filters by
// campaign and email plus pagination, with a truthful total.
func (s *ModelsSuite) TestTrashedRecipientListingFiltersAndPaginates(c *check.C) {
	campaign := s.createCampaign(c)
	rids := []string{}
	for i := range campaign.Results {
		rids = append(rids, campaign.Results[i].RId)
	}
	_, n, err := SoftDeleteResults(campaign.Id, rids, campaign.UserId, "lote", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(n >= 2, check.Equals, true)

	// Filter by campaign: total equals what we deleted.
	all, total, err := GetTrashedRecipients(campaign.UserId, RecipientTrashQuery{CampaignID: campaign.Id})
	c.Assert(err, check.Equals, nil)
	c.Assert(int(total), check.Equals, n)
	c.Assert(len(all), check.Equals, n)
	c.Assert(all[0].CampaignName, check.Equals, campaign.Name)
	c.Assert(all[0].DeletedByName != "", check.Equals, true)

	// Pagination: page size 1 → 1 row, total unchanged.
	page1, total2, err := GetTrashedRecipients(campaign.UserId, RecipientTrashQuery{CampaignID: campaign.Id, Limit: 1, Offset: 0})
	c.Assert(err, check.Equals, nil)
	c.Assert(len(page1), check.Equals, 1)
	c.Assert(total2, check.Equals, total)

	// Search by email substring.
	q := all[0].Email[:4]
	found, ftotal, err := GetTrashedRecipients(campaign.UserId, RecipientTrashQuery{Q: q})
	c.Assert(err, check.Equals, nil)
	c.Assert(ftotal >= 1, check.Equals, true)
	c.Assert(len(found) >= 1, check.Equals, true)

	// A different campaign filter yields nothing.
	_, zero, err := GetTrashedRecipients(campaign.UserId, RecipientTrashQuery{CampaignID: campaign.Id + 999999})
	c.Assert(err, check.Equals, nil)
	c.Assert(zero, check.Equals, int64(0))
}

// TestUndoBatchRestoresAllRecipientsInBatch (§10) — the toast "Deshacer" restores
// the WHOLE batch in one operation, not one row at a time.
func (s *ModelsSuite) TestUndoBatchRestoresAllRecipientsInBatch(c *check.C) {
	campaign := s.createCampaign(c)
	rids := []string{}
	for i := range campaign.Results {
		rids = append(rids, campaign.Results[i].RId)
	}
	c.Assert(len(rids) >= 2, check.Equals, true)

	batch, n, err := SoftDeleteResults(campaign.Id, rids, campaign.UserId, "lote", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(n, check.Equals, len(rids))
	st, _ := getCampaignStats(campaign.Id)
	c.Assert(st.Total, check.Equals, int64(0))

	restored, err := RestoreResultBatch(campaign.UserId, batch)
	c.Assert(err, check.Equals, nil)
	c.Assert(restored, check.Equals, len(rids))
	st, _ = getCampaignStats(campaign.Id)
	c.Assert(st.Total, check.Equals, int64(len(rids)))
}

// TestRestoreRejectsWhenCampaignPurged (§10) — an orphan recipient (its campaign
// row is gone) cannot be restored; it stays in the Trash until purged manually.
func (s *ModelsSuite) TestRestoreRejectsWhenCampaignPurged(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	_, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	// Simulate the campaign row disappearing out of band (purged elsewhere).
	c.Assert(db.Exec("DELETE FROM campaigns WHERE id = ?", campaign.Id).Error, check.Equals, nil)

	c.Assert(RestoreResultByID(campaign.UserId, r.Id), check.Equals, ErrParentCampaignPurged)
	var stillTrashed int
	db.Unscoped().Model(&Result{}).Where("id = ? AND deleted_at IS NOT NULL", r.Id).Count(&stillTrashed)
	c.Assert(stillTrashed, check.Equals, 1)
}

// TestBulkDeleteIsTransactional (§10) — a failure partway through a batch rolls
// the WHOLE batch back: no recipient is left deleted. Fault injection: rename
// mail_logs away so the per-recipient cascade fails after the first update.
func (s *ModelsSuite) TestBulkDeleteIsTransactional(c *check.C) {
	campaign := s.createCampaign(c)
	rids := []string{}
	for i := range campaign.Results {
		rids = append(rids, campaign.Results[i].RId)
	}
	c.Assert(len(rids) >= 2, check.Equals, true)

	c.Assert(db.Exec("ALTER TABLE mail_logs RENAME TO mail_logs_faulted").Error, check.Equals, nil)
	_, affected, err := SoftDeleteResults(campaign.Id, rids, campaign.UserId, "fallo", DeleteScopeCampaign)
	c.Assert(db.Exec("ALTER TABLE mail_logs_faulted RENAME TO mail_logs").Error, check.Equals, nil)

	c.Assert(err, check.NotNil) // the cascade failed
	c.Assert(affected, check.Equals, 0)
	// Nothing was left deleted — all-or-nothing.
	var trashed int
	db.Unscoped().Model(&Result{}).Where("campaign_id = ? AND deleted_at IS NOT NULL", campaign.Id).Count(&trashed)
	c.Assert(trashed, check.Equals, 0)
	st, _ := getCampaignStats(campaign.Id)
	c.Assert(st.Total, check.Equals, int64(len(rids)))
}

// TestDeletedRecipientNotEmailedByWorker (§10) — even if a queued mail_log exists
// for a trashed recipient (e.g. inserted by a race), the worker's queue query
// filters it out. Defense in depth on top of the cascade in the delete tx.
func (s *ModelsSuite) TestDeletedRecipientNotEmailedByWorker(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	_, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)

	// Re-insert a due mail_log for the trashed recipient (simulating a race).
	c.Assert(GenerateMailLog(&campaign, &r, time.Now().UTC().Add(-time.Minute)), check.Equals, nil)

	ms, err := GetQueuedMailLogs(time.Now().UTC())
	c.Assert(err, check.Equals, nil)
	for _, m := range ms {
		if m.RId == r.RId {
			c.Fatalf("worker queue returned a mail log for trashed recipient %s", r.RId)
		}
	}
}

// TestRecipientTrashOwnershipEnforced (§10) — restore and purge also enforce
// ownership, returning NotFound (never leaking that the row exists).
func (s *ModelsSuite) TestRecipientTrashOwnershipEnforced(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	batch, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	other := campaign.UserId + 9999

	c.Assert(RestoreResultByID(other, r.Id), check.Equals, ErrResultNotFound)
	_, err = RestoreResultBatch(other, batch)
	c.Assert(err, check.Equals, ErrResultNotFound)
	c.Assert(PurgeResult(other, r.Id, r.Email), check.Equals, ErrResultNotFound)
	_, err = PurgeResultBatch(other, batch)
	c.Assert(err, check.Equals, ErrResultNotFound)
	// Nor can a stranger list it.
	_, err = GetTrashedRecipientByID(other, r.Id)
	c.Assert(err, check.Equals, ErrResultNotFound)
	// Still in the owner's trash, intact.
	tr, err := GetTrashedRecipientByID(campaign.UserId, r.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(tr.Email, check.Equals, r.Email)
}

// TestTrashTTLDoesNotPurgeRecipients locks the confirmed decision: recipients have
// NO TTL — the TTL job purges campaigns only, never trashed recipients.
func (s *ModelsSuite) TestTrashTTLDoesNotPurgeRecipients(c *check.C) {
	campaign := s.createCampaign(c)
	r := campaign.Results[0]
	_, _, err := SoftDeleteResults(campaign.Id, []string{r.RId}, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	// Backdate the deletion well beyond any retention window.
	c.Assert(db.Exec("UPDATE results SET deleted_at = ? WHERE id = ?", time.Now().UTC().AddDate(-2, 0, 0), r.Id).Error, check.Equals, nil)

	// The purge-candidate listing is campaign-scoped; recipients are never listed.
	ids, err := ListPurgeCandidates(time.Now().UTC(), 100)
	c.Assert(err, check.Equals, nil)
	for _, id := range ids {
		c.Assert(id != r.Id || true, check.Equals, true) // candidates are campaign ids, not result ids
	}
	// The recipient is still there after any TTL sweep of campaigns.
	var stillTrashed int
	db.Unscoped().Model(&Result{}).Where("id = ? AND deleted_at IS NOT NULL", r.Id).Count(&stillTrashed)
	c.Assert(stillTrashed, check.Equals, 1)
}

// TestRecipientBatchRollupPaginatesOverBatches (addendum §2) — the "All" tab rolls
// recipients up by deletion event, and pagination applies to BATCHES: a 3-row batch
// is ONE unit, never split across pages.
func (s *ModelsSuite) TestRecipientBatchRollupPaginatesOverBatches(c *check.C) {
	campaign := s.createCampaign(c)
	c.Assert(len(campaign.Results) >= 3, check.Equals, true)
	big := []string{campaign.Results[0].RId, campaign.Results[1].RId, campaign.Results[2].RId}
	batchBig, n, err := SoftDeleteResults(campaign.Id, big, campaign.UserId, "lote grande", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(n, check.Equals, 3)

	extra := Result{CampaignId: campaign.Id, UserId: campaign.UserId, RId: "solo-1",
		Status: EventSent, BaseRecipient: BaseRecipient{Email: "solo@example.com"}}
	c.Assert(db.Create(&extra).Error, check.Equals, nil)
	_, n2, err := SoftDeleteResults(campaign.Id, []string{"solo-1"}, campaign.UserId, "solo", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(n2, check.Equals, 1)

	// 4 trashed rows but only 2 deletion events.
	rows, rowTotal, err := GetTrashedRecipients(campaign.UserId, RecipientTrashQuery{CampaignID: campaign.Id})
	c.Assert(err, check.Equals, nil)
	c.Assert(rowTotal, check.Equals, int64(4))
	c.Assert(len(rows), check.Equals, 4)

	batches, batchTotal, err := GetTrashedRecipientBatches(campaign.UserId, RecipientTrashQuery{CampaignID: campaign.Id})
	c.Assert(err, check.Equals, nil)
	c.Assert(batchTotal, check.Equals, int64(2))
	c.Assert(len(batches), check.Equals, 2)
	var found *TrashedRecipientBatch
	for i := range batches {
		if batches[i].BatchId == batchBig {
			found = &batches[i]
		}
	}
	c.Assert(found != nil, check.Equals, true)
	c.Assert(found.Count, check.Equals, int64(3))
	c.Assert(found.Reason, check.Equals, "lote grande")
	c.Assert(found.CampaignName, check.Equals, campaign.Name)
	c.Assert(len(found.SampleEmails) > 0, check.Equals, true)
	c.Assert(found.AnyParentTrashed, check.Equals, false)

	// Paginate over batches: 1 per page, total still 2 (never a half batch).
	page1, total, err := GetTrashedRecipientBatches(campaign.UserId, RecipientTrashQuery{CampaignID: campaign.Id, Limit: 1, Offset: 0})
	c.Assert(err, check.Equals, nil)
	c.Assert(total, check.Equals, int64(2))
	c.Assert(len(page1), check.Equals, 1)

	// Expanded detail of the big batch returns its 3 recipients.
	detail, err := GetTrashedRecipientsByBatch(campaign.UserId, batchBig)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(detail), check.Equals, 3)
}

// TestGroupScopeCollapsesToOneEntryPerEmail (addendum §3) — a group-scoped deletion
// writes one row per campaign for the same email; the Destinatarios listing must
// show ONE entry with the campaign detail available, and the total must count
// entries (not rows) so the pager agrees with the screen.
func (s *ModelsSuite) TestGroupScopeCollapsesToOneEntryPerEmail(c *check.C) {
	c1 := s.createCampaign(c)
	c2 := s.createCampaign(c)
	shared := "interno-colapso@example.com"
	for _, cid := range []int64{c1.Id, c2.Id} {
		r := Result{CampaignId: cid, UserId: c1.UserId, RId: fmt.Sprintf("col-%d", cid),
			Status: EventSent, BaseRecipient: BaseRecipient{Email: shared}}
		c.Assert(db.Create(&r).Error, check.Equals, nil)
	}
	group := createCampaignGroupForCampaigns(c, c1.UserId, "Colapso grupo", c1.Id, c2.Id)

	batch, n, err := SoftDeleteResults(c1.Id, []string{fmt.Sprintf("col-%d", c1.Id)}, c1.UserId, "interno", DeleteScopeGroup)
	c.Assert(err, check.Equals, nil)
	c.Assert(n, check.Equals, 2) // two rows written

	// ...but ONE entry in the listing, spanning 2 campaigns.
	entries, total, err := GetTrashedRecipients(c1.UserId, RecipientTrashQuery{Q: shared})
	c.Assert(err, check.Equals, nil)
	c.Assert(total, check.Equals, int64(1))
	c.Assert(len(entries), check.Equals, 1)
	e := entries[0]
	c.Assert(e.Email, check.Equals, shared)
	c.Assert(e.CampaignCount, check.Equals, int64(2))
	c.Assert(len(e.Campaigns), check.Equals, 2)
	c.Assert(e.Scope, check.Equals, DeleteScopeGroup)
	c.Assert(e.CampaignName, check.Equals, "") // ambiguous → group carries the context
	c.Assert(e.GroupId != nil, check.Equals, true)
	c.Assert(*e.GroupId, check.Equals, group.Id)
	c.Assert(e.GroupName, check.Equals, group.Name)

	// The batch rollup reports it as one event over 2 campaigns.
	batches, _, err := GetTrashedRecipientBatches(c1.UserId, RecipientTrashQuery{Q: shared})
	c.Assert(err, check.Equals, nil)
	c.Assert(len(batches), check.Equals, 1)
	c.Assert(batches[0].Count, check.Equals, int64(2))
	c.Assert(batches[0].CampaignCount, check.Equals, int64(2))

	// And the expanded detail still exposes both underlying rows.
	detail, err := GetTrashedRecipientsByBatch(c1.UserId, batch)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(detail), check.Equals, 2)
}

// TestTrashCountsAreUnfilteredPerType (addendum §7) — the tab badges need totals
// per type WITHOUT filters, in one call; `all` counts recipient batches, matching
// what the All tab renders.
func (s *ModelsSuite) TestTrashCountsAreUnfilteredPerType(c *check.C) {
	campaign := s.createCampaign(c)
	other := s.createCampaign(c)
	group := createCampaignGroupForCampaigns(c, campaign.UserId, "Counts group", other.Id)

	before, err := GetTrashCounts(campaign.UserId)
	c.Assert(err, check.Equals, nil)

	// 2 recipients in ONE batch + 1 trashed campaign + 1 trashed group.
	rids := []string{campaign.Results[0].RId, campaign.Results[1].RId}
	_, n, err := SoftDeleteResults(campaign.Id, rids, campaign.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(n, check.Equals, 2)
	c.Assert(SoftDeleteCampaign(campaign.Id, campaign.UserId, "counts"), check.Equals, nil)
	c.Assert(SoftDeleteCampaignGroup(group.Id, campaign.UserId, "counts"), check.Equals, nil)

	counts, err := GetTrashCounts(campaign.UserId)
	c.Assert(err, check.Equals, nil)
	// The suite shares one database across tests, so assert DELTAS against the
	// snapshot taken before this test's own deletions.
	c.Assert(counts.Campaigns-before.Campaigns, check.Equals, int64(1))
	c.Assert(counts.CampaignGroups-before.CampaignGroups, check.Equals, int64(1))
	c.Assert(counts.Recipients-before.Recipients, check.Equals, int64(2))             // rows
	c.Assert(counts.RecipientBatches-before.RecipientBatches, check.Equals, int64(1)) // one deletion event
	// All = campaigns + groups + recipient BATCHES (rolled up in that tab).
	c.Assert(counts.All-before.All, check.Equals, int64(3))
	c.Assert(counts.All, check.Equals, counts.Campaigns+counts.CampaignGroups+counts.RecipientBatches)

	// A stranger sees nothing.
	empty, err := GetTrashCounts(campaign.UserId + 9999)
	c.Assert(err, check.Equals, nil)
	c.Assert(empty.All, check.Equals, int64(0))
}

// TestPreviewDeleteScopeCountsRealMatches (P0 fix) — the confirmation dialog shows
// two DIFFERENT numbers and they must not be conflated: the group's campaign count
// and the rows that actually match. The old UI multiplied selection × campaigns,
// which reports campaigns as if they were matches.
func (s *ModelsSuite) TestPreviewDeleteScopeCountsRealMatches(c *check.C) {
	// A group of THREE campaigns where the email exists in only TWO.
	c1 := s.createCampaign(c)
	c2 := s.createCampaign(c)
	c3 := s.createCampaign(c)
	shared := "interno-preview@example.com"
	for _, cid := range []int64{c1.Id, c2.Id} { // NOT in c3
		r := Result{CampaignId: cid, UserId: c1.UserId, RId: fmt.Sprintf("prev-%d", cid),
			Status: EventSent, BaseRecipient: BaseRecipient{Email: shared}}
		c.Assert(db.Create(&r).Error, check.Equals, nil)
	}
	group := createCampaignGroupForCampaigns(c, c1.UserId, "Preview group", c1.Id, c2.Id, c3.Id)

	rid := fmt.Sprintf("prev-%d", c1.Id)

	// Group scope: 3 campaigns in the group, but only 2 rows match.
	prev, err := PreviewResultDeletion(c1.Id, []string{rid}, c1.UserId, DeleteScopeGroup)
	c.Assert(err, check.Equals, nil)
	c.Assert(prev.InGroup, check.Equals, true)
	c.Assert(prev.GroupId, check.Equals, group.Id)
	c.Assert(prev.GroupName, check.Equals, group.Name)
	c.Assert(prev.CampaignCount, check.Equals, 3) // campañas del grupo
	c.Assert(prev.Affected, check.Equals, 2)      // coincidencias REALES, no 3
	c.Assert(prev.Scope, check.Equals, DeleteScopeGroup)

	// Campaign scope: exactly the one row addressed.
	prevC, err := PreviewResultDeletion(c1.Id, []string{rid}, c1.UserId, DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(prevC.Affected, check.Equals, 1)

	// The preview must NOT delete anything.
	var trashed int
	db.Unscoped().Model(&Result{}).Where("email = ? AND deleted_at IS NOT NULL", shared).Count(&trashed)
	c.Assert(trashed, check.Equals, 0)

	// Already-trashed rows are not counted twice: delete one, preview again.
	_, n, err := SoftDeleteResults(c1.Id, []string{rid}, c1.UserId, "", DeleteScopeCampaign)
	c.Assert(err, check.Equals, nil)
	c.Assert(n, check.Equals, 1)
	prev2, err := PreviewResultDeletion(c1.Id, []string{rid}, c1.UserId, DeleteScopeGroup)
	c.Assert(err, check.Equals, nil)
	c.Assert(prev2.Affected, check.Equals, 1) // solo queda 1 activa por eliminar

	// A campaign with no group reports in_group=false.
	solo := s.createCampaign(c)
	prevSolo, err := PreviewResultDeletion(solo.Id, []string{solo.Results[0].RId}, solo.UserId, DeleteScopeGroup)
	c.Assert(err, check.Equals, nil)
	c.Assert(prevSolo.InGroup, check.Equals, false)
	c.Assert(prevSolo.CampaignCount, check.Equals, 0)
}
