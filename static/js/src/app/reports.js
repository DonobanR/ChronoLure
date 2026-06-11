// Reports page: list + per-section editor (real document structure)
var RAPI = "/api/reports"
var state = { report: null, slots: [], assets: {} }
var PUNTO1_PREFIX = "1. Dirección de correo electrónico:"

// authHeaders, flash, errMsg, fmtDate, dateVal, assetUrl, deleteWithForce live in
// reportCommon.js (loaded before this script).

// Section layout of the report. Image slots come from the catalog (by section).
var SECTIONS = [
    { title: "Portada", fields: [
        { k: "company_name", label: "Empresa", t: "text", req: true, reqMsg: "Falta completar el campo Empresa." },
        { k: "report_date", label: "Fecha del informe", t: "date", req: true, reqMsg: "Falta indicar la Fecha del informe." },
        { k: "executed_from", label: "Ejecución desde", t: "date", req: true, reqMsg: "Falta indicar la fecha de Ejecución desde." },
        { k: "executed_to", label: "Ejecución hasta", t: "date", req: true, reqMsg: "Falta indicar la fecha de Ejecución hasta." },
        { k: "prepared_by", label: "Elaborado por", t: "text", req: true, reqMsg: "Falta completar el campo Elaborado por." } ] },
    { title: "Introducción", note: "Automático: se reemplaza el nombre de empresa y las fechas de ejecución en todo el texto." },
    { title: "Sección 3 – Ejecución de la campaña", fields: [
        { k: "intro_exec", label: "Párrafo introductorio", t: "textarea", req: true, reqMsg: "Debe completar el párrafo de ejecución (Sección 3)." } ] },
    { title: "Sección 4 – Puntos clave", fields: [
        { k: "text_punto_1", label: "Punto 1", t: "textarea", prefix: PUNTO1_PREFIX, req: true, reqMsg: "Debe completar el texto del Punto 1 (Sección 4)." } ] },
    { title: "Sección 5 – Resultados", note: "Esta sección se genera automáticamente utilizando las métricas reales de la campaña (no se sube ninguna imagen)." },
    { title: "Sección 6 – Evidencia de credenciales válidas", fields: [
        { k: "had2fa", label: "¿Se encontraron cuentas con 2FA?", t: "checkbox" } ] },
    { title: "Sección 7 – Evidencias de credenciales válidas" },
    { title: "Sección 8 – Conclusiones", note: "Automático: % y nº de usuarios comprometidos (coinciden con la sección de Resultados)." },
    { title: "Sección 9 – Recomendaciones", note: "Texto fijo del informe; solo se sustituye el nombre de empresa." },
    { title: "Sección 10 – Anexos" }
]

// ---------- subject picker (campañas/grupos por nombre, estilo Campaign Groups) ----------
var subjects = { campaign: [], campaign_group: [] }

function loadSubjects() {
    $.ajax({ url: '/api/campaigns/summary', headers: authHeaders() })
        .done(function (res) {
            subjects.campaign = (res.campaigns || []).map(function (c) { return { id: c.id, name: c.name, status: c.status || '' } })
        })
        .fail(function () { flash('No se pudieron cargar las campañas para el buscador.', 'danger') })
    $.ajax({ url: '/api/campaign-groups/', headers: authHeaders() })
        .done(function (list) {
            subjects.campaign_group = (list || []).map(function (g) { return { id: g.id, name: g.name, status: 'grupo' } })
        })
        .fail(function () { flash('No se pudieron cargar los grupos de campañas para el buscador.', 'danger') })
}

function subjectPicker(query) {
    var kind = $("#subjectKind").val()
    var $d = $("#subjectDropdown")
    var q = (query || '').toLowerCase().trim()
    if (!q) { $d.empty().hide(); return }
    var matches = (subjects[kind] || []).filter(function (s) {
        return s.name && s.name.toLowerCase().indexOf(q) !== -1
    }).slice(0, 30)
    $d.empty()
    if (!matches.length) {
        $d.append($('<div>').text('Sin coincidencias').css({ padding: '8px 12px', color: '#999', fontSize: '13px' }))
    } else {
        matches.forEach(function (s) {
            $('<div>')
                .css({ padding: '8px 12px', cursor: 'pointer', borderBottom: '1px solid #eee', fontSize: '13px' })
                .html('<strong>' + escapeHtml(s.name) + '</strong>&nbsp;<span class="label label-default">' + escapeHtml(s.status) + '</span>')
                .hover(function () { $(this).css('background', '#eef3ff') }, function () { $(this).css('background', '') })
                .on('click', function () {
                    $("#subjectId").val(s.id)
                    $("#subjectSearch").val(s.name)
                    $d.empty().hide()
                })
                .appendTo($d)
        })
    }
    $d.show()
}

// ---------- list view ----------
function loadTemplatesSelect() {
    $.ajax({ url: '/api/report-templates', headers: authHeaders() })
        .done(function (list) {
            var sel = $("#templateId"); sel.empty()
            if (!list || !list.length) { sel.append('<option value="">(crea una plantilla primero)</option>'); return }
            list.forEach(function (t) {
                sel.append('<option value="' + t.id + '">' + escapeHtml(t.name + (t.active_version_id ? '' : ' [sin versión activa]')) + '</option>')
            })
        })
        .fail(function () { flash('No se pudieron cargar las plantillas.', 'danger') })
}

function loadReports() {
    $.ajax({ url: RAPI, headers: authHeaders() }).done(function (list) {
        if (!list || !list.length) { $("#reportsList").html('<p class="text-muted">Sin informes todavía.</p>'); return }
        var html = '<table class="table"><thead><tr><th>id</th><th>Origen</th><th>Empresa</th><th>Estado</th><th>Acciones</th></tr></thead><tbody>'
        list.forEach(function (r) {
            html += '<tr><td>' + r.id + '</td><td>' + escapeHtml(r.subject_kind) + ' #' + r.subject_id + '</td>' +
                '<td>' + escapeHtml(r.company_name || '') + '</td>' +
                '<td><span class="label label-' + (r.status === 'generated' ? 'success' : 'default') + '">' + r.status + '</span></td>' +
                '<td><button class="btn btn-xs btn-primary" onclick="openEditor(' + r.id + ')"><i class="fa fa-edit"></i> Editar</button> ' +
                '<button class="btn btn-xs btn-danger" onclick="delReport(' + r.id + ')"><i class="fa fa-trash"></i></button></td></tr>'
        })
        html += '</tbody></table>'
        $("#reportsList").html(html)
    }).fail(function () { flash('Error cargando informes', 'danger') })
}

function createReport() {
    var data = {
        subject_kind: $("#subjectKind").val(), subject_id: parseInt($("#subjectId").val(), 10),
        template_id: parseInt($("#templateId").val(), 10), company_name: $("#companyName").val()
    }
    if (!data.subject_id) { flash('Busca y selecciona una campaña o grupo por su nombre.', 'warning'); return }
    if (!data.template_id) { flash('Selecciona una plantilla.', 'warning'); return }
    $.ajax({ url: RAPI, method: 'POST', headers: authHeaders(), contentType: 'application/json', data: JSON.stringify(data) })
        .done(function (rep) { openEditor(rep.id) })
        .fail(function (xhr) { flash((xhr.responseJSON && xhr.responseJSON.message) || 'Error creando informe', 'danger') })
}

function delReport(rid) {
    deleteWithForce(RAPI + '/' + rid, {
        confirm: '¿Eliminar este informe?',
        fallback: 'No se pudo eliminar el informe.',
        onDone: function () { flash('Informe eliminado.', 'success'); loadReports() }
    })
}

// ---------- editor ----------
// The slot catalog is static config; fetch it once and reuse it across editor
// opens (audit M-11). Invalidated only by a page refresh.
var _slotsCache = null
function getSlots() {
    if (_slotsCache) {
        return $.Deferred().resolve(_slotsCache).promise()
    }
    return $.ajax({ url: '/api/report-slots', headers: authHeaders() }).then(function (slots) {
        _slotsCache = slots
        return slots
    })
}

function openEditor(rid) {
    $.when(
        $.ajax({ url: RAPI + '/' + rid, headers: authHeaders() }),
        getSlots(),
        $.ajax({ url: RAPI + '/' + rid + '/assets', headers: authHeaders() })
    ).done(function (rep, slots, assets) {
        // rep/assets come from $.ajax ([data,status,xhr]); getSlots resolves with
        // the slots array directly (cached or freshly fetched).
        state.report = rep[0]; state.slots = slots
        state.assets = {}; (assets[0] || []).forEach(function (a) { state.assets[a.slot] = a.mime })
        renderEditor()
        $("#listView").hide(); $("#editorView").show()
        window.scrollTo(0, 0)
    }).fail(function () { flash('Error abriendo el informe', 'danger') })
}

function closeEditor() { $("#editorView").hide(); $("#listView").show(); state.report = null; loadReports() }

function slotsForSection(title) { return state.slots.filter(function (s) { return s.section === title }) }

function renderEditor() {
    var r = state.report
    $("#editorTitle").text('Informe #' + r.id + ' — ' + r.subject_kind + ' #' + r.subject_id)

    var html = ''
    SECTIONS.forEach(function (sec) {
        html += '<div class="panel panel-default"><div class="panel-heading"><strong>' + escapeHtml(sec.title) + '</strong></div><div class="panel-body">'
        if (sec.note) { html += '<p class="text-muted"><i class="fa fa-magic"></i> ' + escapeHtml(sec.note) + '</p>' }
        // editable fields
        ;(sec.fields || []).forEach(function (f) {
            var id = 'f-' + f.k
            var star = f.req ? ' <span class="text-danger" title="Obligatorio">*</span>' : ''
            html += '<div class="form-group" id="fg-' + f.k + '" style="display:block;margin-bottom:10px">'
            if (f.t === 'textarea') {
                var content = r[f.k] || ''
                if (f.prefix && content.indexOf(f.prefix) === 0) {
                    content = content.substring(f.prefix.length).replace(/^\s+/, '')
                }
                if (f.prefix) {
                    html += '<label>' + f.label + star + '</label>' +
                        '<p style="margin:0 0 4px"><strong>' + escapeHtml(f.prefix) + '</strong> <small class="text-muted">(encabezado fijo)</small></p>' +
                        '<textarea class="form-control" id="' + id + '" rows="3" placeholder="Escribe solo la explicación posterior…">' + escapeHtml(content) + '</textarea>'
                } else {
                    html += '<label>' + f.label + star + '</label><textarea class="form-control" id="' + id + '" rows="3">' + escapeHtml(content) + '</textarea>'
                }
            } else if (f.t === 'checkbox') {
                html += '<label class="checkbox-inline"><input type="checkbox" id="' + id + '" onchange="apply2faVisibility()"' + ((r.users_with_2fa > 0) ? ' checked' : '') + '> ' + f.label + '</label>'
            } else if (f.t === 'date') {
                html += '<label>' + f.label + star + '</label><input type="date" class="form-control" id="' + id + '" value="' + dateVal(r[f.k]) + '">'
            } else {
                html += '<label>' + f.label + star + '</label><input type="text" class="form-control" id="' + id + '" value="' + escapeHtml(r[f.k] || '') + '">'
            }
            html += '<div class="help-block text-danger" id="err-' + f.k + '" style="display:none;margin:3px 0 0;font-size:.85em"></div>'
            html += '</div>'
        })
        // image slots (the auto chart slot is never uploadable -> skip it)
        var slots = slotsForSection(sec.title).filter(function (s) { return s.source !== 'auto' })
        if (slots.length) {
            html += '<div class="row">'
            slots.forEach(function (s) { html += renderSlot(s) })
            html += '</div>'
        }
        html += '</div></div>'
    })
    $("#sections").html(html)
    apply2faVisibility()
}

function renderSlot(s) {
    if (s.source === 'auto') {
        return '<div class="col-sm-4" style="margin-bottom:12px"><div class="thumbnail" style="padding:8px">' +
            '<strong>' + escapeHtml(s.title) + '</strong>' +
            '<p class="text-muted" style="margin:6px 0"><i class="fa fa-magic"></i> Automático (se genera del gráfico)</p></div></div>'
    }
    var loaded = !!state.assets[s.key]
    var thumb = loaded
        ? '<img src="' + assetUrl(state.report.id, s.key) + '" style="max-height:90px;max-width:100%;display:block;margin:6px auto">'
        : '<div style="height:90px;background:#f5f5f5;border:1px dashed #ccc;text-align:center;line-height:90px;color:#aaa">sin imagen</div>'
    return '<div class="col-sm-4" id="slot-col-' + s.key + '" style="margin-bottom:12px"><div class="thumbnail" style="padding:8px">' +
        '<strong>' + escapeHtml(s.title) + '</strong>' + (s.required ? ' <span class="text-danger" title="Obligatoria">*</span>' : '') + ' ' +
        (loaded ? '<span class="label label-success">cargada</span>' : (s.required ? '<span class="label label-danger">obligatoria — falta</span>' : '<span class="label label-default">opcional</span>')) +
        '<p class="text-muted" style="font-size:.85em;margin:4px 0">' + escapeHtml(s.evidence) + '</p>' +
        '<div id="thumb-' + s.key + '">' + thumb + '</div>' +
        '<input type="file" accept="image/*" style="margin-top:6px" onchange="uploadAsset(\'' + s.key + '\', this)">' +
        '</div></div>'
}

// has2faChecked reads the live checkbox (defaults to the saved value if absent).
function has2faChecked() {
    var el = document.getElementById('f-had2fa')
    return el ? el.checked : (state.report && state.report.users_with_2fa > 0)
}

// apply2faVisibility shows Figura 8 only when 2FA was found (esa sección usa el
// texto especial de 2FA y existe evidencia relacionada); la oculta si no hubo
// 2FA. Se actualiza al instante, sin guardar.
function apply2faVisibility() {
    var show = has2faChecked()
    var col = document.getElementById('slot-col-figura_8')
    if (col) col.style.display = show ? '' : 'none'
    renderStatus()
}

function renderStatus() {
    var show8 = has2faChecked()
    var manual = state.slots.filter(function (s) {
        // figura_8 solo es obligatoria cuando hay 2FA (visible).
        return s.source !== 'auto' && s.required && !(s.key === 'figura_8' && !show8)
    })
    var loaded = manual.filter(function (s) { return !!state.assets[s.key] })
    var missing = manual.filter(function (s) { return !state.assets[s.key] }).map(function (s) { return s.key })
    var ok = missing.length === 0
    $("#statusBar").html('<div class="alert alert-' + (ok ? 'success' : 'warning') + '">' +
        '<strong>Imágenes:</strong> ' + loaded.length + ' / ' + manual.length + ' cargadas. ' +
        (ok ? 'Todas las obligatorias están.' : 'Faltan: ' + escapeHtml(missing.join(', '))) + '</div>')
}

function uploadAsset(slot, input) {
    if (!input.files || !input.files[0]) return
    var fd = new FormData(); fd.append('file', input.files[0])
    $.ajax({ url: RAPI + '/' + state.report.id + '/assets/' + slot, method: 'POST', headers: authHeaders(), data: fd, processData: false, contentType: false })
        .done(function () {
            state.assets[slot] = 'image'
            $("#thumb-" + slot).html('<img src="' + assetUrl(state.report.id, slot) + '" style="max-height:90px;max-width:100%;display:block;margin:6px auto">')
            renderStatus()
            flash('Imagen "' + slot + '" cargada', 'success')
        })
        .fail(function (xhr) { flash((xhr.responseJSON && xhr.responseJSON.message) || 'Error subiendo imagen', 'danger') })
}

function collectFields() {
    var r = state.report
    var data = {
        subject_kind: r.subject_kind, subject_id: r.subject_id, template_id: r.template_id,
        company_name: $("#f-company_name").val(), prepared_by: $("#f-prepared_by").val(),
        intro_exec: $("#f-intro_exec").val(),
        // Punto 1: solo el contenido. El encabezado fijo vive en la plantilla
        // (run propio, sin negrita) y el "1." es numeración automática del Word.
        text_punto_1: $("#f-text_punto_1").val().trim(),
        users_with_2fa: $("#f-had2fa").is(":checked") ? 1 : 0
    }
    ;["report_date", "executed_from", "executed_to"].forEach(function (k) {
        var v = $("#f-" + k).val()
        if (v) data[k] = v + "T00:00:00Z"
    })
    return data
}

function saveFields() {
    $.ajax({ url: RAPI + '/' + state.report.id, method: 'PUT', headers: authHeaders(), contentType: 'application/json', data: JSON.stringify(collectFields()) })
        .done(function (rep) { state.report = rep; flash('Campos guardados', 'success') })
        .fail(function (xhr) { flash((xhr.responseJSON && xhr.responseJSON.message) || 'Error guardando', 'danger') })
}

// ---------- validación previa ----------
function clearValidation() {
    $('.field-error, .help-block.text-danger[id^="err-"]').hide().text('')
    $('.has-error').removeClass('has-error')
    $('.slot-missing').removeClass('slot-missing').css('border', '')
    $('#validationSummary').empty()
}

function markFieldError(k, msg) {
    $('#fg-' + k).addClass('has-error')
    $('#err-' + k).text(msg).show()
}

// Devuelve la lista de problemas (vacía = listo para generar).
function validateReport() {
    clearValidation()
    var problems = []
    SECTIONS.forEach(function (sec) {
        (sec.fields || []).forEach(function (f) {
            if (!f.req) return
            var v = $('#f-' + f.k).val()
            if (!v || !v.trim()) {
                markFieldError(f.k, 'Este campo es obligatorio.')
                problems.push(f.reqMsg || ('Falta completar el campo ' + f.label + '.'))
            }
        })
    })
    var show8 = has2faChecked()
    state.slots.filter(function (s) {
        return s.source !== 'auto' && s.required && !(s.key === 'figura_8' && !show8)
    }).forEach(function (s) {
        if (!state.assets[s.key]) {
            problems.push('Falta cargar la imagen: ' + s.title + '.')
            $('#slot-col-' + s.key + ' .thumbnail').addClass('slot-missing').css('border', '2px solid #d9534f')
        }
    })
    return problems
}

function showValidationSummary(problems) {
    var html = '<div class="alert alert-danger"><strong><i class="fa fa-exclamation-triangle"></i> No es posible generar el informe porque hay elementos pendientes:</strong>' +
        '<ul style="margin:6px 0 0 18px">'
    problems.forEach(function (p) { html += '<li>' + escapeHtml(p) + '</li>' })
    html += '</ul></div>'
    $('#validationSummary').html(html)
    var top = $('#validationSummary').offset()
    if (top) window.scrollTo(0, top.top - 70)
}

function generateCurrent() {
    var problems = validateReport()
    if (problems.length) {
        $("#genResult").empty()
        showValidationSummary(problems)
        return
    }
    $("#genResult").html('<span class="text-muted">Guardando…</span>')
    // Guardar primero; SOLO generar si el guardado tuvo éxito, para no generar
    // con datos inconsistentes (I-8).
    $.ajax({ url: RAPI + '/' + state.report.id, method: 'PUT', headers: authHeaders(), contentType: 'application/json', data: JSON.stringify(collectFields()) })
        .done(function () {
            $("#genResult").html('<span class="text-muted">Generando…</span>')
            $.ajax({ url: RAPI + '/' + state.report.id + '/generate', method: 'POST', headers: authHeaders() })
                .done(function (render) {
                    $("#validationSummary").empty()
                    var xlsxBtn = render.output_xlsx_sha256
                        ? '<a class="btn btn-xs btn-success" href="/api/renders/' + render.id + '/download-xlsx?api_key=' + user.api_key + '" target="_blank"><i class="fa fa-file-excel-o"></i> Descargar Excel</a> '
                        : ''
                    $("#genResult").html('<span class="label label-success">generado</span> ' +
                        '<a class="btn btn-xs btn-success" href="/api/renders/' + render.id + '/download?api_key=' + user.api_key + '" target="_blank"><i class="fa fa-download"></i> Descargar DOCX</a> ' +
                        xlsxBtn +
                        '<small class="text-muted">' + (render.output_size || 0) + ' B</small>')
                })
                .fail(function (xhr) {
                    var resp = xhr.responseJSON || {}
                    // El servidor también valida: si devuelve una lista de pendientes, mostrarla.
                    if (resp.problems && resp.problems.length) {
                        $("#genResult").empty()
                        showValidationSummary(resp.problems)
                        return
                    }
                    $("#genResult").html('<span class="label label-danger">error</span> ' + escapeHtml(resp.message || 'No se pudo generar el informe.'))
                })
        })
        .fail(function (xhr) {
            // El guardado falló: NO se genera para no usar datos inconsistentes.
            var resp = xhr.responseJSON || {}
            $("#genResult").html('<span class="label label-danger">error</span> ' + escapeHtml(resp.message || 'No se pudieron guardar los cambios; el informe no se generó. Inténtalo de nuevo.'))
        })
}

$(document).ready(function () {
    loadTemplatesSelect()
    loadSubjects()
    loadReports()
    $("#newReportForm").submit(function (e) { e.preventDefault(); createReport() })
    // Picker de campaña/grupo por nombre (estilo Campaign Groups).
    $("#subjectSearch").on('input', function () { $("#subjectId").val(''); subjectPicker($(this).val()) })
    $("#subjectKind").on('change', function () {
        $("#subjectSearch").val(''); $("#subjectId").val(''); $("#subjectDropdown").empty().hide()
    })
    $(document).on('click', function (e) {
        if (!$(e.target).closest('#subjectSearch, #subjectDropdown').length) $("#subjectDropdown").hide()
    })
})
