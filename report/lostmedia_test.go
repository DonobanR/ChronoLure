package report

import (
	"archive/zip"
	"bytes"
	"testing"
)

// buildDocxTwoSlots builds a minimal DOCX with two image slots, each backed by
// its own media part, so tests can verify per-slot swap behavior. The docPr
// labels are provided verbatim (descr1/descr2) to exercise Alt-Text variations.
func buildDocxTwoSlots(t *testing.T, descr1, media1 string, bytes1 []byte, descr2, media2 string, bytes2 []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, content []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	add("[Content_Types].xml", []byte(`<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`))
	add("_rels/.rels", []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`))
	add("word/_rels/document.xml.rels", []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/>`+
		`<Relationship Id="rId6" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image2.png"/>`+
		`</Relationships>`))
	body := `<w:p><w:r><w:drawing><wp:docPr id="1" descr="` + descr1 + `"/><a:blip r:embed="rId5"/></w:drawing></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:docPr id="2" descr="` + descr2 + `"/><a:blip r:embed="rId6"/></w:drawing></w:r></w:p>`
	add("word/document.xml", []byte(`<?xml version="1.0"?><w:document xmlns:w="x" xmlns:r="y" xmlns:wp="z" xmlns:a="w"><w:body>`+body+`</w:body></w:document>`))
	add(media1, bytes1)
	add(media2, bytes2)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// TestSlotKeyMatchIsCaseAndSpaceInsensitive covers CL-104: a template whose
// Alt-Text varies in case/separators ("Figura 1", "FIGURA_1") must still resolve
// to the uploaded "figura_1" image, instead of leaving the "missing" placeholder.
func TestSlotKeyMatchIsCaseAndSpaceInsensitive(t *testing.T) {
	variants := []string{"figura_1", "Figura_1", "FIGURA_1", "Figura 1", " figura_1 ", "figura1"}
	for _, descr := range variants {
		t.Run(descr, func(t *testing.T) {
			tmpl := buildDocxWithImage(t,
				`<w:p><w:r><w:drawing><wp:docPr id="1" descr="`+descr+`"/><a:blip r:embed="rId5"/></w:drawing></w:r></w:p>`,
				"word/media/image1.png", []byte("OLD"))
			out, _, err := Render(RenderInput{
				Template: tmpl,
				Images:   map[string][]byte{"figura_1": []byte("NEW")},
			})
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if got := string(unzipEntry(t, out, "word/media/image1.png")); got != "NEW" {
				t.Fatalf("descr=%q: expected uploaded image to be swapped, got %q", descr, got)
			}
		})
	}
}

// TestPartialMissingDoesNotFlagPresentSlots covers CL-104: when only some slots
// are provided, the present ones are swapped and the missing ones are left
// exactly as the template had them — no cross-contamination, no present slot
// treated as missing.
func TestPartialMissingDoesNotFlagPresentSlots(t *testing.T) {
	tmpl := buildDocxTwoSlots(t,
		"figura_1", "word/media/image1.png", []byte("PLACEHOLDER-1"),
		"figura_2", "word/media/image2.png", []byte("PLACEHOLDER-2"))

	// Provide figura_1 only; figura_2 is intentionally missing.
	out, _, err := Render(RenderInput{
		Template: tmpl,
		Images:   map[string][]byte{"figura_1": []byte("UPLOADED-1")},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := string(unzipEntry(t, out, "word/media/image1.png")); got != "UPLOADED-1" {
		t.Fatalf("present slot figura_1 not swapped, got %q", got)
	}
	if got := string(unzipEntry(t, out, "word/media/image2.png")); got != "PLACEHOLDER-2" {
		t.Fatalf("missing slot figura_2 should keep its placeholder, got %q", got)
	}
}

// TestInspectCanonicalizesSlotKeys covers CL-104 at template-validation time: a
// template whose Alt-Text varies in case/separators is still recognized as the
// canonical slot (not reported unknown/missing).
func TestInspectCanonicalizesSlotKeys(t *testing.T) {
	tmpl := buildDocxWithImage(t,
		`<w:p><w:r><w:drawing><wp:docPr id="1" descr="FIGURA 1"/><a:blip r:embed="rId5"/></w:drawing></w:r></w:p>`,
		"word/media/image1.png", []byte("x"))
	insp, err := Inspect(tmpl)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	found := false
	for _, s := range insp.ImageSlots {
		if s == "figura_1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected canonical slot figura_1 in ImageSlots, got %v", insp.ImageSlots)
	}
}

// TestCanonicalSlotKey unit-checks the fold used across inspect/render/upload.
func TestCanonicalSlotKey(t *testing.T) {
	ok := map[string]string{
		"figura_1":        "figura_1",
		"Figura 1":        "figura_1",
		"FIGURA_1":        "figura_1",
		" figura_1 ":      "figura_1",
		"figura1":         "figura_1",
		"grafico 1":       "grafico_1",
		"Evidencia Flujo": "evidencia_flujo",
		"LOGO":            "logo",
	}
	for in, want := range ok {
		if got, ok := CanonicalSlotKey(in); !ok || got != want {
			t.Fatalf("CanonicalSlotKey(%q)=%q,%v want %q,true", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "figura_99", "no-soy-slot", "figura"} {
		if got, ok := CanonicalSlotKey(bad); ok {
			t.Fatalf("CanonicalSlotKey(%q) should be unknown, got %q", bad, got)
		}
	}
}
