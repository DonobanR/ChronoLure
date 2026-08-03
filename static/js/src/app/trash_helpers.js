// Pure helpers for the recipient Trash UI (CL-102R-b).
//
// These are deliberately free of jQuery/DOM so they can be unit-tested in Node
// (see test/js/trash_helpers.test.js). The UMD wrapper exposes them as
// window.TrashHelpers in the browser and as a CommonJS module under Node.
(function (root, factory) {
    if (typeof module === 'object' && module.exports) {
        module.exports = factory();
    } else {
        root.TrashHelpers = factory();
    }
})(typeof self !== 'undefined' ? self : this, function () {
    'use strict';

    // plural is THE single pluralization helper. Spanish needs the VERB to agree too
    // ("Se eliminará 1 registro" vs "Se eliminarán 2 registros"), so callers pass both
    // whole phrases with %d instead of gluing an "s" onto a noun. Three pluralization
    // bugs shipped before this existed; every new count string must go through it.
    function plural(n, one, other) {
        n = Number(n) || 0;
        return String(n === 1 ? one : other).replace('%d', n);
    }

    // pluralizeRecipients renders the COUNT prominently. This number is what
    // reconciles the two badges: "All" counts deletion batches while
    // "Destinatarios" counts rows, so a rollup row must state how many
    // recipients it stands for or the user thinks a badge is broken.
    function pluralizeRecipients(n) {
        return plural(n, '%d destinatario', '%d destinatarios');
    }

    function pluralizeCampaigns(n) {
        return plural(n, '%d campaña', '%d campañas');
    }

    // relativeTime renders a short "hace X" string. nowMs is injected so the
    // function stays pure and testable.
    function relativeTime(whenIso, nowMs) {
        if (!whenIso) return '';
        var then = new Date(whenIso).getTime();
        if (isNaN(then)) return '';
        var secs = Math.max(0, Math.floor((nowMs - then) / 1000));
        if (secs < 60) return 'hace segundos';
        var mins = Math.floor(secs / 60);
        if (mins < 60) return 'hace ' + mins + ' min';
        var hours = Math.floor(mins / 60);
        if (hours < 24) return hours === 1 ? 'hace 1 hora' : 'hace ' + hours + ' horas';
        var days = Math.floor(hours / 24);
        return days === 1 ? 'hace 1 día' : 'hace ' + days + ' días';
    }

    // rollupLabel builds the collapsed row label for the "All" tab:
    //   "38 destinatarios · Campaña «X» · hace 5 min"
    // For a group-scoped batch of a single email the subject is the email and the
    // context is the group, since naming one campaign of several would mislead:
    //   "qa@empresa.com · 4 campañas del grupo «Simulación Q3» · hace 5 min"
    function rollupLabel(batch, nowMs) {
        batch = batch || {};
        var parts = [];
        var singleEmailGroup = batch.scope === 'group' &&
            batch.sample_emails && batch.sample_emails.length === 1 &&
            batch.campaign_count > 1;
        if (singleEmailGroup) {
            parts.push(batch.sample_emails[0]);
            parts.push(pluralizeCampaigns(batch.campaign_count) +
                (batch.group_name ? ' del grupo «' + batch.group_name + '»' : ''));
        } else {
            parts.push(pluralizeRecipients(batch.count || 0));
            if (batch.campaign_count > 1) {
                parts.push(pluralizeCampaigns(batch.campaign_count) +
                    (batch.group_name ? ' del grupo «' + batch.group_name + '»' : ''));
            } else if (batch.campaign_name) {
                parts.push('Campaña «' + batch.campaign_name + '»');
            }
        }
        var rel = relativeTime(batch.deleted_at, nowMs);
        if (rel) parts.push(rel);
        return parts.join(' · ');
    }

    // scopeLabel describes the reach of a deletion in user words.
    function scopeLabel(scope) {
        return scope === 'group' ? 'En todo el grupo' : 'Solo en esta campaña';
    }

    // countsReconcile explains the two badges so the UI can show a truthful
    // tooltip: All counts deletion events, Destinatarios counts recipients.
    function countsReconcile(counts) {
        counts = counts || {};
        var batches = counts.recipient_batches || 0;
        var rows = counts.recipients || 0;
        if (rows === 0) return 'Sin destinatarios en la papelera.';
        // One deletion per recipient: the "N eliminaciones" half adds nothing, but the
        // noun must still be there ("3 en la papelera." read like a bug).
        if (batches === rows) return pluralizeRecipients(rows) + ' en la papelera.';
        return pluralizeRecipients(rows) + ' en ' +
            plural(batches, '%d eliminación', '%d eliminaciones') + '.';
    }

    // bulkConfirmList renders up to `max` emails plus an overflow line, because
    // recognizing beats recalling: the user must see WHAT is going away.
    function bulkConfirmList(emails, max) {
        emails = emails || [];
        max = max || 10;
        var shown = emails.slice(0, max);
        var rest = emails.length - shown.length;
        return { shown: shown, rest: rest,
            overflow: rest > 0 ? plural(rest, 'y %d más', 'y %d más') : '' };
    }

    // selectionState drives the bulk action bar. The bar appears from ONE
    // selected row (the bulk path is also the "several" path), and the confirm
    // button always repeats number and object, never "Aceptar".
    function selectionState(selectedIds) {
        var n = (selectedIds || []).length;
        return {
            count: n,
            showBar: n > 0,
            confirmLabel: n === 0 ? '' : 'Eliminar ' + pluralizeRecipients(n)
        };
    }

    // deepLinkQuery builds the /trash query string for a filtered view, and
    // parseDeepLink reads it back, so the filter survives the back button and can
    // be shared.
    function deepLinkQuery(filter) {
        filter = filter || {};
        var qs = [];
        if (filter.type) qs.push('type=' + encodeURIComponent(filter.type));
        if (filter.campaign) qs.push('campaign=' + encodeURIComponent(filter.campaign));
        if (filter.group) qs.push('group=' + encodeURIComponent(filter.group));
        if (filter.q) qs.push('q=' + encodeURIComponent(filter.q));
        return qs.length ? '?' + qs.join('&') : '';
    }

    function parseDeepLink(search) {
        var out = { type: 'all', campaign: 0, group: 0, q: '' };
        var s = String(search || '').replace(/^\?/, '');
        if (!s) return out;
        s.split('&').forEach(function (pair) {
            var kv = pair.split('=');
            var k = decodeURIComponent(kv[0] || '');
            var v = decodeURIComponent((kv[1] || '').replace(/\+/g, ' '));
            if (k === 'type' && v) out.type = v;
            if (k === 'campaign') out.campaign = parseInt(v, 10) || 0;
            if (k === 'group') out.group = parseInt(v, 10) || 0;
            if (k === 'q') out.q = v;
        });
        return out;
    }


    // emptyState distinguishes TWO situations that must never share a message:
    //   - a genuinely empty bucket → teach the feature (what will appear here, and
    //     that it can be restored). Saying only "there is nothing" teaches nothing.
    //   - a filter/search with no matches → say so and offer to clear the filters.
    // Conflating them makes a user who searched for an address believe the record
    // was lost.
    function emptyState(tab, filter) {
        filter = filter || {};
        var filtered = !!(filter.q || filter.campaign || filter.group);
        if (filtered) {
            var what = tab === 'recipient' ? 'destinatario' : 'elemento';
            var crit;
            if (filter.q) crit = '«' + filter.q + '»';
            else if (filter.campaign) crit = 'la campaña #' + filter.campaign;
            else crit = 'el grupo #' + filter.group;
            return {
                filtered: true,
                title: 'Ningún ' + what + ' coincide con ' + crit + '.',
                hint: 'Revisa el término de búsqueda o quita los filtros para ver todo lo que hay en la papelera.',
                showClear: true
            };
        }
        var byTab = {
            'recipient': {
                title: 'No hay destinatarios en la papelera.',
                hint: 'Cuando elimines destinatarios de una campaña aparecerán aquí y podrás restaurarlos.'
            },
            'campaign': {
                title: 'No hay campañas en la papelera.',
                hint: 'Cuando elimines una campaña aparecerá aquí y podrás restaurarla o eliminarla definitivamente.'
            },
            'campaign_group': {
                title: 'No hay grupos de campañas en la papelera.',
                hint: 'Cuando elimines un grupo aparecerá aquí y podrás restaurarlo o eliminarlo definitivamente.'
            },
            'all': {
                title: 'La papelera está vacía.',
                hint: 'Aquí aparecerá lo que elimines —campañas, grupos y destinatarios— y podrás restaurarlo.'
            }
        };
        var m = byTab[tab] || byTab.all;
        return { filtered: false, title: m.title, hint: m.hint, showClear: false };
    }


    // esc is a DOM-free HTML escaper so the dialog builder stays testable in Node
    // (the app's escapeHtml uses jQuery).
    function esc(t) {
        return String(t == null ? '' : t).replace(/[&<>"']/g, function (c) {
            return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
        });
    }

    // buildBulkConfirmHtml builds the bulk-delete confirmation body.
    //
    // It is a PURE function of (emails, preview) so the resulting DOM can be
    // asserted in tests. That matters: this markup shipped once with the group
    // radio missing entirely, because the data feeding it came back null and no
    // test looked at the rendered dialog.
    //
    // `preview` comes from GET /campaigns/{id}/results/delete-preview and carries
    // BOTH numbers from the server: campaign_count (group size) and affected (rows
    // that really match). They are different numbers and must not be conflated.
    function buildBulkConfirmHtml(emails, preview) {
        emails = emails || [];
        preview = preview || {};
        var list = bulkConfirmList(emails, 10);
        var html = '<ul class="bulk-confirm-list" style="text-align:left;max-height:180px;overflow:auto;padding-left:20px;margin:8px 0">';
        list.shown.forEach(function (e) { html += '<li>' + esc(e) + '</li>'; });
        if (list.overflow) html += '<li class="text-muted">' + esc(list.overflow) + '</li>';
        html += '</ul>';

        var labelStyle = 'font-weight:normal;display:flex;align-items:flex-start;gap:8px;min-height:44px;padding:6px 0;margin:0';
        html += '<div class="bulk-scope" style="text-align:left;margin-top:10px">' +
            '<strong>Alcance</strong>' +
            '<label style="' + labelStyle + '">' +
            '<input type="radio" name="bulkScope" value="campaign" checked style="margin:3px 0 0 0">' +
            '<span>Solo en esta campaña</span></label>';
        if (preview.in_group) {
            html += '<label style="' + labelStyle + '">' +
                '<input type="radio" name="bulkScope" value="group" style="margin:3px 0 0 0">' +
                '<span>En todo el grupo «' + esc(preview.group_name) + '» (' +
                pluralizeCampaigns(preview.campaign_count || 0) + ')' +
                '<span class="bulk-scope-affected text-muted" style="display:block;font-size:.9em">' +
                plural(preview.affected || 0,
                    'Se eliminará %d registro en total.',
                    'Se eliminarán %d registros en total.') + '</span>' +
                '</span></label>';
        }
        html += '</div>';

        html += '<div style="text-align:left;margin-top:10px">' +
            '<label for="bulkReason" style="font-weight:normal">Motivo (opcional)</label>' +
            '<input id="bulkReason" type="text" class="form-control" value="" ' +
            'placeholder="Correos internos de validación" style="min-height:44px">' +
            '</div>';

        html += '<div class="text-muted" style="text-align:left;margin-top:10px;font-size:.9em">' +
            '<i class="fa fa-info-circle"></i> Podrás restaurarlos desde la Papelera. ' +
            'Los informes ya generados no cambian.</div>';
        return html;
    }

    // buildPurgeConfirmHtml builds the body of the written-confirmation purge
    // dialog (the irreversible one), also as a pure function so its markup is
    // asserted instead of assumed.
    function buildPurgeConfirmHtml(what, expected) {
        return '<div style="text-align:left">' +
            '<p>Esta acción <strong>no se puede deshacer</strong>.</p>' +
            '<p>Para confirmar, escribe <strong class="purge-expected">' + esc(expected) + '</strong>:</p>' +
            '</div>';
    }

    // restoreBlockedReason returns the user-facing reason a restore is disabled,
    // or "" when it is allowed. The button is DISABLED with this as its tooltip —
    // never hidden, so the user can see why.
    function restoreBlockedReason(item) {
        if (item && item.parent_campaign_trashed) {
            var name = item.campaign_name || (item.campaigns && item.campaigns.length === 1 ? item.campaigns[0].campaign_name : '');
            return name
                ? 'No se puede restaurar porque la campaña «' + name + '» está en la papelera. Restaura primero la campaña.'
                : 'No se puede restaurar porque su campaña está en la papelera. Restaura primero la campaña.';
        }
        return '';
    }

    return {
        pluralizeRecipients: pluralizeRecipients,
        pluralizeCampaigns: pluralizeCampaigns,
        relativeTime: relativeTime,
        rollupLabel: rollupLabel,
        scopeLabel: scopeLabel,
        countsReconcile: countsReconcile,
        bulkConfirmList: bulkConfirmList,
        selectionState: selectionState,
        deepLinkQuery: deepLinkQuery,
        parseDeepLink: parseDeepLink,
        plural: plural,
        emptyState: emptyState,
        esc: esc,
        buildBulkConfirmHtml: buildBulkConfirmHtml,
        buildPurgeConfirmHtml: buildPurgeConfirmHtml,
        restoreBlockedReason: restoreBlockedReason
    };
});
