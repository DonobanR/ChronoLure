// ── Global Trash Page ─────────────────────────────────────────────────────────
//
// Handles the unified /trash page.
// All state lives in these module-level variables:
//
//   allItems        — full list loaded from the API
//   filteredItems   — currently displayed list (subset of allItems)
//   currentFilter   — "all" | "campaign" | "campaign_group"
//   currentIndex    — index into filteredItems for the active modal
//
// API calls go through api.globalTrash.* (defined in gophish.js).

(function () {
    'use strict';

    // ── State ──────────────────────────────────────────────────────────────────
    var allItems = [];
    var filteredItems = [];
    var currentFilter = 'all';
    var currentIndex = -1;
    var dt; // DataTable instance
    // CL-102R-b: recipients are shown rolled up by deletion batch in the "All"
    // tab and individually in the "Destinatarios" tab.
    var recipientBatches = [];   // rollup rows for "All"
    var recipientRows = [];      // individual entries for the Destinatarios tab
    var trashCounts = {};        // unfiltered per-type totals for the badges
    var recipientFilter = { campaign: 0, group: 0, q: '' };

    // ── Helpers ────────────────────────────────────────────────────────────────

    function escHtml(text) {
        if (text == null) return '';
        return String(text).replace(/[&<>"']/g, function (c) {
            return { '&': '&amp;', '<': '&lt;', '>': '&gt;',
                     '"': '&quot;', "'": '&#039;' }[c];
        });
    }

    function typeBadge(type) {
        var map = {
            'campaign':         ['primary', 'Campaña'],
            'campaign_group':   ['info',    'Grupo de campañas'],
            'recipient':        ['warning', 'Destinatario'],
            'recipient_batch':  ['warning', 'Destinatarios']
        };
        var pair = map[type] || ['default', escHtml(type)];
        return '<span class="label label-' + pair[0] + '">' + pair[1] + '</span>';
    }

    function errorFlash(msg) {
        $('#flashes').html(
            '<div class="alert alert-danger">' +
            '<i class="fa fa-exclamation-circle"></i> ' + escHtml(msg) +
            '</div>'
        );
    }

    // ── Filter helpers ─────────────────────────────────────────────────────────

    function applyFilter(items, filter) {
        if (filter === 'all') return items.slice();
        return items.filter(function (item) { return item.type === filter; });
    }

    function countByType(items, type) {
        return items.filter(function (i) { return i.type === type; }).length;
    }

    // Badges show the UNFILTERED total per type, from a single /trash/counts call.
    // "All" counts deletion EVENTS (batches) while "Destinatarios" counts rows, so
    // the note below the tabs spells out the relationship — otherwise All(7) next
    // to Destinatarios(41) reads as a bug.
    function updateBadges() {
        var c = trashCounts || {};
        $('#badge-all').text(c.all || 0);
        $('#badge-campaign').text(c.campaigns || 0);
        $('#badge-campaign_group').text(c.campaign_groups || 0);
        $('#badge-recipient').text(c.recipients || 0);
        $('#trash-counts-note').text(TrashHelpers.countsReconcile(c));
    }

    // ── Load ───────────────────────────────────────────────────────────────────

    // loadTrash fetches everything the page needs in parallel: the campaign/group
    // items, the recipient rollup (for "All"), the individual recipients (for the
    // Destinatarios tab) and the unfiltered counts (for the badges). Loading them
    // together is what keeps the badges, the table and the note consistent.
    function loadTrash() {
        $('#trash-loading').show();
        $('#trashTable').hide();
        $('#trash-empty').hide();
        $('#trash-error').hide();

        var f = recipientFilter;
        $.when(
            api.globalTrash.list(),
            api.recipientTrash.batches(f),
            api.recipientTrash.list(f),
            api.recipientTrash.counts()
        ).done(function (itemsRes, batchesRes, rowsRes, countsRes) {
            $('#trash-loading').hide();
            var items = pick(itemsRes);
            var batches = pick(batchesRes);
            var rows = pick(rowsRes);
            trashCounts = pick(countsRes) || {};
            // Campaigns and groups only: recipients enter through the rollup so 40
            // deletions never bury them in the "All" tab.
            allItems = ((items && items.items) || []).filter(function (i) {
                return i.type !== 'recipient';
            });
            recipientBatches = (batches && batches.batches) || [];
            recipientRows = ((rows && rows.items) || []).filter(function (i) {
                return i.type === 'recipient';
            });
            updateBadges();
            renderTable(currentFilter);
        }).fail(function (xhr) {
            $('#trash-loading').hide();
            $('#trash-error').show();
            var msg = 'No se pudo cargar la papelera.';
            if (xhr && xhr.responseJSON && xhr.responseJSON.message) {
                msg = xhr.responseJSON.message;
            }
            $('#trash-error-text').text(msg);
        });
    }

    // pick unwraps a jQuery $.when argument ([data, status, xhr] or data).
    function pick(res) {
        return (Array.isArray(res) && res.length === 3) ? res[0] : res;
    }

    // Expose for inline onclick (Retry button)
    window.loadTrash = loadTrash;

    // ── Recipient rollup + rows (addendum §2/§3) ───────────────────────────────

    // asRollupItem turns a server-side batch into a TrashItem-shaped row.
    function asRollupItem(b) {
        return {
            type: 'recipient_batch',
            batch_id: b.batch_id,
            name: TrashHelpers.rollupLabel(b, Date.now()),
            count: b.count,
            campaign_id: b.campaign_id,
            campaign_name: b.campaign_name,
            campaign_count: b.campaign_count,
            group_id: b.group_id,
            group_name: b.group_name,
            scope: b.scope,
            deleted_at: b.deleted_at,
            deleted_by_name: b.deleted_by_name,
            delete_reason: b.reason,
            parent_campaign_trashed: b.any_parent_trashed,
            sample_emails: b.sample_emails
        };
    }

    // rollupRow renders a collapsed deletion event. The COUNT leads the label
    // (TrashHelpers.rollupLabel) because it is the only thing that reconciles the
    // "All" badge (events) with the "Destinatarios" badge (recipients).
    function rollupRow(item, idx, deletedAt) {
        var blocked = TrashHelpers.restoreBlockedReason(item);
        var restoreBtn = blocked
            ? '<button class="btn btn-default btn-sm" disabled data-toggle="tooltip" title="' + escHtml(blocked) + '" ' +
              'style="min-height:44px;min-width:44px"><i class="fa fa-undo"></i> Restaurar</button>'
            : '<button class="btn btn-success btn-sm" style="min-height:44px;min-width:44px" ' +
              'onclick="trashRestoreBatch(' + idx + ')"><i class="fa fa-undo"></i> Restaurar</button>';
        var actions =
            '<div class="btn-group" role="group">' + restoreBtn +
            '<button class="btn btn-danger btn-sm" style="min-height:44px;min-width:44px" ' +
                'onclick="trashPurgeBatch(' + idx + ')">' +
                '<i class="fa fa-trash"></i> Eliminar definitivamente</button>' +
            '<button class="btn btn-default btn-sm" style="min-height:44px;min-width:44px" ' +
                'onclick="trashExpandBatch(' + idx + ')" aria-expanded="false">' +
                '<i class="fa fa-list"></i> Ver</button>' +
            '</div>';
        var name = '<strong>' + escHtml(item.name) + '</strong>' +
            (item.parent_campaign_trashed
                ? ' <span class="label label-warning"><i class="fa fa-exclamation-triangle"></i> Campaña en papelera</span>'
                : '') +
            '<div class="text-muted" style="font-size:.9em">' +
            escHtml((item.sample_emails || []).join(', ')) +
            (item.count > (item.sample_emails || []).length
                ? ' y ' + (item.count - (item.sample_emails || []).length) + ' más'
                : '') + '</div>' +
            '<div id="batch-detail-' + idx + '" style="display:none"></div>';
        return [typeBadge(item.type), name, deletedAt, escHtml(item.delete_reason || '—'), actions];
    }

    // recipientRow renders ONE recipient in the Destinatarios tab. A group-scoped
    // deletion of the same email across N campaigns already arrives collapsed from
    // the server, so it shows once with its campaign count.
    function recipientRow(item, idx, deletedAt) {
        var blocked = TrashHelpers.restoreBlockedReason(item);
        var restoreBtn = blocked
            ? '<button class="btn btn-default btn-sm" disabled data-toggle="tooltip" title="' + escHtml(blocked) + '" ' +
              'style="min-height:44px;min-width:44px"><i class="fa fa-undo"></i> Restaurar</button>'
            : '<button class="btn btn-success btn-sm" style="min-height:44px;min-width:44px" ' +
              'onclick="trashRestore(' + idx + ')"><i class="fa fa-undo"></i> Restaurar</button>';
        var actions = '<div class="btn-group" role="group">' + restoreBtn +
            '<button class="btn btn-danger btn-sm" style="min-height:44px;min-width:44px" ' +
            'onclick="trashPurge(' + idx + ')"><i class="fa fa-trash"></i> Eliminar definitivamente</button></div>';
        var context = item.campaign_count > 1
            ? TrashHelpers.pluralizeCampaigns(item.campaign_count) +
              (item.group_name ? ' del grupo «' + escHtml(item.group_name) + '»' : '')
            : escHtml(item.campaign_name || item.context || '—');
        var name = escHtml(item.name) +
            (item.parent_campaign_trashed
                ? ' <span class="label label-warning"><i class="fa fa-exclamation-triangle"></i> Campaña en papelera</span>'
                : '') +
            '<div class="text-muted" style="font-size:.9em">' + context +
            (item.deleted_by_name ? ' · por ' + escHtml(item.deleted_by_name) : '') + '</div>';
        return [typeBadge(item.type), name, deletedAt, escHtml(item.delete_reason || '—'), actions];
    }

    // renderFilterContext shows which deep-link filter is active and how to clear it.
    function renderFilterContext() {
        var f = recipientFilter;
        var bits = [];
        if (f.campaign) bits.push('campaña #' + f.campaign);
        if (f.group) bits.push('grupo #' + f.group);
        if (f.q) bits.push('búsqueda «' + f.q + '»');
        $('#recipient-filter-context').html(bits.length
            ? 'Filtrado por ' + escHtml(bits.join(' · ')) +
              ' <a href="#" onclick="trashClearFilters(); return false;">Quitar filtros</a>'
            : '');
    }

    // ── Render table ───────────────────────────────────────────────────────────

    function renderTable(filter) {
        currentFilter = filter || 'all';
        // The Destinatarios tab lists recipients individually; every other tab uses
        // the campaign/group items, and "All" additionally gets the recipient ROLLUP
        // (one row per deletion event, never 40 rows burying the campaigns).
        if (currentFilter === 'recipient') {
            filteredItems = recipientRows.slice();
        } else {
            filteredItems = applyFilter(allItems, currentFilter);
            if (currentFilter === 'all') {
                filteredItems = filteredItems.concat(recipientBatches.map(asRollupItem));
                filteredItems.sort(function (a, b) {
                    return new Date(b.deleted_at) - new Date(a.deleted_at);
                });
            }
        }
        $('#recipient-filters').toggle(currentFilter === 'recipient');
        renderFilterContext();

        if (dt) {
            try { dt.destroy(); } catch (e) { /* already destroyed */ }
            dt = null;
        }
        $('#trashTable').hide();
        $('#trashTable tbody').empty();
        $('#trash-empty').hide();

        if (filteredItems.length === 0) {
            // A filtered search with no matches is NOT an empty trash: saying
            // "there is nothing here" would make the user believe the record was
            // lost. Two distinct states, two distinct messages.
            var es = TrashHelpers.emptyState(currentFilter, recipientFilter);
            $('#trash-empty-text').text(es.title);
            $('#trash-empty-hint').text(es.hint);
            $('#trash-empty-actions').html(es.showClear
                ? '<button class="btn btn-sm btn-default" style="min-height:44px" ' +
                  'onclick="trashClearFilters()">Quitar filtros</button>'
                : '');
            $('#trash-empty').show();
            return;
        }

        var rows = filteredItems.map(function (item, idx) {
            var deletedAt = item.deleted_at
                ? moment(item.deleted_at).format('DD/MM/YYYY HH:mm')
                : '—';

            if (item.type === 'recipient_batch') return rollupRow(item, idx, deletedAt);
            if (item.type === 'recipient') return recipientRow(item, idx, deletedAt);

            var reason = item.delete_reason || '—';
            if (reason.length > 35) {
                reason = '<span data-toggle="tooltip" title="' +
                    escHtml(item.delete_reason) + '">' +
                    escHtml(reason.substring(0, 35)) + '…</span>';
            } else {
                reason = escHtml(reason);
            }

            var actions =
                '<div class="btn-group" role="group">' +
                '<button class="btn btn-success btn-sm" ' +
                    'onclick="trashRestore(' + idx + ')" ' +
                    'data-toggle="tooltip" title="Restaurar">' +
                    '<i class="fa fa-undo"></i> Restaurar</button>' +
                '<button class="btn btn-danger btn-sm" ' +
                    'onclick="trashPurge(' + idx + ')" ' +
                    'data-toggle="tooltip" title="Eliminar definitivamente">' +
                    '<i class="fa fa-trash"></i> Eliminar definitivamente</button>' +
                '</div>';

            return [typeBadge(item.type), escHtml(item.name), deletedAt, reason, actions];
        });

        dt = $('#trashTable').DataTable({
            data: rows,
            columnDefs: [{ orderable: false, targets: 'no-sort' }],
            order: [[2, 'desc']]
        });

        $('#trashTable').show();
        $('[data-toggle="tooltip"]').tooltip();
    }

    // ── Tab switching ──────────────────────────────────────────────────────────

    $('#trashFilterTabs li').on('click', function () {
        var filter = $(this).data('filter');
        $('#trashFilterTabs li').removeClass('active');
        $(this).addClass('active');
        syncDeepLink(filter);
        renderTable(filter);
    });

    // syncDeepLink writes the active view into the URL so the back button works and
    // the filtered view can be shared (addendum §5).
    function syncDeepLink(filter) {
        var qs = TrashHelpers.deepLinkQuery({
            type: filter, campaign: recipientFilter.campaign,
            group: recipientFilter.group, q: recipientFilter.q
        });
        if (window.history && window.history.replaceState) {
            window.history.replaceState(null, '', window.location.pathname + qs);
        }
    }

    // Read the deep link on load: ?type=recipient&campaign={id}|&group={id}&q=…
    (function () {
        var parsed = TrashHelpers.parseDeepLink(window.location.search);
        recipientFilter = { campaign: parsed.campaign, group: parsed.group, q: parsed.q };
        var type = parsed.type || 'all';
        var $tab = $('#trashFilterTabs li[data-filter="' + type + '"]');
        if ($tab.length) {
            $('#trashFilterTabs li').removeClass('active');
            $tab.addClass('active');
            currentFilter = type;
        }
        if (parsed.q) $('#recipient-search').val(parsed.q);
    })();

    // Debounced email search inside the Destinatarios tab.
    var searchTimer = null;
    $('#recipient-search').on('input', function () {
        var val = $(this).val();
        clearTimeout(searchTimer);
        searchTimer = setTimeout(function () {
            recipientFilter.q = val;
            syncDeepLink(currentFilter);
            loadTrash();
        }, 300);
    });

    window.trashClearFilters = function () {
        recipientFilter = { campaign: 0, group: 0, q: '' };
        $('#recipient-search').val('');
        syncDeepLink(currentFilter);
        loadTrash();
    };

    // ── Recipient actions (rollup + individual) ────────────────────────────────

    // trashExpandBatch loads and shows the recipients behind a rolled-up row.
    window.trashExpandBatch = function (idx) {
        var item = filteredItems[idx];
        var $box = $('#batch-detail-' + idx);
        if ($box.is(':visible')) { $box.hide().empty(); return; }
        api.recipientTrash.batchDetail(item.batch_id)
            .success(function (data) {
                var items = (data && data.items) || [];
                $box.html('<ul style="margin:6px 0 0 0;padding-left:18px">' +
                    items.map(function (r) {
                        return '<li>' + escHtml(r.email) + ' <span class="text-muted">· ' +
                            escHtml(r.campaign_name || '—') + '</span></li>';
                    }).join('') + '</ul>').show();
            })
            .error(function () { $box.html('<span class="text-danger">No se pudo cargar el detalle.</span>').show(); });
    };

    // trashRestoreBatch restores an entire deletion event in one operation.
    window.trashRestoreBatch = function (idx) {
        var item = filteredItems[idx];
        api.recipientTrash.restoreBatch(item.batch_id)
            .success(function (r) {
                Swal.fire({
                    toast: true, position: 'top-end', type: 'success',
                    title: 'Se restauraron ' + TrashHelpers.pluralizeRecipients(r.restored || 0) + '.',
                    showConfirmButton: false, timer: 4000
                });
                loadTrash();
            })
            .error(recipientTrashError);
    };

    // trashPurgeBatch permanently deletes a whole batch. Irreversible → the dialog
    // demands the literal word ELIMINAR, validated again in the backend.
    window.trashPurgeBatch = function (idx) {
        var item = filteredItems[idx];
        Swal.fire({
            title: 'Eliminar definitivamente ' + TrashHelpers.pluralizeRecipients(item.count),
            html: 'Esta acción no se puede deshacer. Para confirmar, escribe <strong>ELIMINAR</strong>.',
            input: 'text',
            inputPlaceholder: 'ELIMINAR',
            type: 'warning',
            animation: false,
            showCancelButton: true,
            focusCancel: true,
            reverseButtons: true,
            allowOutsideClick: false,
            confirmButtonText: 'Eliminar definitivamente',
            confirmButtonColor: '#c9302c',
            cancelButtonText: 'Cancelar'
        }).then(function (result) {
            if (!result.value) return;
            api.recipientTrash.purgeBatch(item.batch_id)
                .success(function (r) {
                    Swal.fire({
                        toast: true, position: 'top-end', type: 'success',
                        title: 'Se eliminaron definitivamente ' + TrashHelpers.pluralizeRecipients(r.purged || 0) + '.',
                        showConfirmButton: false, timer: 4000
                    });
                    loadTrash();
                })
                .error(recipientTrashError);
        });
    };

    function recipientTrashError(xhr) {
        var msg = (xhr && xhr.responseJSON && xhr.responseJSON.message) ||
            'No se pudo completar la acción sobre el destinatario.';
        Swal.fire({ type: 'error', title: 'Error', text: msg });
    }

    // ── Restore ────────────────────────────────────────────────────────────────

    window.trashRestore = function (idx) {
        currentIndex = idx;
        var item = filteredItems[idx];
        // Recipients restore without a modal (reversible, and the nested-trash guard
        // lives server-side); campaigns/groups keep their existing dialog.
        if (item.type === 'recipient') {
            api.recipientTrash.restore(item.id)
                .success(function () {
                    Swal.fire({
                        toast: true, position: 'top-end', type: 'success',
                        title: 'Se restauró el destinatario.', showConfirmButton: false, timer: 4000
                    });
                    loadTrash();
                })
                .error(recipientTrashError);
            return;
        }
        $('#restore-item-name').text(item.name);
        $('#restore-warning').hide();
        $('#restoreModal').modal('show');
    };

    function doRestore() {
        if (currentIndex < 0) return;
        var item = filteredItems[currentIndex];
        var $btn = $('#confirm-restore');
        $btn.prop('disabled', true).html('<i class="fa fa-spinner fa-spin"></i> Restaurando…');

        api.globalTrash.restore(item.type, item.id)
            .success(function (data) {
                $('#restoreModal').modal('hide');
                var html = 'Se restauró <strong>' + escHtml(item.name) + '</strong>.';
                if (data.name_changed) {
                    html += '<br><small class="text-warning">Nombre ajustado a: <strong>' +
                        escHtml(data.new_name) + '</strong></small>';
                }
                Swal.fire({
                    title: '¡Restaurado!', html: html, type: 'success',
                    confirmButtonText: 'OK'
                }).then(function () { loadTrash(); });
            })
            .error(function (xhr) {
                $('#restoreModal').modal('hide');
                var msg = 'No se pudo restaurar el elemento.';
                if (xhr.responseJSON && xhr.responseJSON.message) msg = xhr.responseJSON.message;
                errorFlash(msg);
            })
            .always(function () {
                $btn.prop('disabled', false).html('<i class="fa fa-undo"></i> Restaurar');
            });
    }

    // ── Purge ──────────────────────────────────────────────────────────────────

    window.trashPurge = function (idx) {
        var itemForPurge = filteredItems[idx];
        if (itemForPurge && itemForPurge.type === 'recipient') {
            // Irreversible → written confirmation with the EXACT email, re-validated
            // by the backend. Typing forces the switch from automatic to deliberate.
            Swal.fire({
                title: 'Eliminar definitivamente',
                html: 'Escribe el correo <strong>' + escHtml(itemForPurge.name) + '</strong> para confirmar.',
                input: 'text',
                inputPlaceholder: itemForPurge.name,
                type: 'warning',
                animation: false,
                showCancelButton: true,
                focusCancel: true,
                reverseButtons: true,
                allowOutsideClick: false,
                confirmButtonText: 'Eliminar definitivamente',
                confirmButtonColor: '#c9302c',
                cancelButtonText: 'Cancelar'
            }).then(function (result) {
                if (!result.value) return;
                api.recipientTrash.purge(itemForPurge.id, result.value)
                    .success(function () {
                        Swal.fire({
                            toast: true, position: 'top-end', type: 'success',
                            title: 'Destinatario eliminado definitivamente.', showConfirmButton: false, timer: 4000
                        });
                        loadTrash();
                    })
                    .error(recipientTrashError);
            });
            return;
        }
        currentIndex = idx;
        var item = filteredItems[idx];
        $('#purge-item-name').text(item.name);
        $('#purge-expected-name').text(item.name);
        $('#purge-confirm-input').val('');
        $('#confirm-purge').prop('disabled', true);
        $('#purgeModal').modal('show');
        setTimeout(function () { $('#purge-confirm-input').focus(); }, 400);
    };

    // Normalize whitespace so names with trailing/double spaces (which the
    // browser collapses when rendering the modal) can still be matched.
    function normalizeConfirmName(s) {
        return (s || '')
            .replace(/[\u00AD\u200B-\u200F\u202A-\u202E\u2060\uFEFF]/g, '')
            .replace(/\s+/g, ' ')
            .trim();
    }

    $('#purge-confirm-input').on('input', function () {
        if (currentIndex < 0) return;
        var item = filteredItems[currentIndex];
        $('#confirm-purge').prop('disabled',
            normalizeConfirmName($(this).val()) !== normalizeConfirmName(item.name));
    });

    function doPurge() {
        if (currentIndex < 0) return;
        var item = filteredItems[currentIndex];
        var $btn = $('#confirm-purge');
        $btn.prop('disabled', true).html('<i class="fa fa-spinner fa-spin"></i> Eliminando…');

        api.globalTrash.purge(item.type, item.id, {
            confirmation: $('#purge-confirm-input').val()
        })
            .success(function () {
                $('#purgeModal').modal('hide');
                Swal.fire({
                    title: 'Eliminado definitivamente',
                    html: 'Se eliminó definitivamente <strong>' + escHtml(item.name) + '</strong>.',
                    type: 'success', confirmButtonText: 'OK'
                }).then(function () { loadTrash(); });
            })
            .error(function (xhr) {
                $('#purgeModal').modal('hide');
                var msg = 'No se pudo eliminar definitivamente el elemento.';
                if (xhr.responseJSON && xhr.responseJSON.message) msg = xhr.responseJSON.message;
                if (xhr.status === 403) {
                    Swal.fire({
                        title: 'Permiso denegado',
                        text: 'Solo los administradores pueden eliminar campañas definitivamente.',
                        type: 'error', confirmButtonText: 'OK'
                    });
                } else {
                    errorFlash(msg);
                }
            })
            .always(function () {
                $btn.prop('disabled', false).html('<i class="fa fa-trash"></i> Eliminar definitivamente');
            });
    }

    // ── Init ───────────────────────────────────────────────────────────────────

    $(document).ready(function () {
        loadTrash();

        $('#confirm-restore').on('click', doRestore);
        $('#confirm-purge').on('click', doPurge);

        $('#restoreModal').on('hidden.bs.modal', function () { currentIndex = -1; });
        $('#purgeModal').on('hidden.bs.modal', function () {
            currentIndex = -1;
            $('#purge-confirm-input').val('');
        });
    });

}()); // end IIFE
