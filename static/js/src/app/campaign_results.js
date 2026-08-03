var map = null
var doPoll = true;
var linkSelected = [] // [{id, name, status}] – additional campaigns for the link modal

// statuses is a helper map to point result statuses to ui classes
var statuses = {
    "Email Sent": {
        color: "#1abc9c",
        label: "label-success",
        icon: "fa-envelope",
        point: "ct-point-sent"
    },
    "Emails Sent": {
        color: "#1abc9c",
        label: "label-success",
        icon: "fa-envelope",
        point: "ct-point-sent"
    },
    "In progress": {
        label: "label-primary"
    },
    "Queued": {
        label: "label-info"
    },
    "Completed": {
        label: "label-success"
    },
    "Email Opened": {
        color: "#f9bf3b",
        label: "label-warning",
        icon: "fa-envelope-open",
        point: "ct-point-opened"
    },
    "Clicked Link": {
        color: "#F39C12",
        label: "label-clicked",
        icon: "fa-mouse-pointer",
        point: "ct-point-clicked"
    },
    "Success": {
        color: "#f05b4f",
        label: "label-danger",
        icon: "fa-exclamation",
        point: "ct-point-clicked"
    },
    //not a status, but is used for the campaign timeline and user timeline
    "Email Reported": {
        color: "#45d6ef",
        label: "label-info",
        icon: "fa-bullhorn",
        point: "ct-point-reported"
    },
    "Error": {
        color: "#6c7a89",
        label: "label-default",
        icon: "fa-times",
        point: "ct-point-error"
    },
    "Error Sending Email": {
        color: "#6c7a89",
        label: "label-default",
        icon: "fa-times",
        point: "ct-point-error"
    },
    "Submitted Data": {
        color: "#f05b4f",
        label: "label-danger",
        icon: "fa-exclamation",
        point: "ct-point-clicked"
    },
    "Unknown": {
        color: "#6c7a89",
        label: "label-default",
        icon: "fa-question",
        point: "ct-point-error"
    },
    "Sending": {
        color: "#428bca",
        label: "label-primary",
        icon: "fa-spinner",
        point: "ct-point-sending"
    },
    "Retrying": {
        color: "#6c7a89",
        label: "label-default",
        icon: "fa-clock-o",
        point: "ct-point-error"
    },
    "Scheduled": {
        color: "#428bca",
        label: "label-primary",
        icon: "fa-clock-o",
        point: "ct-point-sending"
    },
    "Campaign Created": {
        label: "label-success",
        icon: "fa-rocket"
    }
}

var statusMapping = {
    "Email Sent": "sent",
    "Email Opened": "opened",
    "Clicked Link": "clicked",
    "Submitted Data": "submitted_data",
    "Email Reported": "reported",
}

// This is an underwhelming attempt at an enum
// until I have time to refactor this appropriately.
var progressListing = [
    "Email Sent",
    "Email Opened",
    "Clicked Link",
    "Submitted Data"
]

var campaign = {}
var bubbles = []

function dismiss() {
    $("#modal\\.flashes").empty()
    $("#modal").modal('hide')
    $("#resultsTable").dataTable().DataTable().clear().draw()
}

// Deletes a campaign after prompting the user
function deleteCampaign() {
    Swal.fire({
        title: "Are you sure?",
        text: "This will delete the campaign. This can't be undone!",
        type: "warning",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Delete Campaign",
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false
    }).then(function (result) {
        if (result.value) {
            deleteCampaignRequest(false)
        }
    })
}

function deleteCampaignRequest(acknowledgeCampaignGroups) {
    api.campaignId.delete(campaign.id, {
        acknowledge_campaign_groups: acknowledgeCampaignGroups
    })
        .success(function () {
            Swal.fire(
                'Campaign Deleted!',
                'This campaign has been deleted!',
                'success'
            )
            $('button:contains("OK")').on('click', function () {
                location.href = '/campaigns'
            })
        })
        .error(function (data) {
            var response = data.responseJSON || {}
            if (data.status === 409 && response.requires_acknowledgement) {
                var names = (response.campaign_groups || []).map(function (group) {
                    return escapeHtml(group.name)
                }).join(', ')
                var count = response.campaign_groups_count || 0
                var groupWord = count === 1 ? 'Campaign Group' : 'Campaign Groups'
                Swal.fire({
                    title: "Campaign Group Link",
                    html: "This campaign belongs to " + count + " " + groupWord + ": <strong>" + names + "</strong>. Moving it to Trash may affect the group view and aggregated results. Do you want to continue?",
                    type: "warning",
                    animation: false,
                    showCancelButton: true,
                    confirmButtonText: "Move to Trash",
                    confirmButtonColor: "#d9534f",
                    reverseButtons: true,
                    allowOutsideClick: false
                }).then(function (result) {
                    if (result.value) {
                        deleteCampaignRequest(true)
                    }
                })
                return
            }
            errorFlash(response.message || "Error deleting campaign")
        })
}

// Completes a campaign after prompting the user
function completeCampaign() {
    Swal.fire({
        title: "Are you sure?",
        text: "Gophish will stop processing events for this campaign",
        type: "warning",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Complete Campaign",
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false,
        showLoaderOnConfirm: true,
        preConfirm: function () {
            return new Promise(function (resolve, reject) {
                api.campaignId.complete(campaign.id)
                    .success(function (msg) {
                        resolve()
                    })
                    .error(function (data) {
                        reject(data.responseJSON.message)
                    })
            })
        }
    }).then(function (result) {
        if (result.value){
            Swal.fire(
                'Campaign Completed!',
                'This campaign has been completed!',
                'success'
            );
            $('#complete_button')[0].disabled = true;
            $('#complete_button').text('Completed!')
            doPoll = false;
        }
    })
}

// Exports campaign results as a CSV file
function exportAsCSV(scope) {
    exportHTML = $("#exportButton").html()
    var csvScope = null
    var filename = campaign.name + ' - ' + capitalize(scope) + '.csv'
    switch (scope) {
        case "results":
            csvScope = campaign.results
            break;
        case "events":
            csvScope = campaign.timeline
            break;
    }
    if (!csvScope) {
        return
    }
    $("#exportButton").html('<i class="fa fa-spinner fa-spin"></i>')
    var csvString = Papa.unparse(csvScope, {
        'escapeFormulae': true
    })
    var csvData = new Blob([csvString], {
        type: 'text/csv;charset=utf-8;'
    });
    if (navigator.msSaveBlob) {
        navigator.msSaveBlob(csvData, filename);
    } else {
        var csvURL = window.URL.createObjectURL(csvData);
        var dlLink = document.createElement('a');
        dlLink.href = csvURL;
        dlLink.setAttribute('download', filename)
        document.body.appendChild(dlLink)
        dlLink.click();
        document.body.removeChild(dlLink)
    }
    $("#exportButton").html(exportHTML)
}

function replay(event_idx) {
    request = campaign.timeline[event_idx]
    details = JSON.parse(request.details)
    url = null
    form = $('<form>').attr({
        method: 'POST',
        target: '_blank',
    })
    /* Create a form object and submit it */
    $.each(Object.keys(details.payload), function (i, param) {
        if (param == "rid") {
            return true;
        }
        if (param == "__original_url") {
            url = details.payload[param];
            return true;
        }
        $('<input>').attr({
            name: param,
        }).val(details.payload[param]).appendTo(form);
    })
    /* Ensure we know where to send the user */
    // Prompt for the URL
    Swal.fire({
        title: 'Where do you want the credentials submitted to?',
        input: 'text',
        showCancelButton: true,
        inputPlaceholder: "http://example.com/login",
        inputValue: url || "",
        inputValidator: function (value) {
            return new Promise(function (resolve, reject) {
                if (value) {
                    resolve();
                } else {
                    reject('Invalid URL.');
                }
            });
        }
    }).then(function (result) {
        if (result.value){
            url = result.value
            submitForm()
        }
    })
    return
    submitForm()

    function submitForm() {
        form.attr({
            action: url
        })
        form.appendTo('body').submit().remove()
    }
}

/**
 * Returns an HTML string that displays the OS and browser that clicked the link
 * or submitted credentials.
 * 
 * @param {object} event_details - The "details" parameter for a campaign
 *  timeline event
 * 
 */
var renderDevice = function (event_details) {
    var ua = UAParser(details.browser['user-agent'])
    var detailsString = '<div class="timeline-device-details">'

    var deviceIcon = 'laptop'
    if (ua.device.type) {
        if (ua.device.type == 'tablet' || ua.device.type == 'mobile') {
            deviceIcon = ua.device.type
        }
    }

    var deviceVendor = ''
    if (ua.device.vendor) {
        deviceVendor = ua.device.vendor.toLowerCase()
        if (deviceVendor == 'microsoft') deviceVendor = 'windows'
    }

    var deviceName = 'Unknown'
    if (ua.os.name) {
        deviceName = ua.os.name
        if (deviceName == "Mac OS") {
            deviceVendor = 'apple'
        } else if (deviceName == "Windows") {
            deviceVendor = 'windows'
        }
        if (ua.device.vendor && ua.device.model) {
            deviceName = ua.device.vendor + ' ' + ua.device.model
        }
    }

    if (ua.os.version) {
        deviceName = deviceName + ' (OS Version: ' + ua.os.version + ')'
    }

    deviceString = '<div class="timeline-device-os"><span class="fa fa-stack">' +
        '<i class="fa fa-' + escapeHtml(deviceIcon) + ' fa-stack-2x"></i>' +
        '<i class="fa fa-vendor-icon fa-' + escapeHtml(deviceVendor) + ' fa-stack-1x"></i>' +
        '</span> ' + escapeHtml(deviceName) + '</div>'

    detailsString += deviceString

    var deviceBrowser = 'Unknown'
    var browserIcon = 'info-circle'
    var browserVersion = ''

    if (ua.browser && ua.browser.name) {
        deviceBrowser = ua.browser.name
        // Handle the "mobile safari" case
        deviceBrowser = deviceBrowser.replace('Mobile ', '')
        if (deviceBrowser) {
            browserIcon = deviceBrowser.toLowerCase()
            if (browserIcon == 'ie') browserIcon = 'internet-explorer'
        }
        browserVersion = '(Version: ' + ua.browser.version + ')'
    }

    var browserString = '<div class="timeline-device-browser"><span class="fa fa-stack">' +
        '<i class="fa fa-' + escapeHtml(browserIcon) + ' fa-stack-1x"></i></span> ' +
        deviceBrowser + ' ' + browserVersion + '</div>'

    detailsString += browserString
    detailsString += '</div>'
    return detailsString
}

function renderTimeline(data) {
    record = {
        "id": data[0],
        "first_name": data[2],
        "last_name": data[3],
        "email": data[4],
        "position": data[5],
        "status": data[6],
        "reported": data[7],
        "send_date": data[8]
    }
    results = '<div class="timeline col-sm-12 well well-lg">' +
        '<h6>Timeline for ' + escapeHtml(record.first_name) + ' ' + escapeHtml(record.last_name) +
        '</h6><span class="subtitle">Email: ' + escapeHtml(record.email) +
        '<br>Result ID: ' + escapeHtml(record.id) + '</span>' +
        '<div class="timeline-graph col-sm-6">'
    $.each(campaign.timeline, function (i, event) {
        if (!event.email || event.email == record.email) {
            // Add the event
            results += '<div class="timeline-entry">' +
                '    <div class="timeline-bar"></div>'
            results +=
                '    <div class="timeline-icon ' + statuses[event.message].label + '">' +
                '    <i class="fa ' + statuses[event.message].icon + '"></i></div>' +
                '    <div class="timeline-message">' + escapeHtml(event.message) +
                '    <span class="timeline-date">' + moment.utc(event.time).local().format('MMMM Do YYYY h:mm:ss a') + '</span>'
            if (event.details) {
                details = JSON.parse(event.details)
                if (event.message == "Clicked Link" || event.message == "Submitted Data") {
                    deviceView = renderDevice(details)
                    if (deviceView) {
                        results += deviceView
                    }
                }
                if (event.message == "Submitted Data") {
                    results += '<div class="timeline-replay-button"><button onclick="replay(' + i + ')" class="btn btn-success">'
                    results += '<i class="fa fa-refresh"></i> Replay Credentials</button></div>'
                    results += '<div class="timeline-event-details"><i class="fa fa-caret-right"></i> View Details</div>'
                }
                if (details.payload) {
                    results += '<div class="timeline-event-results">'
                    results += '    <table class="table table-condensed table-bordered table-striped">'
                    results += '        <thead><tr><th>Parameter</th><th>Value(s)</tr></thead><tbody>'
                    $.each(Object.keys(details.payload), function (i, param) {
                        if (param == "rid") {
                            return true;
                        }
                        results += '    <tr>'
                        results += '        <td>' + escapeHtml(param) + '</td>'
                        results += '        <td>' + escapeHtml(details.payload[param]) + '</td>'
                        results += '    </tr>'
                    })
                    results += '       </tbody></table>'
                    results += '</div>'
                }
                if (details.error) {
                    results += '<div class="timeline-event-details"><i class="fa fa-caret-right"></i> View Details</div>'
                    results += '<div class="timeline-event-results">'
                    results += '<span class="label label-default">Error</span> ' + details.error
                    results += '</div>'
                }
            }
            results += '</div></div>'
        }
    })
    // Add the scheduled send event at the bottom
    if (record.status == "Scheduled" || record.status == "Retrying") {
        results += '<div class="timeline-entry">' +
            '    <div class="timeline-bar"></div>'
        results +=
            '    <div class="timeline-icon ' + statuses[record.status].label + '">' +
            '    <i class="fa ' + statuses[record.status].icon + '"></i></div>' +
            '    <div class="timeline-message">' + "Scheduled to send at " + record.send_date + '</span>'
    }
    results += '</div></div>'
    return results
}

var renderTimelineChart = function (chartopts) {
    return Highcharts.chart('timeline_chart', {
        chart: {
            zoomType: 'x',
            type: 'line',
            height: "200px"
        },
        title: {
            text: 'Campaign Timeline'
        },
        xAxis: {
            type: 'datetime',
            dateTimeLabelFormats: {
                second: '%l:%M:%S',
                minute: '%l:%M',
                hour: '%l:%M',
                day: '%b %d, %Y',
                week: '%b %d, %Y',
                month: '%b %Y'
            }
        },
        yAxis: {
            min: 0,
            max: 2,
            visible: false,
            tickInterval: 1,
            labels: {
                enabled: false
            },
            title: {
                text: ""
            }
        },
        tooltip: {
            formatter: function () {
                return Highcharts.dateFormat('%A, %b %d %l:%M:%S %P', new Date(this.x)) +
                    '<br>Event: ' + this.point.message + '<br>Email: <b>' + this.point.email + '</b>'
            }
        },
        legend: {
            enabled: false
        },
        plotOptions: {
            series: {
                marker: {
                    enabled: true,
                    symbol: 'circle',
                    radius: 3
                },
                cursor: 'pointer',
            },
            line: {
                states: {
                    hover: {
                        lineWidth: 1
                    }
                }
            }
        },
        credits: {
            enabled: false
        },
        series: [{
            data: chartopts['data'],
            dashStyle: "shortdash",
            color: "#cccccc",
            lineWidth: 1,
            turboThreshold: 0
        }]
    })
}

/* Renders a pie chart using the provided chartops */
var renderPieChart = function (chartopts) {
    return Highcharts.chart(chartopts['elemId'], {
        chart: {
            type: 'pie',
            events: {
                load: function () {
                    var chart = this,
                        rend = chart.renderer,
                        pie = chart.series[0],
                        left = chart.plotLeft + pie.center[0],
                        top = chart.plotTop + pie.center[1];
                    this.innerText = rend.text(chartopts['data'][0].count, left, top).
                    attr({
                        'text-anchor': 'middle',
                        'font-size': '24px',
                        'font-weight': 'bold',
                        'fill': chartopts['colors'][0],
                        'font-family': 'Helvetica,Arial,sans-serif'
                    }).add();
                },
                render: function () {
                    this.innerText.attr({
                        text: chartopts['data'][0].count
                    })
                }
            }
        },
        title: {
            text: chartopts['title']
        },
        plotOptions: {
            pie: {
                innerSize: '80%',
                dataLabels: {
                    enabled: false
                }
            }
        },
        credits: {
            enabled: false
        },
        tooltip: {
            formatter: function () {
                if (this.key == undefined) {
                    return false
                }
                return '<span style="color:' + this.color + '">\u25CF</span>' + this.point.name + ': <b>' + this.y + '%</b><br/>'
            }
        },
        series: [{
            data: chartopts['data'],
            colors: chartopts['colors'],
        }]
    })
}

/* Updates the bubbles on the map

@param {campaign.result[]} results - The campaign results to process
*/
var updateMap = function (results) {
    if (!map) {
        return
    }
    bubbles = []
    $.each(campaign.results, function (i, result) {
        // Check that it wasn't an internal IP
        if (result.latitude == 0 && result.longitude == 0) {
            return true;
        }
        newIP = true
        $.each(bubbles, function (i, bubble) {
            if (bubble.ip == result.ip) {
                bubbles[i].radius += 1
                newIP = false
                return false
            }
        })
        if (newIP) {
            bubbles.push({
                latitude: result.latitude,
                longitude: result.longitude,
                name: result.ip,
                fillKey: "point",
                radius: 2
            })
        }
    })
    map.bubbles(bubbles)
}

/**
 * Creates a status label for use in the results datatable
 * @param {string} status 
 * @param {moment(datetime)} send_date 
 */
// ── CL-102R-b: per-recipient actions ─────────────────────────────────────────
//
// Destructive actions live inside an overflow menu (⋯), in red, separated by a
// divider from the benign ones. Fitts's law in reverse: a bare ✕ at the end of
// every row — right next to the scrollbar — is a factory for accidental clicks.
// The menu also keeps a 170-row table readable (Hick's law).

// selectCell renders the row's selection checkbox. It reuses data column 0 (the
// result id, previously hidden) so every other column index stays exactly where
// the rest of this file expects it.
function selectCell(rid) {
    var safeRid = escapeHtml(String(rid))
    // The checkbox glyph is ~18px, so the LABEL provides the ≥44×44 hit area
    // (WCAG 2.5.5): 44px box, checkbox centred inside it. Without the wrapper the
    // clickable region would be 18px — the row looked 44px tall but the target was not.
    return '<label style="display:inline-flex;align-items:center;justify-content:center;' +
        'width:44px;height:44px;margin:0;cursor:pointer">' +
        '<input type="checkbox" class="recipient-select" value="' + safeRid + '" ' +
        'aria-label="Seleccionar destinatario" ' +
        'style="width:18px;height:18px;margin:0;cursor:pointer">' +
        '</label>'
}

// rowActionsMenu renders the ⋯ overflow menu for one row.
function rowActionsMenu(rid) {
    var safeAttr = escapeHtml(String(rid))
    var safeJs = String(rid).replace(/'/g, "\\'")
    return '<div class="btn-group">' +
        '<button type="button" class="btn btn-link dropdown-toggle recipient-menu-btn" data-toggle="dropdown" ' +
        'aria-haspopup="true" aria-expanded="false" aria-label="Acciones del destinatario" ' +
        'style="min-width:44px;min-height:44px;color:#6c7a89">' +
        '<i class="fa fa-ellipsis-h"></i></button>' +
        '<ul class="dropdown-menu dropdown-menu-right" role="menu">' +
        '<li role="presentation"><a role="menuitem" href="#" data-rid="' + safeAttr + '" ' +
        'onclick="toggleRecipientTimeline(\'' + safeJs + '\'); return false;">' +
        '<i class="fa fa-clock-o"></i> Ver cronología</a></li>' +
        '<li role="separator" class="divider"></li>' +
        '<li role="presentation"><a role="menuitem" href="#" style="color:#c9302c" ' +
        'onclick="deleteRecipient(\'' + safeJs + '\'); return false;">' +
        '<i class="fa fa-trash-o"></i> Eliminar destinatario</a></li>' +
        '</ul></div>'
}

// toggleRecipientTimeline expands/collapses a row's timeline — the same thing
// clicking the caret does, exposed as the benign menu action.
function toggleRecipientTimeline(rid) {
    var table = $("#resultsTable").DataTable()
    table.rows().every(function (i) {
        var row = this.row(i)
        if (String(row.data()[0]) !== String(rid)) return
        var tr = $(row.node())
        if (row.child.isShown()) {
            row.child.hide()
            tr.find("#caret").removeClass("fa-caret-down").addClass("fa-caret-right")
        } else {
            row.child(renderTimeline(row.data())).show()
            tr.find("#caret").removeClass("fa-caret-right").addClass("fa-caret-down")
        }
    })
}

// deleteRecipient soft-deletes ONE recipient with no confirmation dialog: the
// action is reversible, and confirming reversible actions teaches the user to
// click "Sí" without reading — which then fails to protect the purge, where it
// matters. Undo > Confirm for anything reversible.
function deleteRecipient(rid) {
    api.campaignId.resultDelete(campaign.id, rid, "", "campaign")
        .success(function (resp) {
            recipientDeletedToast("Se eliminó el destinatario. Ya no cuenta en las métricas.", resp.batch_id)
            refreshAfterRecipientChange()
        })
        .error(recipientActionError)
}

// ── Multi-select + bulk delete ────────────────────────────────────────────────

// selectedRecipientIds returns the checked rows across ALL pages of the table.
function selectedRecipientIds() {
    var ids = []
    $("#resultsTable").find("input.recipient-select:checked").each(function () {
        ids.push($(this).val())
    })
    return ids
}

// updateBulkBar shows/hides the bulk action bar and keeps its label truthful.
function updateBulkBar() {
    var ids = selectedRecipientIds()
    var state = TrashHelpers.selectionState(ids)
    $("#bulkSelectedCount").text(state.count === 1 ? "1 destinatario seleccionado" : state.count + " destinatarios seleccionados")
    $("#bulkDeleteBtn").text(state.confirmLabel || "Eliminar")
    $("#recipientBulkBar").toggle(state.showBar)
}

// emailsForSelected maps the selected result ids to their emails, so the
// confirmation can LIST what is going away (recognition over recall).
function emailsForSelected(ids) {
    var byId = {}
    ;(campaign.results || []).forEach(function (r) { byId[String(r.id)] = r.email })
    return ids.map(function (id) { return byId[String(id)] || id })
}

// deleteScopePreview asks the SERVER what a deletion would touch. Both numbers the
// dialog shows come from here.
//
// The previous version derived this client-side from /campaign-groups/summary and
// was broken twice over: that endpoint returns a bare ARRAY (not
// {campaign_groups:[…]}) and carries no campaigns[], so the lookup always yielded
// null and the group radio NEVER rendered — the group scope was unreachable from
// the UI. It also computed "affected" as selected × campaigns-in-group, which is a
// count of campaigns, not of matching rows.
function deleteScopePreview(rids, callback) {
    api.campaignId.resultsDeletePreview(campaign.id, rids, "group")
        .success(function (prev) { callback(prev || {}) })
        .error(function () { callback({ in_group: false, affected: rids.length }) })
}

// confirmBulkDelete opens the bulk confirmation: lists the emails, forces an
// explicit scope choice with its REAL consequence, states that generated reports do
// not change, defaults to Cancel, and labels the confirm button with number+object.
// The body markup comes from TrashHelpers.buildBulkConfirmHtml, a pure function
// whose resulting DOM is asserted in test/js/dialogs_dom.test.js.
function confirmBulkDelete() {
    var ids = selectedRecipientIds()
    if (!ids.length) return
    var emails = emailsForSelected(ids)

    deleteScopePreview(ids, function (preview) {
        Swal.fire({
            title: "Eliminar " + TrashHelpers.pluralizeRecipients(ids.length),
            html: TrashHelpers.buildBulkConfirmHtml(emails, preview),
            type: "warning",
            animation: false,
            showCancelButton: true,
            focusCancel: true, // destructive actions are never the default
            reverseButtons: true,
            allowOutsideClick: false,
            confirmButtonText: "Eliminar " + TrashHelpers.pluralizeRecipients(ids.length),
            confirmButtonColor: "#c9302c",
            cancelButtonText: "Cancelar",
            preConfirm: function () {
                return {
                    scope: $('input[name="bulkScope"]:checked').val() || "campaign",
                    reason: $("#bulkReason").val() || ""
                }
            }
        }).then(function (result) {
            if (!result.value) return
            api.campaignId.resultsBulkDelete(campaign.id, ids, result.value.reason, result.value.scope)
                .success(function (resp) {
                    recipientDeletedToast(
                        "Se eliminaron " + TrashHelpers.pluralizeRecipients(resp.affected || ids.length) +
                        ". Las métricas se actualizaron.", resp.batch_id)
                    refreshAfterRecipientChange()
                })
                .error(recipientActionError)
        })
    })
}

// recipientDeletedToast shows a success toast offering to undo the whole batch.
// The toast never steals focus (that would break keyboard navigation) but the
// Undo button is the next tabbable element, and the container is aria-live so
// screen readers announce the change. With prefers-reduced-motion the timer is
// stretched and the animation dropped.
function recipientDeletedToast(msg, batchID) {
    var reduced = window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches
    Swal.fire({
        toast: true, position: "top-end", type: "success",
        title: msg,
        animation: !reduced,
        html: batchID
            ? '<button id="undoRecipient" class="btn btn-sm btn-default" style="margin-top:6px;min-height:44px">Deshacer</button> ' +
              '<a class="btn btn-sm btn-link" style="min-height:44px" href="/trash?type=recipient">Ver papelera</a>'
            : undefined,
        showConfirmButton: false,
        timer: reduced ? 20000 : 10000,
        timerProgressBar: true,
        onOpen: function (el) {
            el.setAttribute('role', 'status')
            el.setAttribute('aria-live', 'polite')
            var btn = el.querySelector('#undoRecipient')
            if (btn && batchID) {
                btn.onclick = function () {
                    api.recipientTrash.restoreBatch(batchID)
                        .success(function (r) {
                            Swal.fire({
                                toast: true, position: "top-end", type: "success",
                                title: "Se restauraron " + TrashHelpers.pluralizeRecipients(r.restored || 0) + ".",
                                showConfirmButton: false, timer: 4000
                            })
                            refreshAfterRecipientChange()
                        })
                        .error(recipientActionError)
                }
            }
        }
    })
}

// refreshAfterRecipientChange is the SINGLE refresh path after any recipient
// mutation (delete, undo, restore). The toast, the banner, the "(N en papelera)"
// total, the table and the donuts all read the same numbers from different
// places; refreshing them together is what keeps them from disagreeing. A stale
// badge destroys trust in the number just as effectively as a wrong count.
function refreshAfterRecipientChange() {
    poll()                  // table + donuts + timeline (re-reads results)
    loadTrashedRecipients() // banner + inline panel + "(N en papelera)"
    $("#recipientBulkBar").hide()
    $("#selectAllRecipients").prop("checked", false)
}

// wireRecipientSelection binds the select-all checkbox and per-row checkboxes.
// Bound with delegation so it survives DataTables re-draws and paging.
function wireRecipientSelection(table) {
    var $t = $("#resultsTable")
    // Bind once; DataTables re-renders the cells on every draw.
    if (!$t.data("selectionWired")) {
        $t.on("change", "input.recipient-select", function () {
            updateBulkBar()
            // Keep select-all honest: checked only when every visible row is.
            var total = $t.find("input.recipient-select").length
            var checked = $t.find("input.recipient-select:checked").length
            $("#selectAllRecipients").prop("checked", total > 0 && total === checked)
        })
        // A checkbox click must not also toggle the row's timeline.
        $t.on("click", "input.recipient-select", function (e) { e.stopPropagation() })
        $("#selectAllRecipients").on("change", function () {
            var on = $(this).is(":checked")
            $t.find("input.recipient-select").prop("checked", on)
            updateBulkBar()
        })
        $t.data("selectionWired", true)
    }
    updateBulkBar()
}

// clearRecipientSelection drops the selection and hides the bulk bar.
function clearRecipientSelection() {
    $("#resultsTable").find("input.recipient-select").prop("checked", false)
    $("#selectAllRecipients").prop("checked", false)
    updateBulkBar()
}

// ── Banner + inline panel of trashed recipients (addendum §5) ─────────────────

var trashedRecipients = []

// loadTrashedRecipients keeps the persistent indicator honest: while this
// campaign has recipients in the Trash, the user must be able to see that the
// metrics exclude them — otherwise someone reads 167 where they expected 170 two
// weeks later with no way to find out why.
function loadTrashedRecipients() {
    if (!campaign || !campaign.id) return
    api.campaignId.resultsTrashed(campaign.id)
        .success(function (data) {
            trashedRecipients = (data && data.items) || []
            renderTrashedBanner()
        })
        .error(function () { /* the banner is informational; never block the page */ })
}

function renderTrashedBanner() {
    var n = trashedRecipients.length
    var $banner = $("#recipientTrashBanner")
    if (!n) {
        $banner.hide().empty()
        $("#recipientTotalNote").text("")
        return
    }
    // Total shown as "Total: 167 destinatarios (3 en papelera)".
    var active = (campaign.results || []).length
    $("#recipientTotalNote").text("Total: " + TrashHelpers.pluralizeRecipients(active) +
        "  (" + n + " en papelera)")

    var rows = trashedRecipients.map(function (it) {
        var blocked = TrashHelpers.restoreBlockedReason(it)
        var badge = blocked
            ? ' <span class="label label-warning"><i class="fa fa-exclamation-triangle"></i> Campaña en papelera</span>'
            : ''
        var btn = blocked
            ? '<button class="btn btn-xs btn-default" disabled data-toggle="tooltip" title="' + escapeHtml(blocked) + '" ' +
              'style="min-height:44px;min-width:44px">Restaurar</button>'
            : '<button class="btn btn-xs btn-success" style="min-height:44px;min-width:44px" ' +
              'onclick="restoreOneRecipient(' + it.id + ')">Restaurar</button>'
        return '<tr><td>' + escapeHtml(it.email) + badge + '</td>' +
            '<td class="text-muted">' + escapeHtml(it.reason || '—') + '</td>' +
            '<td class="text-muted">' + escapeHtml(it.deleted_by_name || '—') + '</td>' +
            '<td class="text-right">' + btn + '</td></tr>'
    }).join('')

    $banner.html(
        '<div class="alert alert-warning" role="status" aria-live="polite" style="margin-bottom:10px">' +
        '<i class="fa fa-trash-o"></i> <strong>' + TrashHelpers.pluralizeRecipients(n) +
        ' eliminados</strong> no se están contando en estas métricas. ' +
        '<button class="btn btn-xs btn-default" style="min-height:44px" onclick="$(\'#recipientTrashPanel\').toggle()">Ver / ocultar</button> ' +
        '<a class="btn btn-xs btn-link" style="min-height:44px" href="/trash?type=recipient&campaign=' + campaign.id + '">Ver en papelera</a>' +
        '<div id="recipientTrashPanel" style="display:none;margin-top:10px">' +
        '<table class="table table-condensed" style="margin-bottom:6px"><thead><tr>' +
        '<th>Correo</th><th>Motivo</th><th>Eliminado por</th><th class="text-right">Acción</th>' +
        '</tr></thead><tbody>' + rows + '</tbody></table>' +
        '<button class="btn btn-sm btn-success" style="min-height:44px" onclick="restoreAllTrashedRecipients()">' +
        'Restaurar los ' + n + '</button> ' +
        '<span class="text-muted" style="font-size:.9em">Para eliminarlos definitivamente, ve a la Papelera.</span>' +
        '</div></div>'
    ).show()
    $('[data-toggle="tooltip"]').tooltip()
}

// restoreOneRecipient restores a single recipient from the inline panel.
function restoreOneRecipient(resultID) {
    api.recipientTrash.restore(resultID)
        .success(function () {
            Swal.fire({
                toast: true, position: "top-end", type: "success",
                title: "Se restauró el destinatario.", showConfirmButton: false, timer: 4000
            })
            refreshAfterRecipientChange()
        })
        .error(recipientActionError)
}

// restoreAllTrashedRecipients restores every trashed recipient of this campaign,
// skipping the ones blocked by a trashed parent campaign (it reports them).
function restoreAllTrashedRecipients() {
    var restorable = trashedRecipients.filter(function (it) { return !TrashHelpers.restoreBlockedReason(it) })
    var blocked = trashedRecipients.length - restorable.length
    if (!restorable.length) {
        Swal.fire({ type: "info", title: "Nada que restaurar", text: "Todos están bloqueados porque su campaña está en la papelera." })
        return
    }
    var done = 0
    restorable.forEach(function (it) {
        api.recipientTrash.restore(it.id).always(function () {
            done++
            if (done === restorable.length) {
                Swal.fire({
                    toast: true, position: "top-end", type: "success",
                    title: "Se restauraron " + TrashHelpers.pluralizeRecipients(restorable.length) + "." +
                        (blocked ? " " + blocked + " siguen bloqueados." : ""),
                    showConfirmButton: false, timer: 5000
                })
                refreshAfterRecipientChange()
            }
        })
    })
}

function recipientActionError(xhr) {
    var msg = (xhr && xhr.responseJSON && xhr.responseJSON.message) || "No se pudo completar la acción sobre el destinatario"
    Swal.fire({ type: "error", title: "Error", text: msg })
}

function createStatusLabel(status, send_date) {
    var label = statuses[status].label || "label-default";
    var statusColumn = "<span class=\"label " + label + "\">" + status + "</span>"
    // Add the tooltip if the email is scheduled to be sent
    if (status == "Scheduled" || status == "Retrying") {
        var sendDateMessage = "Scheduled to send at " + send_date
        statusColumn = "<span class=\"label " + label + "\" data-toggle=\"tooltip\" data-placement=\"top\" data-html=\"true\" title=\"" + sendDateMessage + "\">" + status + "</span>"
    }
    return statusColumn
}

/* poll - Queries the API and updates the UI with the results
 *
 * Updates:
 * * Timeline Chart
 * * Email (Donut) Chart
 * * Map Bubbles
 * * Datatables
 */
function poll() {
    api.campaignId.results(campaign.id)
        .success(function (c) {
            campaign = c
            /* Update the timeline */
            var timeline_series_data = []
            $.each(campaign.timeline, function (i, event) {
                var event_date = moment.utc(event.time).local()
                timeline_series_data.push({
                    email: event.email,
                    message: event.message,
                    x: event_date.valueOf(),
                    y: 1,
                    marker: {
                        fillColor: statuses[event.message].color
                    }
                })
            })
            var timeline_chart = $("#timeline_chart").highcharts()
            timeline_chart.series[0].update({
                data: timeline_series_data
            })
            /* Update the results donut chart */
            var email_series_data = {}
            // Load the initial data
            Object.keys(statusMapping).forEach(function (k) {
                email_series_data[k] = 0
            });
            $.each(campaign.results, function (i, result) {
                email_series_data[result.status]++;
                if (result.reported) {
                    email_series_data['Email Reported']++
                }
                // Backfill status values
                var step = progressListing.indexOf(result.status)
                for (var i = 0; i < step; i++) {
                    email_series_data[progressListing[i]]++
                }
            })
                        $.each(email_series_data, function (status, count) {
                var email_data = []
                if (!(status in statusMapping)) {
                    return true
                }
                email_data.push({
                    name: status,
                    y: Math.floor((count / campaign.results.length) * 100),
                    count: count
                })
                email_data.push({
                    name: '',
                    y: 100 - Math.floor((count / campaign.results.length) * 100)
                })
                var chart = $("#" + statusMapping[status] + "_chart").highcharts()
                chart.series[0].update({
                    data: email_data
                })
            })

            /* Update the datatable */
            resultsTable = $("#resultsTable").DataTable()
            resultsTable.rows().every(function (i, tableLoop, rowLoop) {
                var row = this.row(i)
                var rowData = row.data()
                var rid = rowData[0]
                $.each(campaign.results, function (j, result) {
                    if (result.id == rid) {
                        rowData[8] = moment(result.send_date).format('MMMM Do YYYY, h:mm:ss a')
                        rowData[7] = result.reported
                        rowData[6] = result.status
                        resultsTable.row(i).data(rowData)
                        if (row.child.isShown()) {
                            $(row.node()).find("#caret").removeClass("fa-caret-right")
                            $(row.node()).find("#caret").addClass("fa-caret-down")
                            row.child(renderTimeline(row.data()))
                        }
                        return false
                    }
                })
            })
            resultsTable.draw(false)
            /* Update the map information */
            updateMap(campaign.results)
            $('[data-toggle="tooltip"]').tooltip()
            $("#refresh_message").hide()
            $("#refresh_btn").show()
        })
}

function load() {
    campaign.id = window.location.pathname.split('/').slice(-1)[0]
    var use_map = JSON.parse(localStorage.getItem('gophish.use_map'))
    api.campaignId.results(campaign.id)
        .success(function (c) {
            campaign = c
            if (campaign) {
                $("title").text(c.name + " - Gophish")
                $("#loading").hide()
                $("#campaignResults").show()
                // Set the title
                $("#page-title").text("Results for " + c.name)
                if (c.status == "Completed") {
                    $('#complete_button')[0].disabled = true;
                    $('#complete_button').text('Completed!');
                    doPoll = false;
                }
                // Setup viewing the details of a result
                $("#resultsTable").on("click", ".timeline-event-details", function () {
                    // Show the parameters
                    payloadResults = $(this).parent().find(".timeline-event-results")
                    if (payloadResults.is(":visible")) {
                        $(this).find("i").removeClass("fa-caret-down")
                        $(this).find("i").addClass("fa-caret-right")
                        payloadResults.hide()
                    } else {
                        $(this).find("i").removeClass("fa-caret-right")
                        $(this).find("i").addClass("fa-caret-down")
                        payloadResults.show()
                    }
                })
                // Setup the results table
                resultsTable = $("#resultsTable").DataTable({
                    destroy: true,
                    "order": [
                        [2, "asc"]
                    ],
                    columnDefs: [{
                            orderable: false,
                            targets: "no-sort"
                        }, {
                            className: "details-control",
                            "targets": [1]
                        }, {
                            // Column 0 used to be a hidden "Result ID"; it now renders
                            // the selection checkbox from that same value, so every
                            // other column index in this file stays put.
                            "orderable": false,
                            "className": "text-center",
                            "render": function (rid, type) {
                                return type === "display" ? selectCell(rid) : rid
                            },
                            "targets": [0]
                        }, {
                            "visible": false,
                            "targets": [8]
                        }, {
                            // Overflow menu (⋯) with the destructive action inside.
                            "orderable": false,
                            "className": "text-right",
                            "render": function (_, type, row) {
                                return type === "display" ? rowActionsMenu(row[0]) : ""
                            },
                            "targets": [9]
                        },
                        {
                            "render": function (data, type, row) {
                                if (type !== "display") {
                                    return data
                                }
                                // row[0]=rid, row[8]=send_date, row[9]=excluded (carried, not columns)
                                return createStatusLabel(data, row[8])
                            },
                            "targets": [6]
                        },
                        {
                            className: "text-center",
                            "render": function (reported, type, row) {
                                if (type == "display") {
                                    if (reported) {
                                        return "<i class='fa fa-check-circle text-center text-success'></i>"
                                    }
                                    return "<i role='button' class='fa fa-times-circle text-center text-muted' onclick='report_mail(\"" + row[0] + "\", \"" + campaign.id + "\");'></i>"
                                }
                                return reported
                            },
                            "targets": [7]
                        }
                    ]
                });
                resultsTable.clear();
                var email_series_data = {}
                var timeline_series_data = []
                Object.keys(statusMapping).forEach(function (k) {
                    email_series_data[k] = 0
                });
                $.each(campaign.results, function (i, result) {
                    resultsTable.row.add([
                        result.id,
                        "<i id=\"caret\" class=\"fa fa-caret-right\"></i>",
                        escapeHtml(result.first_name) || "",
                        escapeHtml(result.last_name) || "",
                        escapeHtml(result.email) || "",
                        escapeHtml(result.position) || "",
                        result.status,
                        result.reported,
                        moment(result.send_date).format('MMMM Do YYYY, h:mm:ss a'),
                        "" // column 9: rendered as the ⋯ actions menu
                    ])
                    email_series_data[result.status]++;
                    if (result.reported) {
                        email_series_data['Email Reported']++
                    }
                    // Backfill status values
                    var step = progressListing.indexOf(result.status)
                    for (var i = 0; i < step; i++) {
                        email_series_data[progressListing[i]]++
                    }
                })
                resultsTable.draw();
                // Setup tooltips
                $('[data-toggle="tooltip"]').tooltip()
                // CL-102R-b: selection wiring + trashed-recipient banner.
                wireRecipientSelection(resultsTable)
                loadTrashedRecipients()
                // Setup the individual timelines
                $('#resultsTable tbody').on('click', 'td.details-control', function () {
                    var tr = $(this).closest('tr');
                    var row = resultsTable.row(tr);
                    if (row.child.isShown()) {
                        // This row is already open - close it
                        row.child.hide();
                        tr.removeClass('shown');
                        $(this).find("i").removeClass("fa-caret-down")
                        $(this).find("i").addClass("fa-caret-right")
                    } else {
                        // Open this row
                        $(this).find("i").removeClass("fa-caret-right")
                        $(this).find("i").addClass("fa-caret-down")
                        row.child(renderTimeline(row.data())).show();
                        tr.addClass('shown');
                    }
                });
                // Setup the graphs
                $.each(campaign.timeline, function (i, event) {
                    if (event.message == "Campaign Created") {
                        return true
                    }
                    var event_date = moment.utc(event.time).local()
                    timeline_series_data.push({
                        email: event.email,
                        message: event.message,
                        x: event_date.valueOf(),
                        y: 1,
                        marker: {
                            fillColor: statuses[event.message].color
                        }
                    })
                })
                renderTimelineChart({
                    data: timeline_series_data
                })
                                $.each(email_series_data, function (status, count) {
                    var email_data = []
                    if (!(status in statusMapping)) {
                        return true
                    }
                    email_data.push({
                        name: status,
                        y: Math.floor((count / campaign.results.length) * 100),
                        count: count
                    })
                    email_data.push({
                        name: '',
                        y: 100 - Math.floor((count / campaign.results.length) * 100)
                    })
                    var chart = renderPieChart({
                        elemId: statusMapping[status] + '_chart',
                        title: status,
                        name: status,
                        data: email_data,
                        colors: [statuses[status].color, '#dddddd']
                    })
                })

                if (use_map) {
                    $("#resultsMapContainer").show()
                    map = new Datamap({
                        element: document.getElementById("resultsMap"),
                        responsive: true,
                        fills: {
                            defaultFill: "#ffffff",
                            point: "#283F50"
                        },
                        geographyConfig: {
                            highlightFillColor: "#1abc9c",
                            borderColor: "#283F50"
                        },
                        bubblesConfig: {
                            borderColor: "#283F50"
                        }
                    });
                }
                updateMap(campaign.results)
            }
        })
        .error(function () {
            $("#loading").hide()
            errorFlash(" Campaign not found!")
        })
}

var setRefresh

function refresh() {
    if (!doPoll) {
        return;
    }
    $("#refresh_message").show()
    $("#refresh_btn").hide()
    poll()
    clearTimeout(setRefresh)
    setRefresh = setTimeout(refresh, 60000)
};

function report_mail(rid, cid) {
    Swal.fire({
        title: "Are you sure?",
        text: "This result will be flagged as reported (RID: " + rid + ")",
        type: "question",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Continue",
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false,
        showLoaderOnConfirm: true
    }).then(function (result) {
        if (result.value){
            api.campaignId.get(cid).success((function(c) {
                report_url = new URL(c.url)
                report_url.pathname = '/report'
                report_url.search = "?rid=" + rid 
                fetch(report_url)
                .then(response => {
                    if (!response.ok) {
                        throw new Error(`HTTP error! Status: ${response.status}`);
                    }
                    refresh();
                })
                .catch(error => {
                    let errorMessage = error.message;
                    if (error.message === "Failed to fetch") {
                        errorMessage = "This might be due to Mixed Content issues or network problems.";
                    }
                    Swal.fire({
                        title: 'Error',
                        text: errorMessage,
                        type: 'error',
                        confirmButtonText: 'Close'
                    });
                });
            }));
        }
    })
}

$(document).ready(function () {
    Highcharts.setOptions({
        global: {
            useUTC: false
        }
    })
    load();

    // Start the polling loop
    setRefresh = setTimeout(refresh, 60000)

    // Load available campaigns when link modal opens
    $('#linkCampaignsModal').on('show.bs.modal', function () {
        loadCampaignsForLinking()
    })

    // Reset picker state on modal close
    $('#linkCampaignsModal').on('hidden.bs.modal', function () {
        linkSelected = []
        renderLinkTags()
        $('#linkCampaignSearch').val('')
        $('#linkCampaignDropdown').empty().hide()
        $('#link-modal-flashes').empty()
    })

    // Picker: filter as-you-type
    $('#linkCampaignSearch').on('input', function () {
        var q = $(this).val().toLowerCase().trim()
        var $d = $('#linkCampaignDropdown')
        if (!q) { $d.empty().hide(); return }

        var allCampaigns = window._linkAllCampaigns || []
        var selectedIds = linkSelected.map(function (c) { return c.id })
        var matches = allCampaigns.filter(function (c) {
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
                            linkSelected.push({id: cam.id, name: cam.name, status: cam.status})
                            renderLinkTags()
                            $('#linkCampaignSearch').val('').focus()
                            $d.empty().hide()
                        }
                    })(c))
                    .appendTo($d)
            })
        }
        $d.show()
    }).on('keydown', function (e) {
        if (e.key === 'Enter') e.preventDefault()
    })

    // Hide dropdown when clicking outside
    $(document).on('click.linkPicker', function (e) {
        if (!$(e.target).closest('#linkCampaignSearch, #linkCampaignDropdown').length) {
            $('#linkCampaignDropdown').hide()
        }
    })
})

// Load campaigns available for linking (excludes current campaign)
function loadCampaignsForLinking() {
    linkSelected = []
    renderLinkTags()
    $('#linkCampaignSearch').val('')
    $('#linkCampaignDropdown').empty().hide()
    $('#linkGroupName').val('Group - ' + (campaign ? campaign.name : ''))

    api.campaigns.summary()
        .success(function (response) {
            // Store all campaigns except the current one for the picker
            window._linkAllCampaigns = (response.campaigns || []).filter(function (c) {
                return c.id !== parseInt(campaign.id)
            })
        })
        .error(function () {
            linkModalFlash('Error loading campaigns', 'danger')
        })
}

function removeLinkCampaign(id) {
    linkSelected = linkSelected.filter(function (x) { return x.id !== id })
    renderLinkTags()
}

function renderLinkTags() {
    var $c = $('#linkCampaignTags')
    $c.empty()
    // Current campaign – locked (non-removable) green tag
    if (campaign && campaign.name) {
        $('<span>')
            .css({
                display: 'inline-flex', alignItems: 'center',
                margin: '2px 4px 2px 0', padding: '4px 10px',
                background: '#5cb85c', color: '#fff',
                borderRadius: '3px', fontSize: '13px'
            })
            .html('<i class="fa fa-lock" style="margin-right:5px;font-size:11px;"></i>' + escapeHtml(campaign.name))
            .appendTo($c)
    }
    // Additional selected campaigns – removable blue tags
    linkSelected.forEach(function (c) {
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
                return function (e) { e.preventDefault(); removeLinkCampaign(cid) }
            })(c.id))
            .appendTo($tag)
        $c.append($tag)
    })
}

// Link campaigns to a new group
function linkCampaignsToGroup() {
    var groupName = $('#linkGroupName').val().trim()
    if (!groupName) { linkModalFlash('Group name is required', 'danger'); return }

    // Always start with the current campaign at index 0
    var campaignList = [{campaign_id: parseInt(campaign.id), order_index: 0}]
    linkSelected.forEach(function (c, i) {
        campaignList.push({campaign_id: c.id, order_index: i + 1})
    })

    api.campaign_groups.post({
        name: groupName,
        campaigns: campaignList,
        archived: false
    })
    .success(function (response) {
        $('#linkCampaignsModal').modal('hide')
        Swal.fire({
            title: 'Campaign Group Created!',
            text: 'Would you like to view the campaign group now?',
            type: 'success',
            showCancelButton: true,
            confirmButtonText: 'View Group',
            cancelButtonText: 'Stay Here'
        }).then(function (result) {
            if (result.value) window.location.href = '/campaign-groups/' + response.id
        })
    })
    .error(function (res) {
        linkModalFlash((res.responseJSON && res.responseJSON.message) || 'Error creating group', 'danger')
    })
}

// Flash message inside link modal
function linkModalFlash(msg, type) {
    $('#link-modal-flashes').html(
        '<div class="alert alert-' + type + '">' +
        '<button type="button" class="close" data-dismiss="alert">&times;</button>' +
        '<i class="fa fa-exclamation-circle"></i> ' + msg + '</div>'
    )
}
