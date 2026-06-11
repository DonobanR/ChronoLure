package report

import (
	"archive/zip"
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/gophish/gophish/config"
	"github.com/gophish/gophish/models"
)

var reportDBOnce sync.Once

// setupReportTestDB brings up an in-memory database with the full migration set
// (including the reporting tables). No corporate content is involved.
func setupReportTestDB(t *testing.T) {
	t.Helper()
	reportDBOnce.Do(func() {
		conf := &config.Config{
			DBName:         "sqlite3",
			DBPath:         ":memory:",
			MigrationsPath: "../db/db_sqlite3/migrations/",
		}
		if err := models.Setup(conf); err != nil {
			t.Fatalf("failed to set up test database: %v", err)
		}
	})
}

// buildTemplateDocx assembles a synthetic template with text tokens and two
// image slots (figura_1 and grafico_1), each wired to its own media part.
func buildTemplateDocx(t *testing.T) []byte {
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
	body := `<w:p><w:r><w:t>Empresa {{EMPRESA}} usuarios {{N_USUARIOS}} datos {{R_DATOS}} pct {{PCT_DATOS}}</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>{{BLOQUE_2FA}}</w:t></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:docPr id="1" name="figura_1"/><a:blip r:embed="rId5"/></w:drawing></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:docPr id="2" name="grafico_1"/><a:blip r:embed="rId6"/></w:drawing></w:r></w:p>`
	add("word/document.xml", []byte(`<?xml version="1.0"?><w:document xmlns:w="x" xmlns:r="y" xmlns:wp="z" xmlns:a="w"><w:body>`+body+`</w:body></w:document>`))
	add("word/media/image1.png", []byte("ORIGINAL-FIGURA1"))
	add("word/media/image2.png", []byte("ORIGINAL-CHART-PLACEHOLDER"))
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// fixedSource provides deterministic metrics without seeding a full campaign,
// exercising the engine purely against the ReportSource interface.
type fixedSource struct{}

func (fixedSource) Subject() (string, int64, string) { return "campaign_group", 1, "Grupo de prueba" }
func (fixedSource) Stats() (FunnelInput, error) {
	return FunnelInput{TotalRecipients: 17, Sent: 17, Opened: 16, Clicked: 16, Submitted: 2}, nil
}
func (fixedSource) DateRange() (time.Time, time.Time, error) {
	return date(2026, 2, 2), date(2026, 2, 6), nil
}
func (fixedSource) Recipients() ([]Recipient, error) {
	return []Recipient{
		{Email: "a@example.com", Status: "Correo Ignorado"},
		{Email: "b@example.com", Status: "Clic al Enlace"},
		{Email: "c@example.com", Status: "Envío de Datos"},
	}, nil
}

// TestGenerateAndRegenerateReproducible is the Priority-3 exit-criteria test:
// create render -> freeze metrics -> freeze assets -> generate DOCX -> hash ->
// regenerate -> identical hash.
func TestGenerateAndRegenerateReproducible(t *testing.T) {
	setupReportTestDB(t)
	store := models.NewDBBlobStore()

	// Template (stored as a content-addressed blob) + active version.
	tmplSha, err := store.Put(buildTemplateDocx(t))
	if err != nil {
		t.Fatalf("store template: %v", err)
	}
	tpl := &models.ReportTemplate{UserId: 1, Name: "synthetic"}
	if err := models.CreateReportTemplate(tpl); err != nil {
		t.Fatalf("create template: %v", err)
	}
	ver := &models.ReportTemplateVersion{TemplateId: tpl.Id, Version: 1, ContentSha256: tmplSha, UploadedBy: 1}
	if err := models.CreateReportTemplateVersion(ver); err != nil {
		t.Fatalf("create version: %v", err)
	}
	if err := models.SetActiveTemplateVersion(tpl.Id, ver.Id); err != nil {
		t.Fatalf("activate version: %v", err)
	}

	// Draft report + one working asset (figura_1).
	rep := &models.Report{UserId: 1, SubjectKind: "campaign_group", SubjectId: 1, TemplateId: tpl.Id,
		CompanyName: "Empresa Ejemplo, S.A.", UsersWith2FA: 2, Status: "draft"}
	if err := models.CreateReport(rep); err != nil {
		t.Fatalf("create report: %v", err)
	}
	figSha, err := store.Put([]byte("UPLOADED-FIGURA1-BYTES"))
	if err != nil {
		t.Fatalf("store asset: %v", err)
	}
	if err := models.CreateReportAsset(&models.ReportAsset{ReportId: rep.Id, Slot: "figura_1", ContentSha256: figSha, Mime: "image/png"}); err != nil {
		t.Fatalf("create asset: %v", err)
	}

	repLoaded, err := models.GetReport(rep.Id, 1)
	if err != nil {
		t.Fatalf("get report: %v", err)
	}
	verLoaded, err := models.GetActiveTemplateVersion(tpl.Id)
	if err != nil {
		t.Fatalf("get active version: %v", err)
	}

	// 1-5: generate, freezing metrics + assets, producing the DOCX and its hash.
	render, docx1, err := generate(store, repLoaded, verLoaded, fixedSource{}, 1)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if render.OutputSha256 != sha256Hex(docx1) {
		t.Fatalf("stored output hash does not match generated DOCX")
	}

	// Frozen assets must include both the manual figura_1 and the auto grafico_1.
	_, frozen, err := models.GetReportRender(render.Id, 1)
	if err != nil {
		t.Fatalf("get render: %v", err)
	}
	slots := map[string]bool{}
	for _, a := range frozen {
		slots[a.Slot] = true
	}
	if !slots["figura_1"] || !slots["grafico_1"] {
		t.Fatalf("frozen assets missing expected slots: %v", slots)
	}

	// Metrics snapshot must carry the resolved variables (12%, 2 submitted, etc.).
	if !bytes.Contains([]byte(render.MetricsJSON), []byte(`"PCT_DATOS":"12%"`)) {
		t.Fatalf("metrics snapshot missing expected vars: %s", render.MetricsJSON)
	}

	// 6: the final DOCX is stored as a content-addressed blob with fingerprint+size.
	if exists, _ := store.Exists(render.OutputSha256); !exists {
		t.Fatalf("generated DOCX blob was not stored under output_sha256")
	}
	if render.ContentFingerprint == "" || render.OutputSize <= 0 {
		t.Fatalf("render missing fingerprint/size: %+v", render)
	}

	// 7: download returns the exact stored bytes, byte-for-byte.
	dl, err := DownloadRender(store, render.Id, 1)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if !bytes.Equal(dl, docx1) {
		t.Fatalf("download bytes differ from generated DOCX")
	}

	// 8: audit re-renders from frozen data and confirms reproducibility.
	audit, err := AuditRender(store, render.Id, 1)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if !audit.Match {
		t.Fatalf("audit should match: %s", audit.Reason)
	}
}

// TestAuditDetectsTampering verifies the audit reports a mismatch when the
// frozen render's hash does not match the rebuilt bytes (integrity guarantee).
func TestAuditDetectsTampering(t *testing.T) {
	setupReportTestDB(t)
	store := models.NewDBBlobStore()

	tmplSha, _ := store.Put(buildTemplateDocx(t))
	tpl := &models.ReportTemplate{UserId: 1, Name: "synthetic2"}
	models.CreateReportTemplate(tpl)
	ver := &models.ReportTemplateVersion{TemplateId: tpl.Id, Version: 1, ContentSha256: tmplSha, UploadedBy: 1}
	models.CreateReportTemplateVersion(ver)
	models.SetActiveTemplateVersion(tpl.Id, ver.Id)
	rep := &models.Report{UserId: 1, SubjectKind: "campaign", SubjectId: 1, TemplateId: tpl.Id, CompanyName: "ACME", Status: "draft"}
	models.CreateReport(rep)

	// A render row whose stored hash is deliberately wrong, with valid frozen
	// references (template content) so Render succeeds but the hash check fails.
	chartPNG, _ := ChartPNG(FunnelMetrics{})
	chartSha, _ := store.Put(chartPNG)
	bogus := models.ReportRender{
		ReportId:          rep.Id,
		TemplateVersionId: ver.Id,
		MetricsJSON:       `{"vars":{"EMPRESA":"ACME"}}`,
		OutputSha256:      "0000000000000000000000000000000000000000000000000000000000000000",
		GeneratedBy:       1,
	}
	if err := models.CreateReportRender(&bogus, []models.ReportRenderAsset{
		{Slot: "grafico_1", ContentSha256: chartSha, Mime: "image/png"},
	}); err != nil {
		t.Fatalf("create bogus render: %v", err)
	}

	audit, err := AuditRender(store, bogus.Id, 1)
	if err != nil {
		t.Fatalf("audit error: %v", err)
	}
	if audit.Match {
		t.Fatalf("expected audit mismatch for tampered render, got match")
	}
}

// TestBlobStorePutGetDedup verifies content-addressed roundtrip and dedup.
func TestBlobStorePutGetDedup(t *testing.T) {
	setupReportTestDB(t)
	store := models.NewDBBlobStore()

	content := []byte("some-docx-bytes-\x00\x01\x02")
	sha1, err := store.Put(content)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := store.Get(sha1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("roundtrip mismatch")
	}
	// Putting identical content again returns the same key without duplicating.
	sha2, err := store.Put(content)
	if err != nil {
		t.Fatalf("put again: %v", err)
	}
	if sha1 != sha2 {
		t.Fatalf("content-addressed key changed: %s != %s", sha1, sha2)
	}
	exists, _ := store.Exists(sha1)
	if !exists {
		t.Fatalf("expected blob to exist")
	}
}
