var campaignGroup = null
var stats = null
var campaigns = []
var editGroupSelected = [] // [{id, name, status}] for the edit modal

// Journey comparison table globals
var _allJourneys = []
var _orderedCampaigns = []
var _allRows = []       // [{email, $tr}] for the "Todos" tab search
var _ruleCount = 0      // auto-increment ID for filter rules

var FILTER_EVENTS = [
    {key: 'Email Sent',          icon: 'fa-paper-plane'},
    {key: 'Email Opened',        icon: 'fa-envelope-open-o'},
    {key: 'Clicked Link',        icon: 'fa-mouse-pointer'},
    {key: 'Submitted Data',      icon: 'fa-database'},
    {key: 'Email Reported',      icon: 'fa-flag'},
    {key: 'Calendar Opened',     icon: 'fa-calendar'},
    {key: 'Calendar Clicked',    icon: 'fa-calendar-check-o'}
]

// Get campaign group ID from URL
var groupId = window.location.pathname.split('/').pop()

$(document).ready(function () {
    loadCampaignGroup()
    loadCampaigns()

    // Picker: search input for edit modal
    $('#editCampaignSearch').on('input', function () {
        editPickerFilter($(this).val())
    }).on('keydown', function (e) {
        if (e.key === 'Enter') e.preventDefault()
    })

    // Hide dropdown when clicking outside
    $(document).on('click.editPicker', function (e) {
        if (!$(e.target).closest('#editCampaignSearch, #editCampaignDropdown').length) {
            $('#editCampaignDropdown').hide()
        }
    })

    // Reset edit modal state on close
    $('#editGroupModal').on('hidden.bs.modal', function () {
        editGroupSelected = []
        $('#editCampaignSearch').val('')
        $('#editCampaignDropdown').empty().hide()
        $('#edit-modal-flashes').empty()
    })
})

// Load campaign group details
function loadCampaignGroup() {
    api.campaign_groups.get(groupId)
        .success(function(response) {
            campaignGroup = response
            renderCampaignGroup()
            loadStats()
        })
        .error(function() {
            $("#loading").hide()
            errorFlash("Error loading campaign group")
        })
}

// Load campaign group stats
function loadStats() {
    api.campaign_groups.stats(groupId)
        .success(function(response) {
            stats = response
            renderStats()
            renderRecipientJourney()
            $("#loading").hide()
            $("#campaignGroupResults").show()
        })
        .error(function() {
            $("#loading").hide()
            errorFlash("Error loading campaign group statistics")
        })
}

// Load all campaigns for the edit selector
function loadCampaigns() {
    api.campaigns.summary()
        .success(function (response) {
            campaigns = response.campaigns || []
        })
        .error(function () {
            errorFlash('Error loading campaigns')
        })
}

// ── Edit-modal picker helpers ─────────────────────────────────────────────────

function editPickerFilter(query) {
    var $d = $('#editCampaignDropdown')
    var q = query.toLowerCase().trim()
    if (!q) { $d.empty().hide(); return }

    var selectedIds = editGroupSelected.map(function (c) { return c.id })
    var matches = campaigns.filter(function (c) {
        return selectedIds.indexOf(c.id) === -1 &&
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
                .on('click', (function (cam) {
                    return function () {
                        editGroupSelected.push({id: cam.id, name: cam.name, status: cam.status})
                        editPickerRenderTags()
                        $('#editCampaignSearch').val('').focus()
                        $d.empty().hide()
                    }
                })(c))
                .appendTo($d)
        })
    }
    $d.show()
}

function removeEditCampaign(id) {
    editGroupSelected = editGroupSelected.filter(function (x) { return x.id !== id })
    editPickerRenderTags()
}

function editPickerRenderTags() {
    var $c = $('#editCampaignTags')
    $c.empty()
    if (editGroupSelected.length === 0) {
        $c.html('<small class="text-muted">No campaigns added yet.</small>')
        return
    }
    editGroupSelected.forEach(function (c) {
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
                return function (e) { e.preventDefault(); removeEditCampaign(cid) }
            })(c.id))
            .appendTo($tag)
        $c.append($tag)
    })
}

function editInlineFlash(msg, type) {
    $('#edit-modal-flashes').html(
        '<div class="alert alert-' + type + '">' +
        '<button type="button" class="close" data-dismiss="alert">&times;</button>' +
        '<i class="fa fa-exclamation-circle"></i> ' + msg + '</div>'
    )
}

// Render campaign group information
function renderCampaignGroup() {
    $('#groupName').text(campaignGroup.name)
    
    // Render linked campaigns table
    var tbody = $('#linkedCampaignsTable tbody')
    tbody.empty()
    
    campaignGroup.campaigns.forEach(function(cgc) {
        var campaign = cgc.campaign
        if (!campaign || !campaign.id) {
            return
        }
        var statusLabel = getStatusLabel(campaign.status)
        var campaignType = campaign.campaign_type || 'email'
        var inTrash = !!campaign.deleted_at
        var nameHtml = inTrash ?
            escapeHtml(campaign.name) + ' <span class="label label-warning">In Trash</span>' :
            '<a href="/campaigns/' + campaign.id + '">' + escapeHtml(campaign.name) + '</a>'
        var actionHtml = inTrash ?
            '<a href="/trash?type=campaign" class="btn btn-sm btn-warning">' +
            '<i class="fa fa-trash"></i> View in Trash</a>' :
            '<a href="/campaigns/' + campaign.id + '" class="btn btn-sm btn-primary">' +
            '<i class="fa fa-bar-chart"></i> View</a>'
        
        var row = $('<tr>')
        row.append($('<td>').text(cgc.order_index + 1))
        row.append($('<td>').html(nameHtml))
        row.append($('<td>').text(campaignType))
        row.append($('<td>').html(statusLabel))
        row.append($('<td>').text(formatCampaignGroupDate(campaign.created_date)))
        row.append($('<td>').text(formatCampaignGroupDate(campaign.completed_date)))
        row.append($('<td>').html(actionHtml))
        
        tbody.append(row)
    })
    
    // Update archive button text
    if (campaignGroup.archived) {
        $('#archive_button').html('<i class="fa fa-folder-open"></i> Unarchive')
    }
}

function formatCampaignGroupDate(value) {
    if (!value || String(value).indexOf('0001-01-01') === 0) {
        return '-'
    }
    return moment(value).format('MMMM Do YYYY, h:mm a')
}

// Render aggregated statistics
function renderStats() {
    $('#stat_total').text(stats.total_recipients)
    $('#stat_sent').text(stats.sent)
    $('#stat_opened').text(stats.opened)
    $('#stat_clicked').text(stats.clicked)
    $('#stat_submitted').text(stats.submitted_data)
    $('#stat_reported').text(stats.email_reported)
}

// Render recipient journey table (entry point called after stats load)
function renderRecipientJourney() {
    var journeys = stats.recipient_journeys || []
    if (journeys.length === 0) {
        $('#recipientJourneySection').hide()
        $('#journeyEmpty').show()
        return
    }

    _allJourneys = journeys
    _orderedCampaigns = campaignGroup.campaigns.map(function (cgc) { return cgc.campaign })

    buildColToggles()
    buildComparisonHead('#comparisonHead')
    buildComparisonHead('#filteredHead')
    _allRows = buildComparisonRows(journeys, '#comparisonBody')

    $('#tabAllBadge').text(journeys.length)

    // Search in "Todos" tab
    $('#journeyCount').text(_allRows.length + ' de ' + _allJourneys.length + ' destinatarios')
    $('#journeySearch').off('input.journey').on('input.journey', function () {
        var q = $(this).val().toLowerCase().trim()
        var n = 0
        _allRows.forEach(function (item) {
            var show = !q || item.email.toLowerCase().indexOf(q) !== -1
            item.$tr.toggle(show)
            if (show) n++
        })
        $('#journeyCount').text(n + ' de ' + _allJourneys.length + ' destinatarios')
    })

    $('#recipientJourneySection').show()
}

// ── Column toggles ────────────────────────────────────────────────────────────

function buildColToggles() {
    var $wrap = $('<div>')
    $wrap.append(
        $('<span>').css({marginRight: '8px', fontWeight: '600', fontSize: '13px', color: '#555'})
            .html('<i class="fa fa-columns" style="margin-right:5px"></i>Columnas visibles:')
    )
    _orderedCampaigns.forEach(function (c, i) {
        var truncName = c.name.length > 28 ? c.name.substring(0, 28) + '\u2026' : c.name
        var $btn = $('<button>').addClass('btn btn-xs btn-info')
            .css({marginRight: '4px', marginBottom: '4px'})
            .html('<i class="fa fa-eye" style="margin-right:3px"></i>' + escapeHtml(truncName))
            .data('col', i).data('name', truncName).data('visible', true)
        $btn.on('click', function () {
            var $b = $(this), colIdx = $b.data('col'), name = $b.data('name'), vis = $b.data('visible')
            $b.data('visible', !vis)
            if (vis) {
                $b.removeClass('btn-info').addClass('btn-default')
                    .html('<i class="fa fa-eye-slash" style="margin-right:3px"></i>' + escapeHtml(name))
            } else {
                $b.removeClass('btn-default').addClass('btn-info')
                    .html('<i class="fa fa-eye" style="margin-right:3px"></i>' + escapeHtml(name))
            }
            $('.cam-col-' + colIdx).toggle(!vis)
        })
        $wrap.append($btn)
    })
    $('#colToggles').empty().append($wrap)
}

// ── Shared table builder ──────────────────────────────────────────────────────

function buildComparisonHead(headSel) {
    var row1 = '<tr><th rowspan="2" style="vertical-align:middle;min-width:190px;background:#f5f5f5;position:sticky;left:0;z-index:2">Email</th>'
    _orderedCampaigns.forEach(function (c, i) {
        var icon = c.campaign_type === 'calendar' ? 'fa-calendar' : 'fa-envelope'
        row1 += '<th colspan="2" class="cam-col-' + i + ' text-center" style="background:#e8f0fe;border-bottom:2px solid #337ab7">' +
                '<i class="fa ' + icon + '" style="margin-right:5px"></i>' + escapeHtml(c.name) + '</th>'
    })
    row1 += '</tr><tr>'
    _orderedCampaigns.forEach(function (c, i) {
        row1 += '<th class="cam-col-' + i + '" style="min-width:210px;background:#f0f4ff;font-size:11px;text-transform:uppercase;letter-spacing:.5px">Eventos</th>'
        row1 += '<th class="cam-col-' + i + '" style="min-width:160px;background:#f0f4ff;font-size:11px;text-transform:uppercase;letter-spacing:.5px">Datos capturados</th>'
    })
    row1 += '</tr>'
    $(headSel).html(row1)
}

function buildComparisonRows(journeys, bodySel) {
    var $body = $(bodySel).empty()
    var rows = []
    journeys.forEach(function (journey) {
        var resultByCampaign = {}
        ;(journey.campaign_results || []).forEach(function (r) { resultByCampaign[r.campaign_id] = r })

        var $tr = $('<tr>')
        $tr.append($('<td>').css({
            verticalAlign: 'top', fontWeight: '500', whiteSpace: 'nowrap',
            position: 'sticky', left: 0, background: '#fff', zIndex: 1, borderRight: '1px solid #ddd'
        }).text(journey.email))

        _orderedCampaigns.forEach(function (c, i) {
            var r = resultByCampaign[c.id]
            // Events cell
            var $ev = $('<td>').addClass('cam-col-' + i).css({verticalAlign: 'top', lineHeight: '2'})
            if (!r || !r.events || r.events.length === 0) {
                $ev.html('<span class="text-muted">\u2014</span>')
            } else {
                r.events.forEach(function (ev) {
                    $ev.append($('<div>').css({marginBottom: '1px'})
                        .html(buildEventBadge(ev.message) +
                              ' <small class="text-muted">' + moment(ev.time).format('DD/MM HH:mm') + '</small>'))
                })
            }
            $tr.append($ev)
            // Form data cell
            var $fd = $('<td>').addClass('cam-col-' + i).css({verticalAlign: 'top'})
            if (!r || !r.form_data || Object.keys(r.form_data).length === 0) {
                $fd.html('<span class="text-muted">\u2014</span>')
            } else {
                var $tbl = $('<table>').css({fontSize: '12px', width: '100%'})
                Object.keys(r.form_data).forEach(function (key) {
                    if (key === 'rid') return
                    var vals = r.form_data[key]
                    var $row = $('<tr>')
                    $row.append($('<td>').css({color: '#888', paddingRight: '6px', whiteSpace: 'nowrap', verticalAlign: 'top'}).text(key + ':'))
                    $row.append($('<td>').css({fontWeight: '500', wordBreak: 'break-all'}).text(Array.isArray(vals) ? vals.join(', ') : vals))
                    $tbl.append($row)
                })
                $fd.append($tbl)
            }
            $tr.append($fd)
        })
        rows.push({email: journey.email, $tr: $tr})
        $body.append($tr)
    })
    return rows
}

// ── Filter tab logic ──────────────────────────────────────────────────────────

function addFilterRule() {
    _ruleCount++
    var ruleId = _ruleCount

    // Campaign dropdown
    var campOptions = '<option value="">\u2014 Seleccionar campa\u00f1a \u2014</option>'
    _orderedCampaigns.forEach(function (c) {
        campOptions += '<option value="' + c.id + '">' + escapeHtml(c.name) + '</option>'
    })

    // Event checkboxes
    var checkboxes = FILTER_EVENTS.map(function (ev) {
        return '<label style="margin-right:12px;font-weight:normal;cursor:pointer;display:inline-block;margin-bottom:4px">' +
               '<input type="checkbox" class="filter-event-cb" value="' + ev.key + '" style="margin-right:5px">' +
               buildEventBadge(ev.key) + '</label>'
    }).join('')

    var $card = $('<div>').addClass('panel panel-default filter-rule').css({marginTop: '10px'}).attr('data-rule', ruleId)
    $card.append(
        $('<div>').addClass('panel-heading').css({padding: '8px 12px'}).html(
            '<strong>Regla #' + ruleId + '</strong>' +
            '<button class="btn btn-xs btn-danger pull-right" onclick="removeFilterRule(' + ruleId + ')">' +
            '<i class="fa fa-times"></i> Eliminar</button>'
        )
    )
    var $pb = $('<div>').addClass('panel-body').css({paddingBottom: '10px'})
    $pb.append($('<div>').addClass('form-group').css({marginBottom: '8px'}).html(
        '<label style="font-size:11px;text-transform:uppercase;letter-spacing:.5px;color:#666">Campa\u00f1a</label>' +
        '<select class="form-control input-sm filter-campaign-sel">' + campOptions + '</select>'
    ))
    $pb.append($('<div>').addClass('form-group').css({marginBottom: '8px'}).html(
        '<label style="font-size:11px;text-transform:uppercase;letter-spacing:.5px;color:#666">Debe tener los eventos (AND):</label>' +
        '<div style="padding:5px 0">' + checkboxes + '</div>'
    ))
    $pb.append($('<div>').addClass('form-group').css({marginBottom: '0'}).html(
        '<label style="font-size:11px;text-transform:uppercase;letter-spacing:.5px;color:#666">Hasta fecha/hora <span class="text-muted">(opcional \u2014 deja vac\u00edo para cualquier fecha)</span></label>' +
        '<input type="datetime-local" class="form-control input-sm filter-until-date">'
    ))
    $card.append($pb)
    $('#filterRulesContainer').append($card)
}

function removeFilterRule(ruleId) {
    $('[data-rule="' + ruleId + '"]').remove()
}

function applyJourneyFilter() {
    // Read rules from DOM
    var rules = []
    $('.filter-rule').each(function () {
        var $rule = $(this)
        var campaignId = parseInt($rule.find('.filter-campaign-sel').val())
        if (!campaignId) return
        var events = []
        $rule.find('.filter-event-cb:checked').each(function () { events.push($(this).val()) })
        var untilDate = $rule.find('.filter-until-date').val()
        rules.push({campaignId: campaignId, events: events, untilDate: untilDate})
    })

    var PLACEHOLDER = '<tr><td class="text-center text-muted" colspan="99" style="padding:30px">'
    if (rules.length === 0) {
        $('#filterCount').text('A\u00f1ade al menos una regla con campa\u00f1a seleccionada')
        $('#filteredBody').html(PLACEHOLDER + '<i class="fa fa-filter fa-2x"></i><br>A\u00f1ade reglas y haz clic en <strong>Aplicar filtro</strong>.</td></tr>')
        return
    }

    var filtered = _allJourneys.filter(function (journey) {
        return rules.every(function (rule) { return journeyMatchesRule(journey, rule) })
    })

    if (filtered.length === 0) {
        $('#filteredBody').html(PLACEHOLDER + '<i class="fa fa-search fa-2x"></i><br>Ning\u00fan destinatario cumple todas las reglas.</td></tr>')
        $('#filterCount').html('0 destinatarios coinciden')
        $('#tabFilterBadge').text(0).show()
        return
    }

    buildComparisonRows(filtered, '#filteredBody')
    $('#filterCount').html('<strong>' + filtered.length + '</strong> de ' + _allJourneys.length + ' destinatarios coinciden')
    $('#tabFilterBadge').text(filtered.length).show()
}

function journeyMatchesRule(journey, rule) {
    var result = null
    ;(journey.campaign_results || []).forEach(function (r) {
        if (r.campaign_id === rule.campaignId) result = r
    })
    if (!result) return false
    if (rule.events.length === 0) return true   // any presence in campaign is enough
    return rule.events.every(function (req) {
        return (result.events || []).some(function (ev) {
            var timeOk = !rule.untilDate || new Date(ev.time) <= new Date(rule.untilDate)
            return ev.message === req && timeOk
        })
    })
}

function clearJourneyFilter() {
    $('#filterRulesContainer').empty()
    $('#filteredBody').html('<tr id="filterPlaceholder"><td class="text-center text-muted" colspan="99" style="padding:30px">' +
        '<i class="fa fa-filter fa-2x"></i><br>A\u00f1ade reglas y haz clic en <strong>Aplicar filtro</strong> para ver los resultados.</td></tr>')
    $('#filterCount').text('')
    $('#tabFilterBadge').hide().text('')
    _ruleCount = 0
}

// Build a colored badge for a given event message
function buildEventBadge(message) {
    var cfg = {
        'Email Sent':      {cls: 'label-default',  icon: 'fa-paper-plane'},
        'Email Opened':    {cls: 'label-info',      icon: 'fa-envelope-open-o'},
        'Clicked Link':    {cls: 'label-warning',   icon: 'fa-mouse-pointer'},
        'Submitted Data':  {cls: 'label-danger',    icon: 'fa-database'},
        'Email Reported':  {cls: 'label-primary',   icon: 'fa-flag'},
        'Error Sending Email': {cls: 'label-default', icon: 'fa-exclamation-triangle'},
        'Calendar Opened': {cls: 'label-info',      icon: 'fa-calendar'},
        'Calendar Clicked':{cls: 'label-warning',   icon: 'fa-calendar-check-o'}
    }
    var c = cfg[message] || {cls: 'label-default', icon: 'fa-circle'}
    return '<span class="label ' + c.cls + '" style="font-size:11px;padding:3px 6px">' +
           '<i class="fa ' + c.icon + '"></i> ' + escapeHtml(message) + '</span>'
}

// Get status label HTML
function getStatusLabel(status) {
    var labelClass = 'label-default'
    switch(status) {
        case 'In progress':
            labelClass = 'label-primary'
            break
        case 'Completed':
            labelClass = 'label-success'
            break
        case 'Queued':
            labelClass = 'label-info'
            break
        case 'Error':
            labelClass = 'label-danger'
            break
    }
    return '<span class="label ' + labelClass + '">' + status + '</span>'
}

// Edit campaign group
function editCampaignGroup() {
    $('#editGroupName').val(campaignGroup.name)

    // Pre-populate picker with currently linked campaigns
    editGroupSelected = campaignGroup.campaigns.map(function (cgc) {
        return {
            id: cgc.campaign ? cgc.campaign.id : cgc.campaign_id,
            name: cgc.campaign ? cgc.campaign.name : String(cgc.campaign_id),
            status: cgc.campaign ? cgc.campaign.status : ''
        }
    })
    editPickerRenderTags()
    $('#editCampaignSearch').val('')
    $('#editCampaignDropdown').empty().hide()
    $('#edit-modal-flashes').empty()
    $('#editGroupModal').modal('show')
}

// Update campaign group
function updateCampaignGroup() {
    var name = $('#editGroupName').val().trim()
    if (!name) { editInlineFlash('Group name is required', 'danger'); return }
    if (editGroupSelected.length === 0) { editInlineFlash('Add at least one campaign', 'danger'); return }

    api.campaign_groups.put(groupId, {
        name: name,
        campaigns: editGroupSelected.map(function (c, i) {
            return {campaign_id: c.id, order_index: i}
        }),
        archived: campaignGroup.archived
    })
    .success(function () {
        $('#editGroupModal').modal('hide')
        successFlash('Campaign group updated successfully')
        setTimeout(function () { location.reload() }, 1000)
    })
    .error(function (res) {
        editInlineFlash((res.responseJSON && res.responseJSON.message) || 'Error updating group', 'danger')
    })
}

// Archive or unarchive campaign group
function archiveCampaignGroup() {
    var action = campaignGroup.archived ? "unarchive" : "archive"
    var actionTitle = campaignGroup.archived ? "Unarchive" : "Archive"
    
    Swal.fire({
        title: "Are you sure?",
        text: "This will " + action + " the campaign group.",
        type: "question",
        animation: false,
        showCancelButton: true,
        confirmButtonText: actionTitle,
        confirmButtonColor: "#f0ad4e",
        reverseButtons: true,
        allowOutsideClick: false,
        preConfirm: function() {
            return new Promise(function(resolve, reject) {
                api.campaign_groups.archive(groupId, !campaignGroup.archived)
                    .success(function(response) {
                        resolve()
                    })
                    .error(function(response) {
                        reject(response.responseJSON.message || "Error updating campaign group")
                    })
            })
        }
    }).then(function(result) {
        if (result.value) {
            successFlash("Campaign group " + action + "d successfully")
            setTimeout(function() {
                location.reload()
            }, 1000)
        }
    })
}

// Delete campaign group
function deleteCampaignGroup() {
    Swal.fire({
        title: "Are you sure?",
        text: "This will delete the campaign group. Campaigns will not be deleted.",
        type: "warning",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Delete Group",
        confirmButtonColor: "#d9534f",
        reverseButtons: true,
        allowOutsideClick: false,
        preConfirm: function() {
            return new Promise(function(resolve, reject) {
                api.campaign_groups.delete(groupId)
                    .success(function(response) {
                        resolve()
                    })
                    .error(function(response) {
                        reject(response.responseJSON.message || "Error deleting campaign group")
                    })
            })
        }
    }).then(function(result) {
        if (result.value) {
            successFlash("Campaign group deleted successfully")
            setTimeout(function() {
                window.location.href = "/campaign-groups"
            }, 1000)
        }
    })
}

// ── Export functions ─────────────────────────────────────────────────────────

// Exports aggregated group stats as CSV
function exportGroupSummary() {
    if (!stats || !campaignGroup) return
    var rows = [
        ['Group Name', campaignGroup.name],
        [''],
        ['Metric', 'Value'],
        ['Total Recipients', stats.total_recipients],
        ['Emails Sent',      stats.sent],
        ['Emails Opened',    stats.opened],
        ['Clicked Link',     stats.clicked],
        ['Submitted Data',   stats.submitted_data],
        ['Email Reported',   stats.email_reported]
    ]
    if (stats.calendar_opened)  rows.push(['Calendar Opened',  stats.calendar_opened])
    if (stats.calendar_clicked) rows.push(['Calendar Clicked', stats.calendar_clicked])

    var csvString = Papa.unparse(rows, {escapeFormulae: true})
    downloadCSV(csvString, campaignGroup.name + ' - Summary.csv')
}

// Exports recipient journey as a pivot CSV (one boolean column per event per campaign)
// scope: 'all' | 'filtered'
function exportGroupJourney(scope) {
    if (!_allJourneys.length || !_orderedCampaigns.length) {
        errorFlash('No journey data to export yet')
        return
    }

    var journeys = scope === 'filtered'
        ? getFilteredJourneys()
        : _allJourneys

    if (!journeys || journeys.length === 0) {
        errorFlash('No recipients match the current filter')
        return
    }

    // Event columns to show per campaign (in order)
    var EVENT_COLS = [
        {key: 'Email Sent',      label: 'Sent'},
        {key: 'Email Opened',    label: 'Opened'},
        {key: 'Clicked Link',    label: 'Clicked'},
        {key: 'Submitted Data',  label: 'Submitted'},
        {key: 'Email Reported',  label: 'Reported'},
        {key: 'Calendar Opened', label: 'Cal. Opened'},
        {key: 'Calendar Clicked',label: 'Cal. Clicked'}
    ]

    // Row 1: campaign name spanning its columns (merged visually in Excel via repeat)
    // Row 2: individual column labels
    var headerRow1 = ['Email']
    var headerRow2 = ['']
    _orderedCampaigns.forEach(function (c) {
        var shortName = c.name.length > 30 ? c.name.substring(0, 30) + '…' : c.name
        EVENT_COLS.forEach(function (ev) {
            headerRow1.push(shortName)
            headerRow2.push(ev.label)
        })
        headerRow1.push(shortName)
        headerRow2.push('Captured Data')
    })

    var rows = [headerRow1, headerRow2]

    journeys.forEach(function (journey) {
        var resultByCampaign = {}
        ;(journey.campaign_results || []).forEach(function (r) {
            resultByCampaign[r.campaign_id] = r
        })

        var row = [journey.email]
        _orderedCampaigns.forEach(function (c) {
            var r = resultByCampaign[c.id]
            if (!r) {
                EVENT_COLS.forEach(function () { row.push('') })
                row.push('')
                return
            }
            var eventSet = {}
            ;(r.events || []).forEach(function (ev) { eventSet[ev.message] = true })

            EVENT_COLS.forEach(function (ev) {
                row.push(eventSet[ev.key] ? '✓' : '')
            })

            // Captured form data — one "key: value" per line inside the cell
            var fd = r.form_data || {}
            var fdStr = Object.keys(fd)
                .filter(function (k) { return k !== 'rid' })
                .map(function (k) {
                    var v = Array.isArray(fd[k]) ? fd[k].join(', ') : fd[k]
                    return k + ': ' + v
                }).join('\n')
            row.push(fdStr || '')
        })
        rows.push(row)
    })

    var suffix = scope === 'filtered' ? ' - Filtered' : ' - Full'
    var csvString = Papa.unparse(rows, {escapeFormulae: true})
    downloadCSV(csvString, campaignGroup.name + suffix + '.csv')
}

// Returns the journeys currently shown in the filtered tab
function getFilteredJourneys() {
    var rules = []
    $('.filter-rule').each(function () {
        var $rule = $(this)
        var campaignId = parseInt($rule.find('.filter-campaign-sel').val())
        if (!campaignId) return
        var events = []
        $rule.find('.filter-event-cb:checked').each(function () { events.push($(this).val()) })
        var untilDate = $rule.find('.filter-until-date').val()
        rules.push({campaignId: campaignId, events: events, untilDate: untilDate})
    })
    if (rules.length === 0) return _allJourneys
    return _allJourneys.filter(function (journey) {
        return rules.every(function (rule) { return journeyMatchesRule(journey, rule) })
    })
}

// Triggers a CSV file download in the browser
function downloadCSV(csvString, filename) {
    var blob = new Blob([csvString], {type: 'text/csv;charset=utf-8;'})
    if (navigator.msSaveBlob) {
        navigator.msSaveBlob(blob, filename)
    } else {
        var url = window.URL.createObjectURL(blob)
        var a = document.createElement('a')
        a.href = url
        a.setAttribute('download', filename)
        document.body.appendChild(a)
        a.click()
        document.body.removeChild(a)
        window.URL.revokeObjectURL(url)
    }
}

// Refresh data
function refresh() {
    $('#refresh_btn').prop('disabled', true)
    loadCampaignGroup()
    setTimeout(function() {
        $('#refresh_btn').prop('disabled', false)
    }, 1000)
}

// Helper function to escape HTML
function escapeHtml(text) {
    if (!text) return ''
    var map = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#039;'
    }
    return text.replace(/[&<>"']/g, function(m) { return map[m] })
}

// Flash message helpers
function successFlash(message) {
    $("#flashes").empty()
    $("#flashes").append('<div class="alert alert-success">' +
        '<i class="fa fa-check-circle"></i> ' + message + '</div>')
    window.scrollTo(0, 0)
}

function errorFlash(message) {
    $("#flashes").empty()
    $("#flashes").append('<div class="alert alert-danger">' +
        '<i class="fa fa-exclamation-circle"></i> ' + message + '</div>')
    window.scrollTo(0, 0)
}
