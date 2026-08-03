package report

import (
	"regexp"
	"strings"
	"testing"
)

// TestAnnexTableRendersInRealTemplate is the E2E CL-105 could not claim before: it
// renders the ACTUAL corporate template (the CL-105 copy that declares
// {{TABLA_ANEXO}}) and asserts on the resulting document.xml, not on the generator's
// output. Every earlier annex test built a synthetic two-paragraph DOCX, which is
// exactly how CL-103 shipped "verified" while no real template declared its token.
func TestAnnexTableRendersInRealTemplate(t *testing.T) {
	const path = "../plantilla_reina.CL105-b.docx"
	tmpl := requirePlantilla(t, path)

	// The template must declare the token — otherwise the render below would pass
	// trivially by substituting nothing.
	insp, ierr := Inspect(tmpl)
	if ierr != nil {
		t.Fatalf("Inspect: %v", ierr)
	}
	declared := false
	for _, tok := range insp.Tokens {
		if tok == AnnexTableToken {
			declared = true
		}
	}
	if !declared {
		t.Fatalf("la plantilla CL-105 no declara {{%s}}: el E2E sería vacío (tokens=%v)",
			AnnexTableToken, insp.Tokens)
	}

	// A mixed set: two submitters, one click, one open, one ignored. The ranking is
	// what the annex is for, so the E2E must exercise the mix, not a single status.
	rows := []Recipient{
		{"ceci@empresa.com", "Correo Abierto"},
		{"ana@empresa.com", "Envío de Datos"},
		{"dario@empresa.com", "Correo Ignorado"},
		{"beto@empresa.com", "Envío de Datos"},
		{"elena@empresa.com", "Clic al Enlace"},
	}
	out, _, err := Render(RenderInput{
		Template:      tmpl,
		AnnexTableXML: BuildAnnexTableXML(rows),
		Vars:          map[string]string{"NUM_FIGURA_ANEXO": "10"},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	doc := string(unzipEntry(t, out, "word/document.xml"))

	if strings.Contains(doc, "TABLA_ANEXO") {
		t.Fatalf("el marcador se filtró al documento entregado")
	}
	if !strings.Contains(doc, "<w:tbl>") {
		t.Fatalf("no hay ninguna tabla nativa en el documento")
	}
	for _, want := range []string{
		"ana@empresa.com", "beto@empresa.com", "elena@empresa.com", "ceci@empresa.com", "dario@empresa.com",
		"Envío de Datos", "Clic al Enlace", "Correo Abierto", "Correo Ignorado",
		"N", "Estado", annexEmailHeader,
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("falta %q en el documento generado", want)
		}
	}
	// Severity order inside the delivered document, not just in the generator.
	tbl := doc[strings.Index(doc, "<w:tbl>"):]
	order := []string{"ana@empresa.com", "beto@empresa.com", "elena@empresa.com", "ceci@empresa.com", "dario@empresa.com"}
	for i := 1; i < len(order); i++ {
		if strings.Index(tbl, order[i-1]) > strings.Index(tbl, order[i]) {
			t.Fatalf("orden por severidad roto en el documento: %s debe ir antes que %s", order[i-1], order[i])
		}
	}
	// Every colour in the document must be the Excel colour for that state.
	for _, c := range []struct{ label, hex string }{
		{"Envío de Datos", "FF0000"}, {"Clic al Enlace", "F4A020"},
		{"Correo Abierto", "FFFF00"}, {"Correo Ignorado", "92D050"},
	} {
		if !strings.Contains(tbl, `w:fill="`+c.hex+`"`) {
			t.Fatalf("falta el relleno %s de %q en el documento", c.hex, c.label)
		}
	}
	// D3: the caption is still a "Figura" and its numbering token was substituted.
	if !strings.Contains(doc, "Figura ") || strings.Contains(doc, "NUM_FIGURA_ANEXO") {
		t.Fatalf("el pie de figura se perdió o su token no se sustituyó")
	}
	// El pie describe la tabla real: estados mezclados, no solo envíos de datos.
	if !strings.Contains(doc, "Evidencia de los destinatarios con mayor exposición") {
		t.Fatalf("el pie no es el literal acordado")
	}
	if strings.Contains(doc, "usuarios que enviaron datos, archivo adjunto") {
		t.Fatalf("el pie antiguo sigue ahí: describiría mal una tabla con estados mezclados")
	}
	// The old image slot is gone from this version — the table replaces it.
	if strings.Contains(doc, `descr="figura_10"`) {
		t.Fatalf("la plantilla CL-105 todavía lleva la imagen figura_10")
	}
	// The table must sit BEFORE its caption, or the caption would label the wrong thing.
	capIdx := strings.Index(doc, "Evidencia de los destinatarios con mayor exposición")
	if capIdx < 0 {
		t.Fatalf("no se encontró el pie de figura en el documento")
	}
	if strings.Index(doc, "<w:tbl>") > capIdx {
		t.Fatalf("la tabla quedó después de su pie de figura")
	}
	if regexp.MustCompile(`<w:p>\s*</w:p>`).MatchString(doc) {
		t.Fatalf("párrafo vacío residual alrededor de la tabla")
	}
	t.Logf("MEDIDO: DOCX real %d bytes · filas <w:tr>=%d (1 cabecera + 5 destinatarios, mezcla de 4 estados)",
		len(out), strings.Count(doc[strings.Index(doc, "<w:tbl>"):], "<w:tr>"))
}
