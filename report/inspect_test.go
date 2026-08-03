package report

import (
	"archive/zip"
	"bytes"
	"testing"
)

// buildDocx assembles a minimal but valid .docx (a zip with the required parts)
// whose body is the provided XML. No corporate content is involved; the body is
// synthesized entirely from the arguments.
func buildDocx(t *testing.T, bodyXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	add("[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"/>`)
	add("word/document.xml", `<?xml version="1.0"?><w:document xmlns:w="x"><w:body>`+bodyXML+`</w:body></w:document>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func run(text string) string { return `<w:r><w:t>` + text + `</w:t></w:r>` }

// fullValidBody emits every required token plus every required slot via
// Alt-Text (descr), producing a template that should pass validation.
func fullValidBody() string {
	body := ""
	for _, tok := range RequiredTokens {
		body += run("{{" + tok + "}}")
	}
	body += run("{{ELABORADO_POR}}") + run("{{BLOQUE_2FA}}")
	for _, slot := range RequiredSlots() {
		body += `<w:drawing><wp:docPr id="1" name="Picture" descr="` + slot + `"/></w:drawing>`
	}
	return body
}

func TestInspectValidTemplate(t *testing.T) {
	insp, err := Inspect(buildDocx(t, fullValidBody()))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !insp.Valid {
		t.Fatalf("expected valid template, got %+v", insp)
	}
	if !contains(insp.ImageSlots, "figura_1") || !contains(insp.ImageSlots, "grafico_1") {
		t.Fatalf("expected figura_1 and grafico_1 slots, got %v", insp.ImageSlots)
	}
}

// TestInspectSlotByName proves the name attribute still works as a fallback
// identifier when Alt-Text (descr) is absent.
func TestInspectSlotByName(t *testing.T) {
	body := `<w:drawing><wp:docPr id="1" name="figura_7"/></w:drawing>`
	insp, _ := Inspect(buildDocx(t, body))
	if !contains(insp.ImageSlots, "figura_7") {
		t.Fatalf("expected figura_7 discovered by name, got %v", insp.ImageSlots)
	}
}

// TestInspectMissingRequiredSlot rejects a template missing a required slot.
func TestInspectMissingRequiredSlot(t *testing.T) {
	// Full body minus figura_8 (drop it from the required-slot loop).
	body := ""
	for _, tok := range RequiredTokens {
		body += run("{{" + tok + "}}")
	}
	for _, slot := range RequiredSlots() {
		if slot == "figura_8" {
			continue
		}
		body += `<w:drawing><wp:docPr id="1" name="Picture" descr="` + slot + `"/></w:drawing>`
	}
	insp, _ := Inspect(buildDocx(t, body))
	if insp.Valid {
		t.Fatalf("expected invalid (missing figura_8), got valid")
	}
	if !contains(insp.MissingRequiredSlot, "figura_8") {
		t.Fatalf("expected figura_8 in missing required slots, got %v", insp.MissingRequiredSlot)
	}
}

// TestInspectDuplicateSlot rejects a template that declares a slot twice.
func TestInspectDuplicateSlot(t *testing.T) {
	body := fullValidBody() + `<w:drawing><wp:docPr id="2" name="Picture" descr="figura_1"/></w:drawing>`
	insp, _ := Inspect(buildDocx(t, body))
	if insp.Valid {
		t.Fatalf("expected invalid (duplicate figura_1), got valid")
	}
	if !contains(insp.DuplicateSlots, "figura_1") {
		t.Fatalf("expected figura_1 in duplicate slots, got %v", insp.DuplicateSlots)
	}
}

// TestInspectReassemblesFragmentedToken proves run-fragmented placeholders are
// detected: Word may split {{EMPRESA}} across runs.
func TestInspectReassemblesFragmentedToken(t *testing.T) {
	body := `<w:r><w:t>{{EMP</w:t></w:r><w:r><w:t>RESA}}</w:t></w:r>`
	insp, err := Inspect(buildDocx(t, body))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !contains(insp.Tokens, "EMPRESA") {
		t.Fatalf("fragmented token EMPRESA not reassembled, tokens=%v", insp.Tokens)
	}
}

func TestInspectMissingRequiredAndUnknown(t *testing.T) {
	// Only one required token present, plus an unknown token.
	body := run("{{R_TOTAL}}") + run("{{FOO}}")
	insp, err := Inspect(buildDocx(t, body))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Valid {
		t.Fatalf("expected invalid template, got valid: %+v", insp)
	}
	if !contains(insp.Unknown, "FOO") {
		t.Fatalf("expected FOO in unknown, got %v", insp.Unknown)
	}
	if !contains(insp.MissingRequired, "EMPRESA") {
		t.Fatalf("expected EMPRESA in missing required, got %v", insp.MissingRequired)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestInspectKnowsExecutionTokens covers CL-103: {{DEPTO_SUPLANTADO}} is a
// known token, never reported as unknown.
func TestInspectKnowsExecutionTokens(t *testing.T) {
	insp, err := Inspect(buildDocx(t, run("{{SUPLANTANDO}} {{COMUNICADO}}")))
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !contains(insp.Tokens, "SUPLANTANDO") || !contains(insp.Tokens, "COMUNICADO") {
		t.Fatalf("expected SUPLANTANDO+COMUNICADO in tokens, got %v", insp.Tokens)
	}
	if contains(insp.Unknown, "SUPLANTANDO") || contains(insp.Unknown, "COMUNICADO") {
		t.Fatalf("SUPLANTANDO/COMUNICADO should be known tokens, got Unknown=%v", insp.Unknown)
	}
}
