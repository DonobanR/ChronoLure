// CL-111: SVG must be rasterized to PNG in the browser BEFORE upload.
//
// jsdom has no canvas implementation, so the DOM pieces are injected through the
// deps parameter. What is being tested is the decision logic and the guarantee that
// the bytes handed to FormData are a PNG blob, never the original SVG.
const assert = require('assert');
const { JSDOM } = require('jsdom');

const dom = new JSDOM('<!doctype html><html><body></body></html>');
global.window = dom.window;
global.document = dom.window.document;

const ImagePrep = require('../../static/js/src/app/image_prep.js');

let passed = 0;
function test(name, fn) {
    try {
        const r = fn();
        if (r && typeof r.then === 'function') {
            return r.then(
                () => { passed++; console.log('  ok  ' + name); },
                (e) => { console.error('  FAIL ' + name + '\n       ' + e.message); process.exitCode = 1; }
            );
        }
        passed++;
        console.log('  ok  ' + name);
    } catch (e) {
        console.error('  FAIL ' + name + '\n       ' + e.message);
        process.exitCode = 1;
    }
    return Promise.resolve();
}

// fakeFile mimics the parts of File that prepareUpload touches.
function fakeFile(name, type, text) {
    return { name, type, text: () => Promise.resolve(text) };
}

// fakeDeps records what happened so the test can assert on the canvas calls.
function fakeDeps(record) {
    record.drawn = null;
    record.filled = null;
    record.revoked = 0;
    return {
        readText: (f) => f.text(),
        createObjectURL: () => 'blob:fake',
        revokeObjectURL: () => { record.revoked++; },
        newImage: () => {
            const img = {};
            // The browser loads the SVG asynchronously; fire onload on the next tick.
            Object.defineProperty(img, 'src', {
                set() { setTimeout(() => img.onload && img.onload(), 0); }
            });
            return img;
        },
        newCanvas: () => ({
            width: 0,
            height: 0,
            getContext: () => ({
                fillStyle: '',
                fillRect: (x, y, w, h) => { record.filled = { w, h }; },
                drawImage: (img, x, y, w, h) => { record.drawn = { w, h }; }
            }),
            toBlob: (cb, mime) => { record.mime = mime; cb({ type: mime, size: 123 }); }
        })
    };
}

const SVG_WITH_TEXT =
    '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="400">' +
    '<rect width="800" height="400" fill="#eee"/>' +
    '<text x="20" y="200" font-size="40">Servidor DMZ</text></svg>';

async function main() {
    console.log('image_prep');

    await test('TestSVGRasterizedToPNGBeforeUpload: sube PNG, nunca el SVG', async () => {
        const rec = {};
        const file = fakeFile('diagrama.svg', 'image/svg+xml', SVG_WITH_TEXT);
        const out = await ImagePrep.prepareUpload(file, fakeDeps(rec));

        assert.strictEqual(out.converted, true, 'debe marcarse como convertido');
        assert.strictEqual(out.blob.type, 'image/png', 'lo que se sube debe ser PNG');
        assert.strictEqual(rec.mime, 'image/png', 'toBlob debe pedir image/png');
        assert.notStrictEqual(out.blob, file, 'no puede subirse el fichero original');
        assert.ok(/\.png$/.test(out.name), 'el nombre debe pasar a .png, es: ' + out.name);
        assert.ok(out.warning.indexOf('fuentes poco comunes') !== -1, 'debe avisar de la sustitución de fuentes');
        // 800x400 escalado a 1600 de ancho => 1600x800
        assert.deepStrictEqual(rec.drawn, { w: 1600, h: 800 }, 'escala incorrecta');
        // Fondo blanco antes de dibujar: sin él, las zonas transparentes salen negras.
        assert.deepStrictEqual(rec.filled, { w: 1600, h: 800 }, 'falta el fondo blanco');
        assert.strictEqual(rec.revoked, 1, 'el blob URL debe liberarse');
    });

    await test('un PNG normal pasa sin tocarse (lo convierte el servidor)', async () => {
        const file = fakeFile('captura.png', 'image/png', '');
        const out = await ImagePrep.prepareUpload(file, fakeDeps({}));
        assert.strictEqual(out.converted, false);
        assert.strictEqual(out.blob, file, 'un raster no debe re-codificarse en el cliente');
        assert.strictEqual(out.warning, '');
    });

    await test('SVG sin width/height ni viewBox se rechaza con instrucciones', async () => {
        const file = fakeFile('malo.svg', 'image/svg+xml', '<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>');
        await ImagePrep.prepareUpload(file, fakeDeps({})).then(
            () => { throw new Error('debería haberse rechazado'); },
            (e) => {
                assert.ok(e.message.indexOf('no declara tamaño') !== -1, 'mensaje: ' + e.message);
                assert.ok(e.message.indexOf('PNG') !== -1, 'el mensaje debe decir qué hacer');
            }
        );
    });

    await test('SVG con solo viewBox: se usa el viewBox', () => {
        const d = ImagePrep.svgDimensions('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 50"></svg>');
        assert.deepStrictEqual(d, { width: 200, height: 50 });
    });

    await test('width="100%" no es un tamaño: cae al viewBox', () => {
        const d = ImagePrep.svgDimensions('<svg width="100%" height="100%" viewBox="0 0 640 480"></svg>');
        assert.deepStrictEqual(d, { width: 640, height: 480 });
    });

    await test('un SVG que el navegador no puede cargar da un error accionable', async () => {
        const rec = {};
        const deps = fakeDeps(rec);
        deps.newImage = () => {
            const img = {};
            Object.defineProperty(img, 'src', {
                set() { setTimeout(() => img.onerror && img.onerror(), 0); }
            });
            return img;
        };
        const file = fakeFile('roto.svg', 'image/svg+xml', '<svg width="10" height="10">');
        await ImagePrep.prepareUpload(file, deps).then(
            () => { throw new Error('debería haberse rechazado'); },
            (e) => {
                assert.ok(e.message.indexOf('no se pudo interpretar') !== -1, 'mensaje: ' + e.message);
                assert.strictEqual(rec.revoked, 1, 'el blob URL debe liberarse también al fallar');
            }
        );
    });

    await test('detección de SVG por tipo y por extensión', () => {
        assert.ok(ImagePrep.isSVGFile({ name: 'a.svg', type: '' }));
        assert.ok(ImagePrep.isSVGFile({ name: 'a', type: 'image/svg+xml' }));
        assert.ok(ImagePrep.isSVGFile({ name: 'A.SVG', type: '' }));
        assert.ok(!ImagePrep.isSVGFile({ name: 'a.png', type: 'image/png' }));
    });

    await test('el accept del selector ofrece los formatos nuevos', () => {
        for (const ext of ['.png', '.jpg', '.gif', '.bmp', '.tiff', '.webp', '.svg']) {
            assert.ok(ImagePrep.ACCEPT.indexOf(ext) !== -1, 'falta ' + ext + ' en accept');
        }
    });

    console.log(`  ${passed} pruebas`);
}

main();
