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
            'campaign':       ['primary', 'Campaign'],
            'campaign_group': ['info',    'Campaign Group']
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

    function updateBadges(items) {
        $('#badge-all').text(items.length);
        $('#badge-campaign').text(countByType(items, 'campaign'));
        $('#badge-campaign_group').text(countByType(items, 'campaign_group'));
    }

    // ── Load ───────────────────────────────────────────────────────────────────

    function loadTrash() {
        $('#trash-loading').show();
        $('#trashTable').hide();
        $('#trash-empty').hide();
        $('#trash-error').hide();

        api.globalTrash.list()
            .success(function (data) {
                $('#trash-loading').hide();
                allItems = data.items || [];
                updateBadges(allItems);
                renderTable(currentFilter);
            })
            .error(function (xhr) {
                $('#trash-loading').hide();
                $('#trash-error').show();
                var msg = 'Error loading trash.';
                if (xhr.responseJSON && xhr.responseJSON.message) {
                    msg = xhr.responseJSON.message;
                }
                $('#trash-error-text').text(msg);
            });
    }

    // Expose for inline onclick (Retry button)
    window.loadTrash = loadTrash;

    // ── Render table ───────────────────────────────────────────────────────────

    function renderTable(filter) {
        currentFilter = filter || 'all';
        filteredItems = applyFilter(allItems, currentFilter);

        if (dt) {
            try { dt.destroy(); } catch (e) { /* already destroyed */ }
            dt = null;
        }
        $('#trashTable').hide();
        $('#trashTable tbody').empty();
        $('#trash-empty').hide();

        if (filteredItems.length === 0) {
            $('#trash-empty').show();
            return;
        }

        var rows = filteredItems.map(function (item, idx) {
            var deletedAt = item.deleted_at
                ? moment(item.deleted_at).format('DD/MM/YYYY HH:mm')
                : '—';

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
                    'data-toggle="tooltip" title="Restore">' +
                    '<i class="fa fa-undo"></i> Restore</button>' +
                '<button class="btn btn-danger btn-sm" ' +
                    'onclick="trashPurge(' + idx + ')" ' +
                    'data-toggle="tooltip" title="Delete forever">' +
                    '<i class="fa fa-trash"></i> Purge</button>' +
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
        renderTable(filter);
    });

    // Activate the tab that matches the ?type= URL query param (for redirects)
    (function () {
        var params = new URLSearchParams(window.location.search);
        var type = params.get('type') || 'all';
        var $tab = $('#trashFilterTabs li[data-filter="' + type + '"]');
        if ($tab.length) {
            $('#trashFilterTabs li').removeClass('active');
            $tab.addClass('active');
            currentFilter = type;
        }
    })();

    // ── Restore ────────────────────────────────────────────────────────────────

    window.trashRestore = function (idx) {
        currentIndex = idx;
        var item = filteredItems[idx];
        $('#restore-item-name').text(item.name);
        $('#restore-warning').hide();
        $('#restoreModal').modal('show');
    };

    function doRestore() {
        if (currentIndex < 0) return;
        var item = filteredItems[currentIndex];
        var $btn = $('#confirm-restore');
        $btn.prop('disabled', true).html('<i class="fa fa-spinner fa-spin"></i> Restoring…');

        api.globalTrash.restore(item.type, item.id)
            .success(function (data) {
                $('#restoreModal').modal('hide');
                var html = 'Restored <strong>' + escHtml(item.name) + '</strong>.';
                if (data.name_changed) {
                    html += '<br><small class="text-warning">Name adjusted to: <strong>' +
                        escHtml(data.new_name) + '</strong></small>';
                }
                Swal.fire({
                    title: 'Restored!', html: html, type: 'success',
                    confirmButtonText: 'OK'
                }).then(function () { loadTrash(); });
            })
            .error(function (xhr) {
                $('#restoreModal').modal('hide');
                var msg = 'Restore failed.';
                if (xhr.responseJSON && xhr.responseJSON.message) msg = xhr.responseJSON.message;
                errorFlash(msg);
            })
            .always(function () {
                $btn.prop('disabled', false).html('<i class="fa fa-undo"></i> Restore');
            });
    }

    // ── Purge ──────────────────────────────────────────────────────────────────

    window.trashPurge = function (idx) {
        currentIndex = idx;
        var item = filteredItems[idx];
        $('#purge-item-name').text(item.name);
        $('#purge-expected-name').text(item.name);
        $('#purge-confirm-input').val('');
        $('#confirm-purge').prop('disabled', true);
        $('#purgeModal').modal('show');
        setTimeout(function () { $('#purge-confirm-input').focus(); }, 400);
    };

    $('#purge-confirm-input').on('input', function () {
        if (currentIndex < 0) return;
        var item = filteredItems[currentIndex];
        $('#confirm-purge').prop('disabled', $(this).val() !== item.name);
    });

    function doPurge() {
        if (currentIndex < 0) return;
        var item = filteredItems[currentIndex];
        var $btn = $('#confirm-purge');
        $btn.prop('disabled', true).html('<i class="fa fa-spinner fa-spin"></i> Deleting…');

        api.globalTrash.purge(item.type, item.id, {
            confirmation: $('#purge-confirm-input').val()
        })
            .success(function () {
                $('#purgeModal').modal('hide');
                Swal.fire({
                    title: 'Deleted',
                    html: '<strong>' + escHtml(item.name) + '</strong> has been permanently deleted.',
                    type: 'success', confirmButtonText: 'OK'
                }).then(function () { loadTrash(); });
            })
            .error(function (xhr) {
                $('#purgeModal').modal('hide');
                var msg = 'Purge failed.';
                if (xhr.responseJSON && xhr.responseJSON.message) msg = xhr.responseJSON.message;
                if (xhr.status === 403) {
                    Swal.fire({
                        title: 'Permission Denied',
                        text: 'Only administrators can permanently delete campaigns.',
                        type: 'error', confirmButtonText: 'OK'
                    });
                } else {
                    errorFlash(msg);
                }
            })
            .always(function () {
                $btn.prop('disabled', false).html('<i class="fa fa-trash"></i> Delete Forever');
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
