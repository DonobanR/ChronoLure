package report

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// sampleImage is a small, non-uniform image so a re-encode that silently produced a
// blank canvas would be visible in the decoded pixels.
func sampleImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			if (x+y)%2 == 0 {
				img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}
	return img
}

func encodeAs(t *testing.T, format string) []byte {
	t.Helper()
	var buf bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&buf, sampleImage())
	case "jpeg":
		err = jpeg.Encode(&buf, sampleImage(), nil)
	case "gif":
		err = gif.Encode(&buf, sampleImage(), nil)
	case "bmp":
		err = bmp.Encode(&buf, sampleImage())
	case "tiff":
		err = tiff.Encode(&buf, sampleImage(), nil)
	default:
		t.Fatalf("no encoder for %q", format)
	}
	if err != nil {
		t.Fatalf("encode %s: %v", format, err)
	}
	return buf.Bytes()
}

// TestUploadAcceptsAllSupportedFormatsAndStoresPNG: whatever comes in, PNG comes out.
// The assertion is on the RESULT bytes, not on "no error": storing the original
// would also return nil, and that is precisely the failure this guards against.
func TestUploadAcceptsAllSupportedFormatsAndStoresPNG(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif", "bmp", "tiff"} {
		t.Run(format, func(t *testing.T) {
			in := encodeAs(t, format)
			out, err := NormalizeImageToPNG(in)
			if err != nil {
				t.Fatalf("%s rechazado: %v", format, err)
			}
			if got, ok := DetectImageFormat(out); !ok || got != "png" {
				t.Fatalf("lo almacenado es %q, se esperaba png", got)
			}
			img, decoded, err := image.Decode(bytes.NewReader(out))
			if err != nil {
				t.Fatalf("el PNG resultante no decodifica: %v", err)
			}
			if decoded != "png" {
				t.Fatalf("image.Decode dice %q", decoded)
			}
			if b := img.Bounds(); b.Dx() != 8 || b.Dy() != 4 {
				t.Fatalf("dimensiones perdidas: %v", b)
			}
			// A non-PNG input must actually change bytes; a PNG input is re-encoded
			// too, so in both cases the stored blob is this package's own output.
			if format != "png" && bytes.Equal(in, out) {
				t.Fatalf("%s se almacenó tal cual, sin convertir", format)
			}
		})
	}
}

// WebP has no encoder in x/image (decode only), so the sample is a real 8x4 lossless
// WebP, the same checkerboard as sampleImage(), embedded as bytes.
func webpSample() []byte {
	return []byte{
		0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50,
		0x56, 0x50, 0x38, 0x4c, 0x17, 0x00, 0x00, 0x00, 0x2f, 0x07, 0xc0, 0x00,
		0x00, 0x0f, 0xf0, 0x8f, 0xff, 0x27, 0xff, 0xff, 0xe3, 0x7f, 0xfe, 0xe3,
		0x01, 0xaf, 0x52, 0x8d, 0xe8, 0x7f, 0x28, 0x00,
	}
}

func TestUploadAcceptsWebPAndStoresPNG(t *testing.T) {
	out, err := NormalizeImageToPNG(webpSample())
	if err != nil {
		t.Fatalf("webp rechazado: %v", err)
	}
	if got, ok := DetectImageFormat(out); !ok || got != "png" {
		t.Fatalf("lo almacenado es %q, se esperaba png", got)
	}
}

func TestUploadRejectsNonImageFile(t *testing.T) {
	cases := map[string][]byte{
		"PDF":           []byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n"),
		"texto plano":   []byte("esto no es una imagen, es una nota\n"),
		"DOCX (zip)":    []byte("PK\x03\x04\x14\x00\x06\x00"),
		"ejecutable":    []byte("\x7fELF\x02\x01\x01\x00"),
		"fichero vacío": {},
	}
	for name, content := range cases {
		if _, err := NormalizeImageToPNG(content); err == nil {
			t.Fatalf("%s fue aceptado como imagen", name)
		}
	}
	// A GIF whose header is intact but whose data is truncated: the magic bytes get
	// it past detection, so it must fail at decode, not be stored half-read.
	truncated := append(encodeAs(t, "gif")[:12], 0x00)
	if _, err := NormalizeImageToPNG(truncated); !errors.Is(err, ErrImageCorrupt) {
		t.Fatalf("un GIF truncado debería ser ErrImageCorrupt, fue: %v", err)
	}
}

// TestUploadRejectsRawSVG is the defence-in-depth half: the editor rasterizes SVG in
// the browser, but the server must never trust that. An SVG arriving here means an
// API client or a browser path that failed, and accepting it would put an
// unrasterized file — possibly carrying <script> — into the blob store.
func TestUploadRejectsRawSVG(t *testing.T) {
	cases := map[string][]byte{
		"svg simple":       []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`),
		"con declaración":  []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		"con doctype":      []byte(`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" ""><svg></svg>`),
		"mayúsculas":       []byte(`<SVG xmlns="http://www.w3.org/2000/svg"></SVG>`),
		"con script (XSS)": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
	}
	for name, content := range cases {
		_, err := NormalizeImageToPNG(content)
		if !errors.Is(err, ErrRawSVG) {
			t.Fatalf("%s: se esperaba ErrRawSVG, fue: %v", name, err)
		}
	}
}

// TestTIFFDetectedByMagicBytes covers the gap measured in http.DetectContentType,
// which has no TIFF signature and returns application/octet-stream: relying on the
// sniffer alone would reject valid TIFFs as "not an image".
func TestTIFFDetectedByMagicBytes(t *testing.T) {
	for _, c := range []struct {
		name  string
		magic []byte
	}{
		{"little-endian (II*\\0)", []byte{'I', 'I', 42, 0, 8, 0, 0, 0}},
		{"big-endian (MM\\0*)", []byte{'M', 'M', 0, 42, 0, 0, 0, 8}},
	} {
		if got, ok := DetectImageFormat(c.magic); !ok || got != "tiff" {
			t.Fatalf("%s no se detectó como tiff (got=%q ok=%v)", c.name, got, ok)
		}
	}
	// And a real TIFF round-trips to PNG.
	out, err := NormalizeImageToPNG(encodeAs(t, "tiff"))
	if err != nil {
		t.Fatalf("TIFF válido rechazado: %v", err)
	}
	if got, _ := DetectImageFormat(out); got != "png" {
		t.Fatalf("TIFF no se convirtió a PNG (got %q)", got)
	}
}

func TestOversizedFileRejected(t *testing.T) {
	// Valid PNG header so the rejection can only come from the size check.
	big := make([]byte, MaxImageBytes+1)
	copy(big, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	if _, err := NormalizeImageToPNG(big); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("se esperaba ErrImageTooLarge, fue: %v", err)
	}
	// One byte under the limit must NOT be rejected for size (it fails later, as a
	// corrupt PNG) — otherwise the boundary would be off by one.
	edge := make([]byte, MaxImageBytes)
	copy(edge, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	if _, err := NormalizeImageToPNG(edge); errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("un archivo justo en el límite se rechazó por tamaño")
	}
}

// TestOversizedDimensionsRejectedBeforeDecode is the decompression-bomb defence, and
// the "before decode" is the whole point: the input below is a few hundred bytes and
// would allocate gigabytes if it were decoded. The test asserts the rejection is
// driven by the HEADER by using a file whose pixel data is deliberately absent — a
// full decode would fail with a different error.
func TestOversizedDimensionsRejectedBeforeDecode(t *testing.T) {
	// A PNG header declaring 30000x30000 (900 MP ≈ 3.6 GiB decoded), with no IDAT.
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	ihdr := []byte{
		0, 0, 0, 13, 'I', 'H', 'D', 'R',
		0, 0, 0x75, 0x30, // width  = 30000
		0, 0, 0x75, 0x30, // height = 30000
		8, 6, 0, 0, 0,
	}
	buf.Write(ihdr)
	buf.Write([]byte{0x66, 0x27, 0xf8, 0xba}) // real CRC32: DecodeConfig verifies it

	_, err := NormalizeImageToPNG(buf.Bytes())
	if !errors.Is(err, ErrImageTooManyPixels) {
		t.Fatalf("se esperaba ErrImageTooManyPixels, fue: %v", err)
	}
	if buf.Len() > 100 {
		t.Fatalf("el fixture dejó de ser pequeño (%d bytes): ya no demuestra que se rechaza sin descomprimir", buf.Len())
	}
	t.Logf("MEDIDO: %d bytes de entrada declaran 30000x30000 (900 MP) y se rechazan sin descomprimir", buf.Len())
}
