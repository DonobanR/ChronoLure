package report

import (
	"archive/zip"
	"bytes"
	"testing"
)

type zentry struct{ name, data string }

func zipWith(t *testing.T, entries []zentry, method uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
		if err != nil {
			t.Fatalf("create %s: %v", e.name, err)
		}
		w.Write([]byte(e.data))
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// TestFingerprintStability proves the fingerprint ignores compression method and
// entry order, but reflects real content changes.
func TestFingerprintStability(t *testing.T) {
	a := []zentry{
		{"word/document.xml", "<doc>hello</doc>"},
		{"[Content_Types].xml", "<types/>"},
		{"word/media/image1.png", "PNGDATA"},
	}
	// Same content, different order + different compression method.
	b := []zentry{
		{"word/media/image1.png", "PNGDATA"},
		{"word/document.xml", "<doc>hello</doc>"},
		{"[Content_Types].xml", "<types/>"},
	}

	fpA, err := Fingerprint(zipWith(t, a, zip.Deflate))
	if err != nil {
		t.Fatalf("fp A: %v", err)
	}
	fpB, err := Fingerprint(zipWith(t, b, zip.Store))
	if err != nil {
		t.Fatalf("fp B: %v", err)
	}
	if fpA != fpB {
		t.Fatalf("fingerprint should be stable across order/compression: %s != %s", fpA, fpB)
	}

	// A real content change must change the fingerprint.
	c := []zentry{
		{"word/document.xml", "<doc>HELLO</doc>"}, // changed
		{"[Content_Types].xml", "<types/>"},
		{"word/media/image1.png", "PNGDATA"},
	}
	fpC, err := Fingerprint(zipWith(t, c, zip.Deflate))
	if err != nil {
		t.Fatalf("fp C: %v", err)
	}
	if fpC == fpA {
		t.Fatalf("fingerprint must change when content changes")
	}
}
