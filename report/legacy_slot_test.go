package report

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/gophish/gophish/models"
)

// TestReauditOfLegacyRenderMatchesFingerprint blinds the property that broke in
// silence: a frozen render whose template declares a DEPRECATED slot must still
// re-audit byte-identically.
//
// Measured before the fix on 4 real renders from the production DB copy: all four
// returned match=false ("content fingerprint mismatch: evidence altered or renderer
// changed"), because removing figura_10 from the catalog made the render stop
// substituting its image. This test reproduces that scenario from scratch: build a
// template that declares the deprecated slot, generate a render, then re-audit.
func TestReauditOfLegacyRenderMatchesFingerprint(t *testing.T) {
	setupReportTestDB(t)
	store := models.NewDBBlobStore()

	// A template that declares BOTH a live slot and the deprecated figura_10, the
	// shape every one of the 30 stored versions has.
	body := `<w:p><w:r><w:t>{{EMPRESA}}</w:t></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:docPr id="1" descr="figura_1"/><a:blip r:embed="rId5"/></w:drawing></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:docPr id="2" descr="figura_10"/><a:blip r:embed="rId6"/></w:drawing></w:r></w:p>`
	tmpl := buildDocxTwoSlotsBody(t, body)

	tmplSha, err := store.Put(tmpl)
	if err != nil {
		t.Fatalf("store template: %v", err)
	}
	tpl := &models.ReportTemplate{UserId: 1, Name: "legacy-slot-template"}
	if err := models.CreateReportTemplate(tpl); err != nil {
		t.Fatalf("create template: %v", err)
	}
	ver := &models.ReportTemplateVersion{TemplateId: tpl.Id, Version: 1, ContentSha256: tmplSha, UploadedBy: 1}
	if err := models.CreateReportTemplateVersion(ver); err != nil {
		t.Fatalf("create version: %v", err)
	}
	if err := models.SetActiveTemplateVersion(tpl.Id, ver.Id); err != nil {
		t.Fatalf("activate: %v", err)
	}

	cid, _, _ := seedCampaignWithGroup(t, "legacyslot")
	rep := &models.Report{UserId: 1, SubjectKind: "campaign", SubjectId: cid, TemplateId: tpl.Id,
		CompanyName: "Empresa Ejemplo", Status: "draft"}
	if err := models.CreateReport(rep); err != nil {
		t.Fatalf("create report: %v", err)
	}
	// Upload an image for BOTH slots, including the deprecated one (exactly what the
	// 29 frozen renders carry).
	for _, slot := range []string{"figura_1", "figura_10"} {
		sha, err := store.Put([]byte("UPLOADED-" + slot))
		if err != nil {
			t.Fatalf("store asset %s: %v", slot, err)
		}
		if err := models.CreateReportAsset(&models.ReportAsset{
			ReportId: rep.Id, Slot: slot, ContentSha256: sha, Mime: "image/png"}); err != nil {
			t.Fatalf("create asset %s: %v", slot, err)
		}
	}

	render, docx, err := Generate(store, rep.Id, 1)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// The deprecated slot's image MUST have been substituted; if it is not, the
	// re-audit below would fail and 29 stored renders would read as altered.
	if got := string(unzipEntry(t, docx, "word/media/image2.png")); got != "UPLOADED-figura_10" {
		t.Fatalf("deprecated slot not substituted: got %q", got)
	}

	res, err := AuditRender(store, render.Id, 1)
	if err != nil {
		t.Fatalf("AuditRender: %v", err)
	}
	if !res.Match {
		t.Fatalf("re-audit of a render using a deprecated slot must match, got: %s", res.Reason)
	}
}

// TestDeprecatedSlotIsResolvableButNotOfferedOrRequired locks the four properties of
// the deprecation mechanism, so retiring any future slot behaves the same way.
func TestDeprecatedSlotIsResolvableButNotOfferedOrRequired(t *testing.T) {
	// 1) Resolvable → the render substitutes it and old templates keep working.
	canon, ok := CanonicalSlotKey("figura_10")
	if !ok || canon != "figura_10" {
		t.Fatalf("deprecated slot must stay resolvable, got %q ok=%v", canon, ok)
	}
	if !IsSlot("Figura 10") { // messy Alt-Text must resolve too
		t.Fatalf("deprecated slot must resolve case/separator-insensitively")
	}
	if !IsDeprecatedSlot("figura_10") {
		t.Fatalf("figura_10 must be flagged deprecated")
	}
	// 2) Never required → a NEW template that omits it is still valid.
	for _, k := range RequiredSlots() {
		if k == "figura_10" {
			t.Fatalf("a deprecated slot must never be required")
		}
	}
	// 3) Never offered → the editor does not present it for new templates.
	for _, s := range ActiveSlots() {
		if s.Key == "figura_10" {
			t.Fatalf("a deprecated slot must not be offered by ActiveSlots()")
		}
	}
	// 4) Recognized by Inspect (counted as a slot, never "unknown"), and a template
	//    declaring only live slots stays valid.
	tmpl := buildDocxWithImage(t,
		`<w:p><w:r><w:drawing><wp:docPr id="1" descr="figura_10"/><a:blip r:embed="rId5"/></w:drawing></w:r></w:p>`,
		"word/media/image1.png", []byte("x"))
	insp, err := Inspect(tmpl)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	found := false
	for _, s := range insp.ImageSlots {
		if s == "figura_10" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Inspect must recognize the deprecated slot, got %v", insp.ImageSlots)
	}
	if len(insp.Unknown) != 0 {
		t.Fatalf("a deprecated slot must not produce unknown entries: %v", insp.Unknown)
	}
}

// buildDocxTwoSlotsBody assembles a DOCX with the given body and TWO image parts
// (rId5 -> image1.png, rId6 -> image2.png), so a template can declare two slots.
func buildDocxTwoSlotsBody(t *testing.T, bodyXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, content []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	add("[Content_Types].xml", []byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`))
	add("_rels/.rels", []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`))
	add("word/_rels/document.xml.rels", []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId5" Type="img" Target="media/image1.png"/>`+
		`<Relationship Id="rId6" Type="img" Target="media/image2.png"/></Relationships>`))
	add("word/document.xml", []byte(`<?xml version="1.0"?><w:document xmlns:w="x" xmlns:r="y" xmlns:wp="z" xmlns:a="w"><w:body>`+bodyXML+`</w:body></w:document>`))
	add("word/media/image1.png", []byte("ORIGINAL-1"))
	add("word/media/image2.png", []byte("ORIGINAL-2"))
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}
