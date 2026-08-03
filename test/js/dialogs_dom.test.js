// DOM tests for the confirmation dialogs (CL-102R-b, P0 fix).
//
// Why this file exists: the bulk-delete dialog shipped with the group-scope radio
// MISSING — the data feeding it came back null and nothing asserted the rendered
// DOM. Unit tests on the logic passed while the screen was broken. These tests
// parse the ACTUAL markup with a spec-compliant HTML parser (jsdom, the same
// parser jsdom/browsers use) and assert what the user would see.
//
//     node test/js/dialogs_dom.test.js
'use strict';

const assert = require('assert');
const { JSDOM } = require('jsdom');
const H = require('../../static/js/src/app/trash_helpers.js');

let passed = 0;
function test(name, fn) {
    try { fn(); passed++; console.log('  PASS ' + name); }
    catch (e) { console.error('  FAIL ' + name + '\n    ' + e.message); process.exitCode = 1; }
}
function dom(html) {
    return new JSDOM('<!doctype html><body><div id="swal-content">' + html + '</div></body>').window.document;
}

// The real case that broke: campaign 1513 belongs to group 4 «asdasd» (4 campaigns),
// and the selected email exists in 2 of them → affected must be 2, NOT 4.
const PREVIEW_GROUP = {
    scope: 'group', in_group: true, group_id: 4, group_name: 'asdasd',
    campaign_count: 4, affected: 2
};
const PREVIEW_NO_GROUP = { scope: 'campaign', in_group: false, campaign_count: 0, affected: 1 };
const EMAILS = ['dopara@live.com'];

test('TestBulkConfirmHasTwoScopeRadios', () => {
    const d = dom(H.buildBulkConfirmHtml(EMAILS, PREVIEW_GROUP));
    const radios = d.querySelectorAll('input[type="radio"][name="bulkScope"]');
    assert.strictEqual(radios.length, 2, 'deben existir DOS radios de alcance, había ' + radios.length);
    const values = Array.from(radios).map(r => r.getAttribute('value')).sort();
    assert.deepStrictEqual(values, ['campaign', 'group']);
    // Each radio must live inside its own label (so the whole row is clickable).
    radios.forEach(r => {
        assert.strictEqual(r.closest('label') !== null, true, 'cada radio dentro de su <label>');
    });
    // "Solo en esta campaña" is preselected; the destructive wider scope is not.
    const byValue = v => d.querySelector('input[name="bulkScope"][value="' + v + '"]');
    assert.strictEqual(byValue('campaign').hasAttribute('checked'), true);
    assert.strictEqual(byValue('group').hasAttribute('checked'), false);
});

test('TestBulkConfirmReasonInputStartsEmpty', () => {
    const d = dom(H.buildBulkConfirmHtml(EMAILS, PREVIEW_GROUP));
    const reason = d.querySelector('#bulkReason');
    assert.ok(reason, '#bulkReason debe existir');
    assert.strictEqual(reason.value, '', 'el motivo debe empezar VACÍO');
    assert.strictEqual(reason.getAttribute('placeholder'), 'Correos internos de validación');
    // The scope text must NOT have leaked into the reason field (the reported bug).
    assert.ok(!/En todo el grupo/.test(reason.value), 'el texto de alcance no puede estar en el motivo');
    assert.ok(!/En todo el grupo/.test(reason.getAttribute('placeholder')),
        'el texto de alcance no puede ser el placeholder del motivo');
    // And the reason input must be OUTSIDE the scope block.
    assert.strictEqual(reason.closest('.bulk-scope'), null, 'el motivo no puede estar dentro del bloque Alcance');
});

test('TestBulkConfirmShowsGroupCampaignCount', () => {
    const d = dom(H.buildBulkConfirmHtml(EMAILS, PREVIEW_GROUP));
    const label = d.querySelector('input[name="bulkScope"][value="group"]').closest('label').textContent;
    assert.ok(/«asdasd»/.test(label), 'debe nombrar el grupo: ' + label);
    assert.ok(/\(4 campañas\)/.test(label), 'debe decir (4 campañas): ' + label);
});

test('TestBulkConfirmShowsRealAffectedCount', () => {
    const d = dom(H.buildBulkConfirmHtml(EMAILS, PREVIEW_GROUP));
    const affected = d.querySelector('.bulk-scope-affected').textContent;
    assert.ok(/Se eliminarán 2 registros/.test(affected),
        'debe decir 2 registros (coincidencias reales), decía: ' + affected);
    assert.ok(!/Se eliminarán 4 registros/.test(affected),
        'NO debe decir 4: ese es el número de campañas, no de coincidencias');
    // Singular: the VERB must agree too ("se eliminará", not "se eliminarán").
    const one = dom(H.buildBulkConfirmHtml(EMAILS, Object.assign({}, PREVIEW_GROUP, { affected: 1 })));
    const oneTxt = one.querySelector('.bulk-scope-affected').textContent;
    assert.strictEqual(oneTxt, 'Se eliminará 1 registro en total.',
        'concordancia de verbo en singular, decía: ' + oneTxt);
    assert.ok(!/eliminarán 1/.test(oneTxt), 'nunca "se eliminarán 1"');
});

test('TestBulkConfirmHidesGroupRadioWhenNoGroup', () => {
    const d = dom(H.buildBulkConfirmHtml(EMAILS, PREVIEW_NO_GROUP));
    assert.strictEqual(d.querySelectorAll('input[name="bulkScope"]').length, 1,
        'sin grupo solo debe haber un radio');
    assert.strictEqual(d.querySelector('.bulk-scope-affected'), null);
    assert.ok(!/En todo el grupo/.test(d.body.textContent), 'no debe aparecer texto de grupo');
    // The reason field still exists and is still empty.
    assert.strictEqual(d.querySelector('#bulkReason').value, '');
});

test('TestBulkConfirmListsEmailsAndOverflowInDom', () => {
    const many = Array.from({ length: 13 }, (_, i) => 'u' + i + '@x.com');
    const d = dom(H.buildBulkConfirmHtml(many, PREVIEW_NO_GROUP));
    const items = d.querySelectorAll('.bulk-confirm-list li');
    assert.strictEqual(items.length, 11, '10 correos + 1 línea de overflow');
    assert.strictEqual(items[10].textContent, 'y 3 más');
});

test('TestPurgeDialogMarkupIsWellFormed', () => {
    const d = dom(H.buildPurgeConfirmHtml('destinatario', 'qa@empresa.com'));
    assert.ok(/no se puede deshacer/.test(d.body.textContent));
    const exp = d.querySelector('.purge-expected');
    assert.ok(exp, 'debe mostrar el valor exacto a escribir');
    assert.strictEqual(exp.textContent, 'qa@empresa.com');
    // No stray inputs: the typed confirmation is SweetAlert's own input, so this
    // body must not introduce one that could swallow the text.
    assert.strictEqual(d.querySelectorAll('input').length, 0,
        'el cuerpo del purge no debe traer inputs propios');
});

test('TestDialogsEscapeHtmlInUserData', () => {
    const d = dom(H.buildBulkConfirmHtml(['<script>x</script>@x.com'],
        Object.assign({}, PREVIEW_GROUP, { group_name: '<img src=x>' })));
    assert.strictEqual(d.querySelectorAll('script').length, 0, 'no debe inyectar <script>');
    assert.strictEqual(d.querySelectorAll('img').length, 0, 'no debe inyectar <img>');
});

console.log('\n' + passed + ' passed' + (process.exitCode ? ' (with failures)' : ''));
