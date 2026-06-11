// Report Templates — gestión de plantillas (orientada a nombres/fechas).
// authHeaders, flash, fmtDate, errMsg, deleteWithForce live in reportCommon.js
// (loaded before this script).
var TAPI = "/api/report-templates"

var EMPTY_MSG = '<p class="text-muted">Aún no tienes plantillas. Crea una arriba y sube tu documento Word del informe.</p>'

// loadTemplates does the full initial render. Per-template mutations afterwards
// use surgical refresh (refreshTemplate / removeTemplateCard / append) to avoid
// the full reload + 1+N GET cascade and the flicker it caused (audit I-9).
function loadTemplates() {
    $.ajax({ url: TAPI, headers: authHeaders() }).done(function (list) {
        if (!list || !list.length) {
            $("#templatesList").html(EMPTY_MSG)
            return
        }
        var html = ''
        list.forEach(function (t) { html += renderTemplateCard(t) })
        $("#templatesList").html(html)
        list.forEach(function (t) { loadVersions(t) })
    }).fail(function () { flash('No se pudieron cargar las plantillas.', 'danger') })
}

// refreshTemplate re-renders ONLY the given template's card + versions (2 GETs),
// preserving any inspection panel already shown. Optional `after` runs once the
// card is in place.
function refreshTemplate(tid, after) {
    $.ajax({ url: TAPI + '/' + tid, headers: authHeaders() }).done(function (t) {
        var inspectHtml = $("#inspect-" + tid).html() || ''
        if ($("#tpl-" + tid).length) {
            $("#tpl-" + tid).replaceWith(renderTemplateCard(t))
        } else {
            $("#templatesList").append(renderTemplateCard(t))
        }
        $("#inspect-" + tid).html(inspectHtml)
        loadVersions(t)
        if (after) after()
    })
}

function removeTemplateCard(tid) {
    $("#tpl-" + tid).remove()
    if ($("#templatesList .panel").length === 0) {
        $("#templatesList").html(EMPTY_MSG)
    }
}

function renderTemplateCard(t) {
    var estado = t.active_version_id
        ? '<span class="label label-success">Lista para usar</span>'
        : '<span class="label label-warning">Falta subir un documento</span>'
    return '<div class="panel panel-default" id="tpl-' + t.id + '">' +
        '<div class="panel-heading" style="display:flex;align-items:center;gap:10px">' +
        '<div style="flex:1">' +
        '<strong style="font-size:1.08em">' + escapeHtml(t.name) + '</strong> &nbsp;' + estado +
        '<div class="text-muted" style="font-size:.82em;margin-top:3px">Creada: ' + fmtDate(t.created_at) +
        ' · Actualizada: ' + fmtDate(t.updated_at) + ' <span style="opacity:.55">· id interno ' + t.id + '</span></div>' +
        '</div>' +
        '<button class="btn btn-xs btn-danger" onclick="delTemplate(' + t.id + ')"><i class="fa fa-trash"></i> Eliminar</button>' +
        '</div>' +
        '<div class="panel-body">' +
        '<label style="font-weight:600">Subir documento Word (.docx)</label>' +
        '<div class="form-inline"><input type="file" accept=".docx" id="file-' + t.id + '"> ' +
        '<button class="btn btn-sm btn-primary" onclick="uploadVersion(' + t.id + ')"><i class="fa fa-upload"></i> Subir documento</button></div>' +
        '<p class="help-block" style="font-size:.85em">Al subirlo se valida automáticamente. La primera versión queda activa sola; si subes otra, eliges cuál usar.</p>' +
        '<div id="inspect-' + t.id + '" style="margin-top:8px"></div>' +
        '<div id="versions-' + t.id + '" style="margin-top:8px"></div>' +
        '</div></div>'
}

function createTemplate(name) {
    $.ajax({ url: TAPI, method: 'POST', headers: authHeaders(), contentType: 'application/json', data: JSON.stringify({ name: name }) })
        .done(function (t) {
            flash('Plantilla creada. Ahora sube su documento Word.', 'success')
            // Surgical append: drop the empty-state placeholder, add just this card.
            if ($("#templatesList .panel").length === 0) { $("#templatesList").empty() }
            $("#templatesList").append(renderTemplateCard(t))
            loadVersions(t)
        })
        .fail(function () { flash('No se pudo crear la plantilla.', 'danger') })
}

function uploadVersion(tid) {
    var input = document.getElementById('file-' + tid)
    if (!input.files || !input.files[0]) { flash('Selecciona un documento Word (.docx).', 'warning'); return }
    var fd = new FormData()
    fd.append('file', input.files[0])
    $.ajax({ url: TAPI + '/' + tid + '/versions', method: 'POST', headers: authHeaders(), data: fd, processData: false, contentType: false })
        .done(function (resp) {
            // Refresh only this template, then show the inspection on the fresh card.
            refreshTemplate(tid, function () { showInspect(tid, resp.inspection || {}, resp.version) })
        })
        .fail(function (xhr) {
            var insp = xhr.responseJSON && xhr.responseJSON.inspection
            if (insp) { showInspect(tid, insp, null) }
            else { flash('No se pudo subir el documento.', 'danger') }
        })
}

function showInspect(tid, insp, version) {
    var ok = insp.valid
    var html = '<div class="alert alert-' + (ok ? 'success' : 'danger') + '">'
    if (ok) {
        html += '<i class="fa fa-check-circle"></i> <strong>Documento válido.</strong> Se detectaron ' +
            (insp.tokens || []).length + ' campos de texto y ' + (insp.image_slots || []).length + ' imágenes.'
        if (version) html += ' Se guardó la versión ' + version.version + (version.version === 1 ? ' y quedó activa.' : '.')
    } else {
        html += '<i class="fa fa-times-circle"></i> <strong>El documento no es válido y no se guardó.</strong> Corrígelo y vuelve a subirlo.'
        if ((insp.missing_required || []).length) html += '<br>• Faltan campos de texto obligatorios: ' + escapeHtml(insp.missing_required.join(', '))
        if ((insp.unknown || []).length) html += '<br>• Campos de texto no reconocidos: ' + escapeHtml(insp.unknown.join(', '))
        if ((insp.missing_required_slots || []).length) html += '<br>• Faltan imágenes obligatorias: ' + escapeHtml(insp.missing_required_slots.join(', '))
        if ((insp.duplicate_slots || []).length) html += '<br>• Imágenes repetidas: ' + escapeHtml(insp.duplicate_slots.join(', '))
    }
    html += '</div>'
    $("#inspect-" + tid).html(html)
}

function loadVersions(t) {
    $.ajax({ url: TAPI + '/' + t.id + '/versions', headers: authHeaders() }).done(function (vs) {
        if (!vs || !vs.length) { $("#versions-" + t.id).html(''); return }
        var html = '<table class="table table-condensed" style="margin-top:6px"><thead><tr>' +
            '<th>Versión</th><th>Subida</th><th>Estado</th><th></th></tr></thead><tbody>'
        vs.forEach(function (v) {
            var isActive = v.id === t.active_version_id
            html += '<tr' + (isActive ? ' style="background:#eaffea"' : '') + '>' +
                '<td><strong>versión ' + v.version + '</strong></td>' +
                '<td>' + fmtDate(v.created_at) + '</td>' +
                '<td>' + (isActive ? '<span class="label label-success">ACTIVA — se usará al generar</span>' : '<span class="text-muted">inactiva</span>') + '</td>' +
                '<td style="white-space:nowrap">' +
                (isActive ? '' : '<button class="btn btn-xs btn-success" onclick="activate(' + t.id + ',' + v.id + ')">Usar esta</button> ') +
                '<a class="btn btn-xs btn-default" href="' + TAPI + '/' + t.id + '/versions/' + v.id + '/download?api_key=' + user.api_key + '" target="_blank"><i class="fa fa-download"></i> Descargar</a>' +
                '</td></tr>'
        })
        html += '</tbody></table><p class="help-block" style="font-size:.82em"><i class="fa fa-info-circle"></i> Solo hay <strong>una versión activa</strong> a la vez: es la que se usará para generar informes. "Usar esta" cambia cuál está activa.</p>'
        $("#versions-" + t.id).html(html)
    }).fail(function () {
        $("#versions-" + t.id).html('<p class="text-danger" style="font-size:.85em">No se pudieron cargar las versiones de esta plantilla.</p>')
    })
}

function activate(tid, vid) {
    $.ajax({ url: TAPI + '/' + tid + '/active-version', method: 'PUT', headers: authHeaders(), contentType: 'application/json', data: JSON.stringify({ version_id: vid }) })
        .done(function () {
            flash('Versión actualizada. Esta es la que se usará al generar informes.', 'success')
            refreshTemplate(tid) // surgical: only this template's card + versions
        })
        .fail(function () { flash('No se pudo cambiar la versión activa.', 'danger') })
}

function delTemplate(tid) {
    deleteWithForce(TAPI + '/' + tid, {
        confirm: '¿Eliminar esta plantilla?',
        fallback: 'No se pudo eliminar la plantilla.',
        onDone: function () { flash('Plantilla eliminada.', 'success'); removeTemplateCard(tid) }
    })
}

$(document).ready(function () {
    $("#newTemplateForm").submit(function (e) {
        e.preventDefault()
        var n = $("#newTemplateName").val().trim()
        if (n) { createTemplate(n); $("#newTemplateName").val('') }
    })
    loadTemplates()
})
