// Pure rendering + state logic for the report's image slots (CL-104, CL-109).
//
// Why this file exists: the slot card used to be built inline in reports.js and its
// status badge was only produced on a FULL editor render. `uploadAsset` refreshed
// the thumbnail but not the badge, so an uploaded image showed its preview while
// still reading "obligatoria — falta" — and with several uploads in one session it
// looked as if every image were missing. Making the card a pure function lets the
// upload/delete paths re-render exactly one card, and lets the DOM be asserted in
// test/js/report_slots_dom.test.js.
//
// UMD: window.ReportSlotsUI in the browser, CommonJS under Node.
(function (root, factory) {
    if (typeof module === 'object' && module.exports) {
        module.exports = factory();
    } else {
        root.ReportSlotsUI = factory();
    }
})(typeof self !== 'undefined' ? self : this, function () {
    'use strict';

    // acceptAttr lists the formats explicitly instead of "image/*" so the file picker
    // shows what actually works. SVG is in the list because the editor converts it in
    // the browser before uploading; the server still refuses raw SVG.
    function acceptAttr() {
        if (typeof ImagePrep !== 'undefined' && ImagePrep.ACCEPT) return ImagePrep.ACCEPT;
        if (typeof require === 'function') {
            try { return require('./image_prep.js').ACCEPT; } catch (e) { /* browser */ }
        }
        return '.png,.jpg,.jpeg,.gif,.bmp,.tif,.tiff,.webp,.svg';
    }

    function esc(t) {
        return String(t == null ? '' : t).replace(/[&<>"']/g, function (c) {
            return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
        });
    }

    // slotIsLoaded is the ONE definition of "this slot has an image", used by the
    // badge, the summary and the pre-flight so they can never disagree.
    function slotIsLoaded(slot, assets) {
        if (!slot || !assets) return false;
        return !!assets[slot.key];
    }

    // slotApplies filters out what must not be demanded: auto-generated slots, and
    // figura_8 when there were no 2FA accounts.
    function slotApplies(slot, opts) {
        opts = opts || {};
        if (!slot || slot.source === 'auto') return false;
        if (slot.key === 'figura_8' && !opts.had2fa) return false;
        return true;
    }

    // missingRequiredSlots returns ONLY the required slots that are genuinely absent.
    // Per-slot by construction: a missing image never marks its neighbours.
    function missingRequiredSlots(slots, assets, opts) {
        return (slots || []).filter(function (s) {
            return slotApplies(s, opts) && s.required && !slotIsLoaded(s, assets);
        });
    }

    // badgeHtml renders the slot's state. The state is never conveyed by colour
    // alone: each badge carries its own text (WCAG 1.4.1).
    function badgeHtml(slot, loaded) {
        if (loaded) return '<span class="label label-success slot-badge">cargada</span>';
        if (slot.required) return '<span class="label label-danger slot-badge">obligatoria — falta</span>';
        return '<span class="label label-default slot-badge">opcional</span>';
    }

    // deleteButtonHtml (CL-109) appears ONLY when there is an image to remove. It is
    // a labelled button, not a bare icon, and meets the 44px target. Removing a
    // required image is undoable in practice (re-upload), but the click itself is
    // confirmed for required slots by the caller.
    function deleteButtonHtml(slot, loaded) {
        if (!loaded) return '';
        var k = esc(slot.key);
        return '<button type="button" class="btn btn-default btn-sm slot-delete" ' +
            'data-slot="' + k + '" data-required="' + (slot.required ? '1' : '0') + '" ' +
            'style="min-height:44px;min-width:44px;margin-top:6px" ' +
            'onclick="deleteAsset(\'' + String(slot.key).replace(/'/g, "\\'") + '\')">' +
            '<i class="fa fa-trash-o"></i> Eliminar</button>';
    }

    // slotCardHtml renders one slot card whole. Callers re-render the WHOLE card
    // after an upload or a delete, so badge, thumbnail and buttons can never drift
    // apart — the bug this replaces was exactly a partial update.
    function slotCardHtml(slot, opts) {
        opts = opts || {};
        if (slot.source === 'auto') {
            return '<div class="col-sm-4" id="slot-col-' + esc(slot.key) + '" style="margin-bottom:12px">' +
                '<div class="thumbnail" style="padding:8px">' +
                '<strong>' + esc(slot.title) + '</strong>' +
                '<p class="text-muted" style="margin:6px 0"><i class="fa fa-magic"></i> ' +
                'Automático (se genera del gráfico)</p></div></div>';
        }
        var loaded = !!opts.loaded;
        var thumb = loaded
            ? '<img src="' + esc(opts.assetUrl || '') + '" alt="Vista previa de ' + esc(slot.title) +
              '" style="max-height:90px;max-width:100%;display:block;margin:6px auto">'
            : '<div style="height:90px;background:#f5f5f5;border:1px dashed #ccc;text-align:center;' +
              'line-height:90px;color:#aaa">sin imagen</div>';
        return '<div class="col-sm-4" id="slot-col-' + esc(slot.key) + '" style="margin-bottom:12px">' +
            '<div class="thumbnail" style="padding:8px">' +
            '<strong>' + esc(slot.title) + '</strong>' +
            (slot.required ? ' <span class="text-danger" title="Obligatoria">*</span>' : '') + ' ' +
            badgeHtml(slot, loaded) +
            '<p class="text-muted" style="font-size:.85em;margin:4px 0">' + esc(slot.evidence) + '</p>' +
            '<div id="thumb-' + esc(slot.key) + '">' + thumb + '</div>' +
            '<input type="file" accept="' + acceptAttr() + '" style="margin-top:6px" ' +
            'aria-label="Subir imagen para ' + esc(slot.title) + '" ' +
            'onchange="uploadAsset(\'' + String(slot.key).replace(/'/g, "\\'") + '\', this)">' +
            deleteButtonHtml(slot, loaded) +
            '</div></div>';
    }

    // statusSummary describes how many required images are in place. It reports only
    // the genuinely absent ones.
    function statusSummary(slots, assets, opts) {
        var required = (slots || []).filter(function (s) {
            return slotApplies(s, opts) && s.required;
        });
        var missing = missingRequiredSlots(slots, assets, opts);
        return {
            total: required.length,
            loaded: required.length - missing.length,
            missing: missing.map(function (s) { return s.key; }),
            ok: missing.length === 0
        };
    }

    return {
        acceptAttr: acceptAttr,
        esc: esc,
        slotIsLoaded: slotIsLoaded,
        slotApplies: slotApplies,
        missingRequiredSlots: missingRequiredSlots,
        badgeHtml: badgeHtml,
        deleteButtonHtml: deleteButtonHtml,
        slotCardHtml: slotCardHtml,
        statusSummary: statusSummary
    };
});
