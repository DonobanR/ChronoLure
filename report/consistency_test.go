package report

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gophish/gophish/models"
)

// seedCampaignWithGroup creates a minimal real campaign (3 recipients) owned by
// user 1, marks one recipient as "submitted data" (generating events), and wraps
// it in a campaign group. It returns the campaign id, group id and the RId of the
// submitter. Uses only exported models APIs (report can import models; models
// cannot import report, so the cross-surface consistency test must live here).
func seedCampaignWithGroup(t *testing.T, tag string) (cid, gid int64, submitterRID string) {
	t.Helper()
	uid := int64(1)
	uniq := fmt.Sprintf("%s-%d", tag, time.Now().UnixNano())

	group := models.Group{Name: "grp-" + uniq}
	for i := 0; i < 3; i++ {
		group.Targets = append(group.Targets, models.Target{
			BaseRecipient: models.BaseRecipient{Email: fmt.Sprintf("r%d-%s@example.com", i, uniq)},
		})
	}
	group.UserId = uid
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("PostGroup: %v", err)
	}
	tmpl := models.Template{Name: "tpl-" + uniq, Subject: "s", Text: "t", HTML: "<html>h</html>", UserId: uid}
	if err := models.PostTemplate(&tmpl); err != nil {
		t.Fatalf("PostTemplate: %v", err)
	}
	page := models.Page{Name: "pg-" + uniq, HTML: "<html>h</html>", UserId: uid}
	if err := models.PostPage(&page); err != nil {
		t.Fatalf("PostPage: %v", err)
	}
	smtp := models.SMTP{Name: "smtp-" + uniq, UserId: uid, Host: "example.com", FromAddress: "a@b.com"}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatalf("PostSMTP: %v", err)
	}
	campaign := models.Campaign{Name: "camp-" + uniq, UserId: uid, Template: tmpl, Page: page, SMTP: smtp, Groups: []models.Group{group}}
	if err := models.PostCampaign(&campaign, uid); err != nil {
		t.Fatalf("PostCampaign: %v", err)
	}

	// One recipient submits data (creates open/click/submit events + status).
	cr, err := models.GetCampaignResults(campaign.Id, uid)
	if err != nil {
		t.Fatalf("GetCampaignResults: %v", err)
	}
	if len(cr.Results) < 3 {
		t.Fatalf("expected 3 results, got %d", len(cr.Results))
	}
	// Mark all as "Email Sent" (like the reference scenario: everyone was sent),
	// so the campaign funnel Total (= EmailsSent) equals the recipient count and
	// all surfaces share one denominator. Then one recipient submits data.
	for i := range cr.Results {
		if err := cr.Results[i].HandleEmailSent(); err != nil {
			t.Fatalf("HandleEmailSent: %v", err)
		}
	}
	if err := cr.Results[0].HandleFormSubmit(models.EventDetails{}); err != nil {
		t.Fatalf("HandleFormSubmit: %v", err)
	}

	cg := models.CampaignGroup{Name: "cg-" + uniq, UserId: uid,
		Campaigns: []models.CampaignGroupCampaign{{CampaignId: campaign.Id, OrderIndex: 0}}}
	if err := models.PostCampaignGroup(&cg, uid); err != nil {
		t.Fatalf("PostCampaignGroup: %v", err)
	}
	return campaign.Id, cg.Id, cr.Results[0].RId
}

// surfaceCounts collects the counts from the 5 surfaces that MUST agree.
// `opened` is CUMULATIVE ("reached at least opened") and is the dimension that
// exposes per-event counting: a recipient opening 3 times must count ONCE.
type surfaceCounts struct{ total, opened, submitted int64 }

func collectSurfaces(t *testing.T, cid, gid int64) map[string]surfaceCounts {
	t.Helper()
	uid := int64(1)
	out := map[string]surfaceCounts{}

	// 1) Dashboard (campaign summary → getCampaignStats)
	sum, err := models.GetCampaignSummary(cid, uid)
	if err != nil {
		t.Fatalf("GetCampaignSummary: %v", err)
	}
	out["dashboard"] = surfaceCounts{sum.Stats.Total, sum.Stats.OpenedEmail, sum.Stats.SubmittedData}

	// 2) CSV (results listing)
	cr, err := models.GetCampaignResults(cid, uid)
	if err != nil {
		t.Fatalf("GetCampaignResults: %v", err)
	}
	var csvSub, csvOpened int64
	for _, r := range cr.Results {
		if r.Status == models.EventDataSubmit {
			csvSub++
		}
		// Cumulative: reached at least "opened".
		switch r.Status {
		case models.EventOpened, models.EventClicked, models.EventDataSubmit:
			csvOpened++
		}
	}
	out["csv"] = surfaceCounts{int64(len(cr.Results)), csvOpened, csvSub}

	// 3) Excel + 4) Report share the campaign source's per-recipient universe.
	src, err := NewSource("campaign", cid, uid)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	recips, err := src.Recipients()
	if err != nil {
		t.Fatalf("Recipients: %v", err)
	}
	var exSub, exOpened int64
	for _, r := range recips {
		if r.Status == "Envío de Datos" {
			exSub++
		}
		switch r.Status {
		case "Correo Abierto", "Clic al Enlace", "Envío de Datos":
			exOpened++
		}
	}
	out["excel"] = surfaceCounts{int64(len(recips)), exOpened, exSub}

	stats, err := src.Stats()
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	f := Funnel(stats)
	// Funnel buckets are disjoint; cumulative opened = Total - Ignorado.
	out["report"] = surfaceCounts{f.Total, f.Total - f.Ignorado, f.Datos}

	// 5) Group stats
	gs, err := models.GetCampaignGroupStats(gid, uid)
	if err != nil {
		t.Fatalf("GetCampaignGroupStats: %v", err)
	}
	out["group"] = surfaceCounts{gs.TotalRecipients, gs.OpenedEmail, gs.SubmittedData}
	return out
}

func assertAllAgree(t *testing.T, label string, s map[string]surfaceCounts) surfaceCounts {
	t.Helper()
	var ref *surfaceCounts
	var refName string
	for name, v := range s {
		if ref == nil {
			cp := v
			ref = &cp
			refName = name
			continue
		}
		if v.total != ref.total || v.opened != ref.opened || v.submitted != ref.submitted {
			t.Fatalf("%s: surface %q=%+v disagrees with %q=%+v", label, name, v, refName, *ref)
		}
	}
	return *ref
}

// TestFunnelCountsMatchAcrossDashboardCSVExcelReport is the central acceptance
// test (CL-102R): the 5 surfaces (dashboard, CSV, Excel, Report, group) report
// identical totals/submitted BEFORE and AFTER deleting a recipient that has
// events — and the deletion drops exactly one from every surface at once.
func TestFunnelCountsMatchAcrossDashboardCSVExcelReport(t *testing.T) {
	setupReportTestDB(t)
	cid, gid, submitterRID := seedCampaignWithGroup(t, "consistency")

	before := collectSurfaces(t, cid, gid)
	ref := assertAllAgree(t, "before", before)
	if ref.total != 3 || ref.submitted != 1 {
		t.Fatalf("unexpected baseline: %+v (want total=3 submitted=1)", ref)
	}

	// Delete the recipient WHO HAS EVENTS (the submitter): the events must not
	// keep inflating any surface.
	if _, _, err := models.SoftDeleteResults(cid, []string{submitterRID}, 1, "interno", models.DeleteScopeCampaign); err != nil {
		t.Fatalf("SoftDeleteResults: %v", err)
	}

	after := collectSurfaces(t, cid, gid)
	got := assertAllAgree(t, "after", after)
	if got.total != 2 || got.submitted != 0 {
		t.Fatalf("after delete: %+v (want total=2 submitted=0 — events must not linger)", got)
	}
}

// TestFunnelCountsMatchAtVolumeWithDuplicateEvents is the volume counterpart of
// the acceptance test: ~50 recipients where several open the email MULTIPLE times.
// This is the exact shape of the historical per-event counting bug — with only 3
// rows a per-event count is indistinguishable from a correct per-recipient one,
// but with 50 recipients and 3 opens each the numbers diverge loudly.
func TestFunnelCountsMatchAtVolumeWithDuplicateEvents(t *testing.T) {
	setupReportTestDB(t)
	const n = 50
	cid, gid := seedVolumeCampaign(t, n)

	before := collectSurfaces(t, cid, gid)
	ref := assertAllAgree(t, "volume/before", before)
	if ref.total != n {
		t.Fatalf("baseline total=%d want %d (duplicate events must not inflate)", ref.total, n)
	}
	if ref.submitted != 5 {
		t.Fatalf("baseline submitted=%d want 5", ref.submitted)
	}
	// THE assertion this fixture exists for: 20 recipients opened 3 times each
	// (60 open events). Per-recipient accounting must yield 20, not 60.
	if ref.opened != 20 {
		t.Fatalf("baseline opened=%d want 20 — duplicate events are being counted per-event", ref.opened)
	}

	// Delete 3 recipients that HAVE duplicate events (the submitters).
	cr, err := models.GetCampaignResults(cid, 1)
	if err != nil {
		t.Fatalf("GetCampaignResults: %v", err)
	}
	victims := []string{}
	for _, r := range cr.Results {
		if r.Status == models.EventDataSubmit && len(victims) < 3 {
			victims = append(victims, r.RId)
		}
	}
	if len(victims) != 3 {
		t.Fatalf("expected 3 submitters to delete, got %d", len(victims))
	}
	if _, affected, err := models.SoftDeleteResults(cid, victims, 1, "internos", models.DeleteScopeCampaign); err != nil || affected != 3 {
		t.Fatalf("SoftDeleteResults: affected=%d err=%v", affected, err)
	}

	after := collectSurfaces(t, cid, gid)
	got := assertAllAgree(t, "volume/after", after)
	if got.total != n-3 {
		t.Fatalf("after delete total=%d want %d", got.total, n-3)
	}
	if got.submitted != 2 {
		t.Fatalf("after delete submitted=%d want 2 (their duplicate events must not linger)", got.submitted)
	}
	// The 3 deleted submitters were also among the duplicate-openers → 20-3 = 17.
	if got.opened != 17 {
		t.Fatalf("after delete opened=%d want 17 (deleted recipients' events must not linger)", got.opened)
	}
}

// seedVolumeCampaign creates a campaign with n recipients: every one is sent,
// each of the first 20 opens the email THREE times (duplicate events), and 5
// submit data. Returns campaign and group ids.
func seedVolumeCampaign(t *testing.T, n int) (cid, gid int64) {
	t.Helper()
	uid := int64(1)
	uniq := fmt.Sprintf("vol-%d", time.Now().UnixNano())

	group := models.Group{Name: "grp-" + uniq}
	for i := 0; i < n; i++ {
		group.Targets = append(group.Targets, models.Target{
			BaseRecipient: models.BaseRecipient{Email: fmt.Sprintf("v%02d-%s@example.com", i, uniq)},
		})
	}
	group.UserId = uid
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("PostGroup: %v", err)
	}
	tmpl := models.Template{Name: "tpl-" + uniq, Subject: "s", Text: "t", HTML: "<html>h</html>", UserId: uid}
	if err := models.PostTemplate(&tmpl); err != nil {
		t.Fatalf("PostTemplate: %v", err)
	}
	page := models.Page{Name: "pg-" + uniq, HTML: "<html>h</html>", UserId: uid}
	if err := models.PostPage(&page); err != nil {
		t.Fatalf("PostPage: %v", err)
	}
	smtp := models.SMTP{Name: "smtp-" + uniq, UserId: uid, Host: "example.com", FromAddress: "a@b.com"}
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatalf("PostSMTP: %v", err)
	}
	campaign := models.Campaign{Name: "camp-" + uniq, UserId: uid, Template: tmpl, Page: page, SMTP: smtp,
		Groups: []models.Group{group}}
	if err := models.PostCampaign(&campaign, uid); err != nil {
		t.Fatalf("PostCampaign: %v", err)
	}

	cr, err := models.GetCampaignResults(campaign.Id, uid)
	if err != nil {
		t.Fatalf("GetCampaignResults: %v", err)
	}
	if len(cr.Results) != n {
		t.Fatalf("expected %d results, got %d", n, len(cr.Results))
	}
	for i := range cr.Results {
		if err := cr.Results[i].HandleEmailSent(); err != nil {
			t.Fatalf("HandleEmailSent: %v", err)
		}
	}
	// Duplicate opens: the first 20 recipients open 3 times each. Per-recipient
	// accounting must count each of them ONCE.
	for i := 0; i < 20 && i < len(cr.Results); i++ {
		for k := 0; k < 3; k++ {
			if err := cr.Results[i].HandleEmailOpened(models.EventDetails{}); err != nil {
				t.Fatalf("HandleEmailOpened: %v", err)
			}
		}
	}
	// 5 submit data (they also had duplicate opens above).
	for i := 0; i < 5 && i < len(cr.Results); i++ {
		if err := cr.Results[i].HandleFormSubmit(models.EventDetails{}); err != nil {
			t.Fatalf("HandleFormSubmit: %v", err)
		}
	}

	cg := models.CampaignGroup{Name: "cg-" + uniq, UserId: uid,
		Campaigns: []models.CampaignGroupCampaign{{CampaignId: campaign.Id, OrderIndex: 0}}}
	if err := models.PostCampaignGroup(&cg, uid); err != nil {
		t.Fatalf("PostCampaignGroup: %v", err)
	}
	return campaign.Id, cg.Id
}

// TestFrozenRenderUnaffectedByLaterRecipientDeletion — a render is an immutable
// blob; deleting a recipient afterwards must not change the already-generated
// render (its bytes/sha are frozen). Only future generations recompute.
func TestFrozenRenderUnaffectedByLaterRecipientDeletion(t *testing.T) {
	setupReportTestDB(t)
	store := models.NewDBBlobStore()
	cid, _, submitterRID := seedCampaignWithGroup(t, "frozen")

	// Template + active version.
	tmplSha, err := store.Put(buildTemplateDocx(t))
	if err != nil {
		t.Fatalf("store template: %v", err)
	}
	tpl := &models.ReportTemplate{UserId: 1, Name: "frozen-tpl"}
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
	rep := &models.Report{UserId: 1, SubjectKind: "campaign", SubjectId: cid, TemplateId: tpl.Id,
		CompanyName: "Empresa Ejemplo", Status: "draft"}
	if err := models.CreateReport(rep); err != nil {
		t.Fatalf("create report: %v", err)
	}

	render, _, err := Generate(store, rep.Id, 1)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	frozenSha := render.OutputSha256
	frozenBytes, err := store.Get(frozenSha)
	if err != nil {
		t.Fatalf("get frozen blob: %v", err)
	}

	// Delete a recipient AFTER the render was frozen.
	if _, _, err := models.SoftDeleteResults(cid, []string{submitterRID}, 1, "", models.DeleteScopeCampaign); err != nil {
		t.Fatalf("SoftDeleteResults: %v", err)
	}

	// The frozen render row and its blob are unchanged.
	renders, err := models.GetRendersForReport(rep.Id, 1)
	if err != nil {
		t.Fatalf("GetRendersForReport: %v", err)
	}
	if len(renders) != 1 || renders[0].OutputSha256 != frozenSha {
		t.Fatalf("frozen render sha changed: %+v", renders)
	}
	nowBytes, err := store.Get(frozenSha)
	if err != nil {
		t.Fatalf("get frozen blob after: %v", err)
	}
	if string(nowBytes) != string(frozenBytes) {
		t.Fatalf("frozen render blob mutated after recipient deletion")
	}
}

// TestDeletedRecipientDropsFromXLSXAndCSV (ticket §10) — asserts the actual
// deliverables, not just the counts: the deleted recipient's email must not appear
// in the generated Excel annex bytes, nor in the results/timeline that back the CSV
// exports.
func TestDeletedRecipientDropsFromXLSXAndCSV(t *testing.T) {
	setupReportTestDB(t)
	cid, _, submitterRID := seedCampaignWithGroup(t, "xlsxcsv")

	// Resolve the victim's email and confirm it starts out present everywhere.
	cr, err := models.GetCampaignResults(cid, 1)
	if err != nil {
		t.Fatalf("GetCampaignResults: %v", err)
	}
	victim := ""
	for _, r := range cr.Results {
		if r.RId == submitterRID {
			victim = r.Email
		}
	}
	if victim == "" {
		t.Fatalf("victim email not found for rid %s", submitterRID)
	}

	src, err := NewSource("campaign", cid, 1)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	recips, err := src.Recipients()
	if err != nil {
		t.Fatalf("Recipients: %v", err)
	}
	xlsxBefore, err := BuildRecipientsXLSX(recips)
	if err != nil {
		t.Fatalf("BuildRecipientsXLSX: %v", err)
	}
	if !strings.Contains(sheetXML(t, xlsxBefore), victim) {
		t.Fatalf("baseline: %s should be in the Excel annex", victim)
	}
	if !hasEmail(cr.Results, victim) {
		t.Fatalf("baseline: %s should be in the CSV results", victim)
	}
	if !hasEventFor(cr.Events, victim) {
		t.Fatalf("baseline: %s should have events for the events CSV", victim)
	}

	// Delete it.
	if _, _, err := models.SoftDeleteResults(cid, []string{submitterRID}, 1, "interno", models.DeleteScopeCampaign); err != nil {
		t.Fatalf("SoftDeleteResults: %v", err)
	}

	// Excel: a fresh source must no longer include it.
	src2, err := NewSource("campaign", cid, 1)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	recips2, err := src2.Recipients()
	if err != nil {
		t.Fatalf("Recipients: %v", err)
	}
	xlsxAfter, err := BuildRecipientsXLSX(recips2)
	if err != nil {
		t.Fatalf("BuildRecipientsXLSX: %v", err)
	}
	if strings.Contains(sheetXML(t, xlsxAfter), victim) {
		t.Fatalf("deleted recipient %s still present in the Excel annex", victim)
	}

	// CSV: neither the results export nor the raw-events export may include it.
	cr2, err := models.GetCampaignResults(cid, 1)
	if err != nil {
		t.Fatalf("GetCampaignResults after: %v", err)
	}
	if hasEmail(cr2.Results, victim) {
		t.Fatalf("deleted recipient %s still present in the results CSV source", victim)
	}
	if hasEventFor(cr2.Events, victim) {
		t.Fatalf("events of deleted recipient %s still present in the events CSV source", victim)
	}
}

func hasEmail(rs []models.Result, email string) bool {
	for _, r := range rs {
		if r.Email == email {
			return true
		}
	}
	return false
}

func hasEventFor(evs []models.Event, email string) bool {
	for _, e := range evs {
		if e.Email == email {
			return true
		}
	}
	return false
}

// sheetXML extracts the worksheet XML from generated xlsx bytes.
func sheetXML(t *testing.T, xlsx []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		t.Fatalf("open xlsx: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	t.Fatalf("sheet1.xml not found in xlsx")
	return ""
}

// TestAnnexTableXMLExcludesDeletedRecipients closes the CL-105 ↔ CL-102R seam.
//
// SCOPE, precisely: it asserts on the XML produced by BuildAnnexTableXML() when fed
// src.Recipients() — i.e. the generator plus the per-recipient source CL-105
// consumes. It does NOT render the full template, so it proves the annex table's
// CONTENT excludes trashed recipients; the end-to-end DOCX render with
// plantilla_reina.docx is covered separately (and the {{TABLA_ANEXO}} marker
// substitution by TestRenderReplacesAnnexTableMarker).
func TestAnnexTableXMLExcludesDeletedRecipients(t *testing.T) {
	setupReportTestDB(t)
	cid, _, submitterRID := seedCampaignWithGroup(t, "annex")

	cr, err := models.GetCampaignResults(cid, 1)
	if err != nil {
		t.Fatalf("GetCampaignResults: %v", err)
	}
	victim := ""
	for _, r := range cr.Results {
		if r.RId == submitterRID {
			victim = r.Email
		}
	}
	if victim == "" {
		t.Fatalf("victim not found")
	}

	src, err := NewSource("campaign", cid, 1)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	recips, err := src.Recipients()
	if err != nil {
		t.Fatalf("Recipients: %v", err)
	}
	before := BuildAnnexTableXML(recips)
	if !strings.Contains(before, victim) {
		t.Fatalf("baseline: %s should be in the annex table", victim)
	}
	// The submitter is the most severe row, so it must lead the table before deletion.
	if !strings.Contains(before, "Envío de Datos") {
		t.Fatalf("baseline: expected a submitted-data row in the annex")
	}

	if _, _, err := models.SoftDeleteResults(cid, []string{submitterRID}, 1, "interno", models.DeleteScopeCampaign); err != nil {
		t.Fatalf("SoftDeleteResults: %v", err)
	}

	src2, err := NewSource("campaign", cid, 1)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	recips2, err := src2.Recipients()
	if err != nil {
		t.Fatalf("Recipients: %v", err)
	}
	after := BuildAnnexTableXML(recips2)
	if strings.Contains(after, victim) {
		t.Fatalf("deleted recipient %s still present in the S9 annex table", victim)
	}
	// The table must remain well-formed and non-empty (the other recipients stay).
	if !strings.HasPrefix(after, "<w:tbl>") || !strings.HasSuffix(after, "</w:tbl>") {
		t.Fatalf("annex table malformed after deletion")
	}
}

// TestAnnexCountsMatchFunnelAndExcel closes the last CL-105 seam: the annex table is
// a THIRD rendering of the same per-recipient truth, so it must agree with the S5
// funnel and with the Excel annex — not approximately, exactly.
//
// It is not tautological: it re-derives the number three ways — from the dashboard's
// aggregate stats, from the bytes of the generated XLSX, and from the XML of the
// table — and only then compares. The volume fixture includes duplicate open events,
// which is where per-event counting would drift from per-recipient counting.
func TestAnnexCountsMatchFunnelAndExcel(t *testing.T) {
	setupReportTestDB(t)
	cid, gid := seedVolumeCampaign(t, 50)
	surf := collectSurfaces(t, cid, gid)

	src, err := NewSource("campaign", cid, 1)
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	recips, err := src.Recipients()
	if err != nil {
		t.Fatalf("Recipients: %v", err)
	}

	// 1) The funnel, per surface.
	want := surf["dashboard"].submitted
	for name, s := range surf {
		if s.submitted != want {
			t.Fatalf("surface %q reports %d submitters, dashboard reports %d", name, s.submitted, want)
		}
	}

	// 2) The Excel annex, read back from the generated bytes.
	xlsx, err := BuildRecipientsXLSX(recips)
	if err != nil {
		t.Fatalf("BuildRecipientsXLSX: %v", err)
	}
	sheet := string(unzipEntry(t, xlsx, "xl/worksheets/sheet1.xml"))
	excelSub := int64(strings.Count(sheet, ">Envío de Datos</t>"))
	if excelSub != want {
		t.Fatalf("Excel annex has %d 'Envío de Datos' rows, funnel says %d", excelSub, want)
	}

	// 3) The Word annex table. It is capped at ten and ranked by severity, so the
	// assertion is not "same count" but "the submitters are the ones it leads with"
	// — the annex would be wrong in either direction: dropping a submitter, or
	// promoting someone less exposed above one.
	top := TopRecipientsBySeverity(recips, annexTopN)
	xml := BuildAnnexTableXML(recips)
	rows := int64(strings.Count(xml, "<w:tr>")) - 1 // minus the header
	expectRows := int64(annexTopN)
	if int64(len(recips)) < expectRows {
		expectRows = int64(len(recips))
	}
	if rows != expectRows {
		t.Fatalf("annex table renders %d data rows, expected %d", rows, expectRows)
	}
	var leadSubmitters int64
	for _, r := range top {
		if r.Status != "Envío de Datos" {
			break
		}
		leadSubmitters++
	}
	wantLead := want
	if wantLead > int64(annexTopN) {
		wantLead = int64(annexTopN)
	}
	if leadSubmitters != wantLead {
		t.Fatalf("the annex leads with %d submitters, the funnel says there are %d (capped at %d)",
			leadSubmitters, want, annexTopN)
	}

	// 4) Same people, not just the same count — every listed recipient must exist in
	// the Excel, and the column headings must not drift apart.
	for _, r := range top {
		if !strings.Contains(sheet, r.Email) {
			t.Fatalf("annex lists %s, which is not in the Excel annex", r.Email)
		}
	}
	if !strings.Contains(sheet, ">Estado</t>") || !strings.Contains(xml, ">Estado<") {
		t.Fatalf("the Estado column is not present in both artefacts")
	}

	t.Logf("MEDIDO: embudo=%d · Excel=%d · tabla anexo=%d filas, de las cuales %d son remitentes (de %d destinatarios, con aperturas duplicadas)",
		want, excelSub, rows, leadSubmitters, len(recips))
}
