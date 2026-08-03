package report

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gophish/gophish/models"
)

// ErrTemplateInvalid is returned when an uploaded template fails token/slot
// validation. The accompanying Inspection explains what is missing or unknown.
var ErrTemplateInvalid = errors.New("template failed validation")

// ErrUnknownSlot is returned when an asset is uploaded for a slot that is not in
// the fixed catalog.
var ErrUnknownSlot = errors.New("unknown image slot")

// UploadTemplateVersion validates an uploaded DOCX, stores it as a new immutable
// version of the (owned) template, and caches the discovered tokens/slots. It is
// the single place that ties Inspect + BlobStore + persistence together so the
// controller stays a thin orchestrator.
func UploadTemplateVersion(store BlobStore, templateID, uid int64, content []byte, uploadedBy int64) (models.ReportTemplateVersion, Inspection, error) {
	if _, err := models.GetReportTemplate(templateID, uid); err != nil {
		return models.ReportTemplateVersion{}, Inspection{}, err
	}
	insp, err := Inspect(content)
	if err != nil {
		return models.ReportTemplateVersion{}, Inspection{}, err
	}
	if !insp.Valid {
		return models.ReportTemplateVersion{}, insp, ErrTemplateInvalid
	}
	sha, err := store.Put(content)
	if err != nil {
		return models.ReportTemplateVersion{}, insp, err
	}
	tokensJSON, _ := json.Marshal(insp.Tokens)
	slotsJSON, _ := json.Marshal(insp.ImageSlots)
	validationJSON, _ := json.Marshal(insp)

	versions, err := models.GetReportTemplateVersions(templateID, uid)
	if err != nil {
		return models.ReportTemplateVersion{}, insp, err
	}
	next := 1
	for _, v := range versions {
		if v.Version >= next {
			next = v.Version + 1
		}
	}
	version := models.ReportTemplateVersion{
		TemplateId:     templateID,
		Version:        next,
		ContentSha256:  sha,
		TokensJSON:     string(tokensJSON),
		ImageSlotsJSON: string(slotsJSON),
		ValidationJSON: string(validationJSON),
		UploadedBy:     uploadedBy,
	}
	if err := models.CreateReportTemplateVersion(&version); err != nil {
		return version, insp, err
	}
	// Auto-activate the first version so the template is immediately usable.
	if t, err := models.GetReportTemplate(templateID, uid); err == nil && t.ActiveVersionId == 0 {
		_ = models.SetActiveTemplateVersion(templateID, version.Id)
	}
	return version, insp, nil
}

// SaveReportAsset stores an uploaded image for a fixed slot of an owned draft,
// replacing any previous image for that slot.
func SaveReportAsset(store BlobStore, reportID, uid int64, slot string, content []byte) error {
	canon, ok := CanonicalSlotKey(slot)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownSlot, slot)
	}
	slot = canon // always persist the canonical key so UI status matches by key
	// Validate the real content (magic bytes), not the declared type, and normalize
	// it to PNG: what gets stored is always bytes this package produced, so the
	// serve path can never echo an attacker-supplied file or an attacker-chosen type.
	png, err := NormalizeImageToPNG(content)
	if err != nil {
		return err
	}
	if _, err := models.GetReport(reportID, uid); err != nil {
		return err
	}
	sha, err := store.Put(png)
	if err != nil {
		return err
	}
	return models.UpsertReportAsset(reportID, slot, sha, "image/png")
}
