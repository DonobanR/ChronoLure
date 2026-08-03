package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jinzhu/gorm"
)

// Domain errors for the reporting module.
var (
	// ErrReportTemplateInUse is returned when deleting a template that is still
	// referenced by an immutable render (which would break reproducibility).
	ErrReportTemplateInUse = errors.New("template is referenced by historical renders")
	// ErrReportHasRenders is returned when deleting a report that already has
	// immutable renders.
	ErrReportHasRenders = errors.New("report has historical renders")
	// ErrNoActiveTemplateVersion is returned when generating a report whose
	// template has no active version (none uploaded/activated yet).
	ErrNoActiveTemplateVersion = errors.New("la plantilla no tiene una versión activa: sube un DOCX y pulsa Activar")
)

// This file contains the persistence layer for the reporting module (DOCX
// mail-merge). It only defines the storage structs, their table mappings and a
// content-addressed blob store. The report-generation engine itself lives in
// the report/ package and consumes these through plain function calls and the
// report.BlobStore interface (satisfied structurally by DBBlobStore).

// ReportBlob is the content-addressed backing store for every binary used by
// the reporting module: template DOCX bytes, working assets and frozen render
// assets. Rows are immutable and deduplicated by sha256.
type ReportBlob struct {
	Sha256    string    `json:"sha256" gorm:"column:sha256;primary_key"`
	Content   []byte    `json:"-" gorm:"column:content"`
	Size      int64     `json:"size" gorm:"column:size"`
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName specifies the database table for ReportBlob.
func (ReportBlob) TableName() string { return "report_blobs" }

// ReportTemplate is a logical report type (e.g. "Phishing report") that points
// at its currently active immutable version.
type ReportTemplate struct {
	Id              int64     `json:"id"`
	UserId          int64     `json:"-" gorm:"column:user_id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	ActiveVersionId int64     `json:"active_version_id" gorm:"column:active_version_id"`
	IsDefault       bool      `json:"is_default" gorm:"column:is_default"`
	CreatedAt       time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt       time.Time `json:"updated_at" gorm:"column:updated_at"`
}

// TableName specifies the database table for ReportTemplate.
func (ReportTemplate) TableName() string { return "report_templates" }

// ReportTemplateVersion is an immutable version of a template. The DOCX bytes
// are referenced (not embedded) by ContentSha256 to keep versions cheap and
// deduplicated. tokens/slots/validation are the cached result of report.Inspect.
type ReportTemplateVersion struct {
	Id             int64     `json:"id"`
	TemplateId     int64     `json:"template_id" gorm:"column:template_id"`
	Version        int       `json:"version"`
	ContentSha256  string    `json:"content_sha256" gorm:"column:content_sha256"`
	TokensJSON     string    `json:"-" gorm:"column:tokens_json"`
	ImageSlotsJSON string    `json:"-" gorm:"column:image_slots_json"`
	ValidationJSON string    `json:"-" gorm:"column:validation_json"`
	UploadedBy     int64     `json:"uploaded_by" gorm:"column:uploaded_by"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName specifies the database table for ReportTemplateVersion.
func (ReportTemplateVersion) TableName() string { return "report_template_versions" }

// Report is the editable draft. It tracks the template (not the version): while
// in draft it always renders with the template's active version. The exact
// version is only frozen into a ReportRender at generation time.
type Report struct {
	Id           int64     `json:"id"`
	UserId       int64     `json:"-" gorm:"column:user_id"`
	SubjectKind  string    `json:"subject_kind" gorm:"column:subject_kind"`
	SubjectId    int64     `json:"subject_id" gorm:"column:subject_id"`
	TemplateId   int64     `json:"template_id" gorm:"column:template_id"`
	CompanyName  string    `json:"company_name" gorm:"column:company_name"`
	PreparedBy   string    `json:"prepared_by" gorm:"column:prepared_by"`
	ReportDate   time.Time `json:"report_date" gorm:"column:report_date"`
	ExecutedFrom time.Time `json:"executed_from" gorm:"column:executed_from"`
	ExecutedTo   time.Time `json:"executed_to" gorm:"column:executed_to"`
	UsersWith2FA int64     `json:"users_with_2fa" gorm:"column:users_with_2fa"`
	IntroExec    string    `json:"intro_exec" gorm:"column:intro_exec"`
	TextPunto1   string    `json:"text_punto_1" gorm:"column:text_punto_1"`
	// ImpersonatedAs / Communique fill the S3 Ejecución fixed paragraph (CL-103):
	// who was impersonated ({{SUPLANTANDO}}) and which communiqué was faked
	// ({{COMUNICADO}}). {{EMPRESA}} is already covered by CompanyName.
	ImpersonatedAs string    `json:"impersonated_as" gorm:"column:impersonated_as"`
	Communique     string    `json:"communique" gorm:"column:communique"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"column:updated_at"`
}

// TableName specifies the database table for Report.
func (Report) TableName() string { return "reports" }

// ReportAsset is a working (editable) image bound to a fixed slot of a draft.
type ReportAsset struct {
	Id            int64     `json:"id"`
	ReportId      int64     `json:"-" gorm:"column:report_id"`
	Slot          string    `json:"slot"`
	ContentSha256 string    `json:"content_sha256" gorm:"column:content_sha256"`
	Mime          string    `json:"mime"`
	CreatedAt     time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName specifies the database table for ReportAsset.
func (ReportAsset) TableName() string { return "report_assets" }

// ReportRender is the immutable snapshot produced when a report is generated.
// It freezes the exact template version, the metrics and (via
// ReportRenderAsset) the exact bytes of every image used, guaranteeing the
// report can be regenerated byte-identically regardless of later edits.
type ReportRender struct {
	Id                 int64     `json:"id"`
	ReportId           int64     `json:"report_id" gorm:"column:report_id"`
	TemplateVersionId  int64     `json:"template_version_id" gorm:"column:template_version_id"`
	MetricsJSON        string    `json:"metrics_json" gorm:"column:metrics_json"`
	OutputSha256       string    `json:"output_sha256" gorm:"column:output_sha256"`
	OutputXlsxSha256   string    `json:"output_xlsx_sha256" gorm:"column:output_xlsx_sha256"`
	ContentFingerprint string    `json:"content_fingerprint" gorm:"column:content_fingerprint"`
	OutputSize         int64     `json:"output_size" gorm:"column:output_size"`
	GeneratedBy        int64     `json:"generated_by" gorm:"column:generated_by"`
	CreatedAt          time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName specifies the database table for ReportRender.
func (ReportRender) TableName() string { return "report_renders" }

// ReportRenderAsset freezes the bytes of a single slot used by a render.
type ReportRenderAsset struct {
	Id            int64     `json:"id"`
	RenderId      int64     `json:"-" gorm:"column:render_id"`
	Slot          string    `json:"slot"`
	ContentSha256 string    `json:"content_sha256" gorm:"column:content_sha256"`
	Mime          string    `json:"mime"`
	CreatedAt     time.Time `json:"created_at" gorm:"column:created_at"`
}

// TableName specifies the database table for ReportRenderAsset.
func (ReportRenderAsset) TableName() string { return "report_render_assets" }

// DBBlobStore is the database-backed implementation of the report.BlobStore
// interface. It stores content-addressed binary blobs in the report_blobs
// table, deduplicating by sha256. It is defined here (rather than in the report
// package) so all database access stays in the models package; it satisfies
// report.BlobStore structurally, so no import of the report package is needed.
type DBBlobStore struct{}

// NewDBBlobStore returns a database-backed blob store.
func NewDBBlobStore() *DBBlobStore { return &DBBlobStore{} }

// Put stores content keyed by its sha256 hex digest and returns that digest.
// Identical content is stored only once (deduplication).
func (DBBlobStore) Put(content []byte) (string, error) {
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	// Single idempotent UPSERT instead of a pre-existence COUNT + INSERT (audit
	// M-3). The blob is content-addressed, so a duplicate sha256 (the primary
	// key) is simply ignored: re-storing identical content is a no-op and never
	// errors, including under concurrent Puts. Dialect-aware verb: SQLite uses
	// "INSERT OR IGNORE", MySQL uses "INSERT IGNORE".
	verb := "INSERT OR IGNORE"
	if db.Dialect().GetName() == "mysql" {
		verb = "INSERT IGNORE"
	}
	q := verb + " INTO report_blobs (sha256, content, size, created_at) VALUES (?, ?, ?, ?)"
	if err := db.Exec(q, digest, content, int64(len(content)), time.Now().UTC()).Error; err != nil {
		return "", err
	}
	return digest, nil
}

// Get returns the content previously stored under the given sha256 digest.
func (DBBlobStore) Get(digest string) ([]byte, error) {
	blob := ReportBlob{}
	if err := db.Where("sha256 = ?", digest).First(&blob).Error; err != nil {
		return nil, err
	}
	return blob.Content, nil
}

// Exists reports whether a blob with the given digest is stored.
func (DBBlobStore) Exists(digest string) (bool, error) {
	return blobExists(digest)
}

func blobExists(digest string) (bool, error) {
	count := 0
	err := db.Model(&ReportBlob{}).Where("sha256 = ?", digest).Count(&count).Error
	return count > 0, err
}

// GarbageCollectReportBlobs deletes report_blobs that are not referenced by any
// template version, working asset, frozen render asset, or render output. The
// referenced set is the union of:
//   - report_template_versions.content_sha256
//   - report_assets.content_sha256
//   - report_render_assets.content_sha256
//   - report_renders.output_sha256
//
// Blobs in that set are never deleted. It is a callable utility (not scheduled).
func GarbageCollectReportBlobs() (int64, error) {
	const referenced = `
		SELECT content_sha256 FROM report_template_versions WHERE content_sha256 IS NOT NULL
		UNION SELECT content_sha256 FROM report_assets WHERE content_sha256 IS NOT NULL
		UNION SELECT content_sha256 FROM report_render_assets WHERE content_sha256 IS NOT NULL
		UNION SELECT output_sha256 FROM report_renders WHERE output_sha256 IS NOT NULL`
	res := db.Exec("DELETE FROM report_blobs WHERE sha256 NOT IN (" + referenced + ")")
	return res.RowsAffected, res.Error
}

// --- Persistence layer ------------------------------------------------------
//
// Drafts (Report, ReportAsset) are mutable. Renders (ReportRender,
// ReportRenderAsset) are immutable historical evidence: there are deliberately
// no update/delete functions for them. To "fix" a report the operator edits the
// draft and generates a new render, like creating a new commit.

// CreateReportTemplate inserts a new logical template.
func CreateReportTemplate(t *ReportTemplate) error {
	now := time.Now().UTC()
	t.CreatedAt, t.UpdatedAt = now, now
	return db.Create(t).Error
}

// CreateReportTemplateVersion inserts a new immutable template version.
func CreateReportTemplateVersion(v *ReportTemplateVersion) error {
	v.CreatedAt = time.Now().UTC()
	return db.Create(v).Error
}

// SetActiveTemplateVersion points a template at one of its versions.
func SetActiveTemplateVersion(templateID, versionID int64) error {
	return db.Model(&ReportTemplate{}).Where("id = ?", templateID).
		Update(map[string]interface{}{"active_version_id": versionID, "updated_at": time.Now().UTC()}).Error
}

// GetActiveTemplateVersion returns the currently active version of a template.
func GetActiveTemplateVersion(templateID int64) (ReportTemplateVersion, error) {
	t := ReportTemplate{}
	if err := db.Where("id = ?", templateID).First(&t).Error; err != nil {
		return ReportTemplateVersion{}, err
	}
	if t.ActiveVersionId == 0 {
		return ReportTemplateVersion{}, ErrNoActiveTemplateVersion
	}
	return GetReportTemplateVersionByID(t.ActiveVersionId)
}

// GetReportTemplateVersionByID returns a specific (immutable) template version.
func GetReportTemplateVersionByID(id int64) (ReportTemplateVersion, error) {
	v := ReportTemplateVersion{}
	err := db.Where("id = ?", id).First(&v).Error
	return v, err
}

// CreateReport inserts a new draft report.
func CreateReport(r *Report) error {
	now := time.Now().UTC()
	r.CreatedAt, r.UpdatedAt = now, now
	if r.Status == "" {
		r.Status = "draft"
	}
	return db.Create(r).Error
}

// GetReport returns a draft report scoped to its owner.
func GetReport(id, uid int64) (Report, error) {
	r := Report{}
	err := db.Where("id = ? AND user_id = ?", id, uid).First(&r).Error
	return r, err
}

// SetReportStatus updates a report's status (e.g. draft -> generated).
func SetReportStatus(id, uid int64, status string) error {
	return db.Model(&Report{}).Where("id = ? AND user_id = ?", id, uid).
		Update(map[string]interface{}{"status": status, "updated_at": time.Now().UTC()}).Error
}

// CreateReportAsset inserts (or replaces) a working asset for a draft slot.
func CreateReportAsset(a *ReportAsset) error {
	a.CreatedAt = time.Now().UTC()
	return db.Create(a).Error
}

// GetReportAssets returns the working assets bound to a draft.
func GetReportAssets(reportID int64) ([]ReportAsset, error) {
	var assets []ReportAsset
	err := db.Where("report_id = ?", reportID).Find(&assets).Error
	return assets, err
}

// GetReportAssetBySlot returns the working asset bound to a specific slot.
func GetReportAssetBySlot(reportID int64, slot string) (ReportAsset, error) {
	a := ReportAsset{}
	err := db.Where("report_id = ? AND slot = ?", reportID, slot).First(&a).Error
	return a, err
}

// CreateReportRender persists an immutable render snapshot together with its
// frozen asset references, in a single transaction.
func CreateReportRender(r *ReportRender, assets []ReportRenderAsset) error {
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	r.CreatedAt = time.Now().UTC()
	if err := tx.Create(r).Error; err != nil {
		tx.Rollback()
		return err
	}
	for i := range assets {
		assets[i].RenderId = r.Id
		assets[i].CreatedAt = r.CreatedAt
		if err := tx.Create(&assets[i]).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

// GetReportTemplate returns a template scoped to its owner.
func GetReportTemplate(id, uid int64) (ReportTemplate, error) {
	t := ReportTemplate{}
	err := db.Where("id = ? AND user_id = ?", id, uid).First(&t).Error
	return t, err
}

// GetReportTemplates lists all templates owned by uid.
func GetReportTemplates(uid int64) ([]ReportTemplate, error) {
	ts := []ReportTemplate{}
	err := db.Where("user_id = ?", uid).Order("created_at DESC").Find(&ts).Error
	return ts, err
}

// GetReports lists all reports owned by uid.
func GetReports(uid int64) ([]Report, error) {
	rs := []Report{}
	err := db.Where("user_id = ?", uid).Order("created_at DESC").Find(&rs).Error
	return rs, err
}

// GetRendersForReport lists the immutable renders of a report owned by uid.
func GetRendersForReport(reportID, uid int64) ([]ReportRender, error) {
	if _, err := GetReport(reportID, uid); err != nil {
		return nil, err
	}
	rr := []ReportRender{}
	err := db.Where("report_id = ?", reportID).Order("created_at DESC").Find(&rr).Error
	return rr, err
}

// GetReportTemplateVersions lists the versions of a template owned by uid.
func GetReportTemplateVersions(templateID, uid int64) ([]ReportTemplateVersion, error) {
	if _, err := GetReportTemplate(templateID, uid); err != nil {
		return nil, err
	}
	var vs []ReportTemplateVersion
	err := db.Where("template_id = ?", templateID).Order("version DESC").Find(&vs).Error
	return vs, err
}

// DeleteReportTemplate deletes a template and its versions. Unless force is set,
// it refuses if any immutable render still references one of its versions
// (reproducibility). A forced delete (admin) removes the template/versions even
// if referenced; the renders keep their stored output DOCX (download still
// works) but can no longer be rebuilt/audited.
func DeleteReportTemplate(id, uid int64, force bool) error {
	if _, err := GetReportTemplate(id, uid); err != nil {
		return err
	}
	if !force {
		count := 0
		db.Table("report_renders").
			Joins("JOIN report_template_versions v ON v.id = report_renders.template_version_id").
			Where("v.template_id = ?", id).Count(&count)
		if count > 0 {
			return ErrReportTemplateInUse
		}
	}
	tx := db.Begin()
	if err := tx.Where("template_id = ?", id).Delete(ReportTemplateVersion{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Where("id = ? AND user_id = ?", id, uid).Delete(ReportTemplate{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// UpdateReport updates the editable fields of a draft, scoped to its owner.
// Render snapshots are immutable and unaffected.
func UpdateReport(r *Report, uid int64) error {
	return db.Model(&Report{}).Where("id = ? AND user_id = ?", r.Id, uid).
		Update(map[string]interface{}{
			"subject_kind":    r.SubjectKind,
			"subject_id":      r.SubjectId,
			"template_id":     r.TemplateId,
			"company_name":    r.CompanyName,
			"prepared_by":     r.PreparedBy,
			"report_date":     r.ReportDate,
			"executed_from":   r.ExecutedFrom,
			"executed_to":     r.ExecutedTo,
			"users_with_2fa":  r.UsersWith2FA,
			"intro_exec":      r.IntroExec,
			"text_punto_1":    r.TextPunto1,
			"impersonated_as": r.ImpersonatedAs,
			"communique":      r.Communique,
			"updated_at":      time.Now().UTC(),
		}).Error
}

// DeleteReport deletes a draft and its working assets. Unless force is set, it
// refuses if the report has immutable renders (historical evidence). A forced
// delete (admin) also removes the renders and their frozen asset references.
func DeleteReport(id, uid int64, force bool) error {
	if _, err := GetReport(id, uid); err != nil {
		return err
	}
	if !force {
		count := 0
		db.Model(&ReportRender{}).Where("report_id = ?", id).Count(&count)
		if count > 0 {
			return ErrReportHasRenders
		}
	}
	tx := db.Begin()
	if force {
		var renderIDs []int64
		tx.Model(&ReportRender{}).Where("report_id = ?", id).Pluck("id", &renderIDs)
		for _, rid := range renderIDs {
			if err := tx.Where("render_id = ?", rid).Delete(ReportRenderAsset{}).Error; err != nil {
				tx.Rollback()
				return err
			}
		}
		if err := tx.Where("report_id = ?", id).Delete(ReportRender{}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Where("report_id = ?", id).Delete(ReportAsset{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Where("id = ? AND user_id = ?", id, uid).Delete(Report{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

// UpsertReportAsset binds (or replaces) the working image for a draft slot.
// DeleteReportAsset removes the working image bound to one slot of a draft. It
// touches ONLY that slot, and never the content-addressed blob: other reports (and
// frozen renders) may reference the same bytes.
func DeleteReportAsset(reportID int64, slot string) error {
	return db.Where("report_id = ? AND slot = ?", reportID, slot).Delete(&ReportAsset{}).Error
}

func UpsertReportAsset(reportID int64, slot, sha, mime string) error {
	a := ReportAsset{}
	err := db.Where("report_id = ? AND slot = ?", reportID, slot).First(&a).Error
	if err == gorm.ErrRecordNotFound {
		return CreateReportAsset(&ReportAsset{ReportId: reportID, Slot: slot, ContentSha256: sha, Mime: mime})
	}
	if err != nil {
		return err
	}
	return db.Model(&ReportAsset{}).Where("id = ?", a.Id).
		Update(map[string]interface{}{"content_sha256": sha, "mime": mime}).Error
}

// GetReportRender returns an immutable render and its frozen assets, scoped to
// the owner of the parent report.
func GetReportRender(id, uid int64) (ReportRender, []ReportRenderAsset, error) {
	r := ReportRender{}
	err := db.Table("report_renders").
		Select("report_renders.*").
		Joins("JOIN reports ON reports.id = report_renders.report_id").
		Where("report_renders.id = ? AND reports.user_id = ?", id, uid).
		First(&r).Error
	if err != nil {
		return r, nil, err
	}
	var assets []ReportRenderAsset
	err = db.Where("render_id = ?", r.Id).Find(&assets).Error
	return r, assets, err
}
