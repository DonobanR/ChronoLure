package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	ctx "github.com/gophish/gophish/context"
	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/models"
	"github.com/gophish/gophish/report"
	"github.com/gorilla/mux"
)

// This file contains the reporting module's HTTP handlers. The handlers are
// thin orchestrators only: they validate requests, enforce ownership (every
// query is scoped by user_id), call the domain services in report/ and models/,
// and serialize responses. They never build variables, render DOCX, generate
// charts, resolve metrics, compute hashes or manipulate blobs.

const maxReportUploadBytes = 32 << 20 // 32 MiB

// Centralized user-facing messages reused across the reporting handlers (audit
// M-12). These are constants with the SAME text as before — no message changes,
// just a single source so the wording stays consistent.
const (
	msgMethodNotAllowed = "Método no permitido."
	msgTemplateNotFound = "La plantilla no existe o no tienes acceso a ella."
	msgReportNotFound   = "El informe no existe o no tienes acceso a él."
	msgRenderNotFound   = "El documento generado no existe o no tienes acceso a él."
	msgInvalidJSON      = "Los datos enviados no son válidos."
)

// blobStore returns the configured blob store. Centralized so handlers do not
// construct storage details inline.
func blobStore() report.BlobStore { return models.NewDBBlobStore() }

// registerReportRoutes wires the reporting endpoints. It is only called when
// the reporting feature flag is enabled.
func (as *Server) registerReportRoutes(router *mux.Router) {
	router.HandleFunc("/report-templates", as.ReportTemplates)
	router.HandleFunc("/report-templates/{id:[0-9]+}", as.ReportTemplate)
	router.HandleFunc("/report-templates/{id:[0-9]+}/versions", as.ReportTemplateVersions)
	router.HandleFunc("/report-templates/{id:[0-9]+}/versions/{vid:[0-9]+}/download", as.ReportTemplateVersionDownload)
	router.HandleFunc("/report-templates/{id:[0-9]+}/active-version", as.ReportTemplateActiveVersion)
	router.HandleFunc("/reports", as.ReportsCollection)
	router.HandleFunc("/reports/{id:[0-9]+}", as.Report)
	router.HandleFunc("/report-slots", as.ReportSlots)
	router.HandleFunc("/reports/{id:[0-9]+}/assets", as.ReportAssetsList)
	router.HandleFunc("/reports/{id:[0-9]+}/assets/{slot}", as.ReportAsset)
	router.HandleFunc("/reports/{id:[0-9]+}/generate", as.ReportGenerate)
	router.HandleFunc("/reports/{id:[0-9]+}/renders", as.ReportRenders)
	router.HandleFunc("/renders/{id:[0-9]+}", as.Render)
	router.HandleFunc("/renders/{id:[0-9]+}/rebuild", as.RenderRebuild)
	// The download endpoints are registered only when their flag is enabled.
	if as.blobDownloadEnabled {
		router.HandleFunc("/renders/{id:[0-9]+}/download", as.RenderDownload)
		router.HandleFunc("/renders/{id:[0-9]+}/download-xlsx", as.RenderDownloadXLSX)
	}
}

func reportUID(r *http.Request) int64 { return ctx.Get(r, "user_id").(int64) }

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	return id
}

// ReportTemplates handles GET (list) and POST (create) on /report-templates.
func (as *Server) ReportTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		ts, err := models.GetReportTemplates(reportUID(r))
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: "No se pudieron cargar las plantillas."}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, ts, http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	t := models.ReportTemplate{}
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgInvalidJSON}, http.StatusBadRequest)
		return
	}
	t.UserId = reportUID(r)
	t.ActiveVersionId = 0
	if err := models.CreateReportTemplate(&t); err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo crear la plantilla."}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, t, http.StatusCreated)
}

// ReportTemplate handles GET/DELETE /report-templates/{id}.
func (as *Server) ReportTemplate(w http.ResponseWriter, r *http.Request) {
	uid := reportUID(r)
	id := pathID(r)
	t, err := models.GetReportTemplate(id, uid)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgTemplateNotFound}, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		JSONResponse(w, t, http.StatusOK)
	case http.MethodDelete:
		force := r.URL.Query().Get("force") == "true"
		isAdmin := userIsAdmin(r)
		if force && !isAdmin {
			JSONResponse(w, models.Response{Success: false, Message: "Necesitas permisos de administrador para forzar la eliminación de esta plantilla."}, http.StatusForbidden)
			return
		}
		if err := models.DeleteReportTemplate(id, uid, force); err != nil {
			if err == models.ErrReportTemplateInUse {
				msg := "No se puede eliminar esta plantilla porque existen informes generados anteriormente que la utilizan."
				if isAdmin {
					msg = "Esta plantilla está siendo utilizada por informes históricos. Como administrador puedes forzar su eliminación (los informes ya generados se conservan, pero no podrán re-auditarse)."
				}
				JSONResponse(w, map[string]interface{}{"success": false, "message": msg, "can_force": isAdmin}, http.StatusConflict)
				return
			}
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: "Ocurrió un error al eliminar la plantilla. Inténtalo de nuevo."}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Plantilla eliminada"}, http.StatusOK)
	default:
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
	}
}

// ReportTemplateVersions handles GET (list) and POST (upload DOCX) on
// /report-templates/{id}/versions.
func (as *Server) ReportTemplateVersions(w http.ResponseWriter, r *http.Request) {
	uid := reportUID(r)
	id := pathID(r)
	switch r.Method {
	case http.MethodGet:
		vs, err := models.GetReportTemplateVersions(id, uid)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: msgTemplateNotFound}, http.StatusNotFound)
			return
		}
		JSONResponse(w, vs, http.StatusOK)
	case http.MethodPost:
		content, ok := readUpload(w, r)
		if !ok {
			return
		}
		version, insp, err := report.UploadTemplateVersion(blobStore(), id, uid, content, uid)
		if err == report.ErrTemplateInvalid {
			JSONResponse(w, map[string]interface{}{"success": false, "message": "El documento no superó la validación. Revisa los detalles indicados.", "inspection": insp}, http.StatusBadRequest)
			return
		}
		if err != nil {
			// Ownership / not found surfaces here too.
			JSONResponse(w, models.Response{Success: false, Message: msgTemplateNotFound}, http.StatusNotFound)
			return
		}
		JSONResponse(w, map[string]interface{}{"version": version, "inspection": insp}, http.StatusCreated)
	default:
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
	}
}

// ReportTemplateVersionDownload handles GET
// /report-templates/{id}/versions/{vid}/download, serving the stored DOCX of a
// template version so the operator can open/inspect what was uploaded.
func (as *Server) ReportTemplateVersionDownload(w http.ResponseWriter, r *http.Request) {
	uid := reportUID(r)
	id := pathID(r)
	if _, err := models.GetReportTemplate(id, uid); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgTemplateNotFound}, http.StatusNotFound)
		return
	}
	vid, _ := strconv.ParseInt(mux.Vars(r)["vid"], 10, 64)
	version, err := models.GetReportTemplateVersionByID(vid)
	if err != nil || version.TemplateId != id {
		JSONResponse(w, models.Response{Success: false, Message: "La versión indicada no pertenece a esta plantilla."}, http.StatusNotFound)
		return
	}
	content, err := blobStore().Get(version.ContentSha256)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo recuperar el archivo de la plantilla."}, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"plantilla_v%d.docx\"", version.Version))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// ReportTemplateActiveVersion handles PUT /report-templates/{id}/active-version.
func (as *Server) ReportTemplateActiveVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	uid := reportUID(r)
	id := pathID(r)
	if _, err := models.GetReportTemplate(id, uid); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgTemplateNotFound}, http.StatusNotFound)
		return
	}
	var req struct {
		VersionId int64 `json:"version_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgInvalidJSON}, http.StatusBadRequest)
		return
	}
	version, err := models.GetReportTemplateVersionByID(req.VersionId)
	if err != nil || version.TemplateId != id {
		JSONResponse(w, models.Response{Success: false, Message: "La versión indicada no pertenece a esta plantilla."}, http.StatusBadRequest)
		return
	}
	if err := models.SetActiveTemplateVersion(id, req.VersionId); err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo activar la versión seleccionada."}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, models.Response{Success: true, Message: "Versión activa actualizada."}, http.StatusOK)
}

// ReportsCollection handles GET (list) and POST (create draft) on /reports.
func (as *Server) ReportsCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		rs, err := models.GetReports(reportUID(r))
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: "No se pudieron cargar los informes."}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, rs, http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	uid := reportUID(r)
	rep := models.Report{}
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgInvalidJSON}, http.StatusBadRequest)
		return
	}
	rep.UserId = uid
	rep.Status = "draft"
	if _, err := models.GetReportTemplate(rep.TemplateId, uid); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "La plantilla seleccionada no existe o no tienes acceso a ella. Elige una plantilla válida."}, http.StatusBadRequest)
		return
	}
	if !ownsSubject(rep.SubjectKind, rep.SubjectId, uid) {
		subj := "campaña"
		if rep.SubjectKind == "campaign_group" {
			subj = "grupo de campañas"
		}
		JSONResponse(w, models.Response{Success: false, Message: "La " + subj + " seleccionada no existe, fue eliminada o no tienes permisos para acceder a ella."}, http.StatusBadRequest)
		return
	}
	if err := models.CreateReport(&rep); err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo crear el informe."}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, rep, http.StatusCreated)
}

// Report handles GET/PUT/DELETE /reports/{id}.
func (as *Server) Report(w http.ResponseWriter, r *http.Request) {
	uid := reportUID(r)
	id := pathID(r)
	existing, err := models.GetReport(id, uid)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgReportNotFound}, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		JSONResponse(w, existing, http.StatusOK)
	case http.MethodPut:
		var body models.Report
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: msgInvalidJSON}, http.StatusBadRequest)
			return
		}
		body.Id = id
		if body.TemplateId != 0 && body.TemplateId != existing.TemplateId {
			if _, err := models.GetReportTemplate(body.TemplateId, uid); err != nil {
				JSONResponse(w, models.Response{Success: false, Message: msgTemplateNotFound}, http.StatusBadRequest)
				return
			}
		}
		if err := models.UpdateReport(&body, uid); err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: "No se pudo guardar el informe."}, http.StatusInternalServerError)
			return
		}
		updated, _ := models.GetReport(id, uid)
		JSONResponse(w, updated, http.StatusOK)
	case http.MethodDelete:
		force := r.URL.Query().Get("force") == "true"
		isAdmin := userIsAdmin(r)
		if force && !isAdmin {
			JSONResponse(w, models.Response{Success: false, Message: "Necesitas permisos de administrador para forzar la eliminación de este informe."}, http.StatusForbidden)
			return
		}
		if err := models.DeleteReport(id, uid, force); err != nil {
			if err == models.ErrReportHasRenders {
				msg := "No es posible eliminar este informe porque ya existen documentos generados asociados a él."
				if isAdmin {
					msg = "Este informe ya fue utilizado para generar documentos. Como administrador puedes forzar su eliminación (se borrarán también los documentos generados asociados)."
				}
				JSONResponse(w, map[string]interface{}{"success": false, "message": msg, "can_force": isAdmin}, http.StatusConflict)
				return
			}
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: "Ocurrió un error al eliminar el informe. Inténtalo de nuevo."}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Informe eliminado"}, http.StatusOK)
	default:
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
	}
}

// ReportSlots handles GET /report-slots, returning the fixed image-slot catalog
// (key, title, evidence, section, source, required) so the UI can lay the editor
// out by report section. Source is serialized as "manual"/"auto" so the UI can
// hide auto slots (e.g. the results chart) from the upload controls.
func (as *Server) ReportSlots(w http.ResponseWriter, r *http.Request) {
	type slotDTO struct {
		Key      string `json:"key"`
		Title    string `json:"title"`
		Evidence string `json:"evidence"`
		Section  string `json:"section"`
		Source   string `json:"source"`
		Required bool   `json:"required"`
	}
	out := make([]slotDTO, 0, len(report.Slots))
	for _, s := range report.Slots {
		out = append(out, slotDTO{s.Key, s.Title, s.Evidence, s.Section, s.SourceName(), s.Required})
	}
	JSONResponse(w, out, http.StatusOK)
}

// ReportAssetsList handles GET /reports/{id}/assets, returning which slots have
// a working image (slot + mime), so the UI can show loaded/missing status.
func (as *Server) ReportAssetsList(w http.ResponseWriter, r *http.Request) {
	uid := reportUID(r)
	id := pathID(r)
	if _, err := models.GetReport(id, uid); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgReportNotFound}, http.StatusNotFound)
		return
	}
	assets, err := models.GetReportAssets(id)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "No se pudieron cargar las imágenes del informe."}, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]string, 0, len(assets))
	for _, a := range assets {
		out = append(out, map[string]string{"slot": a.Slot, "mime": a.Mime})
	}
	JSONResponse(w, out, http.StatusOK)
}

// ReportAsset handles GET (serve the image for a slot, for thumbnails) and POST
// (upload/replace the image) on /reports/{id}/assets/{slot}.
func (as *Server) ReportAsset(w http.ResponseWriter, r *http.Request) {
	uid := reportUID(r)
	id := pathID(r)
	slot := mux.Vars(r)["slot"]

	switch r.Method {
	case http.MethodGet:
		if _, err := models.GetReport(id, uid); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: msgReportNotFound}, http.StatusNotFound)
			return
		}
		asset, err := models.GetReportAssetBySlot(id, slot)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "No se encontró la imagen solicitada."}, http.StatusNotFound)
			return
		}
		content, err := blobStore().Get(asset.ContentSha256)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "No se pudo recuperar el contenido de la imagen."}, http.StatusNotFound)
			return
		}
		mime := asset.Mime
		if mime == "" {
			mime = http.DetectContentType(content)
		}
		// Serve evidence inline (the UI shows thumbnails) but never let the
		// browser sniff a different type than the stored, validated image MIME.
		w.Header().Set("Content-Type", mime)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	case http.MethodPost:
		content, ok := readUpload(w, r)
		if !ok {
			return
		}
		err := report.SaveReportAsset(blobStore(), id, uid, slot, content)
		if err == report.ErrUnknownSlot {
			JSONResponse(w, models.Response{Success: false, Message: "La imagen indicada no corresponde a ninguna figura del informe."}, http.StatusBadRequest)
			return
		}
		if err == report.ErrUnsupportedImageType {
			JSONResponse(w, models.Response{Success: false, Message: "Formato de imagen no compatible. Usa " + report.SupportedImageFormats + "."}, http.StatusBadRequest)
			return
		}
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: msgReportNotFound}, http.StatusNotFound)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Imagen guardada."}, http.StatusOK)
	default:
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
	}
}

// ReportGenerate handles POST /reports/{id}/generate, producing an immutable
// render. The DOCX itself is obtained via the render rebuild endpoint.
func (as *Server) ReportGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	uid := reportUID(r)
	id := pathID(r)
	// Pre-flight: block generation while anything required is missing, with a
	// specific, actionable list the UI shows to the user.
	if problems := reportProblems(id, uid); len(problems) > 0 {
		JSONResponse(w, map[string]interface{}{
			"success":  false,
			"message":  "No es posible generar el informe porque hay elementos pendientes por completar.",
			"problems": problems,
		}, http.StatusBadRequest)
		return
	}
	render, _, err := report.Generate(blobStore(), id, uid)
	if err != nil {
		if _, gerr := models.GetReport(id, uid); gerr != nil {
			JSONResponse(w, models.Response{Success: false, Message: msgReportNotFound}, http.StatusNotFound)
			return
		}
		log.Error(err)
		msg := "No se pudo generar el informe. Revisa que la plantilla tenga una versión activa y que los datos estén completos."
		if err == models.ErrNoActiveTemplateVersion {
			msg = "La plantilla seleccionada no tiene una versión activa. Sube y activa un documento en Report Templates."
		}
		JSONResponse(w, models.Response{Success: false, Message: msg}, http.StatusBadRequest)
		return
	}
	JSONResponse(w, render, http.StatusCreated)
}

// reportProblems returns a user-facing list of everything missing that would
// prevent generating a complete DOCX. Empty means the report is ready. The
// checks are driven by what the ACTIVE template actually uses: a field or image
// is only required if its token/slot is present in the template, so a template
// that doesn't use a given section never demands it.
func reportProblems(id, uid int64) []string {
	rep, err := models.GetReport(id, uid)
	if err != nil {
		return []string{"El informe no existe o no tienes acceso a él."}
	}
	tmplTokens := map[string]bool{}
	tmplSlots := map[string]bool{}
	if ver, verr := models.GetActiveTemplateVersion(rep.TemplateId); verr == nil {
		var insp report.Inspection
		// Reuse the inspection computed and cached at upload time (validation_json)
		// instead of re-reading the multi-MB DOCX blob and re-parsing it on every
		// generate. Fall back to a live Inspect only for legacy versions whose
		// cache is empty. Same tokens/slots either way (I-2).
		if ver.ValidationJSON != "" && json.Unmarshal([]byte(ver.ValidationJSON), &insp) == nil {
			// cached
		} else if docx, derr := blobStore().Get(ver.ContentSha256); derr == nil {
			insp, _ = report.Inspect(docx)
		}
		for _, tk := range insp.Tokens {
			tmplTokens[tk] = true
		}
		for _, s := range insp.ImageSlots {
			tmplSlots[s] = true
		}
	}

	var p []string
	for _, fc := range []struct{ token, value, msg string }{
		{"EMPRESA", rep.CompanyName, "Falta completar el campo Empresa."},
		{"ELABORADO_POR", rep.PreparedBy, "Falta completar el campo Elaborado por."},
		{"PARRAFO_EJECUCION", rep.IntroExec, "Debe completar el párrafo de ejecución (Sección 3)."},
		{"TEXTO_PUNTO_1", rep.TextPunto1, "Debe completar el texto del Punto 1 (Sección 4)."},
	} {
		if tmplTokens[fc.token] && strings.TrimSpace(fc.value) == "" {
			p = append(p, fc.msg)
		}
	}
	if tmplTokens["FECHA_INFORME"] && (rep.ReportDate.IsZero() || rep.ReportDate.Year() < 1971) {
		p = append(p, "Falta indicar la Fecha del informe.")
	}
	if tmplTokens["FECHA_EJECUCION"] || tmplTokens["RANGO_FECHAS"] {
		if rep.ExecutedFrom.IsZero() || rep.ExecutedFrom.Year() < 1971 {
			p = append(p, "Falta indicar la fecha de Ejecución desde.")
		}
		if rep.ExecutedTo.IsZero() || rep.ExecutedTo.Year() < 1971 {
			p = append(p, "Falta indicar la fecha de Ejecución hasta.")
		}
	}
	if !ownsSubject(rep.SubjectKind, rep.SubjectId, uid) {
		p = append(p, "Falta seleccionar una campaña o grupo válido.")
	}

	assets, _ := models.GetReportAssets(id)
	have := make(map[string]bool, len(assets))
	for _, a := range assets {
		have[a.Slot] = true
	}
	had2fa := rep.UsersWith2FA > 0
	for _, s := range report.Slots {
		if s.Source != report.SourceManual || !s.Required || !tmplSlots[s.Key] {
			continue // no es manual/obligatoria, o la plantilla activa no la usa
		}
		if s.Key == "figura_8" && !had2fa {
			continue // la Figura 8 solo existe cuando hubo cuentas con 2FA
		}
		if !have[s.Key] {
			p = append(p, "Falta cargar la imagen: "+s.Title+".")
		}
	}
	return p
}

// ReportRenders handles GET /reports/{id}/renders, listing a report's renders.
func (as *Server) ReportRenders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	rr, err := models.GetRendersForReport(pathID(r), reportUID(r))
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgReportNotFound}, http.StatusNotFound)
		return
	}
	JSONResponse(w, rr, http.StatusOK)
}

// Render handles GET /renders/{id}, returning the immutable render metadata.
func (as *Server) Render(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	uid := reportUID(r)
	id := pathID(r)
	render, assets, err := models.GetReportRender(id, uid)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgRenderNotFound}, http.StatusNotFound)
		return
	}
	JSONResponse(w, map[string]interface{}{"render": render, "assets": assets}, http.StatusOK)
}

// RenderRebuild handles POST /renders/{id}/rebuild. With C1 this is an AUDIT,
// not a reconstruction: it re-renders from frozen data and reports whether the
// result still matches the frozen render (by content fingerprint), without
// serving any bytes. Use the download endpoint to obtain the actual DOCX.
func (as *Server) RenderRebuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	uid := reportUID(r)
	id := pathID(r)
	if _, _, err := models.GetReportRender(id, uid); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgRenderNotFound}, http.StatusNotFound)
		return
	}
	audit, err := report.AuditRender(blobStore(), id, uid)
	if err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo verificar el documento generado."}, http.StatusInternalServerError)
		return
	}
	status := http.StatusOK
	if !audit.Match {
		status = http.StatusConflict
	}
	JSONResponse(w, audit, status)
}

// RenderDownload handles GET /renders/{id}/download, serving the stored DOCX
// byte-for-byte from the content-addressed blob store.
func (as *Server) RenderDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	uid := reportUID(r)
	id := pathID(r)
	if _, _, err := models.GetReportRender(id, uid); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgRenderNotFound}, http.StatusNotFound)
		return
	}
	docx, err := report.DownloadRender(blobStore(), id, uid)
	if err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "El documento generado no está disponible."}, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", "attachment; filename=\"report.docx\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(docx)
}

// RenderDownloadXLSX serves the per-recipient Excel produced alongside the DOCX.
func (as *Server) RenderDownloadXLSX(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	uid := reportUID(r)
	id := pathID(r)
	render, _, err := models.GetReportRender(id, uid)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: msgRenderNotFound}, http.StatusNotFound)
		return
	}
	if render.OutputXlsxSha256 == "" {
		JSONResponse(w, models.Response{Success: false, Message: "Este informe no tiene un Excel asociado. Vuelve a generarlo para obtenerlo."}, http.StatusNotFound)
		return
	}
	xlsx, err := blobStore().Get(render.OutputXlsxSha256)
	if err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "El Excel generado no está disponible."}, http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"resultados.xlsx\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(xlsx)
}

// readUpload reads a multipart "file" upload (bounded), writing an error
// response and returning ok=false on failure.
func readUpload(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if err := r.ParseMultipartForm(maxReportUploadBytes); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "La carga no es válida. Envía el archivo en el formato esperado."}, http.StatusBadRequest)
		return nil, false
	}
	f, _, err := r.FormFile("file")
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Falta seleccionar un archivo."}, http.StatusBadRequest)
		return nil, false
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, maxReportUploadBytes))
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo leer el archivo."}, http.StatusBadRequest)
		return nil, false
	}
	return content, true
}

// userIsAdmin reports whether the user has the system-modify permission.
// userIsAdmin reports whether the authenticated user has the system-modify
// permission. The user is read from the request context (set by RequireAPIKey)
// instead of re-querying the DB (audit M-8).
func userIsAdmin(r *http.Request) bool {
	u, ok := ctx.Get(r, "user").(models.User)
	if !ok {
		return false
	}
	allowed, _ := u.HasPermission(models.PermissionModifySystem)
	return allowed
}

// ownsSubject verifies the campaign or campaign group exists and belongs to uid.
func ownsSubject(kind string, id, uid int64) bool {
	switch kind {
	case "campaign":
		_, err := models.GetCampaign(id, uid)
		return err == nil
	case "campaign_group":
		_, err := models.GetCampaignGroup(id, uid)
		return err == nil
	}
	return false
}
