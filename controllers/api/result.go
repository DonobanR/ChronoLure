package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	ctx "github.com/gophish/gophish/context"
	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/models"
	"github.com/gorilla/mux"
)

// recipientDeleteError maps model errors to user-facing HTTP responses shared by
// the delete/bulk-delete handlers. Messages follow the "qué pasó + causa +
// acción" convention (no raw "Bad request"). Ownership failures surface as 404,
// never 403, so another user's data existence is not leaked.
func recipientDeleteError(w http.ResponseWriter, err error) {
	switch err {
	case models.ErrResultNotFound:
		JSONResponse(w, models.Response{Success: false, Message: "No se encontró el destinatario en esta campaña."}, http.StatusNotFound)
	case models.ErrBatchTooLarge:
		JSONResponse(w, models.Response{Success: false, Message: "Demasiados destinatarios en una sola operación (máximo 500). Divídelo en lotes."}, http.StatusRequestEntityTooLarge)
	default:
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo eliminar el destinatario."}, http.StatusInternalServerError)
	}
}

func normalizeScope(s string) string {
	if s == models.DeleteScopeGroup {
		return models.DeleteScopeGroup
	}
	return models.DeleteScopeCampaign
}

// CampaignResultDelete soft-deletes a single recipient (CL-102R).
//
//	DELETE /api/campaigns/{id}/results/{rid}   body: {reason?, scope}
func (as *Server) CampaignResultDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	vars := mux.Vars(r)
	cid, _ := strconv.ParseInt(vars["id"], 10, 64)
	rid := vars["rid"]
	userID := ctx.Get(r, "user_id").(int64)

	var body struct {
		Reason string `json:"reason"`
		Scope  string `json:"scope"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // body optional

	batchID, affected, err := models.SoftDeleteResults(cid, []string{rid}, userID, body.Reason, normalizeScope(body.Scope))
	if err != nil {
		recipientDeleteError(w, err)
		return
	}
	JSONResponse(w, map[string]interface{}{
		"success":  true,
		"message":  "Se eliminó el destinatario. Ya no cuenta en las métricas.",
		"batch_id": batchID,
		"affected": affected,
	}, http.StatusOK)
}

// CampaignResultsBulkDelete soft-deletes several recipients in one batch.
//
//	POST /api/campaigns/{id}/results/bulk-delete   body: {result_ids:[...], reason?, scope}
func (as *Server) CampaignResultsBulkDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	vars := mux.Vars(r)
	cid, _ := strconv.ParseInt(vars["id"], 10, 64)
	userID := ctx.Get(r, "user_id").(int64)

	var body struct {
		ResultIds []string `json:"result_ids"`
		Reason    string   `json:"reason"`
		Scope     string   `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo leer la solicitud."}, http.StatusBadRequest)
		return
	}
	if len(body.ResultIds) == 0 {
		JSONResponse(w, models.Response{Success: false, Message: "Selecciona al menos un destinatario para eliminar."}, http.StatusBadRequest)
		return
	}
	batchID, affected, err := models.SoftDeleteResults(cid, body.ResultIds, userID, body.Reason, normalizeScope(body.Scope))
	if err != nil {
		recipientDeleteError(w, err)
		return
	}
	JSONResponse(w, map[string]interface{}{
		"success":  true,
		"message":  "Se eliminaron " + strconv.Itoa(affected) + " destinatarios. Las métricas se actualizaron.",
		"batch_id": batchID,
		"affected": affected,
	}, http.StatusOK)
}

// CampaignResultsTrashed lists the soft-deleted recipients of a campaign, feeding
// the "N en papelera" banner.
//
//	GET /api/campaigns/{id}/results/trashed
func (as *Server) CampaignResultsTrashed(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	cid, _ := strconv.ParseInt(vars["id"], 10, 64)
	userID := ctx.Get(r, "user_id").(int64)
	if _, err := models.GetCampaign(cid, userID); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Campaña no encontrada."}, http.StatusNotFound)
		return
	}
	recips, total, err := models.GetTrashedRecipients(userID, models.RecipientTrashQuery{CampaignID: cid})
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo cargar la papelera de destinatarios."}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, map[string]interface{}{"items": recips, "total": total}, http.StatusOK)
}

// CampaignGroupResultsTrashed lists the soft-deleted recipients across every
// campaign of a group, feeding the group-level banner/panel (addendum §4/§5).
//
//	GET /api/campaign-groups/{id}/results/trashed
func (as *Server) CampaignGroupResultsTrashed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	gid, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	userID := ctx.Get(r, "user_id").(int64)
	// Ownership: the group must belong to the user (404, never 403).
	if _, err := models.GetCampaignGroup(gid, userID); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Grupo de campañas no encontrado."}, http.StatusNotFound)
		return
	}
	recips, total, err := models.GetTrashedRecipients(userID, models.RecipientTrashQuery{GroupID: gid})
	if err != nil {
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo cargar la papelera de destinatarios del grupo."}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, map[string]interface{}{"items": recips, "total": total}, http.StatusOK)
}

// CampaignResultsDeletePreview answers "what would this deletion touch?" WITHOUT
// deleting, so the confirmation dialog can state the scope's real consequence.
// Both numbers come from the server: campaign_count (size of the group) and
// affected (rows that actually match). Computing "affected" client-side from the
// campaign count would show a false number — the exact mistake §6.3 warns about.
//
//	GET /api/campaigns/{id}/results/delete-preview?rids=a,b&scope=group
func (as *Server) CampaignResultsDeletePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	cid, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	userID := ctx.Get(r, "user_id").(int64)
	q := r.URL.Query()
	rids := []string{}
	for _, raw := range strings.Split(q.Get("rids"), ",") {
		if v := strings.TrimSpace(raw); v != "" {
			rids = append(rids, v)
		}
	}
	if len(rids) == 0 {
		JSONResponse(w, models.Response{Success: false, Message: "Indica al menos un destinatario."}, http.StatusBadRequest)
		return
	}
	prev, err := models.PreviewResultDeletion(cid, rids, userID, normalizeScope(q.Get("scope")))
	if err != nil {
		if err == models.ErrBatchTooLarge {
			JSONResponse(w, models.Response{Success: false, Message: "Demasiados destinatarios en una sola operación (máximo 500)."}, http.StatusRequestEntityTooLarge)
			return
		}
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo calcular el alcance de la eliminación."}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, prev, http.StatusOK)
}

// RecipientRestore restores a single trashed recipient. POST /api/trash/recipient/{id}/restore
func (as *Server) RecipientRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	userID := ctx.Get(r, "user_id").(int64)
	// Capture the context BEFORE restoring so a blocked restore can name the
	// recipient and its campaign in the message (addendum §6).
	tr, _ := models.GetTrashedRecipientByID(userID, id)
	if err := models.RestoreResultByID(userID, id); err != nil {
		if msg := nestedTrashMessage(err, tr); msg != "" {
			JSONResponse(w, models.Response{Success: false, Message: msg}, http.StatusBadRequest)
			return
		}
		recipientRestoreError(w, err)
		return
	}
	JSONResponse(w, models.Response{Success: true, Message: "Se restauró el destinatario."}, http.StatusOK)
}

// RecipientRestoreBatch restores every recipient of a batch (the toast "Deshacer").
//
//	POST /api/trash/recipient/restore-batch   body: {batch_id}
func (as *Server) RecipientRestoreBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	userID := ctx.Get(r, "user_id").(int64)
	var body struct {
		BatchID string `json:"batch_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo leer la solicitud."}, http.StatusBadRequest)
		return
	}
	n, err := models.RestoreResultBatch(userID, body.BatchID)
	if err != nil {
		recipientRestoreError(w, err)
		return
	}
	JSONResponse(w, map[string]interface{}{
		"success":  true,
		"message":  "Se restauraron " + strconv.Itoa(n) + " destinatarios.",
		"restored": n,
	}, http.StatusOK)
}

// nestedTrashMessage builds the user-facing message for a restore blocked by the
// state of the parent campaign (addendum §6), naming the recipient and campaign.
// Returns "" when err is not a nested-trash error.
func nestedTrashMessage(err error, tr models.TrashedRecipient) string {
	who := tr.Email
	if who == "" {
		who = "el destinatario"
	}
	camp := tr.CampaignName
	if camp == "" {
		camp = "asociada"
	}
	switch err {
	case models.ErrParentCampaignTrashed:
		return "No se puede restaurar " + who + " porque la campaña «" + camp + "» está en la papelera. Restaura primero la campaña."
	case models.ErrParentCampaignPurged:
		return "No se puede restaurar " + who + " porque la campaña «" + camp + "» fue eliminada definitivamente. El registro seguirá en la papelera hasta que lo elimines."
	}
	return ""
}

func recipientRestoreError(w http.ResponseWriter, err error) {
	switch err {
	case models.ErrResultNotFound:
		JSONResponse(w, models.Response{Success: false, Message: "No se encontró el destinatario en la papelera."}, http.StatusNotFound)
	case models.ErrResultActiveDuplicate:
		JSONResponse(w, models.Response{Success: false, Message: "Ya existe un destinatario activo con ese correo en la campaña. Revísalo antes de restaurar."}, http.StatusBadRequest)
	case models.ErrParentCampaignTrashed:
		JSONResponse(w, models.Response{Success: false, Message: "No se puede restaurar el destinatario porque su campaña está en la papelera. Restaura primero la campaña."}, http.StatusBadRequest)
	case models.ErrParentCampaignPurged:
		JSONResponse(w, models.Response{Success: false, Message: "No se puede restaurar el destinatario porque su campaña fue eliminada definitivamente. El registro seguirá en la papelera hasta que lo elimines."}, http.StatusBadRequest)
	default:
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo restaurar el destinatario."}, http.StatusInternalServerError)
	}
}

// RecipientPurge permanently deletes a trashed recipient. Requires the exact
// email as confirmation, validated in the backend.
//
//	POST /api/trash/recipient/{id}/purge   body: {confirm: "<email exacto>"}
func (as *Server) RecipientPurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	id, _ := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	userID := ctx.Get(r, "user_id").(int64)
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Confirm == "" {
		JSONResponse(w, models.Response{Success: false, Message: "Para eliminar definitivamente, escribe el correo del destinatario tal como aparece."}, http.StatusBadRequest)
		return
	}
	if err := models.PurgeResult(userID, id, body.Confirm); err != nil {
		recipientPurgeError(w, err)
		return
	}
	JSONResponse(w, models.Response{Success: true, Message: "Destinatario eliminado definitivamente."}, http.StatusOK)
}

// RecipientPurgeBatch permanently deletes every trashed recipient of a batch.
//
//	POST /api/trash/recipient/purge-batch   body: {batch_id, confirm: "ELIMINAR"}
func (as *Server) RecipientPurgeBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, models.Response{Success: false, Message: msgMethodNotAllowed}, http.StatusMethodNotAllowed)
		return
	}
	userID := ctx.Get(r, "user_id").(int64)
	var body struct {
		BatchID string `json:"batch_id"`
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo leer la solicitud."}, http.StatusBadRequest)
		return
	}
	if body.Confirm != "ELIMINAR" {
		JSONResponse(w, models.Response{Success: false, Message: "Para eliminar definitivamente el lote, escribe ELIMINAR."}, http.StatusBadRequest)
		return
	}
	n, err := models.PurgeResultBatch(userID, body.BatchID)
	if err != nil {
		recipientPurgeError(w, err)
		return
	}
	JSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Se eliminaron definitivamente " + strconv.Itoa(n) + " destinatarios.",
		"purged":  n,
	}, http.StatusOK)
}

func recipientPurgeError(w http.ResponseWriter, err error) {
	switch {
	case err == models.ErrResultNotFound:
		JSONResponse(w, models.Response{Success: false, Message: "No se encontró el destinatario en la papelera."}, http.StatusNotFound)
	case err.Error() == "confirmation does not match":
		JSONResponse(w, models.Response{Success: false, Message: "La confirmación no coincide con el correo del destinatario."}, http.StatusBadRequest)
	case err.Error() == "can only purge recipients in trash":
		JSONResponse(w, models.Response{Success: false, Message: "Solo se pueden eliminar definitivamente destinatarios que estén en la papelera."}, http.StatusBadRequest)
	default:
		log.Error(err)
		JSONResponse(w, models.Response{Success: false, Message: "No se pudo eliminar definitivamente el destinatario."}, http.StatusInternalServerError)
	}
}
