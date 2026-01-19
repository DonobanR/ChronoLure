// Campaign Trash Management
var trashCampaigns = [];
var trashTable;
var currentCampaignIndex = -1;

// Escape HTML to prevent XSS (Bug #2 fix)
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

// Robust error flash handler (Bug #3 fix)
function errorFlash(message) {
    const flashes = document.getElementById('flashes');
    const safeMsg = escapeHtml(message || 'Unexpected error');

    if (!flashes) {
        console.error(safeMsg);
        alert(safeMsg);
        return;
    }

    flashes.innerHTML =
        '<div class="alert alert-danger">' +
        '<i class="fa fa-exclamation-circle"></i> ' +
        safeMsg +
        '</div>';
}

// Load trash campaigns from API
function loadTrashCampaigns() {
    $("#loading").show();
    $("#trashTable").hide();
    $("#emptyMessage").hide();
    $("#errorMessage").hide();

    api.campaignsTrash.get()
        .success(function (data) {
            $("#loading").hide();
            trashCampaigns = data.campaigns || [];

            if (trashCampaigns.length > 0) {
                $("#trashTable").show();
                renderTrashTable();
            } else {
                $("#emptyMessage").show();
            }
        })
        .error(function (data) {
            $("#loading").hide();
            $("#errorMessage").show();
            var errorMsg = "Error loading trash campaigns.";
            if (data && data.responseJSON && data.responseJSON.message) {
                errorMsg = data.responseJSON.message;
            }
            $("#errorText").text(errorMsg);
        });
}

// Expose globally for onclick="loadTrashCampaigns()" (Bug #1 fix)
window.loadTrashCampaigns = loadTrashCampaigns;

// Render trash table
function renderTrashTable() {
    // Destroy existing table if it exists
    if (trashTable) {
        trashTable.destroy();
        $("#trashTable tbody").empty();
    }

    var rows = [];
    $.each(trashCampaigns, function (i, campaign) {
        // Format deleted_at date
        var deletedAt = "—";
        if (campaign.deleted_at) {
            deletedAt = moment(campaign.deleted_at).format('MMMM Do YYYY, h:mm:ss a');
        }

        // Format deleted_by
        var deletedBy = "—";
        if (campaign.deleted_by && campaign.deleted_by > 0) {
            deletedBy = "User #" + campaign.deleted_by;
        } else if (campaign.deleted_by === 0) {
            deletedBy = "System";
        }

        // Format status before delete
        var statusBefore = campaign.status_before_delete || campaign.status || "—";

        // Format delete reason
        var deleteReason = campaign.delete_reason || "—";
        if (deleteReason.length > 30) {
            deleteReason = '<span data-toggle="tooltip" title="' + escapeHtml(deleteReason) + '">' +
                escapeHtml(deleteReason.substring(0, 30)) + '...</span>';
        } else {
            deleteReason = escapeHtml(deleteReason);
        }

        // Build action buttons
        var actions = '<div class="btn-group" role="group">';
        
        // Restore button
        actions += '<button class="btn btn-success btn-sm" onclick="showRestoreModal(' + i + ')" ' +
            'data-toggle="tooltip" title="Restore this campaign">' +
            '<i class="fa fa-undo"></i> Restore</button>';

        // Purge button (only for admins or check permissions)
        actions += '<button class="btn btn-danger btn-sm" onclick="showPurgeModal(' + i + ')" ' +
            'data-toggle="tooltip" title="Permanently delete this campaign">' +
            '<i class="fa fa-trash"></i> Delete Forever</button>';
        
        actions += '</div>';

        var row = [
            escapeHtml(campaign.name),
            escapeHtml(statusBefore),
            deletedAt,
            escapeHtml(deletedBy),
            deleteReason,
            actions
        ];
        rows.push(row);
    });

    // Initialize DataTable
    trashTable = $("#trashTable").DataTable({
        data: rows,
        columnDefs: [{
            orderable: false,
            targets: "no-sort"
        }],
        order: [
            [2, "desc"] // Sort by deleted_at descending
        ],
        destroy: true
    });

    // Initialize tooltips
    $('[data-toggle="tooltip"]').tooltip();
}

// Show restore modal
window.showRestoreModal = function(idx) {
    currentCampaignIndex = idx;
    var campaign = trashCampaigns[idx];
    
    $("#restoreCampaignName").text(campaign.name);
    $("#restoreWarning").hide();
    
    $("#restoreModal").modal('show');
}

// Restore campaign
function restoreCampaign() {
    if (currentCampaignIndex < 0) return;
    
    var campaign = trashCampaigns[currentCampaignIndex];
    var $btn = $("#confirmRestore");
    
    // Disable button and show loading
    $btn.prop('disabled', true).html('<i class="fa fa-spinner fa-spin"></i> Restoring...');
    
    api.campaignId.restore(campaign.id)
        .success(function (data) {
            $("#restoreModal").modal('hide');
            
            // Show success message
            Swal.fire({
                title: 'Campaign Restored!',
                html: 'The campaign "<strong>' + escapeHtml(campaign.name) + '</strong>" has been restored.<br>' +
                      '<small class="text-muted">It is now in a paused state and can be found in the Active Campaigns tab.</small>',
                type: 'success',
                confirmButtonText: 'OK'
            }).then(function() {
                // Reload trash campaigns
                loadTrashCampaigns();
            });
        })
        .error(function (data) {
            $("#restoreModal").modal('hide');
            
            var errorMsg = "Failed to restore campaign.";
            if (data && data.responseJSON && data.responseJSON.message) {
                errorMsg = data.responseJSON.message;
            }
            
            // Check for specific error cases
            if (errorMsg.includes("conflict") || errorMsg.includes("already exists")) {
                Swal.fire({
                    title: 'Conflict Detected',
                    html: 'The campaign was restored but the name was adjusted due to a conflict.<br><br>' +
                          '<small>' + escapeHtml(errorMsg) + '</small>',
                    type: 'warning',
                    confirmButtonText: 'OK'
                }).then(function() {
                    loadTrashCampaigns();
                });
            } else if (data.status === 404) {
                Swal.fire({
                    title: 'Campaign Not Found',
                    text: 'This campaign may have been automatically purged by the TTL job.',
                    type: 'info',
                    confirmButtonText: 'OK'
                }).then(function() {
                    loadTrashCampaigns();
                });
            } else {
                errorFlash(errorMsg);
            }
        })
        .always(function() {
            // Re-enable button
            $btn.prop('disabled', false).html('<i class="fa fa-undo"></i> Restore Campaign');
        });
}

// Show purge modal
window.showPurgeModal = function(idx) {
    currentCampaignIndex = idx;
    var campaign = trashCampaigns[idx];
    
    $("#purgeCampaignName").text(campaign.name);
    $("#purgeExpectedName").text(campaign.name);
    $("#purgeConfirmText").val('');
    $("#confirmPurge").prop('disabled', true);
    
    $("#purgeModal").modal('show');
    
    // Focus on input field
    setTimeout(function() {
        $("#purgeConfirmText").focus();
    }, 500);
}

// Validate purge confirmation text
$("#purgeConfirmText").on('input', function() {
    var campaign = trashCampaigns[currentCampaignIndex];
    var inputText = $(this).val();
    var isValid = inputText === campaign.name;
    
    $("#confirmPurge").prop('disabled', !isValid);
});

// Purge campaign permanently
function purgeCampaign() {
    if (currentCampaignIndex < 0) return;
    
    var campaign = trashCampaigns[currentCampaignIndex];
    var $btn = $("#confirmPurge");
    
    // Disable button and show loading
    $btn.prop('disabled', true).html('<i class="fa fa-spinner fa-spin"></i> Deleting...');
    
    // Send confirmation data
    var confirmData = {
        confirm: true
    };
    
    api.campaignId.purge(campaign.id, confirmData)
        .success(function (data) {
            $("#purgeModal").modal('hide');
            
            // Show success message
            Swal.fire({
                title: 'Campaign Deleted',
                html: 'The campaign "<strong>' + escapeHtml(campaign.name) + '</strong>" has been permanently deleted.',
                type: 'success',
                confirmButtonText: 'OK'
            }).then(function() {
                // Reload trash campaigns
                loadTrashCampaigns();
            });
        })
        .error(function (data) {
            $("#purgeModal").modal('hide');
            
            var errorMsg = "Failed to delete campaign.";
            if (data && data.responseJSON && data.responseJSON.message) {
                errorMsg = data.responseJSON.message;
            }
            
            // Check for permission errors
            if (data.status === 403) {
                Swal.fire({
                    title: 'Permission Denied',
                    text: 'You do not have permission to permanently delete campaigns. Only administrators can perform this action.',
                    type: 'error',
                    confirmButtonText: 'OK'
                });
            } else if (data.status === 404) {
                Swal.fire({
                    title: 'Campaign Not Found',
                    text: 'This campaign may have been automatically purged by the TTL job.',
                    type: 'info',
                    confirmButtonText: 'OK'
                }).then(function() {
                    loadTrashCampaigns();
                });
            } else {
                errorFlash(errorMsg);
            }
        })
        .always(function() {
            // Re-enable button
            $btn.prop('disabled', false).html('<i class="fa fa-trash"></i> Permanently Delete');
        });
}

// Initialize on document ready
$(document).ready(function () {
    // Load trash campaigns
    loadTrashCampaigns();
    
    // Setup modal event handlers
    $("#confirmRestore").on('click', function() {
        restoreCampaign();
    });
    
    $("#confirmPurge").on('click', function() {
        purgeCampaign();
    });
    
    // Reset form when modal is hidden
    $("#restoreModal").on('hidden.bs.modal', function() {
        currentCampaignIndex = -1;
    });
    
    $("#purgeModal").on('hidden.bs.modal', function() {
        currentCampaignIndex = -1;
        $("#purgeConfirmText").val('');
    });
});
