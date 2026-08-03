// Unit tests for the pure Trash UI helpers (CL-102R-b).
//
// Plain Node + assert: this repo has no JS test framework and adding one would be
// a toolchain change nobody asked for. Run with:
//     node test/js/trash_helpers.test.js
'use strict';

const assert = require('assert');
const H = require('../../static/js/src/app/trash_helpers.js');

let passed = 0;
function test(name, fn) {
    try {
        fn();
        passed++;
        console.log('  PASS ' + name);
    } catch (e) {
        console.error('  FAIL ' + name + '\n    ' + e.message);
        process.exitCode = 1;
    }
}

const NOW = Date.parse('2026-07-29T22:00:00Z');
const fiveMinAgo = '2026-07-29T21:55:00Z';

// 1) The big number is what reconciles the two badges (All=batches vs
//    Destinatarios=rows), so it must lead the rollup label.
test('TestRollupLabelShowsBigNumber', () => {
    const label = H.rollupLabel({
        count: 38, campaign_count: 1, campaign_name: 'Simulación Q3',
        scope: 'campaign', deleted_at: fiveMinAgo
    }, NOW);
    assert.strictEqual(label, '38 destinatarios · Campaña «Simulación Q3» · hace 5 min');
    assert.ok(label.indexOf('38 destinatarios') === 0, 'the count must lead the label');
});

// 2) Singular vs plural, both for recipients and campaigns.
test('TestRollupLabelSingularPlural', () => {
    assert.strictEqual(H.pluralizeRecipients(1), '1 destinatario');
    assert.strictEqual(H.pluralizeRecipients(2), '2 destinatarios');
    assert.strictEqual(H.pluralizeCampaigns(1), '1 campaña');
    assert.strictEqual(H.pluralizeCampaigns(4), '4 campañas');
    const one = H.rollupLabel({ count: 1, campaign_count: 1, campaign_name: 'X', deleted_at: fiveMinAgo }, NOW);
    assert.ok(one.startsWith('1 destinatario ·'), one);
});

// 3) A group-scoped batch of ONE email across N campaigns names the email and the
//    group — naming one of the four campaigns would mislead.
test('TestRollupLabelGroupScope', () => {
    const label = H.rollupLabel({
        count: 4, campaign_count: 4, scope: 'group', group_name: 'Simulación Q3',
        sample_emails: ['qa@empresa.com'], deleted_at: fiveMinAgo
    }, NOW);
    assert.strictEqual(label, 'qa@empresa.com · 4 campañas del grupo «Simulación Q3» · hace 5 min');
    // A multi-email group batch still leads with the count.
    const many = H.rollupLabel({
        count: 8, campaign_count: 4, scope: 'group', group_name: 'Simulación Q3',
        sample_emails: ['a@x.com', 'b@x.com'], deleted_at: fiveMinAgo
    }, NOW);
    assert.ok(many.startsWith('8 destinatarios ·'), many);
});

// 4) The tooltip that explains why All(7) and Destinatarios(41) differ.
test('TestCountsReconcileAllVsRecipients', () => {
    assert.strictEqual(H.countsReconcile({ recipients: 0, recipient_batches: 0 }),
        'Sin destinatarios en la papelera.');
    assert.strictEqual(H.countsReconcile({ recipients: 41, recipient_batches: 3 }),
        '41 destinatarios en 3 eliminaciones.');
    assert.strictEqual(H.countsReconcile({ recipients: 5, recipient_batches: 1 }),
        '5 destinatarios en 1 eliminación.');
    // Plural of BOTH nouns, including the user's fixture (6 rows / 2 batches).
    assert.strictEqual(H.countsReconcile({ recipients: 6, recipient_batches: 2 }),
        '6 destinatarios en 2 eliminaciones.');
    assert.strictEqual(H.countsReconcile({ recipients: 2, recipient_batches: 1 }),
        '2 destinatarios en 1 eliminación.');
    // One deletion per recipient: the noun must still appear ("3 en la papelera."
    // read like a bug) and the singular must be singular.
    assert.strictEqual(H.countsReconcile({ recipients: 3, recipient_batches: 3 }),
        '3 destinatarios en la papelera.');
    assert.strictEqual(H.countsReconcile({ recipients: 1, recipient_batches: 1 }),
        '1 destinatario en la papelera.');
});

// 5) Selection drives the bulk bar, and the confirm button repeats number+object.
test('TestSelectionStateEnablesBulkBar', () => {
    assert.deepStrictEqual(H.selectionState([]), { count: 0, showBar: false, confirmLabel: '' });
    const one = H.selectionState(['a']);
    assert.strictEqual(one.showBar, true);
    assert.strictEqual(one.confirmLabel, 'Eliminar 1 destinatario');
    const three = H.selectionState(['a', 'b', 'c']);
    assert.strictEqual(three.confirmLabel, 'Eliminar 3 destinatarios');
    assert.ok(three.confirmLabel.indexOf('Aceptar') === -1, 'never a generic label');
});

// 6) The confirmation lists what is going away: 10 + overflow.
test('TestBulkConfirmListsFirstTenAndOverflow', () => {
    const few = H.bulkConfirmList(['a@x.com', 'b@x.com']);
    assert.strictEqual(few.shown.length, 2);
    assert.strictEqual(few.rest, 0);
    assert.strictEqual(few.overflow, '');
    const many = H.bulkConfirmList(Array.from({ length: 13 }, (_, i) => 'u' + i + '@x.com'));
    assert.strictEqual(many.shown.length, 10);
    assert.strictEqual(many.rest, 3);
    assert.strictEqual(many.overflow, 'y 3 más');
});

// 7) Deep links survive the back button and can be shared.
test('TestDeepLinkQueryRoundTrip', () => {
    assert.strictEqual(H.deepLinkQuery({ type: 'recipient', campaign: 21 }),
        '?type=recipient&campaign=21');
    assert.strictEqual(H.deepLinkQuery({ type: 'recipient', group: 5, q: 'qa@' }),
        '?type=recipient&group=5&q=qa%40');
    const parsed = H.parseDeepLink('?type=recipient&campaign=21&q=qa%40empresa.com');
    assert.strictEqual(parsed.type, 'recipient');
    assert.strictEqual(parsed.campaign, 21);
    assert.strictEqual(parsed.q, 'qa@empresa.com');
    assert.strictEqual(parsed.group, 0);
    // Empty search yields the default view.
    assert.deepStrictEqual(H.parseDeepLink(''), { type: 'all', campaign: 0, group: 0, q: '' });
});

// 8) Nested trash: the reason a restore is DISABLED (never hidden).
test('TestRestoreBlockedReasonNamesCampaign', () => {
    assert.strictEqual(H.restoreBlockedReason({ parent_campaign_trashed: false }), '');
    const msg = H.restoreBlockedReason({ parent_campaign_trashed: true, campaign_name: 'Devel Security' });
    assert.ok(msg.indexOf('«Devel Security»') !== -1, msg);
    assert.ok(msg.indexOf('Restaura primero la campaña') !== -1, msg);
});

// 9) relativeTime stays pure and covers the ranges used by the rollup.
test('TestRelativeTimeRanges', () => {
    assert.strictEqual(H.relativeTime('2026-07-29T21:59:30Z', NOW), 'hace segundos');
    assert.strictEqual(H.relativeTime('2026-07-29T21:30:00Z', NOW), 'hace 30 min');
    assert.strictEqual(H.relativeTime('2026-07-29T20:00:00Z', NOW), 'hace 2 horas');
    assert.strictEqual(H.relativeTime('2026-07-27T22:00:00Z', NOW), 'hace 2 días');
    assert.strictEqual(H.relativeTime('', NOW), '');
});

// 10) An empty bucket and a filtered search with no matches are DIFFERENT states.
//     Conflating them makes a user who searched for an address think it was lost.
test('TestEmptyStateDiffersFromNoResults', () => {
    // Genuinely empty → teaches the feature.
    const empty = H.emptyState('recipient', {});
    assert.strictEqual(empty.filtered, false);
    assert.strictEqual(empty.title, 'No hay destinatarios en la papelera.');
    assert.ok(/Cuando elimines destinatarios/.test(empty.hint), empty.hint);
    assert.strictEqual(empty.showClear, false);

    // Search with no matches → says so, names the term, offers to clear filters.
    const noMatch = H.emptyState('recipient', { q: 'pepe@x.com' });
    assert.strictEqual(noMatch.filtered, true);
    assert.strictEqual(noMatch.title, 'Ningún destinatario coincide con «pepe@x.com».');
    assert.strictEqual(noMatch.showClear, true);
    assert.notStrictEqual(noMatch.title, empty.title);
    assert.notStrictEqual(noMatch.hint, empty.hint);

    // campaign_id / group_id filters get their own wording, also with the clear action.
    const byCampaign = H.emptyState('recipient', { campaign: 21 });
    assert.ok(byCampaign.filtered && byCampaign.showClear);
    assert.ok(/la campaña #21/.test(byCampaign.title), byCampaign.title);
    const byGroup = H.emptyState('recipient', { group: 4 });
    assert.ok(/el grupo #4/.test(byGroup.title), byGroup.title);

    // The other three tabs teach their own feature, all in Spanish.
    ['all', 'campaign', 'campaign_group'].forEach(function (tab) {
        const st = H.emptyState(tab, {});
        assert.ok(st.title.length > 0 && st.hint.length > 0, tab);
        assert.ok(!/Nothing in the trash/.test(st.title), 'no English copy: ' + tab);
        assert.strictEqual(st.filtered, false);
        // Filtered variant of the old tabs also differs.
        const f = H.emptyState(tab, { q: 'x' });
        assert.notStrictEqual(f.title, st.title);
        assert.strictEqual(f.showClear, true);
    });
});

// 11) THE single pluralization helper. Three pluralization bugs shipped before it
//     existed; every count string must go through it, verb agreement included.
test('TestPluralHelperAgreesVerbAndNoun', () => {
    const f = n => H.plural(n, 'Se eliminará %d registro en total.', 'Se eliminarán %d registros en total.');
    assert.strictEqual(f(1), 'Se eliminará 1 registro en total.');
    assert.strictEqual(f(2), 'Se eliminarán 2 registros en total.');
    assert.strictEqual(f(0), 'Se eliminarán 0 registros en total.');
    // Nouns built on top of it stay consistent.
    assert.strictEqual(H.pluralizeRecipients(1), '1 destinatario');
    assert.strictEqual(H.pluralizeRecipients(3), '3 destinatarios');
    assert.strictEqual(H.pluralizeCampaigns(1), '1 campaña');
    assert.strictEqual(H.pluralizeCampaigns(4), '4 campañas');
    // Non-numeric input degrades to the plural form instead of printing NaN.
    assert.strictEqual(H.plural(undefined, '%d cosa', '%d cosas'), '0 cosas');
});

console.log('\n' + passed + ' passed' + (process.exitCode ? ' (with failures)' : ''));
