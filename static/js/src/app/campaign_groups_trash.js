// Campaign Groups Trash Management
var trashGroups = [];
var trashTable;
var currentGroupIndex = -1;

// Escape HTML to prevent XSS
function escapeHtml(text) {
    if (text === null || text === undefined) return '';
    return String(text).replace(/[&<>"']/g, function (s) {
        return {
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#039;'
        }[s];
    });
}

// Show an error flash message
function errorFlash(message) {
    var flashes = document.getElementById('flashes');
    var safeMsg = escapeHtml(message || 'Unexpected error');
    if (!flashes) {
        console.error(safeMsg);
        return;
    }
    flashes.innerHTML =
        '<div class="alert alert-danger">' +
        '<i class="fa fa-exclamation-circle"></i> ' +
        safeMsg +
        '</div>';
}

// Load trashed campaign groups from API
function loadTrashGroups() {
    $("#loading").show();
    $("#trashTable").hide();
    $("#emptyMessage").hide();
    $("#errorMessage").hide();

    api.campaign_groups.trash()
        .success(function (data) {
            $("#loading").hide();
            trashGroups = data.groups || [];

            if (trashGroups.length > 0) {
                $("#trashTable").show();
                renderTrashTable();
            } else {
                $("#emptyMessage").show();
            }
        })
        .error(function (data) {
            $("#loading").hide();
            $("#errorMessage").show();
            var errorMsg = "Error loading trash.";
            if (data && data.responseJSON && data.responseJSON.message) {
                errorMsg = data.responseJSON.message;
            }
            $("#errorText").text(errorMsg);
        });
}

// Make loadTrashGroups accessible from onclick attributes
window.loadTrashGroups = loadTrashGroups;

// Render the trash table
function renderTrashTable() {
    if (trashTable) {
        trashTable.destroy();
        $("#trashTable tbody").empty();
    }

    var rows = [];
    $.each(trashGroups, function (i, group) {
        // Format deleted_at
        var deletedAt = "—";
        if (group.deleted_at) {
            deletedAt = moment(group.deleted_at).format('MMMM Do YYYY, h:mm:ss a');
        }

        // Format deleted_by
        var deletedBy = "—";
        if (group.deleted_by && group.deleted_by > 0) {
            deletedBy = "User #" + group.deleted_by;
        } else if (group.deleted_by === 0) {
            deletedBy = "System";
        }

        // Format delete reason (truncate long text)
        var deleteReason = group.delete_reason || "—";
        if (deleteReason.length > 40) {
            deleteReason = '<span data-toggle="tooltip" title="' + escapeHtml(deleteReason) + '">' +
                escapeHtml(deleteReason.substring(0, 40)) + '...</span>';
        } else {
            deleteReason = escapeHtml(deleteReason);
        }

        // Action buttons
        var actions =
            '<div class="btn-group" role="group">' +
            '<button class="btn btn-success btn-sm" onclick="showRestoreModal(' + i + ')" ' +
                'data-toggle="tooltip" title="Restore this group">' +
                '<i class="fa fa-undo"></i> Restore</button>' +
            '<button class="btn btn-danger btn-sm" onclick="showPurgeModal(' + i + ')" ' +
                'data-toggle="tooltip" title="Permanently delete this group">' +
                '<i class="fa fa-trash"></i> Delete Forever</button>' +
            '</div>';

        rows.push([
            escapeHtml(group.name),
            deletedAt,
            escapeHtml(deletedBy),
            deleteReason,
            actions
        ]);
    });

    trashTable = $("#trashTable").DataTable({
        data: rows,
        columnDefs: [{
            orderable: false,
            targets: "no-sort"
        }],
        order: [[1, "desc"]], // Sort by deleted_at descending
        destroy: true
    });

    $('[data-toggle="tooltip"]').tooltip();
}

// ── Restore ───────────────────────────────────────────────────────────────────

window.showRestoreModal = function (idx) {
    currentGroupIndex = idx;
    var group = trashGroups[idx];
    $("#restoreGroupName").text(group.name);
    $("#restoreWarning").hide();
    $("#restoreModal").modal('show');
};

function restoreGroup() {
    if (currentGroupIndex < 0) return;

    var group = trashGroups[currentGroupIndex];
    var $btn = $("#confirmRestore");
    $btn.prop('disabled', true).html('<i class="fa fa-spinner fa-spin"></i> Restoring...');

    api.campaign_groups.restore(group.id)
        .success(function (data) {
            $("#restoreModal").modal('hide');

            var html = 'The campaign group "<strong>' + escapeHtml(group.name) + '</strong>" has been restored.';
            if (data.name_changed) {
                html += '<br><small class="text-warning">Name changed to: <strong>' +
                    escapeHtml(data.new_name) + '</strong></small>';
            }
            html += '<br><small class="text-muted">Find it in the <a href="/campaign-groups">Campaign Groups</a> page.</small>';

            Swal.fire({
                title: 'Group Restored!',
                html: html,
                type: 'success',
                confirmButtonText: 'OK'
            }).then(function () {
                loadTrashGroups();
            });
        })
        .error(function (data) {
            $("#restoreModal").modal('hide');

            var errorMsg = "Failed to restore group.";
            if (data && data.responseJSON && data.responseJSON.message) {
                errorMsg = data.responseJSON.message;
            }

            if (data.status === 404) {
                Swal.fire({
                    title: 'Group Not Found',
                    text: 'This group may have already been purged.',
                    type: 'info',
                    confirmButtonText: 'OK'
                }).then(function () { loadTrashGroups(); });
            } else {
                errorFlash(errorMsg);
            }
        })
        .always(function () {
            $btn.prop('disabled', false).html('<i class="fa fa-undo"></i> Restore Group');
        });
}

// ── Purge ─────────────────────────────────────────────────────────────────────

window.showPurgeModal = function (idx) {
    currentGroupIndex = idx;
    var group = trashGroups[idx];
    $("#purgeGroupName").text(group.name);
    $("#purgeExpectedName").text(group.name);
    $("#purgeConfirmText").val('');
    $("#confirmPurge").prop('disabled', true);
    $("#purgeModal").modal('show');
    setTimeout(function () { $("#purgeConfirmText").focus(); }, 500);
};

// Enable purge button only when name matches exactly
$("#purgeConfirmText").on('input', function () {
    if (currentGroupIndex < 0) return;
    var group = trashGroups[currentGroupIndex];
    var isValid = $(this).val() === group.name;
    $("#confirmPurge").prop('disabled', !isValid);
});

function purgeGroup() {
    if (currentGroupIndex < 0) return;

    var group = trashGroups[currentGroupIndex];
    var $btn = $("#confirmPurge");
    $btn.prop('disabled', true).html('<i class="fa fa-spinner fa-spin"></i> Deleting...');

    api.campaign_groups.purge(group.id)
        .success(function () {
            $("#purgeModal").modal('hide');
            Swal.fire({
                title: 'Group Deleted',
                html: 'The campaign group "<strong>' + escapeHtml(group.name) + '</strong>" has been permanently deleted.',
                type: 'success',
                confirmButtonText: 'OK'
            }).then(function () {
                loadTrashGroups();
            });
        })
        .error(function (data) {
            $("#purgeModal").modal('hide');

            var errorMsg = "Failed to delete group.";
            if (data && data.responseJSON && data.responseJSON.message) {
                errorMsg = data.responseJSON.message;
            }

            if (data.status === 404) {
                Swal.fire({
                    title: 'Group Not Found',
                    text: 'This group may have already been purged.',
                    type: 'info',
                    confirmButtonText: 'OK'
                }).then(function () { loadTrashGroups(); });
            } else {
                errorFlash(errorMsg);
            }
        })
        .always(function () {
            $btn.prop('disabled', false).html('<i class="fa fa-trash"></i> Permanently Delete');
        });
}

// ── Init ──────────────────────────────────────────────────────────────────────

$(document).ready(function () {
    loadTrashGroups();

    $("#confirmRestore").on('click', function () { restoreGroup(); });
    $("#confirmPurge").on('click', function () { purgeGroup(); });

    $("#restoreModal").on('hidden.bs.modal', function () {
        currentGroupIndex = -1;
    });

    $("#purgeModal").on('hidden.bs.modal', function () {
        currentGroupIndex = -1;
        $("#purgeConfirmText").val('');
    });
});
