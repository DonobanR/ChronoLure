// Client-side image preparation for report evidence (CL-111).
//
// Why this exists: users asked to upload SVG. The server refuses raw SVG on purpose
// — the pure-Go rasterizers drop <text> entirely (measured: an SVG with only a text
// element rasterizes to zero painted pixels), so a diagram exported from draw.io
// would reach the client with its boxes intact and every label missing, silently.
//
// The browser, on the other hand, has a complete SVG engine. So the conversion
// happens here: the SVG is drawn into a <canvas> and what gets uploaded is the PNG.
// The server still rejects raw SVG — the client converts, the server does not trust.
//
// A second benefit falls out of the same design: an SVG loaded through <img> runs no
// scripts and fetches no external resources, and the file itself is never stored, so
// the stored-XSS shape disappears rather than being filtered.
//
// UMD: window.ImagePrep in the browser, CommonJS under Node.
(function (root, factory) {
    if (typeof module === 'object' && module.exports) {
        module.exports = factory();
    } else {
        root.ImagePrep = factory();
    }
})(typeof self !== 'undefined' ? self : this, function () {
    'use strict';

    // RASTER_WIDTH is the width the SVG is rendered at. It is deliberately larger
    // than any figure will be printed: the DOCX scales the image down to the slot's
    // frame, and downscaling looks clean while upscaling looks pixelated.
    var RASTER_WIDTH = 1600;

    // MAX_PIXELS mirrors the server's own limit so an absurd viewBox is refused here
    // too, with a message, instead of producing a canvas the browser cannot allocate.
    var MAX_PIXELS = 50000000;

    var ACCEPT = '.png,.jpg,.jpeg,.gif,.bmp,.tif,.tiff,.webp,.svg,' +
        'image/png,image/jpeg,image/gif,image/bmp,image/tiff,image/webp,image/svg+xml';

    function isSVGFile(file) {
        if (!file) return false;
        if (file.type === 'image/svg+xml') return true;
        return /\.svg$/i.test(file.name || '');
    }

    // parseLength reads an SVG length attribute. Percentages are NOT a size — "100%"
    // means "as big as the container", which is exactly the case that needs the
    // viewBox instead.
    function parseLength(v) {
        if (!v) return 0;
        var s = String(v).trim();
        if (/%\s*$/.test(s)) return 0;
        var n = parseFloat(s);
        return isFinite(n) && n > 0 ? n : 0;
    }

    // svgDimensions derives the intrinsic size: width/height attributes if usable,
    // otherwise the viewBox. Returns null when neither is present — the caller must
    // refuse rather than invent a size, because guessing would silently crop or
    // stretch a diagram.
    function svgDimensions(text) {
        var head = String(text || '').slice(0, 4096);
        var tag = head.match(/<svg\b[^>]*>/i);
        if (!tag) return null;
        tag = tag[0];

        var w = parseLength((tag.match(/\bwidth\s*=\s*"([^"]*)"/i) || tag.match(/\bwidth\s*=\s*'([^']*)'/i) || [])[1]);
        var h = parseLength((tag.match(/\bheight\s*=\s*"([^"]*)"/i) || tag.match(/\bheight\s*=\s*'([^']*)'/i) || [])[1]);
        if (w > 0 && h > 0) return { width: w, height: h };

        var vb = (tag.match(/\bviewBox\s*=\s*"([^"]*)"/i) || tag.match(/\bviewBox\s*=\s*'([^']*)'/i) || [])[1];
        if (vb) {
            var p = vb.trim().split(/[\s,]+/).map(parseFloat);
            if (p.length === 4 && isFinite(p[2]) && isFinite(p[3]) && p[2] > 0 && p[3] > 0) {
                return { width: p[2], height: p[3] };
            }
        }
        return null;
    }

    // targetSize scales to RASTER_WIDTH keeping the aspect ratio. It never upscales
    // beyond the cap and never returns a zero dimension for a very wide, short image.
    function targetSize(width, height, maxWidth) {
        var mw = maxWidth || RASTER_WIDTH;
        var scale = mw / width;
        return {
            width: Math.max(1, Math.round(width * scale)),
            height: Math.max(1, Math.round(height * scale))
        };
    }

    // errorFor produces the Spanish message for a refusal: what happened, why, and
    // what to do — never a bare "no se pudo procesar el archivo".
    function errorFor(kind) {
        switch (kind) {
            case 'no-size':
                return 'El SVG no declara tamaño (le faltan width/height y viewBox), así que no se puede ' +
                    'convertir a imagen sin adivinar sus proporciones. Ábrelo en tu editor y expórtalo como PNG.';
            case 'too-big':
                return 'El SVG declara un tamaño desproporcionado y no se puede convertir. ' +
                    'Redúcelo en tu editor o expórtalo directamente como PNG.';
            case 'broken':
                return 'El SVG no se pudo interpretar: puede estar incompleto o no ser un SVG válido. ' +
                    'Vuelve a exportarlo desde tu editor, o expórtalo como PNG.';
            default:
                return 'No se pudo preparar la imagen. Vuelve a exportarla desde tu editor e inténtalo de nuevo.';
        }
    }

    // FONT_WARNING is shown after every successful conversion. It is not hidden in a
    // tooltip: an SVG that references a font the machine does not have gets rendered
    // with a substitute, and the text WILL look different. The preview shown next to
    // it is the actual PNG that will be uploaded, so the user can catch it here
    // rather than in the delivered document.
    var FONT_WARNING = 'El SVG se convirtió a imagen. Revisa la vista previa: ' +
        'si usa fuentes poco comunes, el texto puede verse distinto.';

    // rasterizeSVG converts an SVG File/Blob into a PNG Blob.
    //
    // deps exists for testability: under jsdom there is no canvas implementation, so
    // the test injects fakes. In the browser the defaults are the real globals.
    function rasterizeSVG(file, deps) {
        deps = deps || {};
        var readText = deps.readText || function (f) { return f.text(); };
        var createObjectURL = deps.createObjectURL ||
            function (b) { return URL.createObjectURL(b); };
        var revokeObjectURL = deps.revokeObjectURL ||
            function (u) { return URL.revokeObjectURL(u); };
        var newImage = deps.newImage || function () { return new Image(); };
        var newCanvas = deps.newCanvas || function () { return document.createElement('canvas'); };

        return readText(file).then(function (text) {
            var dim = svgDimensions(text);
            if (!dim) throw new Error(errorFor('no-size'));

            var size = targetSize(dim.width, dim.height, RASTER_WIDTH);
            if (size.width * size.height > MAX_PIXELS) throw new Error(errorFor('too-big'));

            return new Promise(function (resolve, reject) {
                var url = createObjectURL(file);
                var img = newImage();
                var done = false;
                function finish(fn) {
                    if (done) return;
                    done = true;
                    revokeObjectURL(url); // always: a leaked blob URL pins the file in memory
                    fn();
                }
                img.onload = function () {
                    finish(function () {
                        try {
                            var canvas = newCanvas();
                            canvas.width = size.width;
                            canvas.height = size.height;
                            var ctx = canvas.getContext('2d');
                            // SVG has no background of its own; without this the
                            // transparent areas become black in some Word versions.
                            ctx.fillStyle = '#ffffff';
                            ctx.fillRect(0, 0, size.width, size.height);
                            ctx.drawImage(img, 0, 0, size.width, size.height);
                            canvas.toBlob(function (blob) {
                                if (blob) resolve(blob);
                                else reject(new Error(errorFor('broken')));
                            }, 'image/png');
                        } catch (e) {
                            reject(new Error(errorFor('broken')));
                        }
                    });
                };
                img.onerror = function () {
                    finish(function () { reject(new Error(errorFor('broken'))); });
                };
                img.src = url;
            });
        });
    }

    // prepareUpload is what the editor calls. Non-SVG files pass through untouched —
    // the server converts those, and re-encoding them here would only lose quality.
    function prepareUpload(file, deps) {
        if (!isSVGFile(file)) {
            return Promise.resolve({ blob: file, name: file.name, converted: false, warning: '' });
        }
        return rasterizeSVG(file, deps).then(function (blob) {
            return {
                blob: blob,
                name: String(file.name || 'imagen.svg').replace(/\.svg$/i, '') + '.png',
                converted: true,
                warning: FONT_WARNING
            };
        });
    }

    return {
        ACCEPT: ACCEPT,
        RASTER_WIDTH: RASTER_WIDTH,
        MAX_PIXELS: MAX_PIXELS,
        FONT_WARNING: FONT_WARNING,
        isSVGFile: isSVGFile,
        svgDimensions: svgDimensions,
        targetSize: targetSize,
        errorFor: errorFor,
        rasterizeSVG: rasterizeSVG,
        prepareUpload: prepareUpload
    };
});
