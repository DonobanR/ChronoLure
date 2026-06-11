// reportCommon.js — shared helpers for the Reporting admin pages
// (reports.js and report_templates.js). Loaded BEFORE both page scripts so the
// implementation lives in exactly one place (audit I-12). escapeHtml is provided
// globally by gophish.js.

function authHeaders() { return { 'Authorization': 'Bearer ' + user.api_key } }

// flash renders a dismissable alert. The message is escaped (it can include
// server strings such as user-controlled template names).
function flash(msg, type) {
    $("#flashes").html('<div class="alert alert-' + (type || 'info') + '">' + escapeHtml(msg) + '</div>')
}

// errMsg pulls a server error message out of a jqXHR, falling back to a default.
function errMsg(xhr, fallback) {
    return (xhr && xhr.responseJSON && xhr.responseJSON.message) || fallback
}

// fmtDate renders a human date+time for display ('—' for the zero/0001 sentinel).
function fmtDate(s) {
    if (!s || s.indexOf('0001') === 0) return '—'
    var d = new Date(s)
    if (isNaN(d.getTime())) return s.substring(0, 10)
    return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

// dateVal renders YYYY-MM-DD for <input type=date> ('' for the zero sentinel).
function dateVal(v) { return (v && v.indexOf('0001') !== 0) ? v.substring(0, 10) : '' }

// assetUrl builds the (cache-busted) URL to fetch/serve a report's evidence image.
function assetUrl(reportId, key) {
    return '/api/reports/' + reportId + '/assets/' + key + '?api_key=' + user.api_key + '&ts=' + Date.now()
}

// deleteWithForce performs a DELETE that may require an admin "force" confirmation
// (the can_force flow), shared by reports and templates.
// opts: { confirm: string, fallback: string, onDone: function }.
function deleteWithForce(url, opts) {
    if (opts.confirm && !confirm(opts.confirm)) return
    function run(force) {
        $.ajax({ url: url + (force ? '?force=true' : ''), method: 'DELETE', headers: authHeaders() })
            .done(opts.onDone)
            .fail(function (xhr) {
                var r = xhr.responseJSON || {}
                if (r.can_force) {
                    if (confirm(r.message + '\n\n¿Forzar la eliminación como administrador?')) run(true)
                } else {
                    flash(r.message || opts.fallback, 'danger')
                }
            })
    }
    run(false)
}
