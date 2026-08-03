function errorFlash(message) {
    $("#flashes").empty()
    $("#flashes").append("<div style=\"text-align:center\" class=\"alert alert-danger\">\
        <i class=\"fa fa-exclamation-circle\"></i> " + message + "</div>")
}

function successFlash(message) {
    $("#flashes").empty()
    $("#flashes").append("<div style=\"text-align:center\" class=\"alert alert-success\">\
        <i class=\"fa fa-check-circle\"></i> " + message + "</div>")
}

// Fade message after n seconds
function errorFlashFade(message, fade) {
    $("#flashes").empty()
    $("#flashes").append("<div style=\"text-align:center\" class=\"alert alert-danger\">\
        <i class=\"fa fa-exclamation-circle\"></i> " + message + "</div>")
    setTimeout(function(){ 
        $("#flashes").empty() 
    }, fade * 1000);
}
// Fade message after n seconds
function successFlashFade(message, fade) {  
    $("#flashes").empty()
    $("#flashes").append("<div style=\"text-align:center\" class=\"alert alert-success\">\
        <i class=\"fa fa-check-circle\"></i> " + message + "</div>")
    setTimeout(function(){ 
        $("#flashes").empty() 
    }, fade * 1000);

}

function modalError(message) {
    $("#modal\\.flashes").empty().append("<div style=\"text-align:center\" class=\"alert alert-danger\">\
        <i class=\"fa fa-exclamation-circle\"></i> " + message + "</div>")
}

function query(endpoint, method, data, async) {
    return $.ajax({
        url: "/api" + endpoint,
        async: async,
        method: method,
        data: JSON.stringify(data),
        dataType: "json",
        contentType: "application/json",
        beforeSend: function (xhr) {
            xhr.setRequestHeader('Authorization', 'Bearer ' + user.api_key);
        }
    })
}

function escapeHtml(text) {
    return $("<div/>").text(text).html()
}
window.escapeHtml = escapeHtml

function unescapeHtml(html) {
    return $("<div/>").html(html).text()
}

/**
 * 
 * @param {string} string - The input string to capitalize
 * 
 */
var capitalize = function (string) {
    return string.charAt(0).toUpperCase() + string.slice(1);
}

/*
Define our API Endpoints
*/
var api = {
    // campaigns contains the endpoints for /campaigns
    campaigns: {
        // get() - Queries the API for GET /campaigns
        get: function () {
            return query("/campaigns/", "GET", {}, false)
        },
        // post() - Posts a campaign to POST /campaigns
        post: function (data) {
            return query("/campaigns/", "POST", data, false)
        },
        // summary() - Queries the API for GET /campaigns/summary
        summary: function () {
            return query("/campaigns/summary", "GET", {}, false)
        }
    },
    // campaignId contains the endpoints for /campaigns/:id
    campaignId: {
        // get() - Queries the API for GET /campaigns/:id
        get: function (id) {
            return query("/campaigns/" + id, "GET", {}, true)
        },
        // delete() - Deletes a campaign at DELETE /campaigns/:id (soft delete - moves to trash)
        delete: function (id, data) {
            return query("/campaigns/" + id, "DELETE", data || {}, false)
        },
        // results() - Queries the API for GET /campaigns/:id/results
        results: function (id) {
            return query("/campaigns/" + id + "/results", "GET", {}, true)
        },
        // resultDelete() - Soft-deletes a recipient (moves to Trash, reversible).
        // DELETE /campaigns/:id/results/:rid
        resultDelete: function (id, rid, reason, scope) {
            return query("/campaigns/" + id + "/results/" + encodeURIComponent(rid), "DELETE", { reason: reason || "", scope: scope || "campaign" }, false)
        },
        // resultsBulkDelete() - Soft-deletes several recipients in one batch.
        resultsBulkDelete: function (id, resultIds, reason, scope) {
            return query("/campaigns/" + id + "/results/bulk-delete", "POST", { result_ids: resultIds, reason: reason || "", scope: scope || "campaign" }, false)
        },
        // resultsDeletePreview() - GET what a deletion would touch, without deleting.
        resultsDeletePreview: function (id, rids, scope) {
            return query("/campaigns/" + id + "/results/delete-preview?rids=" +
                encodeURIComponent((rids || []).join(",")) + "&scope=" + encodeURIComponent(scope || "campaign"),
                "GET", null, false)
        },
        // resultsTrashed() - Lists the soft-deleted recipients of a campaign.
        resultsTrashed: function (id) {
            return query("/campaigns/" + id + "/results/trashed", "GET", {}, true)
        },
        // complete() - Completes a campaign at POST /campaigns/:id/complete
        complete: function (id) {
            return query("/campaigns/" + id + "/complete", "GET", {}, true)
        },
        // summary() - Queries the API for GET /campaigns/summary
        summary: function (id) {
            return query("/campaigns/" + id + "/summary", "GET", {}, true)
        },
        // restore() - Restores a campaign from trash at POST /campaigns/:id/restore
        restore: function (id) {
            return query("/campaigns/" + id + "/restore", "POST", {}, false)
        },
        // purge() - Permanently deletes a campaign at DELETE /campaigns/:id/purge
        purge: function (id, confirmData) {
            return query("/campaigns/" + id + "/purge", "DELETE", confirmData || {}, false)
        }
    },
    // campaignsTrash contains endpoints for trash operations
    campaignsTrash: {
        // get() - Queries the API for GET /campaigns/trash
        get: function () {
            return query("/campaigns/trash", "GET", {}, false)
        }
    },
    // campaign_groups contains the endpoints for /campaign-groups
    campaign_groups: {
        // get() - Queries the API for GET /campaign-groups
        get: function (id) {
            return query("/campaign-groups/" + id, "GET", {}, false)
        },
        // post() - Posts a campaign group to POST /campaign-groups
        post: function (data) {
            return query("/campaign-groups/", "POST", data, false)
        },
        // put() - Updates a campaign group at PUT /campaign-groups/:id
        put: function (id, data) {
            return query("/campaign-groups/" + id, "PUT", data, false)
        },
        // delete() - Soft-deletes a campaign group (moves to trash) at DELETE /campaign-groups/:id
        delete: function (id, reason) {
            return query("/campaign-groups/" + id, "DELETE", {reason: reason || ""}, false)
        },
        // summary() - Queries the API for GET /campaign-groups/summary
        summary: function () {
            return query("/campaign-groups/summary", "GET", {}, false)
        },
        // stats() - Queries the API for GET /campaign-groups/:id/stats
        stats: function (id) {
            return query("/campaign-groups/" + id + "/stats", "GET", {}, false)
        },
        // resultsTrashed() - GET /campaign-groups/:id/results/trashed (CL-102R-b §5)
        resultsTrashed: function (id) {
            return query("/campaign-groups/" + id + "/results/trashed", "GET", {}, true)
        },
        // archive() - Archives or unarchives a campaign group at POST /campaign-groups/:id/archive
        archive: function (id, archived) {
            return query("/campaign-groups/" + id + "/archive", "POST", {archived: archived}, false)
        }
    },
    // globalTrash contains endpoints for the unified global trash
    globalTrash: {
        // list() - GET /api/trash?type=...  (type defaults to "all")
        list: function (type) {
            var qs = (type && type !== 'all') ? '?type=' + encodeURIComponent(type) : '';
            return query('/trash' + qs, 'GET', null, false)
        },
        // restore() - POST /api/trash/{type}/{id}/restore
        restore: function (itemType, id) {
            return query('/trash/' + encodeURIComponent(itemType) + '/' + id + '/restore', 'POST', {}, false)
        },
        // purge() - DELETE /api/trash/{type}/{id}/purge
        purge: function (itemType, id, confirmData) {
            return query('/trash/' + encodeURIComponent(itemType) + '/' + id + '/purge', 'DELETE', confirmData || {}, false)
        }
    },
    // recipientTrash — CL-102R recipient soft-delete lifecycle endpoints
    recipientTrash: {
        // list() - GET /api/trash?type=recipient with optional filters
        list: function (f) {
            f = f || {}
            var qs = ['type=recipient']
            if (f.campaign) qs.push('campaign_id=' + encodeURIComponent(f.campaign))
            if (f.group) qs.push('group_id=' + encodeURIComponent(f.group))
            if (f.q) qs.push('q=' + encodeURIComponent(f.q))
            return query('/trash?' + qs.join('&'), 'GET', null, false)
        },
        // batches() - GET /api/trash?type=recipient&group_by=batch (rollup for "All")
        batches: function (f) {
            f = f || {}
            var qs = ['type=recipient', 'group_by=batch']
            if (f.campaign) qs.push('campaign_id=' + encodeURIComponent(f.campaign))
            if (f.group) qs.push('group_id=' + encodeURIComponent(f.group))
            if (f.q) qs.push('q=' + encodeURIComponent(f.q))
            return query('/trash?' + qs.join('&'), 'GET', null, false)
        },
        // counts() - GET /api/trash/counts (unfiltered per-type totals for badges)
        counts: function () {
            return query('/trash/counts', 'GET', null, false)
        },
        // batchDetail() - GET /api/trash/recipient/batch/{batch_id}
        batchDetail: function (batchID) {
            return query('/trash/recipient/batch/' + encodeURIComponent(batchID), 'GET', null, false)
        },
        // restoreBatch() - the toast "Deshacer": restores a whole delete batch
        restoreBatch: function (batchID) {
            return query('/trash/recipient/restore-batch', 'POST', { batch_id: batchID }, false)
        },
        restore: function (id) {
            return query('/trash/recipient/' + id + '/restore', 'POST', {}, false)
        },
        purge: function (id, confirmEmail) {
            return query('/trash/recipient/' + id + '/purge', 'POST', { confirm: confirmEmail }, false)
        },
        purgeBatch: function (batchID) {
            return query('/trash/recipient/purge-batch', 'POST', { batch_id: batchID, confirm: 'ELIMINAR' }, false)
        }
    },
    // groups contains the endpoints for /groups
    groups: {
        // get() - Queries the API for GET /groups
        get: function () {
            return query("/groups/", "GET", {}, false)
        },
        // post() - Posts a group to POST /groups
        post: function (group) {
            return query("/groups/", "POST", group, false)
        },
        // summary() - Queries the API for GET /groups/summary
        summary: function () {
            return query("/groups/summary", "GET", {}, true)
        }
    },
    // groupId contains the endpoints for /groups/:id
    groupId: {
        // get() - Queries the API for GET /groups/:id
        get: function (id) {
            return query("/groups/" + id, "GET", {}, false)
        },
        // put() - Puts a group to PUT /groups/:id
        put: function (group) {
            return query("/groups/" + group.id, "PUT", group, false)
        },
        // delete() - Deletes a group at DELETE /groups/:id
        delete: function (id) {
            return query("/groups/" + id, "DELETE", {}, false)
        }
    },
    // templates contains the endpoints for /templates
    templates: {
        // get() - Queries the API for GET /templates
        get: function () {
            return query("/templates/", "GET", {}, false)
        },
        // post() - Posts a template to POST /templates
        post: function (template) {
            return query("/templates/", "POST", template, false)
        }
    },
    // templateId contains the endpoints for /templates/:id
    templateId: {
        // get() - Queries the API for GET /templates/:id
        get: function (id) {
            return query("/templates/" + id, "GET", {}, false)
        },
        // put() - Puts a template to PUT /templates/:id
        put: function (template) {
            return query("/templates/" + template.id, "PUT", template, false)
        },
        // delete() - Deletes a template at DELETE /templates/:id
        delete: function (id) {
            return query("/templates/" + id, "DELETE", {}, false)
        }
    },
    // pages contains the endpoints for /pages
    pages: {
        // get() - Queries the API for GET /pages
        get: function () {
            return query("/pages/", "GET", {}, false)
        },
        // post() - Posts a page to POST /pages
        post: function (page) {
            return query("/pages/", "POST", page, false)
        }
    },
    // pageId contains the endpoints for /pages/:id
    pageId: {
        // get() - Queries the API for GET /pages/:id
        get: function (id) {
            return query("/pages/" + id, "GET", {}, false)
        },
        // put() - Puts a page to PUT /pages/:id
        put: function (page) {
            return query("/pages/" + page.id, "PUT", page, false)
        },
        // delete() - Deletes a page at DELETE /pages/:id
        delete: function (id) {
            return query("/pages/" + id, "DELETE", {}, false)
        }
    },
    // SMTP contains the endpoints for /smtp
    SMTP: {
        // get() - Queries the API for GET /smtp
        get: function () {
            return query("/smtp/", "GET", {}, false)
        },
        // post() - Posts a SMTP to POST /smtp
        post: function (smtp) {
            return query("/smtp/", "POST", smtp, false)
        }
    },
    // SMTPId contains the endpoints for /smtp/:id
    SMTPId: {
        // get() - Queries the API for GET /smtp/:id
        get: function (id) {
            return query("/smtp/" + id, "GET", {}, false)
        },
        // put() - Puts a SMTP to PUT /smtp/:id
        put: function (smtp) {
            return query("/smtp/" + smtp.id, "PUT", smtp, false)
        },
        // delete() - Deletes a SMTP at DELETE /smtp/:id
        delete: function (id) {
            return query("/smtp/" + id, "DELETE", {}, false)
        }
    },
    // IMAP containts the endpoints for /imap/
    IMAP: {
        get: function() {
            return query("/imap/", "GET", {}, !1)
        },
        post: function(e) {
            return query("/imap/", "POST", e, !1)
        },
        validate: function(e) {
            return query("/imap/validate", "POST", e, true)
        }
    },
    // users contains the endpoints for /users
    users: {
        // get() - Queries the API for GET /users
        get: function () {
            return query("/users/", "GET", {}, true)
        },
        // post() - Posts a user to POST /users
        post: function (user) {
            return query("/users/", "POST", user, true)
        }
    },
    // userId contains the endpoints for /users/:id
    userId: {
        // get() - Queries the API for GET /users/:id
        get: function (id) {
            return query("/users/" + id, "GET", {}, true)
        },
        // put() - Puts a user to PUT /users/:id
        put: function (user) {
            return query("/users/" + user.id, "PUT", user, true)
        },
        // delete() - Deletes a user at DELETE /users/:id
        delete: function (id) {
            return query("/users/" + id, "DELETE", {}, true)
        }
    },
    webhooks: {
        get: function() {
            return query("/webhooks/", "GET", {}, false)
        },
        post: function(webhook) {
            return query("/webhooks/", "POST", webhook, false)
        },
    },
    webhookId: {
        get: function(id) {
            return query("/webhooks/" + id, "GET", {}, false)
        },
        put: function(webhook) {
            return query("/webhooks/" + webhook.id, "PUT", webhook, true)
        },
        delete: function(id) {
            return query("/webhooks/" + id, "DELETE", {}, false)
        },
        ping: function(id) {
            return query("/webhooks/" + id + "/validate", "POST", {}, true)
        },
    },
    // import handles all of the "import" functions in the api
    import_email: function (req) {
        return query("/import/email", "POST", req, false)
    },
    // clone_site handles importing a site by url
    clone_site: function (req) {
        return query("/import/site", "POST", req, false)
    },
    // send_test_email sends an email to the specified email address
    send_test_email: function (req) {
        return query("/util/send_test_email", "POST", req, true)
    },
    reset: function () {
        return query("/reset", "POST", {}, true)
    }
}
window.api = api

// Register our moment.js datatables listeners
$(document).ready(function () {
    // Setup nav highlighting
    var path = location.pathname;
    $('.nav-sidebar li').each(function () {
        var $this = $(this);
        // if the current path is like this link, make it active
        if ($this.find("a").attr('href') === path) {
            $this.addClass('active');
        }
    })
    $.fn.dataTable.moment('MMMM Do YYYY, h:mm:ss a');
    // Setup tooltips
    $('[data-toggle="tooltip"]').tooltip()
});
