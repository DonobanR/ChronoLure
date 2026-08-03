package models

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/gophish/gophish/logger"
	"github.com/jinzhu/gorm"
)

var (
	// ErrResultNotFound is returned when a recipient result is missing or not
	// owned by the requesting user. We return NotFound (never Forbidden) so
	// existence of another user's data is not leaked.
	ErrResultNotFound = errors.New("recipient not found")
	// ErrResultActiveDuplicate blocks restoring onto an active same-email row.
	ErrResultActiveDuplicate = errors.New("an active recipient with this email already exists")
	// ErrParentCampaignTrashed blocks restoring a recipient whose campaign is
	// itself in the Trash (nested trash): the campaign must be restored first,
	// otherwise the recipient would come back into a campaign nobody can see.
	ErrParentCampaignTrashed = errors.New("parent campaign is in trash")
	// ErrParentCampaignPurged blocks restoring an orphan recipient whose campaign
	// no longer exists. The row stays in the Trash until purged manually.
	ErrParentCampaignPurged = errors.New("parent campaign was permanently deleted")
	// ErrBatchTooLarge caps a single bulk operation.
	ErrBatchTooLarge = errors.New("batch too large")
	// MaxRecipientBatch is the server-side cap for a single bulk delete.
	MaxRecipientBatch = 500
)

// DeleteScopeCampaign / DeleteScopeGroup are the valid deletion scopes.
const (
	DeleteScopeCampaign = "campaign"
	DeleteScopeGroup    = "group"
)

// newBatchID returns a random UUID-v4-like identifier for grouping a bulk
// deletion so "Undo" / batch-restore can address all rows at once.
func newBatchID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: time-based; collisions are irrelevant for a grouping key.
		return fmt.Sprintf("batch-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// resultsForScope resolves which result rows a delete affects. For campaign
// scope it is exactly the given (campaignID, rid) rows. For group scope it is
// every result (owned by the user, still active) whose email matches those rids
// across all campaigns of every group that contains campaignID.
func resultsForScope(tx *gorm.DB, campaignID int64, rids []string, userID int64, scope string) ([]Result, error) {
	base := []Result{}
	// Unscoped() so already-deleted rows are also seen (for idempotent skip); the
	// loop decides. gorm treats deleted_at as its soft-delete field and would
	// otherwise hide them.
	if err := tx.Unscoped().Where("campaign_id = ? AND user_id = ? AND r_id IN (?)", campaignID, userID, rids).
		Find(&base).Error; err != nil {
		return nil, err
	}
	if scope != DeleteScopeGroup {
		return base, nil
	}
	// Expand to the whole group: gather the campaigns of every group containing
	// this campaign, then all active results with the same emails.
	// NOTE: the link table column is `group_id` (see CampaignGroupCampaign), not
	// `campaign_group_id` — using the latter made scope=group fail outright.
	var campaignIDs []int64
	if err := tx.Table("campaign_group_campaigns").
		Where("group_id IN (?)",
			tx.Table("campaign_group_campaigns").Select("group_id").Where("campaign_id = ?", campaignID).QueryExpr()).
		Pluck("DISTINCT campaign_id", &campaignIDs).Error; err != nil {
		return nil, err
	}
	if len(campaignIDs) == 0 {
		campaignIDs = []int64{campaignID}
	}
	emails := make([]string, 0, len(base))
	for _, r := range base {
		emails = append(emails, r.Email)
	}
	expanded := []Result{}
	if err := tx.Unscoped().Where("campaign_id IN (?) AND user_id = ? AND email IN (?) AND deleted_at IS NULL",
		campaignIDs, userID, emails).Find(&expanded).Error; err != nil {
		return nil, err
	}
	return expanded, nil
}

// SoftDeleteResults moves recipients to the Trash (CL-102R). It is transactional,
// idempotent (already-deleted rows are skipped), enforces ownership via user_id,
// deletes each affected recipient's pending mail_logs in the SAME tx (so a
// running campaign never emails a deleted recipient), and writes one audit entry
// per recipient. Returns the batch id (for Undo) and the number affected.
func SoftDeleteResults(campaignID int64, rids []string, userID int64, reason, scope string) (string, int, error) {
	if len(rids) == 0 {
		return "", 0, nil
	}
	if len(rids) > MaxRecipientBatch {
		return "", 0, ErrBatchTooLarge
	}
	if scope != DeleteScopeGroup {
		scope = DeleteScopeCampaign
	}

	tx := db.Begin()
	if tx.Error != nil {
		return "", 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	targets, err := resultsForScope(tx, campaignID, rids, userID, scope)
	if err != nil {
		tx.Rollback()
		return "", 0, err
	}
	if len(targets) == 0 {
		tx.Rollback()
		return "", 0, ErrResultNotFound
	}

	batchID := newBatchID()
	now := time.Now().UTC()
	affected := 0
	for i := range targets {
		r := &targets[i]
		if r.IsDeleted() {
			continue // idempotent
		}
		if err := tx.Unscoped().Model(&Result{}).Where("id = ?", r.Id).Updates(map[string]interface{}{
			"deleted_at":      now,
			"deleted_by":      userID,
			"delete_reason":   reason,
			"delete_scope":    scope,
			"delete_batch_id": batchID,
			"restored_at":     nil,
			"restored_by":     nil,
		}).Error; err != nil {
			tx.Rollback()
			return "", 0, err
		}
		// Cascade: drop this recipient's pending mail_logs so the worker never
		// sends to a deleted recipient (same P0 fix as campaigns).
		if err := tx.Where("campaign_id = ? AND r_id = ?", r.CampaignId, r.RId).Delete(&MailLog{}).Error; err != nil {
			tx.Rollback()
			return "", 0, err
		}
		audit := userAuditLog(tx, userID, AuditRecipientSoftDeleted, "result", r.Id)
		audit.SetMetadata(map[string]interface{}{
			"campaign_id": r.CampaignId,
			"email":       r.Email,
			"reason":      reason,
			"scope":       scope,
			"batch_id":    batchID,
		})
		if err := tx.Create(audit).Error; err != nil {
			log.Errorf("audit recipient_soft_deleted (non-blocking): %v", err)
		}
		affected++
	}

	if err := tx.Commit().Error; err != nil {
		return "", 0, err
	}
	return batchID, affected, nil
}

// restoreResultRows clears the soft-delete on the given already-loaded rows,
// blocking when an active duplicate email exists in the same campaign.
func restoreResultRows(tx *gorm.DB, targets []Result, userID int64) (int, error) {
	now := time.Now().UTC()
	restored := 0
	for i := range targets {
		r := &targets[i]
		if !r.IsDeleted() {
			continue // idempotent
		}
		// Nested trash (CL-102R addendum §6): the parent campaign must be active.
		// Restoring into a trashed campaign would resurrect a recipient nobody can
		// see; restoring an orphan (campaign purged) is impossible.
		// Raw SQL on purpose: `campaigns` also carries gorm's soft-delete field, so
		// a scoped query would hide the very trashed campaign we need to detect.
		parents := []parentCampaignState{}
		if err := tx.Raw("SELECT deleted_at FROM campaigns WHERE id = ?", r.CampaignId).Scan(&parents).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if len(parents) == 0 {
			tx.Rollback()
			return 0, ErrParentCampaignPurged
		}
		if parents[0].DeletedAt != nil {
			tx.Rollback()
			return 0, ErrParentCampaignTrashed
		}
		var dupes int64
		if err := tx.Unscoped().Model(&Result{}).
			Where("campaign_id = ? AND email = ? AND deleted_at IS NULL AND id != ?", r.CampaignId, r.Email, r.Id).
			Count(&dupes).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if dupes > 0 {
			tx.Rollback()
			return 0, ErrResultActiveDuplicate
		}
		if err := tx.Unscoped().Model(&Result{}).Where("id = ?", r.Id).Updates(map[string]interface{}{
			"deleted_at":      nil,
			"deleted_by":      nil,
			"delete_reason":   "",
			"delete_scope":    "",
			"delete_batch_id": "",
			"restored_at":     now,
			"restored_by":     userID,
		}).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		audit := userAuditLog(tx, userID, AuditRecipientRestored, "result", r.Id)
		audit.SetMetadata(map[string]interface{}{"campaign_id": r.CampaignId, "email": r.Email})
		if err := tx.Create(audit).Error; err != nil {
			log.Errorf("audit recipient_restored (non-blocking): %v", err)
		}
		restored++
	}
	return restored, nil
}

// RestoreResultBatch restores every recipient of a soft-delete batch in one op
// (the toast "Undo"). Ownership is enforced via user_id.
func RestoreResultBatch(userID int64, batchID string) (int, error) {
	if batchID == "" {
		return 0, ErrResultNotFound
	}
	tx := db.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	targets := []Result{}
	if err := tx.Unscoped().Where("delete_batch_id = ? AND user_id = ? AND deleted_at IS NOT NULL", batchID, userID).
		Find(&targets).Error; err != nil {
		tx.Rollback()
		return 0, err
	}
	if len(targets) == 0 {
		tx.Rollback()
		return 0, ErrResultNotFound
	}
	n, err := restoreResultRows(tx, targets, userID)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit().Error
}

// RestoreResultByID restores a single trashed recipient by its id.
func RestoreResultByID(userID, resultID int64) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	targets := []Result{}
	if err := tx.Unscoped().Where("id = ? AND user_id = ?", resultID, userID).Find(&targets).Error; err != nil {
		tx.Rollback()
		return err
	}
	if len(targets) == 0 {
		tx.Rollback()
		return ErrResultNotFound
	}
	if _, err := restoreResultRows(tx, targets, userID); err != nil {
		return err
	}
	return tx.Commit().Error
}

// purgeResultRows hard-deletes the given trashed rows and their dependent events
// / calendar_events, writing an audit entry (with the email) BEFORE deletion so
// the trail survives the physical purge.
func purgeResultRows(tx *gorm.DB, targets []Result, userID int64) (int, error) {
	purged := 0
	for i := range targets {
		r := &targets[i]
		if !r.IsDeleted() {
			// Only trashed recipients can be purged.
			continue
		}
		audit := userAuditLog(tx, userID, AuditRecipientPurged, "result", r.Id)
		audit.SetMetadata(map[string]interface{}{"campaign_id": r.CampaignId, "email": r.Email})
		if err := tx.Create(audit).Error; err != nil {
			tx.Rollback()
			return 0, fmt.Errorf("failed to create audit log for recipient purge: %w", err)
		}
		if err := tx.Exec("DELETE FROM calendar_events WHERE result_id = ?", r.Id).Error; err != nil {
			log.Warnf("purge calendar_events for result %d: %v", r.Id, err)
		}
		if err := tx.Where("campaign_id = ? AND email = ?", r.CampaignId, r.Email).Delete(&Event{}).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Where("campaign_id = ? AND r_id = ?", r.CampaignId, r.RId).Delete(&MailLog{}).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		if err := tx.Unscoped().Delete(r).Error; err != nil {
			tx.Rollback()
			return 0, err
		}
		purged++
	}
	return purged, nil
}

// PurgeResult permanently deletes a single trashed recipient. confirmEmail must
// match the recipient's email (validated by the caller too, but re-checked here).
func PurgeResult(userID, resultID int64, confirmEmail string) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	r := Result{}
	if err := tx.Unscoped().Where("id = ? AND user_id = ?", resultID, userID).First(&r).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrResultNotFound
		}
		return err
	}
	if !r.IsDeleted() {
		tx.Rollback()
		return errors.New("can only purge recipients in trash")
	}
	if confirmEmail != r.Email {
		tx.Rollback()
		return errors.New("confirmation does not match")
	}
	if _, err := purgeResultRows(tx, []Result{r}, userID); err != nil {
		return err
	}
	return tx.Commit().Error
}

// PurgeResultBatch permanently deletes every trashed recipient of a batch.
func PurgeResultBatch(userID int64, batchID string) (int, error) {
	if batchID == "" {
		return 0, ErrResultNotFound
	}
	tx := db.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	targets := []Result{}
	if err := tx.Unscoped().Where("delete_batch_id = ? AND user_id = ? AND deleted_at IS NOT NULL", batchID, userID).
		Find(&targets).Error; err != nil {
		tx.Rollback()
		return 0, err
	}
	if len(targets) == 0 {
		tx.Rollback()
		return 0, ErrResultNotFound
	}
	n, err := purgeResultRows(tx, targets, userID)
	if err != nil {
		return 0, err
	}
	return n, tx.Commit().Error
}

// parentCampaignState is the minimal projection used to detect nested trash.
type parentCampaignState struct {
	DeletedAt *time.Time
}

// GetTrashedResults lists a user's soft-deleted recipients (newest first) for
// the global Trash "recipient" type.
func GetTrashedResults(userID int64) ([]Result, error) {
	rs := []Result{}
	err := db.Unscoped().Where("user_id = ? AND deleted_at IS NOT NULL", userID).
		Order("deleted_at DESC").Find(&rs).Error
	return rs, err
}

// TrashedRecipientBatch is one deletion event rolled up server-side: the N rows a
// single delete produced, presented as ONE unit (addendum §2/§3). Pagination must
// happen over these, not over rows, or a 38-row batch split across pages would
// render as two separate deletions.
type TrashedRecipientBatch struct {
	BatchId          string     `json:"batch_id"`
	Count            int64      `json:"count"`
	Scope            string     `json:"scope"`
	CampaignId       int64      `json:"campaign_id,omitempty"`
	CampaignName     string     `json:"campaign_name,omitempty"`
	CampaignCount    int64      `json:"campaign_count"`
	GroupId          *int64     `json:"group_id,omitempty"`
	GroupName        string     `json:"group_name,omitempty"`
	DeletedBy        *int64     `json:"deleted_by,omitempty"`
	DeletedByName    string     `json:"deleted_by_name,omitempty"`
	DeletedAt        *time.Time `json:"deleted_at"`
	Reason           string     `json:"reason,omitempty"`
	AnyParentTrashed bool       `json:"any_parent_trashed"`
	// SampleEmails holds up to 3 emails so the collapsed row can preview itself
	// without a second request ("qa@…, test@… y 36 más").
	SampleEmails []string `json:"sample_emails,omitempty"`
}

// GetTrashedRecipientBatches rolls the user's trashed recipients up by
// delete_batch_id and paginates over BATCHES (addendum §2). Rows with no batch id
// (legacy/migrated) are each returned as a batch of one keyed by "row:<id>" so
// nothing disappears from the listing.
func GetTrashedRecipientBatches(userID int64, q RecipientTrashQuery) ([]TrashedRecipientBatch, int64, error) {
	// Pull the matching rows (unpaginated) and aggregate in Go: the grouping key
	// mixes batch_id with a per-row fallback, which SQL cannot express portably.
	rows, _, err := GetTrashedRecipients(userID, RecipientTrashQuery{
		CampaignID: q.CampaignID, GroupID: q.GroupID, Q: q.Q, NoCollapse: true,
	})
	if err != nil {
		return nil, 0, err
	}

	order := []string{}
	byKey := map[string]*TrashedRecipientBatch{}
	campaignsSeen := map[string]map[int64]bool{}
	for _, r := range rows {
		key := r.BatchId
		if key == "" {
			key = fmt.Sprintf("row:%d", r.ResultId)
		}
		b, ok := byKey[key]
		if !ok {
			b = &TrashedRecipientBatch{
				BatchId:       key,
				Scope:         r.Scope,
				CampaignId:    r.CampaignId,
				CampaignName:  r.CampaignName,
				GroupId:       r.GroupId,
				GroupName:     r.GroupName,
				DeletedBy:     r.DeletedBy,
				DeletedByName: r.DeletedByName,
				DeletedAt:     r.DeletedAt,
				Reason:        r.Reason,
			}
			byKey[key] = b
			campaignsSeen[key] = map[int64]bool{}
			order = append(order, key)
		}
		// Rows are fetched uncollapsed (NoCollapse), so each one counts once: a
		// group-scoped deletion of 1 email over 4 campaigns is "4 destinatarios".
		b.Count++
		campaignsSeen[key][r.CampaignId] = true
		if r.ParentCampaignTrashed {
			b.AnyParentTrashed = true
		}
		if len(b.SampleEmails) < 3 {
			b.SampleEmails = append(b.SampleEmails, r.Email)
		}
	}
	for key, b := range byKey {
		b.CampaignCount = int64(len(campaignsSeen[key]))
		// A group-scoped batch spans several campaigns: the single campaign name
		// would be misleading, so drop it and let the group name carry the context.
		if b.CampaignCount > 1 {
			b.CampaignId = 0
			b.CampaignName = ""
		}
	}

	total := int64(len(order))
	if q.Limit > 0 {
		start := q.Offset
		if start > len(order) {
			start = len(order)
		}
		end := start + q.Limit
		if end > len(order) {
			end = len(order)
		}
		order = order[start:end]
	}
	out := make([]TrashedRecipientBatch, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out, total, nil
}

// GetTrashedRecipientsByBatch returns every trashed recipient of one batch (the
// expanded detail of a rolled-up row), scoped to the owner.
func GetTrashedRecipientsByBatch(userID int64, batchID string) ([]TrashedRecipient, error) {
	if batchID == "" {
		return nil, ErrResultNotFound
	}
	// Support the "row:<id>" pseudo-batch used for rows without a batch id.
	if strings.HasPrefix(batchID, "row:") {
		var id int64
		if _, err := fmt.Sscanf(batchID, "row:%d", &id); err != nil {
			return nil, ErrResultNotFound
		}
		r, err := GetTrashedRecipientByID(userID, id)
		if err != nil {
			return nil, err
		}
		return []TrashedRecipient{r}, nil
	}
	all, _, err := GetTrashedRecipients(userID, RecipientTrashQuery{NoCollapse: true})
	if err != nil {
		return nil, err
	}
	out := []TrashedRecipient{}
	for _, r := range all {
		if r.BatchId == batchID {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return nil, ErrResultNotFound
	}
	return out, nil
}

// TrashCounts are the unfiltered per-type totals behind the tab badges
// (addendum §7). One call, so the UI does not fire four requests.
type TrashCounts struct {
	All            int64 `json:"all"`
	Campaigns      int64 `json:"campaigns"`
	CampaignGroups int64 `json:"campaign_groups"`
	Recipients     int64 `json:"recipients"`
	// RecipientBatches is what the "All" tab actually shows for recipients, since
	// they are rolled up by batch there.
	RecipientBatches int64 `json:"recipient_batches"`
}

// GetTrashCounts returns the unfiltered counts per trash type for the user.
func GetTrashCounts(userID int64) (TrashCounts, error) {
	c := TrashCounts{}
	if err := db.Raw("SELECT COUNT(*) FROM campaigns WHERE user_id = ? AND deleted_at IS NOT NULL", userID).Row().Scan(&c.Campaigns); err != nil {
		return c, err
	}
	if err := db.Raw("SELECT COUNT(*) FROM campaign_groups WHERE user_id = ? AND deleted_at IS NOT NULL", userID).Row().Scan(&c.CampaignGroups); err != nil {
		return c, err
	}
	if err := db.Raw("SELECT COUNT(*) FROM results WHERE user_id = ? AND deleted_at IS NOT NULL", userID).Row().Scan(&c.Recipients); err != nil {
		return c, err
	}
	// Distinct deletion events: batches plus one per batch-less row.
	if err := db.Raw(`SELECT COUNT(*) FROM (
	        SELECT COALESCE(NULLIF(delete_batch_id, ''), 'row:' || id) AS k
	          FROM results WHERE user_id = ? AND deleted_at IS NOT NULL GROUP BY k)`, userID).
		Row().Scan(&c.RecipientBatches); err != nil {
		return c, err
	}
	// "All" shows campaigns + groups + recipient BATCHES (rolled up, §2).
	c.All = c.Campaigns + c.CampaignGroups + c.RecipientBatches
	return c, nil
}

// DeletePreview is the read-only answer to "what would this deletion touch?".
// The two numbers the confirmation dialog shows MUST come from here, not from the
// client: CampaignCount is how many campaigns the group has, while Affected is how
// many result ROWS actually match — and those differ (a group of 4 campaigns where
// the email exists in 2 affects 2 rows). Showing the campaign count as if it were
// the affected count would make the user decide on a false number.
type DeletePreview struct {
	Scope         string `json:"scope"`
	InGroup       bool   `json:"in_group"`
	GroupId       int64  `json:"group_id,omitempty"`
	GroupName     string `json:"group_name,omitempty"`
	CampaignCount int    `json:"campaign_count"`
	Affected      int    `json:"affected"`
}

// PreviewResultDeletion computes what SoftDeleteResults would touch, without
// touching anything. It reuses resultsForScope — the SAME resolution the delete
// uses — so the preview cannot drift from the action it describes.
func PreviewResultDeletion(campaignID int64, rids []string, userID int64, scope string) (DeletePreview, error) {
	out := DeletePreview{Scope: DeleteScopeCampaign}
	if scope == DeleteScopeGroup {
		out.Scope = DeleteScopeGroup
	}
	if len(rids) == 0 {
		return out, nil
	}
	if len(rids) > MaxRecipientBatch {
		return out, ErrBatchTooLarge
	}

	// Group context: the first group containing this campaign, and its size.
	var gid int64
	row := db.Raw(`SELECT MIN(group_id) FROM campaign_group_campaigns WHERE campaign_id = ?`, campaignID).Row()
	_ = row.Scan(&gid)
	if gid > 0 {
		var name string
		var count int
		if err := db.Raw(`SELECT name FROM campaign_groups WHERE id = ? AND deleted_at IS NULL`, gid).Row().Scan(&name); err == nil && name != "" {
			if err := db.Raw(`SELECT COUNT(DISTINCT campaign_id) FROM campaign_group_campaigns WHERE group_id = ?`, gid).Row().Scan(&count); err == nil {
				out.InGroup = true
				out.GroupId = gid
				out.GroupName = name
				out.CampaignCount = count
			}
		}
	}

	// Affected rows: exactly what the delete would mark, counting only rows that
	// are still active (already-trashed rows are skipped by the delete too).
	tx := db.Begin()
	if tx.Error != nil {
		return out, tx.Error
	}
	defer tx.Rollback() // read-only: never commit
	targets, err := resultsForScope(tx, campaignID, rids, userID, out.Scope)
	if err != nil {
		return out, err
	}
	for i := range targets {
		if !targets[i].IsDeleted() {
			out.Affected++
		}
	}
	return out, nil
}

// GetTrashedRecipientByID returns one trashed recipient with its context, so the
// API can build precise messages ("no se puede restaurar X porque la campaña «Y»
// está en la papelera"). Returns ErrResultNotFound if it is not in the user's trash.
func GetTrashedRecipientByID(userID, resultID int64) (TrashedRecipient, error) {
	rs, _, err := GetTrashedRecipients(userID, RecipientTrashQuery{NoCollapse: true})
	if err != nil {
		return TrashedRecipient{}, err
	}
	for _, r := range rs {
		if r.ResultId == resultID {
			return r, nil
		}
	}
	return TrashedRecipient{}, ErrResultNotFound
}

// TrashedRecipient is a soft-deleted recipient enriched with the context the
// Trash UI needs (addendum §4): which campaign/group it came from, who deleted
// it, the batch it belongs to, and whether its parent campaign is itself in the
// Trash (so the UI can disable "Restaurar" instead of hiding it).
type TrashedRecipient struct {
	ResultId              int64      `json:"id"`
	Email                 string     `json:"email"`
	CampaignId            int64      `json:"campaign_id"`
	CampaignName          string     `json:"campaign_name"`
	GroupId               *int64     `json:"group_id,omitempty"`
	GroupName             string     `json:"group_name,omitempty"`
	BatchId               string     `json:"batch_id,omitempty"`
	Scope                 string     `json:"scope,omitempty"`
	DeletedBy             *int64     `json:"deleted_by,omitempty"`
	DeletedByName         string     `json:"deleted_by_name,omitempty"`
	DeletedAt             *time.Time `json:"deleted_at"`
	Reason                string     `json:"reason,omitempty"`
	ParentCampaignTrashed bool       `json:"parent_campaign_trashed"`
	// A group-scoped deletion produces one row per campaign for the same email;
	// those rows collapse into ONE entry server-side (addendum §3). These fields
	// carry the expandable detail. CampaignCount is 1 for ordinary rows.
	CampaignCount int64                  `json:"campaign_count"`
	Campaigns     []RecipientCampaignRef `json:"campaigns,omitempty"`
}

// RecipientCampaignRef names one campaign spanned by a collapsed entry.
type RecipientCampaignRef struct {
	CampaignId   int64  `json:"campaign_id"`
	ResultId     int64  `json:"result_id"`
	CampaignName string `json:"campaign_name"`
	Trashed      bool   `json:"parent_campaign_trashed"`
}

// RecipientTrashQuery holds the filters and pagination for the recipient trash
// listing. Limit <= 0 means "no pagination" (return everything).
type RecipientTrashQuery struct {
	CampaignID int64
	GroupID    int64
	Q          string // case-insensitive email substring
	Offset     int
	Limit      int
	// NoCollapse returns the RAW rows without collapsing group-scoped duplicates.
	// Needed by callers that must see the underlying rows: the batch rollup (which
	// counts rows), the expanded batch detail, and per-result lookups.
	NoCollapse bool
}

// recipientTrashRow is the raw scan target for the enriched listing.
type recipientTrashRow struct {
	Id            int64
	Email         string
	CampaignId    int64
	CampaignName  string
	ParentDeleted *time.Time
	BatchId       string
	Scope         string
	DeletedBy     *int64
	DeletedByName string
	DeletedAt     *time.Time
	Reason        string
	GroupId       *int64
}

// GetTrashedRecipients returns a user's soft-deleted recipients with full context,
// filtered and paginated, plus the total matching count (for the pager and the
// tab badge). Ownership is per-user by design — there is no cross-tenant view
// (consistent with campaigns and groups).
//
// Raw SQL on purpose: it needs to read `campaigns.deleted_at` (whose own
// soft-delete scope would hide trashed campaigns) and to LEFT JOIN so an orphan
// recipient still lists. The group id comes from a correlated subquery so a
// campaign belonging to several groups does not multiply rows.
func GetTrashedRecipients(userID int64, q RecipientTrashQuery) ([]TrashedRecipient, int64, error) {
	where := "r.user_id = ? AND r.deleted_at IS NOT NULL"
	args := []interface{}{userID}
	if q.CampaignID > 0 {
		where += " AND r.campaign_id = ?"
		args = append(args, q.CampaignID)
	}
	if q.GroupID > 0 {
		where += " AND r.campaign_id IN (SELECT campaign_id FROM campaign_group_campaigns WHERE group_id = ?)"
		args = append(args, q.GroupID)
	}
	if s := strings.TrimSpace(q.Q); s != "" {
		where += " AND LOWER(r.email) LIKE ?"
		args = append(args, "%"+strings.ToLower(s)+"%")
	}

	sel := `SELECT r.id AS id, r.email AS email, r.campaign_id AS campaign_id,
	               COALESCE(c.name, '') AS campaign_name, c.deleted_at AS parent_deleted,
	               COALESCE(r.delete_batch_id, '') AS batch_id, COALESCE(r.delete_scope, '') AS scope,
	               r.deleted_by AS deleted_by, COALESCE(u.username, '') AS deleted_by_name,
	               r.deleted_at AS deleted_at, COALESCE(r.delete_reason, '') AS reason,
	               (SELECT MIN(cgc.group_id) FROM campaign_group_campaigns cgc
	                 WHERE cgc.campaign_id = r.campaign_id) AS group_id
	          FROM results r
	          LEFT JOIN campaigns c ON c.id = r.campaign_id
	          LEFT JOIN users u ON u.id = r.deleted_by
	         WHERE ` + where + ` ORDER BY r.deleted_at DESC, r.id DESC`
	// Fetch every match, then collapse and paginate in Go: a group-scoped deletion
	// yields one row per campaign for the same email and those must render as ONE
	// entry, so the page size has to apply to the COLLAPSED units. A SQL LIMIT would
	// slice mid-entry and the pager would disagree with what is shown.
	rows := []recipientTrashRow{}
	if err := db.Raw(sel, args...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	// Resolve the (few) distinct group names in one pass.
	groupNames := map[int64]string{}
	for _, rw := range rows {
		if rw.GroupId != nil {
			groupNames[*rw.GroupId] = ""
		}
	}
	if len(groupNames) > 0 {
		ids := make([]int64, 0, len(groupNames))
		for id := range groupNames {
			ids = append(ids, id)
		}
		type gn struct {
			Id   int64
			Name string
		}
		gs := []gn{}
		if err := db.Raw("SELECT id, name FROM campaign_groups WHERE id IN (?)", ids).Scan(&gs).Error; err == nil {
			for _, g := range gs {
				groupNames[g.Id] = g.Name
			}
		}
	}

	// Map rows, collapsing group-scoped duplicates by (batch_id, email).
	out := []TrashedRecipient{}
	idx := map[string]int{}
	for _, rw := range rows {
		ref := RecipientCampaignRef{
			CampaignId: rw.CampaignId, ResultId: rw.Id,
			CampaignName: rw.CampaignName, Trashed: rw.ParentDeleted != nil,
		}
		collapseKey := ""
		if !q.NoCollapse && rw.Scope == DeleteScopeGroup && rw.BatchId != "" {
			collapseKey = rw.BatchId + "\x00" + rw.Email
		}
		if collapseKey != "" {
			if at, ok := idx[collapseKey]; ok {
				e := &out[at]
				e.CampaignCount++
				e.Campaigns = append(e.Campaigns, ref)
				if ref.Trashed {
					e.ParentCampaignTrashed = true
				}
				continue
			}
		}
		tr := TrashedRecipient{
			ResultId:              rw.Id,
			Email:                 rw.Email,
			CampaignId:            rw.CampaignId,
			CampaignName:          rw.CampaignName,
			BatchId:               rw.BatchId,
			Scope:                 rw.Scope,
			DeletedBy:             rw.DeletedBy,
			DeletedByName:         rw.DeletedByName,
			DeletedAt:             rw.DeletedAt,
			Reason:                rw.Reason,
			ParentCampaignTrashed: rw.ParentDeleted != nil,
			CampaignCount:         1,
			Campaigns:             []RecipientCampaignRef{ref},
		}
		// Group context is only meaningful for group-scoped deletions.
		if rw.Scope == DeleteScopeGroup && rw.GroupId != nil {
			tr.GroupId = rw.GroupId
			tr.GroupName = groupNames[*rw.GroupId]
		}
		out = append(out, tr)
		if collapseKey != "" {
			idx[collapseKey] = len(out) - 1
		}
	}
	// A collapsed entry spans several campaigns, so a single campaign name would
	// mislead: drop it and let the group name carry the context.
	for i := range out {
		if out[i].CampaignCount > 1 {
			out[i].CampaignId = 0
			out[i].CampaignName = ""
		}
	}

	total := int64(len(out))
	if q.Limit > 0 {
		start := q.Offset
		if start > len(out) {
			start = len(out)
		}
		end := start + q.Limit
		if end > len(out) {
			end = len(out)
		}
		out = out[start:end]
	}
	return out, total, nil
}
