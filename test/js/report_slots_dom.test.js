// DOM tests for the report image slots (CL-104 reopened, CL-109).
//
// The bug these lock down: an uploaded image showed its thumbnail while its badge
// still read "obligatoria — falta", because the upload path refreshed only part of
// the card. With several uploads in one session it looked as if every image were
// missing — the fifth "green in tests, broken on screen" of this project.
//
//     node test/js/report_slots_dom.test.js
'use strict';

const assert = require('assert');
const { JSDOM } = require('jsdom');
const UI = require('../../static/js/src/app/report_slots_ui.js');

let passed = 0;
function test(name, fn) {
    try { fn(); passed++; console.log('  PASS ' + name); }
    catch (e) { console.error('  FAIL ' + name + '\n    ' + e.message); process.exitCode = 1; }
}
const doc = html => new JSDOM('<!doctype html><body><div id="grid">' + html + '</div></body>').window.document;

const SLOTS = [
    { key: 'figura_1', title: 'Figura 1: Correos', evidence: 'e1', required: true, source: 'manual' },
    { key: 'figura_2', title: 'Figura 2: Plantilla', evidence: 'e2', required: true, source: 'manual' },
    { key: 'figura_3', title: 'Figura 3: Remitente', evidence: 'e3', required: true, source: 'manual' },
    { key: 'grafico_1', title: 'Gráfico 1', evidence: 'auto', required: true, source: 'auto' },
    { key: 'figura_8', title: 'Figura 8: 2FA', evidence: 'e8', required: true, source: 'manual' }
];
const render = (assets, opts) => doc(SLOTS.map(s =>
    UI.slotCardHtml(s, { loaded: UI.slotIsLoaded(s, assets), assetUrl: '/a/' + s.key })).join(''));

// ── CL-104 ────────────────────────────────────────────────────────────────────

test('TestSlotWithUploadedImageNeverShowsMissingBadge', () => {
    const d = render({ figura_1: 'image/png', figura_2: 'image/jpeg' });
    ['figura_1', 'figura_2'].forEach(k => {
        const card = d.querySelector('#slot-col-' + k);
        const badge = card.querySelector('.slot-badge').textContent;
        const hasImg = !!card.querySelector('#thumb-' + k + ' img');
        assert.strictEqual(hasImg, true, k + ' debe mostrar miniatura');
        assert.strictEqual(badge, 'cargada', k + ' con imagen NUNCA puede decir "' + badge + '"');
        assert.ok(!/falta/.test(badge), k + ': miniatura y "falta" no pueden coexistir');
    });
});

test('TestOnlyAbsentSlotsAreFlagged', () => {
    // 2 de 4 obligatorias faltan (figura_8 no aplica sin 2FA).
    const assets = { figura_1: 'image/png' };
    const d = render(assets);
    assert.strictEqual(d.querySelector('#slot-col-figura_1 .slot-badge').textContent, 'cargada');
    ['figura_2', 'figura_3'].forEach(k => {
        assert.strictEqual(d.querySelector('#slot-col-' + k + ' .slot-badge').textContent, 'obligatoria — falta');
    });
    const missing = UI.missingRequiredSlots(SLOTS, assets, { had2fa: false }).map(s => s.key);
    assert.deepStrictEqual(missing, ['figura_2', 'figura_3'],
        'solo las ausentes; ni la cargada ni la automática ni figura_8 sin 2FA');
    // Con 2FA, figura_8 sí entra.
    assert.deepStrictEqual(
        UI.missingRequiredSlots(SLOTS, assets, { had2fa: true }).map(s => s.key),
        ['figura_2', 'figura_3', 'figura_8']);
});

test('TestPreflightSummaryListsOnlyReallyMissing', () => {
    const st = UI.statusSummary(SLOTS, { figura_1: 'i', figura_2: 'i' }, { had2fa: false });
    assert.strictEqual(st.total, 3, 'obligatorias aplicables: figura_1..3 (grafico_1 es auto, figura_8 sin 2FA)');
    assert.strictEqual(st.loaded, 2);
    assert.deepStrictEqual(st.missing, ['figura_3']);
    assert.strictEqual(st.ok, false);
    const all = UI.statusSummary(SLOTS, { figura_1: 'i', figura_2: 'i', figura_3: 'i' }, { had2fa: false });
    assert.strictEqual(all.ok, true);
    assert.deepStrictEqual(all.missing, []);
});

test('TestAutoSlotIsNeverRequestedNorFlagged', () => {
    const d = render({});
    const card = d.querySelector('#slot-col-grafico_1');
    assert.ok(card, 'la automática se muestra');
    assert.strictEqual(card.querySelector('.slot-badge'), null, 'sin badge de estado');
    assert.strictEqual(card.querySelector('input[type="file"]'), null, 'sin subida');
    assert.strictEqual(card.querySelector('.slot-delete'), null, 'sin borrado');
});

test('TestBadgeNeverConveysStateByColourAlone', () => {
    const d = render({ figura_1: 'i' });
    ['figura_1', 'figura_2'].forEach(k => {
        const t = d.querySelector('#slot-col-' + k + ' .slot-badge').textContent.trim();
        assert.ok(t.length > 0, k + ': el badge debe llevar TEXTO, no solo color');
    });
    // La opcional también se nombra.
    const opt = doc(UI.slotCardHtml({ key: 'x', title: 'X', evidence: 'e', required: false, source: 'manual' }, {}));
    assert.strictEqual(opt.querySelector('.slot-badge').textContent, 'opcional');
});

// ── CL-109 ────────────────────────────────────────────────────────────────────

test('TestDeleteButtonOnlyWhenImageLoaded', () => {
    const d = render({ figura_1: 'image/png' });
    assert.ok(d.querySelector('#slot-col-figura_1 .slot-delete'), 'con imagen debe existir el botón');
    assert.strictEqual(d.querySelector('#slot-col-figura_2 .slot-delete'), null,
        'sin imagen NO debe aparecer el botón');
});

test('TestDeleteButtonHasTextAndTouchTarget', () => {
    const d = render({ figura_1: 'image/png' });
    const btn = d.querySelector('#slot-col-figura_1 .slot-delete');
    assert.ok(/Eliminar/.test(btn.textContent), 'debe decir "Eliminar", no ser un icono suelto');
    const style = btn.getAttribute('style') || '';
    assert.ok(/min-height:44px/.test(style) && /min-width:44px/.test(style),
        'objetivo táctil ≥44px, era: ' + style);
    assert.strictEqual(btn.getAttribute('type'), 'button', 'no debe enviar formularios');
    assert.strictEqual(btn.getAttribute('data-required'), '1',
        'expone si es obligatoria, para pedir confirmación');
});

test('TestSlotReturnsToMissingAfterDelete', () => {
    // Estado tras borrar: la tarjeta se re-renderiza sin la imagen.
    const after = doc(UI.slotCardHtml(SLOTS[0], { loaded: false, assetUrl: '/a/figura_1' }));
    assert.strictEqual(after.querySelector('.slot-badge').textContent, 'obligatoria — falta');
    assert.strictEqual(after.querySelector('#thumb-figura_1 img'), null);
    assert.strictEqual(after.querySelector('.slot-delete'), null, 'sin imagen, sin botón de borrar');
    assert.deepStrictEqual(
        UI.missingRequiredSlots(SLOTS, {}, { had2fa: false }).map(s => s.key),
        ['figura_1', 'figura_2', 'figura_3']);
});

test('TestSlotCardEscapesUserData', () => {
    const d = doc(UI.slotCardHtml({ key: 'figura_1', title: '<img src=x onerror=1>', evidence: '<b>e</b>', required: true, source: 'manual' },
        { loaded: true, assetUrl: '"><script>x</script>' }));
    assert.strictEqual(d.querySelectorAll('script').length, 0);
    assert.strictEqual(d.querySelectorAll('img[onerror]').length, 0);
});

console.log('\n' + passed + ' passed' + (process.exitCode ? ' (with failures)' : ''));
