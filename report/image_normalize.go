package report

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"

	// Decoders registered for image.Decode / image.DecodeConfig. jpeg and gif are
	// stdlib; bmp, tiff and webp come from golang.org/x/image, which was already an
	// indirect dependency (go-chart pulls it for font rendering), so accepting six
	// formats adds no module to go.mod.
	_ "image/gif"
	_ "image/jpeg"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// Evidence images are normalized to PNG on upload: whatever the user provides is
// decoded and re-encoded, and only the PNG is stored. Two consequences are the
// point of the design:
//
//   - The DOCX always carries PNG, so rendering never depends on which formats a
//     particular Word or LibreOffice build understands.
//   - Nothing the user uploaded is ever served back verbatim. The thumbnail route
//     hands out bytes this package produced, which removes the stored-XSS shape
//     entirely (an SVG served as image/svg+xml executes its <script> when the URL
//     is opened directly; X-Content-Type-Options does not prevent that).
const (
	// MaxImageBytes bounds the compressed upload.
	MaxImageBytes = 32 << 20 // 32 MiB

	// MaxImagePixels bounds the DECOMPRESSED size, which the byte limit does not:
	// a few hundred KiB of PNG can declare 30000x30000 and demand ~3.6 GiB of RAM
	// once decoded. Checked from the header alone, before any pixel is read.
	// 50 MP is far above any real screenshot (a 5K display is ~15 MP).
	MaxImagePixels = 50_000_000
)

// SupportedImageFormats is the human-facing list of accepted evidence formats.
const SupportedImageFormats = "PNG, JPEG, GIF, BMP, TIFF o WebP"

var (
	// ErrUnsupportedImageType is returned when the content is not one of the
	// supported image formats, verified by magic bytes — never by the file name or
	// by the MIME the client declared.
	ErrUnsupportedImageType = errors.New("unsupported image type")

	// ErrRawSVG is its own error because SVG deserves its own message: the editor
	// rasterizes SVG in the browser before uploading, so a raw SVG reaching the
	// server means either an API client or a browser path that failed. Accepting it
	// is not an option — see the note on rasterizers below.
	ErrRawSVG = errors.New("raw svg upload")

	// ErrImageTooLarge / ErrImageTooManyPixels are the two independent size
	// defences: bytes on the wire, and pixels after decompression.
	ErrImageTooLarge      = errors.New("image file too large")
	ErrImageTooManyPixels = errors.New("image dimensions too large")

	// ErrImageCorrupt means the format was recognised but the data could not be
	// decoded — a truncated download, a renamed file, a partial upload.
	ErrImageCorrupt = errors.New("image could not be decoded")
)

// Why SVG is refused server-side rather than rasterized here: the pure-Go
// rasterizers (srwiley/oksvg + rasterx) DROP <text> entirely — measured: an SVG
// containing only a text element rasterizes to zero painted pixels. Diagrams
// exported from draw.io or Figma would arrive with their boxes and arrows intact
// and every label missing, silently, inside a document delivered to a client. A
// rejection is visible; a mute diagram is not. The editor converts SVG with the
// browser's own engine (which does render text) and uploads the resulting PNG.

// imageMagic maps a format to its signature. Detection is by content because the
// file name is attacker-controlled and the browser's declared MIME is a hint, not
// evidence.
//
// TIFF is here for a concrete reason: http.DetectContentType has NO TIFF signature
// and returns application/octet-stream for it, so relying on the sniffer alone
// would reject perfectly valid TIFFs as "not an image".
var imageMagic = []struct {
	name  string
	magic []byte
}{
	{"png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}},
	{"jpeg", []byte{0xFF, 0xD8, 0xFF}},
	{"gif", []byte("GIF87a")},
	{"gif", []byte("GIF89a")},
	{"bmp", []byte("BM")},
	{"tiff", []byte{'I', 'I', 42, 0}}, // little-endian
	{"tiff", []byte{'M', 'M', 0, 42}}, // big-endian
	{"webp", []byte("RIFF")},          // refined below: RIFF....WEBP
}

// DetectImageFormat returns the format name of an evidence image based on its
// magic bytes, and ok=false if it is not a supported format.
func DetectImageFormat(b []byte) (format string, ok bool) {
	for _, m := range imageMagic {
		if len(b) < len(m.magic) || !bytes.Equal(b[:len(m.magic)], m.magic) {
			continue
		}
		if m.name == "webp" {
			// RIFF alone is a container marker (also used by WAV/AVI); the WEBP
			// form-type sits at offset 8.
			if len(b) < 12 || !bytes.Equal(b[8:12], []byte("WEBP")) {
				continue
			}
		}
		return m.name, true
	}
	return "", false
}

// looksLikeSVG reports whether the content is an SVG document. SVG has no magic
// bytes — it is XML — so this scans the head for an <svg root, tolerating an XML
// declaration, a doctype, comments and a UTF-8 BOM before it.
func looksLikeSVG(b []byte) bool {
	head := b
	if len(head) > 1024 {
		head = head[:1024]
	}
	lower := bytes.ToLower(head)
	return bytes.Contains(lower, []byte("<svg"))
}

// NormalizeImageToPNG validates an uploaded evidence image and returns it re-encoded
// as PNG. The order of the checks is the security-relevant part: size, then format,
// then dimensions FROM THE HEADER, and only then the full decode.
func NormalizeImageToPNG(content []byte) ([]byte, error) {
	if len(content) > MaxImageBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrImageTooLarge, len(content))
	}
	if len(content) == 0 {
		return nil, ErrImageCorrupt
	}
	if looksLikeSVG(content) {
		return nil, ErrRawSVG
	}
	format, ok := DetectImageFormat(content)
	if !ok {
		return nil, ErrUnsupportedImageType
	}

	// Header only: DecodeConfig reads the dimensions without touching pixel data,
	// so a decompression bomb is rejected before it can allocate anything.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrImageCorrupt, format, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("%w: %dx%d", ErrImageCorrupt, cfg.Width, cfg.Height)
	}
	if int64(cfg.Width)*int64(cfg.Height) > MaxImagePixels {
		return nil, fmt.Errorf("%w: %dx%d", ErrImageTooManyPixels, cfg.Width, cfg.Height)
	}

	img, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrImageCorrupt, format, err)
	}

	// Everything is re-encoded, PNG included, so exactly one code path produces the
	// bytes that get stored: a decodable-but-malformed file is rewritten clean, and
	// oddities that survive a byte-for-byte copy (APNG animation, oversized colour
	// profiles, trailing data after IEND) do not reach the document.
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
