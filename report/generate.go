package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/models"
)

// RenderSnapshot is the immutable, self-contained record of everything (other
// than the template bytes and asset bytes, which live in the BlobStore) needed
// to rebuild a report byte-for-byte. It is stored as report_renders.metrics_json
// and is never read from live campaign state again.
type RenderSnapshot struct {
	Funnel          FunnelMetrics     `json:"funnel"`
	TotalRecipients int64             `json:"total_recipients"`
	UsersWith2FA    int64             `json:"users_with_2fa"`
	Vars            map[string]string `json:"vars"`
	// AnnexTableXML freezes the S9 annex table (CL-105) so the audit rebuild
	// reproduces the DOCX byte-for-byte. Empty for legacy renders (image annex).
	AnnexTableXML string `json:"annex_table_xml,omitempty"`
}

// Generate is the production entry point: it resolves the report's data source
// (Campaign or Campaign Group, transparently, through ReportSource), renders the
// DOCX and freezes an immutable render. It works only against the interface;
// the funnel derivation and 2FA logic are identical for both subject kinds.
func Generate(store BlobStore, reportID, uid int64) (models.ReportRender, []byte, error) {
	report, err := models.GetReport(reportID, uid)
	if err != nil {
		return models.ReportRender{}, nil, err
	}
	version, err := models.GetActiveTemplateVersion(report.TemplateId)
	if err != nil {
		return models.ReportRender{}, nil, err
	}
	src, err := NewSource(report.SubjectKind, report.SubjectId, uid)
	if err != nil {
		return models.ReportRender{}, nil, err
	}
	return generate(store, report, version, src, uid)
}

// generate is the source-agnostic core. It is unexported and accepts an
// explicit ReportSource so it can be exercised without seeding a full campaign.
func generate(store BlobStore, report models.Report, version models.ReportTemplateVersion, src ReportSource, generatedBy int64) (models.ReportRender, []byte, error) {
	// 1) Resolve metrics and the token map from the data source.
	funnel, total, vars, err := buildReportVars(src, report)
	if err != nil {
		return models.ReportRender{}, nil, err
	}
	// 2) Load the frozen template bytes once.
	templateBytes, err := store.Get(version.ContentSha256)
	if err != nil {
		return models.ReportRender{}, nil, fmt.Errorf("loading template version: %w", err)
	}
	// 3) Gather the images to swap + their frozen references (uploaded assets
	//    plus, only if the template uses an image for it, the results chart).
	images, frozen, err := buildImages(store, report.Id, funnel, templateBytes)
	if err != nil {
		return models.ReportRender{}, nil, err
	}
	// 4) Render the DOCX and obtain its fingerprint, then store it as a blob. The
	//    S9 annex table (CL-105) is built from the SAME per-recipient rows as the
	//    Excel annex (already excludes internal validation recipients, CL-102).
	annexTableXML := ""
	if recips, rerr := src.Recipients(); rerr != nil {
		log.Errorf("report %d: could not load recipients for annex table: %v", report.Id, rerr)
	} else {
		annexTableXML = BuildAnnexTableXML(recips)
	}
	docx, fingerprint, err := buildDocument(templateBytes, vars, images, funnel, report.UsersWith2FA > 0, annexTableXML)
	if err != nil {
		return models.ReportRender{}, nil, err
	}
	outSha, err := store.Put(docx)
	if err != nil {
		return models.ReportRender{}, nil, err
	}
	// 5) Best-effort Excel annex (never fails the DOCX; failures are logged).
	xlsxSha := buildExcelAnnex(store, src, report.Id)
	// 6) Freeze the immutable render row + its asset references, mark generated.
	render, err := createRender(report, version, renderOutputs{
		funnel: funnel, total: total, vars: vars,
		outSha: outSha, xlsxSha: xlsxSha, fingerprint: fingerprint, outputSize: int64(len(docx)),
		generatedBy: generatedBy, frozen: frozen, annexTableXML: annexTableXML,
	})
	if err != nil {
		return models.ReportRender{}, nil, err
	}
	return render, docx, nil
}

// buildReportVars resolves the funnel metrics, total recipients and the token
// map from the data source. Extracted from generate for clarity (audit M-12);
// behavior is identical.
func buildReportVars(src ReportSource, report models.Report) (FunnelMetrics, int64, map[string]string, error) {
	stats, err := src.Stats()
	if err != nil {
		return FunnelMetrics{}, 0, nil, err
	}
	funnel := Funnel(stats)
	start, end, err := src.DateRange()
	if err != nil {
		return FunnelMetrics{}, 0, nil, err
	}
	total := stats.TotalRecipients
	if total == 0 {
		total = funnel.Total
	}
	vars := BuildVars(VarInput{
		CompanyName:     report.CompanyName,
		PreparedBy:      report.PreparedBy,
		ReportDate:      report.ReportDate,
		ExecutedFrom:    pick(report.ExecutedFrom, start),
		ExecutedTo:      pick(report.ExecutedTo, end),
		IntroExec:       report.IntroExec,
		ImpersonatedAs:  report.ImpersonatedAs,
		Communique:      report.Communique,
		TextPunto1:      report.TextPunto1,
		Had2FA:          report.UsersWith2FA > 0,
		TotalRecipients: total,
		Funnel:          funnel,
	})
	return funnel, total, vars, nil
}

// buildImages loads the uploaded assets (and their frozen references) for the
// report, and — only when the template fills grafico_1 with an actual image
// (not a native chart, audit I-1) — generates, stores and freezes the results
// chart PNG. Behavior identical to the previous inline code.
func buildImages(store BlobStore, reportID int64, funnel FunnelMetrics, templateBytes []byte) (map[string][]byte, []models.ReportRenderAsset, error) {
	images := make(map[string][]byte)
	var frozen []models.ReportRenderAsset

	assets, err := models.GetReportAssets(reportID)
	if err != nil {
		return nil, nil, err
	}
	for _, a := range assets {
		b, err := store.Get(a.ContentSha256)
		if err != nil {
			return nil, nil, fmt.Errorf("loading asset %q: %w", a.Slot, err)
		}
		images[a.Slot] = b
		frozen = append(frozen, models.ReportRenderAsset{Slot: a.Slot, ContentSha256: a.ContentSha256, Mime: a.Mime})
	}

	if backed, berr := SlotIsImageBacked(templateBytes, "grafico_1"); berr == nil && backed {
		chartPNG, cerr := ChartPNG(funnel)
		if cerr != nil {
			return nil, nil, cerr
		}
		chartSha, perr := store.Put(chartPNG)
		if perr != nil {
			return nil, nil, perr
		}
		images["grafico_1"] = chartPNG
		frozen = append(frozen, models.ReportRenderAsset{Slot: "grafico_1", ContentSha256: chartSha, Mime: "image/png"})
	}
	return images, frozen, nil
}

// buildDocument renders the DOCX and returns it with its content fingerprint
// (computed by Render from its in-memory parts). The native results chart is
// driven by the SAME funnel buckets as the results table.
func buildDocument(templateBytes []byte, vars map[string]string, images map[string][]byte, funnel FunnelMetrics, had2FA bool, annexTableXML string) ([]byte, string, error) {
	return Render(RenderInput{
		Template: templateBytes,
		Vars:     vars,
		Images:   images,
		ChartValues: []float64{
			float64(funnel.Ignorado), float64(funnel.Abierto),
			float64(funnel.Clic), float64(funnel.Datos),
		},
		Conditions:    map[string]bool{"HAD_2FA": had2FA},
		AnnexTableXML: annexTableXML,
	})
}

// buildExcelAnnex builds and stores the per-recipient Excel annex and returns
// its blob sha (empty if there is nothing to store). The annex is best-effort:
// it NEVER fails the DOCX report, but each failure mode is logged and
// distinguished from a legitimately empty recipient list (audit I-6).
func buildExcelAnnex(store BlobStore, src ReportSource, reportID int64) string {
	recips, rerr := src.Recipients()
	switch {
	case rerr != nil:
		log.Errorf("report %d: could not load recipients for Excel annex: %v", reportID, rerr)
		return ""
	case len(recips) == 0:
		log.Infof("report %d: no recipients; Excel annex omitted (legitimately empty)", reportID)
		return ""
	}
	xlsx, xerr := BuildRecipientsXLSX(recips)
	if xerr != nil {
		log.Errorf("report %d: building Excel annex failed: %v", reportID, xerr)
		return ""
	}
	sha, perr := store.Put(xlsx)
	if perr != nil {
		log.Errorf("report %d: storing Excel annex failed: %v", reportID, perr)
		return ""
	}
	return sha
}

// renderOutputs bundles the inputs createRender needs (keeps its signature
// readable).
type renderOutputs struct {
	funnel        FunnelMetrics
	total         int64
	vars          map[string]string
	outSha        string
	xlsxSha       string
	fingerprint   string
	outputSize    int64
	generatedBy   int64
	frozen        []models.ReportRenderAsset
	annexTableXML string
}

// createRender freezes the immutable render row (with its metrics snapshot and
// asset references) and marks the report generated.
func createRender(report models.Report, version models.ReportTemplateVersion, o renderOutputs) (models.ReportRender, error) {
	snapshot := RenderSnapshot{
		Funnel:          o.funnel,
		TotalRecipients: o.total,
		UsersWith2FA:    report.UsersWith2FA,
		Vars:            o.vars,
		AnnexTableXML:   o.annexTableXML,
	}
	metricsJSON, err := json.Marshal(snapshot)
	if err != nil {
		return models.ReportRender{}, err
	}
	render := models.ReportRender{
		ReportId:           report.Id,
		TemplateVersionId:  version.Id,
		MetricsJSON:        string(metricsJSON),
		OutputSha256:       o.outSha,
		OutputXlsxSha256:   o.xlsxSha,
		ContentFingerprint: o.fingerprint,
		OutputSize:         o.outputSize,
		GeneratedBy:        o.generatedBy,
	}
	if err := models.CreateReportRender(&render, o.frozen); err != nil {
		return models.ReportRender{}, err
	}
	if err := models.SetReportStatus(report.Id, report.UserId, "generated"); err != nil {
		return models.ReportRender{}, err
	}
	return render, nil
}

// DownloadRender returns the exact, byte-for-byte DOCX of a frozen render from
// the content-addressed blob store. This is the normal download path and never
// re-renders, so it is independent of render.go/go-chart/Go versions.
//
// Legacy renders (created before C1, with output_sha256 but no stored blob) are
// handled by lazy backfill: the DOCX is rebuilt from frozen data and audited;
// if it matches, the blob is backfilled and served; if it does not match, an
// error is returned because the exact historical bytes cannot be recovered.
func DownloadRender(store BlobStore, renderID, uid int64) ([]byte, error) {
	render, assets, err := models.GetReportRender(renderID, uid)
	if err != nil {
		return nil, err
	}
	if render.OutputSha256 != "" {
		if exists, _ := store.Exists(render.OutputSha256); exists {
			return store.Get(render.OutputSha256)
		}
	}
	// Legacy render without a stored blob: rebuild + audit + lazy backfill. Reuse
	// the assets already loaded above instead of re-fetching them (audit M-6).
	docx, audit, err := rebuildAndAudit(store, render, assets)
	if err != nil {
		return nil, err
	}
	if !audit.Match {
		return nil, fmt.Errorf("legacy render %d cannot be reproduced exactly: %s", renderID, audit.Reason)
	}
	if _, err := store.Put(docx); err != nil {
		return nil, err
	}
	return docx, nil
}

// AuditResult is the verdict of a reproducibility audit.
type AuditResult struct {
	Match  bool   `json:"match"`
	Reason string `json:"reason"`
}

// AuditRender re-renders a frozen render from frozen data only and compares the
// result against the stored content fingerprint (preferred) or output hash
// (legacy). It NEVER serves the rebuilt bytes; it returns a verdict. A mismatch
// means the stored evidence was altered or the renderer/toolchain changed.
func AuditRender(store BlobStore, renderID, uid int64) (AuditResult, error) {
	render, assets, err := models.GetReportRender(renderID, uid)
	if err != nil {
		return AuditResult{}, err
	}
	// Reuse the render + assets already loaded; rebuildAndAudit no longer
	// re-queries them (audit M-6).
	_, audit, err := rebuildAndAudit(store, render, assets)
	return audit, err
}

// rebuildAndAudit reconstructs the DOCX from the frozen template version,
// metrics snapshot and asset references, and produces an audit verdict.
func rebuildAndAudit(store BlobStore, render models.ReportRender, assets []models.ReportRenderAsset) ([]byte, AuditResult, error) {
	version, err := models.GetReportTemplateVersionByID(render.TemplateVersionId)
	if err != nil {
		return nil, AuditResult{}, err
	}
	templateBytes, err := store.Get(version.ContentSha256)
	if err != nil {
		return nil, AuditResult{}, err
	}
	var snapshot RenderSnapshot
	if err := json.Unmarshal([]byte(render.MetricsJSON), &snapshot); err != nil {
		return nil, AuditResult{}, fmt.Errorf("decoding frozen metrics: %w", err)
	}
	images := make(map[string][]byte)
	for _, a := range assets {
		b, err := store.Get(a.ContentSha256)
		if err != nil {
			return nil, AuditResult{}, fmt.Errorf("loading frozen asset %q: %w", a.Slot, err)
		}
		images[a.Slot] = b
	}
	docx, fp, err := Render(RenderInput{
		Template: templateBytes,
		Vars:     snapshot.Vars,
		Images:   images,
		ChartValues: []float64{
			float64(snapshot.Funnel.Ignorado), float64(snapshot.Funnel.Abierto),
			float64(snapshot.Funnel.Clic), float64(snapshot.Funnel.Datos),
		},
		Conditions:    map[string]bool{"HAD_2FA": snapshot.UsersWith2FA > 0},
		AnnexTableXML: snapshot.AnnexTableXML,
	})
	if err != nil {
		return nil, AuditResult{}, err
	}

	// Prefer the toolchain-stable fingerprint (computed by Render); fall back to
	// the raw zip hash for legacy renders without a stored fingerprint.
	if render.ContentFingerprint != "" {
		if fp == render.ContentFingerprint {
			return docx, AuditResult{Match: true, Reason: "content fingerprint matches frozen render"}, nil
		}
		return docx, AuditResult{Match: false, Reason: "content fingerprint mismatch: evidence altered or renderer changed"}, nil
	}
	if got := sha256Hex(docx); got == render.OutputSha256 {
		return docx, AuditResult{Match: true, Reason: "output hash matches frozen render"}, nil
	}
	return docx, AuditResult{Match: false, Reason: "output hash mismatch: evidence altered or toolchain changed"}, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func pick(t, fallback time.Time) time.Time {
	if t.IsZero() {
		return fallback
	}
	return t
}
