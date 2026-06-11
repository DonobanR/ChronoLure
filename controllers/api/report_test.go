package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gophish/gophish/models"
	"github.com/gophish/gophish/report"
)

// doReq issues an authenticated request against the given server.
func doReq(t *testing.T, srv *Server, method, url string, body []byte, apiKey string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, url, rdr)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func enabledReportServer() *Server {
	return NewServer(WithReporting(true), WithBlobDownload(true))
}
func disabledReportServer() *Server { return NewServer(WithReporting(false)) }

// makeReportUser creates a non-admin user with its own API key.
func makeReportUser(t *testing.T) models.User {
	t.Helper()
	role, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatalf("GetRoleBySlug: %v", err)
	}
	u := models.User{
		Username: fmt.Sprintf("userB-%d", time.Now().UnixNano()),
		ApiKey:   fmt.Sprintf("bkey-%d", time.Now().UnixNano()),
		Role:     role,
		RoleID:   role.ID,
	}
	if err := models.PutUser(&u); err != nil {
		t.Fatalf("PutUser: %v", err)
	}
	return u
}

// synthTemplateDocx builds a minimal valid template carrying every required
// token plus the figura_1 and grafico_1 slots. No corporate content.
func synthTemplateDocx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(n string, b []byte) {
		w, _ := zw.Create(n)
		w.Write(b)
	}
	add("[Content_Types].xml", []byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`))
	add("_rels/.rels", []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`))
	add("word/_rels/document.xml.rels", []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId5" Type="img" Target="media/image1.png"/>`+
		`<Relationship Id="rId6" Type="img" Target="media/image2.png"/></Relationships>`))
	body := ""
	for _, tok := range report.RequiredTokens {
		body += `<w:r><w:t>{{` + tok + `}}</w:t></w:r>`
	}
	body += `<w:drawing><wp:docPr id="1" name="figura_1"/><a:blip r:embed="rId5"/></w:drawing>`
	body += `<w:drawing><wp:docPr id="2" name="grafico_1"/><a:blip r:embed="rId6"/></w:drawing>`
	add("word/document.xml", []byte(`<?xml version="1.0"?><w:document xmlns:w="x" xmlns:r="y" xmlns:wp="z" xmlns:a="w"><w:body>`+body+`</w:body></w:document>`))
	add("word/media/image1.png", []byte("img1"))
	add("word/media/image2.png", []byte("img2"))
	zw.Close()
	return buf.Bytes()
}

// seedActiveTemplate creates a template owned by uid with one active version.
func seedActiveTemplate(t *testing.T, uid int64) int64 {
	t.Helper()
	tpl := &models.ReportTemplate{UserId: uid, Name: "synthetic"}
	if err := models.CreateReportTemplate(tpl); err != nil {
		t.Fatalf("CreateReportTemplate: %v", err)
	}
	sha, err := models.NewDBBlobStore().Put(synthTemplateDocx(t))
	if err != nil {
		t.Fatalf("blob put: %v", err)
	}
	ver := &models.ReportTemplateVersion{TemplateId: tpl.Id, Version: 1, ContentSha256: sha, UploadedBy: uid}
	if err := models.CreateReportTemplateVersion(ver); err != nil {
		t.Fatalf("CreateReportTemplateVersion: %v", err)
	}
	if err := models.SetActiveTemplateVersion(tpl.Id, ver.Id); err != nil {
		t.Fatalf("SetActiveTemplateVersion: %v", err)
	}
	return tpl.Id
}

// Exit criterion 1: with the feature disabled the routes do not exist (404).
func TestReportingDisabledRoutes404(t *testing.T) {
	ctx := setupTest(t)
	srv := disabledReportServer()

	for _, tc := range []struct {
		method, url string
	}{
		{http.MethodPost, "/api/report-templates"},
		{http.MethodGet, "/api/reports/1"},
		{http.MethodPost, "/api/reports/1/generate"},
		{http.MethodGet, "/api/renders/1"},
		{http.MethodPost, "/api/renders/1/rebuild"},
		{http.MethodGet, "/api/renders/1/download"},
	} {
		w := doReq(t, srv, tc.method, tc.url, []byte(`{}`), ctx.apiKey)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s: expected 404 (route absent), got %d", tc.method, tc.url, w.Code)
		}
	}
}

// Exit criterion 2: with the feature enabled the routes are registered and work.
func TestReportingEnabledRoutesRegistered(t *testing.T) {
	ctx := setupTest(t)
	srv := enabledReportServer()

	w := doReq(t, srv, http.MethodPost, "/api/report-templates", []byte(`{"name":"My template"}`), ctx.apiKey)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating template, got %d: %s", w.Code, w.Body.String())
	}
	var tpl models.ReportTemplate
	if err := json.Unmarshal(w.Body.Bytes(), &tpl); err != nil || tpl.Id == 0 {
		t.Fatalf("unexpected create response: %s (err=%v)", w.Body.String(), err)
	}
}

// Exit criterion 3: a user cannot access another user's templates/reports/renders.
func TestReportCrossUserIsolation(t *testing.T) {
	ctx := setupTest(t)
	createTestData(t)
	srv := enabledReportServer()
	userB := makeReportUser(t)

	campaigns, err := models.GetCampaigns(1)
	if err != nil || len(campaigns) == 0 {
		t.Fatalf("expected a seeded campaign for admin: %v", err)
	}
	campaignID := campaigns[0].Id

	templateID := seedActiveTemplate(t, 1)

	// admin creates a report
	body := fmt.Sprintf(`{"subject_kind":"campaign","subject_id":%d,"template_id":%d,"company_name":"ACME","users_with_2fa":0}`, campaignID, templateID)
	w := doReq(t, srv, http.MethodPost, "/api/reports", []byte(body), ctx.apiKey)
	if w.Code != http.StatusCreated {
		t.Fatalf("admin create report: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var rep models.Report
	json.Unmarshal(w.Body.Bytes(), &rep)

	// Complete the report so it passes pre-generation validation: the synthetic
	// template uses {{EMPRESA}} (set above) and the figura_1 image slot.
	asha, err := models.NewDBBlobStore().Put([]byte("pngdata"))
	if err != nil {
		t.Fatalf("seed asset blob: %v", err)
	}
	if err := models.UpsertReportAsset(rep.Id, "figura_1", asha, "image/png"); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	// admin generates a render
	w = doReq(t, srv, http.MethodPost, fmt.Sprintf("/api/reports/%d/generate", rep.Id), nil, ctx.apiKey)
	if w.Code != http.StatusCreated {
		t.Fatalf("admin generate: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var render models.ReportRender
	json.Unmarshal(w.Body.Bytes(), &render)

	// userB must not reach any of admin's resources -> 404
	cases := []struct {
		method, url string
	}{
		{http.MethodGet, fmt.Sprintf("/api/report-templates/%d", templateID)},
		{http.MethodGet, fmt.Sprintf("/api/reports/%d", rep.Id)},
		{http.MethodGet, fmt.Sprintf("/api/renders/%d", render.Id)},
		{http.MethodPost, fmt.Sprintf("/api/renders/%d/rebuild", render.Id)},
	}
	for _, c := range cases {
		w := doReq(t, srv, c.method, c.url, nil, userB.ApiKey)
		if w.Code != http.StatusNotFound {
			t.Fatalf("userB %s %s: expected 404, got %d: %s", c.method, c.url, w.Code, w.Body.String())
		}
	}
}

// Exit criteria 4 & 5: generate creates an immutable render, and rebuild
// reproduces the historical DOCX.
func TestGenerateAndRebuildViaAPI(t *testing.T) {
	ctx := setupTest(t)
	createTestData(t)
	srv := enabledReportServer()

	campaigns, _ := models.GetCampaigns(1)
	campaignID := campaigns[0].Id
	templateID := seedActiveTemplate(t, 1)

	body := fmt.Sprintf(`{"subject_kind":"campaign","subject_id":%d,"template_id":%d,"company_name":"ACME","users_with_2fa":0}`, campaignID, templateID)
	w := doReq(t, srv, http.MethodPost, "/api/reports", []byte(body), ctx.apiKey)
	if w.Code != http.StatusCreated {
		t.Fatalf("create report: %d %s", w.Code, w.Body.String())
	}
	var rep models.Report
	json.Unmarshal(w.Body.Bytes(), &rep)

	// Complete the report so it passes pre-generation validation (synthetic
	// template uses {{EMPRESA}} — set above — and the figura_1 image slot).
	asha, err := models.NewDBBlobStore().Put([]byte("pngdata"))
	if err != nil {
		t.Fatalf("seed asset blob: %v", err)
	}
	if err := models.UpsertReportAsset(rep.Id, "figura_1", asha, "image/png"); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	// 4: generate -> immutable render with an output hash.
	w = doReq(t, srv, http.MethodPost, fmt.Sprintf("/api/reports/%d/generate", rep.Id), nil, ctx.apiKey)
	if w.Code != http.StatusCreated {
		t.Fatalf("generate: %d %s", w.Code, w.Body.String())
	}
	var render models.ReportRender
	json.Unmarshal(w.Body.Bytes(), &render)
	if render.Id == 0 || render.OutputSha256 == "" {
		t.Fatalf("render not created properly: %+v", render)
	}

	// 5a: rebuild is now an AUDIT -> JSON verdict {match:true}.
	w = doReq(t, srv, http.MethodPost, fmt.Sprintf("/api/renders/%d/rebuild", render.Id), nil, ctx.apiKey)
	if w.Code != http.StatusOK {
		t.Fatalf("rebuild/audit: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var audit report.AuditResult
	if err := json.Unmarshal(w.Body.Bytes(), &audit); err != nil || !audit.Match {
		t.Fatalf("expected audit match=true, got %s (err=%v)", w.Body.String(), err)
	}

	// 5b: download serves the stored DOCX byte-for-byte.
	w = doReq(t, srv, http.MethodGet, fmt.Sprintf("/api/renders/%d/download", render.Id), nil, ctx.apiKey)
	if w.Code != http.StatusOK {
		t.Fatalf("download: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if out := w.Body.Bytes(); !bytes.HasPrefix(out, []byte("PK")) {
		t.Fatalf("download did not return a docx (zip), got %d bytes", len(out))
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("unexpected content-type: %q", ct)
	}
}

// TestBlobDownloadFlagGating: with reporting enabled but blob download disabled,
// the download route is not registered (natural 404), while rebuild still works.
func TestBlobDownloadFlagGating(t *testing.T) {
	ctx := setupTest(t)
	srv := NewServer(WithReporting(true), WithBlobDownload(false))
	w := doReq(t, srv, http.MethodGet, "/api/renders/1/download", nil, ctx.apiKey)
	if w.Code != http.StatusNotFound {
		t.Fatalf("download route should be absent when flag off, got %d", w.Code)
	}
}
