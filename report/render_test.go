package report

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
)

// buildDocxWithImage builds a synthetic .docx including a media part and a
// drawing that references it through a relationship, so image swapping can be
// exercised. No corporate content is involved.
func buildDocxWithImage(t *testing.T, bodyXML, mediaName string, mediaBytes []byte) []byte {
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
	add("word/_rels/document.xml.rels", []byte(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/></Relationships>`))
	add("word/document.xml", []byte(`<?xml version="1.0"?><w:document xmlns:w="x" xmlns:r="y" xmlns:wp="z" xmlns:a="w"><w:body>`+bodyXML+`</w:body></w:document>`))
	add(mediaName, mediaBytes)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func unzipEntry(t *testing.T, docx []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("open output: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			return b
		}
	}
	t.Fatalf("entry %s not found in output", name)
	return nil
}

func TestRenderReplacesTokensAndImage(t *testing.T) {
	body := `<w:p><w:r><w:t>Empresa: {{EMPRESA}}</w:t></w:r></w:p>` +
		// fragmented token across two runs:
		`<w:p><w:r><w:t>Total {{R_</w:t></w:r><w:r><w:t>TOTAL}} correos</w:t></w:r></w:p>` +
		// drawing referencing the image part via rId5:
		`<w:p><w:r><w:drawing><wp:docPr id="1" name="figura_1"/><a:blip r:embed="rId5"/></w:drawing></w:r></w:p>`

	tmpl := buildDocxWithImage(t, body, "word/media/image1.png", []byte("OLD-IMAGE-BYTES"))

	out, _, err := Render(RenderInput{
		Template: tmpl,
		Vars:     map[string]string{"EMPRESA": "ACME, S.A.", "R_TOTAL": "17"},
		Images:   map[string][]byte{"figura_1": []byte("NEW-IMAGE-BYTES")},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	doc := string(unzipEntry(t, out, "word/document.xml"))
	// Visible text (tags stripped): min-span keeps runs separate, so compare the
	// concatenated run text rather than a contiguous XML substring.
	visible := tagRe.ReplaceAllString(doc, "")
	if !strings.Contains(visible, "Empresa: ACME, S.A.") {
		t.Fatalf("clean token not replaced:\n%s", doc)
	}
	if !strings.Contains(visible, "Total 17 correos") {
		t.Fatalf("fragmented token not reassembled/replaced:\n%s", doc)
	}
	if strings.Contains(doc, "{{") {
		t.Fatalf("unresolved tokens remain:\n%s", doc)
	}

	media := unzipEntry(t, out, "word/media/image1.png")
	if string(media) != "NEW-IMAGE-BYTES" {
		t.Fatalf("image not swapped, got %q", media)
	}

	// Output must still be a valid (openable) zip/docx.
	if _, err := zip.NewReader(bytes.NewReader(out), int64(len(out))); err != nil {
		t.Fatalf("output is not a valid docx zip: %v", err)
	}
}

func TestRenderEscapesXML(t *testing.T) {
	body := `<w:p><w:r><w:t>{{EMPRESA}}</w:t></w:r></w:p>`
	tmpl := buildDocxWithImage(t, body, "word/media/image1.png", []byte("x"))
	out, _, err := Render(RenderInput{Template: tmpl, Vars: map[string]string{"EMPRESA": "A & B <test>"}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	doc := string(unzipEntry(t, out, "word/document.xml"))
	if !strings.Contains(doc, "A &amp; B &lt;test&gt;") {
		t.Fatalf("value not XML-escaped:\n%s", doc)
	}
}

func TestRenderLeavesUnknownTokenUntouched(t *testing.T) {
	body := `<w:p><w:r><w:t>{{EMPRESA}} {{FOO}}</w:t></w:r></w:p>`
	tmpl := buildDocxWithImage(t, body, "word/media/image1.png", []byte("x"))
	out, _, err := Render(RenderInput{Template: tmpl, Vars: map[string]string{"EMPRESA": "ACME"}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	doc := string(unzipEntry(t, out, "word/document.xml"))
	if !strings.Contains(doc, "ACME {{FOO}}") {
		t.Fatalf("expected ACME and untouched {{FOO}}:\n%s", doc)
	}
}

// TestRenderMinSpanPreservesFormatting (M6) verifies a formatted run that holds
// no token text keeps its content/formatting; only the runs the fragmented
// token spans are rewritten.
func TestRenderMinSpanPreservesFormatting(t *testing.T) {
	body := `<w:p>` +
		`<w:r><w:rPr><w:b/></w:rPr><w:t>BOLD</w:t></w:r>` +
		`<w:r><w:t>{{EMP</w:t></w:r>` +
		`<w:r><w:t>RESA}}</w:t></w:r>` +
		`</w:p>`
	tmpl := buildDocxWithImage(t, body, "word/media/image1.png", []byte("x"))
	out, _, err := Render(RenderInput{Template: tmpl, Vars: map[string]string{"EMPRESA": "ACME"}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	doc := string(unzipEntry(t, out, "word/document.xml"))
	if !strings.Contains(doc, `<w:b/></w:rPr><w:t>BOLD</w:t>`) {
		t.Fatalf("bold run not preserved:\n%s", doc)
	}
	if !strings.Contains(doc, "ACME") {
		t.Fatalf("token not replaced:\n%s", doc)
	}
	if strings.Contains(doc, "BOLDACME") {
		t.Fatalf("whole-paragraph collapse happened (formatting lost):\n%s", doc)
	}
	if strings.Contains(doc, "{{") {
		t.Fatalf("unresolved token remains:\n%s", doc)
	}
}

// TestRenderFingerprintMatchesStandalone locks in I-4: the fingerprint Render
// computes from its in-memory parts must be byte-identical to Fingerprint() of
// the produced DOCX (including media swaps), so dropping the second unzip never
// changes the stored/audited fingerprint.
func TestRenderFingerprintMatchesStandalone(t *testing.T) {
	body := `<w:p><w:r><w:t>{{EMPRESA}}</w:t></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:docPr id="1" name="figura_1"/><a:blip r:embed="rId5"/></w:drawing></w:r></w:p>`
	tmpl := buildDocxWithImage(t, body, "word/media/image1.png", []byte("OLD-IMAGE-BYTES"))
	out, fp, err := Render(RenderInput{
		Template: tmpl,
		Vars:     map[string]string{"EMPRESA": "ACME"},
		Images:   map[string][]byte{"figura_1": []byte("NEW-IMAGE-BYTES")},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want, err := Fingerprint(out)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	if fp != want {
		t.Fatalf("Render fingerprint %q != Fingerprint(out) %q", fp, want)
	}
}

// TestSlotIsImageBacked verifies the I-1 detection: a drawing with r:embed is
// image-backed; a slot without r:embed (e.g. a native chart) is not.
func TestSlotIsImageBacked(t *testing.T) {
	body := `<w:p><w:r><w:drawing><wp:docPr id="1" name="figura_1"/><a:blip r:embed="rId5"/></w:drawing></w:r></w:p>` +
		`<w:p><w:r><w:drawing><wp:docPr id="2" descr="grafico_1"/><c:chart r:id="rId6"/></w:drawing></w:r></w:p>`
	tmpl := buildDocxWithImage(t, body, "word/media/image1.png", []byte("x"))
	if b, err := SlotIsImageBacked(tmpl, "figura_1"); err != nil || !b {
		t.Fatalf("figura_1 should be image-backed (b=%v err=%v)", b, err)
	}
	if b, err := SlotIsImageBacked(tmpl, "grafico_1"); err != nil || b {
		t.Fatalf("grafico_1 (native chart, no r:embed) should NOT be image-backed (b=%v err=%v)", b, err)
	}
}
