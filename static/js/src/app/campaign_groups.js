var campaignGroups = []
var campaigns = []
var newGroupSelected = [] // [{id, name, status}] for the "new group" modal

$(document).ready(function () {
    loadCampaignGroups()
    loadCampaigns()

    $('a[data-toggle="tab"]').on('shown.bs.tab', function (e) {
        var target = $(e.target).attr("href")
        if (target === "#activeCampaignGroups") renderCampaignGroups(false)
        else if (target === "#archivedCampaignGroups") renderCampaignGroups(true)
    })

    $('#newGroupModal').on('hidden.bs.modal', resetNewGroupModal)

    // Picker: filter as-you-type
    $('#newCampaignSearch').on('input', function () {
        pickerFilter($(this).val(), '#newCampaignDropdown', newGroupSelected, null, addNewCampaign)
    }).on('keydown', function (e) {
        if (e.key === 'Enter') e.preventDefault() // never submit form
    })

    // Hide dropdown when clicking outside
    $(document).on('click.newPicker', function (e) {
        if (!$(e.target).closest('#newCampaignSearch, #newCampaignDropdown').length) {
            $('#newCampaignDropdown').hide()
        }
    })
})

// ── Picker logic ─────────────────────────────────────────────────────────────

function pickerFilter(query, dropdownSel, selectedArr, excludeId, onSelect) {
    var $d = $(dropdownSel)
    var q = query.toLowerCase().trim()
    if (!q) { $d.empty().hide(); return }

    var selectedIds = selectedArr.map(function (c) { return c.id })
    var matches = campaigns.filter(function (c) {
        return selectedIds.indexOf(c.id) === -1 &&
               (excludeId == null || c.id !== excludeId) &&
               c.name.toLowerCase().indexOf(q) !== -1
    })

    $d.empty()
    if (matches.length === 0) {
        $d.append($('<div>').text('No campaigns found')
            .css({padding: '8px 12px', color: '#999', fontSize: '13px'}))
    } else {
        matches.forEach(function (c) {
            $('<div>')
                .css({padding: '8px 12px', cursor: 'pointer', borderBottom: '1px solid #eee', fontSize: '13px'})
                .html('<strong>' + escapeHtml(c.name) + '</strong>&nbsp;<span class="label label-default">' + escapeHtml(c.status) + '</span>')
                .hover(
                    function () { $(this).css('background', '#eef3ff') },
                    function () { $(this).css('background', '') }
                )
                .on('click', function () { onSelect(c) })
                .appendTo($d)
        })
    }
    $d.show()
}

function pickerRenderTags(arr, containerSel, onRemoveFn) {
    var $c = $(containerSel)
    $c.empty()
    if (arr.length === 0) {
        $c.html('<small class="text-muted">No campaigns added yet.</small>')
        return
    }
    arr.forEach(function (c) {
        var $tag = $('<span>')
            .css({
                display: 'inline-flex', alignItems: 'center',
                margin: '2px 4px 2px 0', padding: '4px 10px',
                background: '#337ab7', color: '#fff',
                borderRadius: '3px', fontSize: '13px'
            })
            .text(c.name)
        $('<a href="#">')
            .html('&nbsp;&times;')
            .css({color: 'rgba(255,255,255,.75)', textDecoration: 'none', fontWeight: 'bold', marginLeft: '4px', fontSize: '15px'})
            .on('click', (function (cid) {
                return function (e) { e.preventDefault(); onRemoveFn(cid) }
            })(c.id))
            .appendTo($tag)
        $c.append($tag)
    })
}

function addNewCampaign(c) {
    for (var i = 0; i < newGroupSelected.length; i++) {
        if (newGroupSelected[i].id === c.id) return
    }
    newGroupSelected.push({id: c.id, name: c.name, status: c.status})
    pickerRenderTags(newGroupSelected, '#newCampaignTags', removeNewCampaign)
    $('#newCampaignSearch').val('').focus()
    $('#newCampaignDropdown').empty().hide()
}

function removeNewCampaign(id) {
    newGroupSelected = newGroupSelected.filter(function (x) { return x.id !== id })
    pickerRenderTags(newGroupSelected, '#newCampaignTags', removeNewCampaign)
}

function resetNewGroupModal() {
    $('#groupName').val('')
    newGroupSelected = []
    pickerRenderTags(newGroupSelected, '#newCampaignTags', removeNewCampaign)
    $('#newCampaignSearch').val('')
    $('#newCampaignDropdown').empty().hide()
    $('#modal-flashes').empty()
}

// ── API & data ────────────────────────────────────────────────────────────────

function loadCampaignGroups() {
    api.campaign_groups.summary()
        .success(function (res) {
            campaignGroups = res
            $('#loading').hide()
            renderCampaignGroups(false)
        })
        .error(function () {
            $('#loading').hide()
            errorFlash('Error loading campaign groups')
        })
}

function loadCampaigns() {
    api.campaigns.summary()
        .success(function (res) { campaigns = res.campaigns || [] })
        .error(function () { errorFlash('Error loading campaigns') })
}

function createCampaignGroup() {
    var name = $('#groupName').val().trim()
    if (!name) { inlineFlash('modal-flashes', 'Group name is required', 'danger'); return }
    if (newGroupSelected.length === 0) { inlineFlash('modal-flashes', 'Add at least one campaign', 'danger'); return }

    api.campaign_groups.post({
        name: name,
        campaigns: newGroupSelected.map(function (c, i) { return {campaign_id: c.id, order_index: i} }),
        archived: false
    })
    .success(function (res) {
        $('#newGroupModal').modal('hide')
        window.location.href = '/campaign-groups/' + res.id
    })
    .error(function (res) {
        inlineFlash('modal-flashes', (res.responseJSON && res.responseJSON.message) || 'Error creating group', 'danger')
    })
}

// ── Table rendering ───────────────────────────────────────────────────────────

function renderCampaignGroups(archived) {
    var groups = campaignGroups.filter(function (g) { return g.archived === archived })
    var tableId = archived ? '#campaignGroupTableArchive' : '#campaignGroupTable'
    var emptyId = archived ? '#emptyMessageArchived' : '#emptyMessage'

    if (groups.length === 0) { $(tableId).hide(); $(emptyId).show(); return }
    $(emptyId).hide()
    $(tableId).show()

    var dt = $(tableId).DataTable({
        destroy: true,
        columnDefs: [{orderable: false, targets: 'no-sort'}],
        order: [[2, 'desc']]
    })
    dt.clear()
    groups.forEach(function (g) {
        dt.row.add([
            escapeHtml(g.name),
            g.campaign_count,
            moment(g.created_date).format('MMMM Do YYYY, h:mm:ss a'),
            g.archived
                ? '<span class="label label-default">Archived</span>'
                : '<span class="label label-success">Active</span>',
            '<div class="btn-group">' +
            '<button class="btn btn-primary btn-sm" onclick="viewCampaignGroup(' + g.id + ')"><i class="fa fa-bar-chart"></i> View</button> ' +
            '<button class="btn btn-danger btn-sm" onclick="deleteCampaignGroupConfirm(' + g.id + ',\'' + escapeHtml(g.name) + '\')"><i class="fa fa-trash-o"></i></button>' +
            '</div>'
        ])
    })
    dt.draw()
}

function viewCampaignGroup(id) {
    window.location.href = '/campaign-groups/' + id
}

function deleteCampaignGroupConfirm(id, name) {
    Swal.fire({
        title: 'Move to Trash?', type: 'warning', animation: false,
        html: "<b>" + escapeHtml(name) + "</b> will be moved to trash.<br><small>Campaigns inside the group will not be affected.</small>",
        showCancelButton: true, confirmButtonText: 'Move to Trash', confirmButtonColor: '#d9534f',
        reverseButtons: true, allowOutsideClick: false,
        preConfirm: function () {
            return new Promise(function (resolve, reject) {
                api.campaign_groups.delete(id, '')
                    .success(resolve)
                    .error(function (r) { reject((r.responseJSON && r.responseJSON.message) || 'Error') })
            })
        }
    }).then(function (result) {
        if (result.value) {
            Swal.fire({
                title: 'Moved to Trash',
                html: '<b>' + escapeHtml(name) + '</b> has been moved to trash.<br>' +
                      '<small><a href="/campaign-groups/trash">View Campaign Groups Trash</a></small>',
                type: 'success',
                confirmButtonText: 'OK'
            }).then(function() { loadCampaignGroups() })
        }
    })
}

// ── Flash helpers ─────────────────────────────────────────────────────────────

function inlineFlash(id, msg, type) {
    $('#' + id).html(
        '<div class="alert alert-' + type + '">' +
        '<button type="button" class="close" data-dismiss="alert">&times;</button>' +
        '<i class="fa fa-exclamation-circle"></i> ' + msg + '</div>'
    )
}

function successFlash(msg) {
    $('#flashes').html('<div class="alert alert-success"><i class="fa fa-check-circle"></i> ' + msg + '</div>')
    window.scrollTo(0, 0)
}

function errorFlash(msg) {
    $('#flashes').html('<div class="alert alert-danger"><i class="fa fa-exclamation-circle"></i> ' + msg + '</div>')
    window.scrollTo(0, 0)
}

function escapeHtml(t) {
    if (!t) return ''
    return String(t).replace(/[&<>"']/g, function (m) {
        return {'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;'}[m]
    })
}
    
