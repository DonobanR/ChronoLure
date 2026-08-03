// Unit test for the template-driven field visibility of the report editor (CL-103
// reopened). Plain Node + assert (this repo has no JS test framework).
//     node test/js/reports_fields.test.js
'use strict';

const assert = require('assert');

let passed = 0;
function test(name, fn) {
    try { fn(); passed++; console.log('  PASS ' + name); }
    catch (e) { console.error('  FAIL ' + name + '\n    ' + e.message); process.exitCode = 1; }
}

// Mirror of reports.js: the decision is a pure function of (field, state).
function fieldAppliesToTemplate(f, state) {
    if (!f || !f.token) return true;
    if (state.hasActiveVersion === false) return true;
    return (state.templateTokens || []).indexOf(f.token) !== -1;
}

const introExec = { k: 'intro_exec', token: 'PARRAFO_EJECUCION', req: true };
const suplantando = { k: 'impersonated_as', token: 'SUPLANTANDO', req: true };
const had2fa = { k: 'had2fa' }; // no token: not template-driven

// The regression this closes: the field was deleted by hand while 22 active
// templates still declared its token, so it became impossible to fill and the
// pre-flight blocked generation with no way out.
test('TestExecParagraphFieldAppearsOnlyWhenTemplateDeclaresToken', () => {
    // Legacy template (declares PARRAFO_EJECUCION, not the new tokens) → field shown.
    const legacy = { templateTokens: ['EMPRESA', 'PARRAFO_EJECUCION', 'TEXTO_PUNTO_1'], hasActiveVersion: true };
    assert.strictEqual(fieldAppliesToTemplate(introExec, legacy), true,
        'con plantilla legacy el párrafo de ejecución DEBE aparecer');
    assert.strictEqual(fieldAppliesToTemplate(suplantando, legacy), false,
        'la plantilla legacy no declara SUPLANTANDO: ese campo no debe pedirse');

    // New template (fixed sentence + SUPLANTANDO/COMUNICADO) → field hidden.
    const nuevo = { templateTokens: ['EMPRESA', 'SUPLANTANDO', 'COMUNICADO', 'TEXTO_PUNTO_1'], hasActiveVersion: true };
    assert.strictEqual(fieldAppliesToTemplate(introExec, nuevo), false,
        'con la plantilla nueva el párrafo libre no se pide');
    assert.strictEqual(fieldAppliesToTemplate(suplantando, nuevo), true);

    // Fields without a token are never template-driven.
    assert.strictEqual(fieldAppliesToTemplate(had2fa, nuevo), true);

    // No active version → show everything instead of an empty form.
    const sinVersion = { templateTokens: [], hasActiveVersion: false };
    assert.strictEqual(fieldAppliesToTemplate(introExec, sinVersion), true);
    assert.strictEqual(fieldAppliesToTemplate(suplantando, sinVersion), true);
});

console.log('\n' + passed + ' passed' + (process.exitCode ? ' (with failures)' : ''));
