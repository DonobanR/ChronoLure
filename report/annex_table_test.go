package report

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// submitters builds n recipients that all submitted data, with emails that are
// deliberately NOT in insertion order, so an "ordered" assertion cannot pass by
// accident on input order.
func submitters(n int) []Recipient {
	rows := make([]Recipient, 0, n)
	for i := n - 1; i >= 0; i-- {
		rows = append(rows, Recipient{Email: fmt.Sprintf("u%02d@x.com", i), Status: "Envío de Datos"})
	}
	return rows
}

// TestAnnexTableRanksBySeverity pins the content rule: the annex shows the ten most
// EXPOSED recipients, not only those who submitted. With few submitters the table
// still has to say who came closest, so the ranking is
// Datos > Clic > Abierto > Ignorado, and only then alphabetical.
func TestAnnexTableRanksBySeverity(t *testing.T) {
	rows := []Recipient{
		{"z@x.com", "Correo Ignorado"},
		{"a@x.com", "Envío de Datos"},
		{"b@x.com", "Correo Abierto"},
		{"c@x.com", "Clic al Enlace"},
		{"d@x.com", "Envío de Datos"},
	}
	got := TopRecipientsBySeverity(rows, 10)
	want := []string{"a@x.com", "d@x.com", "c@x.com", "b@x.com", "z@x.com"}
	for i := range want {
		if got[i].Email != want[i] {
			t.Fatalf("pos %d = %s, want %s (order=%v)", i, got[i].Email, want[i], got)
		}
	}

	// The mixed case the rule exists for: 7 submitters must be followed by the next
	// most severe, not by nothing.
	mixed := submitters(7)
	for i := 0; i < 4; i++ {
		mixed = append(mixed, Recipient{Email: fmt.Sprintf("c%d@x.com", i), Status: "Clic al Enlace"})
	}
	mixed = append(mixed, Recipient{Email: "ig@x.com", Status: "Correo Ignorado"})
	top := TopRecipientsBySeverity(mixed, annexTopN)
	if len(top) != 10 {
		t.Fatalf("expected 10 rows, got %d", len(top))
	}
	for i := 0; i < 7; i++ {
		if top[i].Status != "Envío de Datos" {
			t.Fatalf("row %d should be a submitter, got %+v", i, top[i])
		}
	}
	for i := 7; i < 10; i++ {
		if top[i].Status != "Clic al Enlace" {
			t.Fatalf("row %d should be the next severity down (Clic), got %+v", i, top[i])
		}
	}
	if strings.Contains(BuildAnnexTableXML(mixed), "ig@x.com") {
		t.Fatalf("a merely-ignored recipient displaced someone more exposed")
	}
}

// TestAnnexTableOrdersByEmailAscending pins the tie-break WITHIN one status. Without
// it two renders of the same campaign could list the same ten people in a different
// order and the fingerprint re-audit would flag a change that never happened.
func TestAnnexTableOrdersByEmailAscending(t *testing.T) {
	got := TopRecipientsBySeverity(submitters(5), 10)
	want := []string{"u00@x.com", "u01@x.com", "u02@x.com", "u03@x.com", "u04@x.com"}
	for i := range want {
		if got[i].Email != want[i] {
			t.Fatalf("pos %d = %s, want %s (got=%v)", i, got[i].Email, want[i], got)
		}
	}
}

// TestAnnexTableCapsAtTenWithoutOverflow: at most ten rows, and NOTHING else. The
// complete listing is the attached Excel, which the surrounding paragraph points to.
func TestAnnexTableCapsAtTenWithoutOverflow(t *testing.T) {
	xml := BuildAnnexTableXML(submitters(17))
	if n := strings.Count(xml, "<w:tr>"); n != annexTopN+1 {
		t.Fatalf("expected %d rows (header + 10), got %d:\n%s", annexTopN+1, n, xml)
	}
	for _, forbidden := range []string{"más", "destinatarios m", "…"} {
		if strings.Contains(xml, forbidden) {
			t.Fatalf("overflow row leaked back in (%q):\n%s", forbidden, xml)
		}
	}
	if strings.Contains(xml, "u10@x.com") {
		t.Fatalf("11th recipient rendered despite the top-10 cap")
	}
}

func TestAnnexTableEmptyWhenNoRecipients(t *testing.T) {
	xml := BuildAnnexTableXML(nil)
	if !strings.Contains(xml, "Sin destinatarios que mostrar") {
		t.Fatalf("expected explicit empty state, got:\n%s", xml)
	}
	if n := strings.Count(xml, "<w:tr>"); n != 2 {
		t.Fatalf("expected header + 1 empty-state row, got %d", n)
	}
	// With recipients of ANY status the table must NOT be empty any more.
	some := BuildAnnexTableXML([]Recipient{{"a@x.com", "Correo Abierto"}})
	if strings.Contains(some, "Sin destinatarios") || !strings.Contains(some, "a@x.com") {
		t.Fatalf("a recipient who only opened the mail must still be listed:\n%s", some)
	}
}

// TestAnnexTableEstadoHasTextNotOnlyColor: the status must survive greyscale
// printing, a colour-blind reader and a text extraction of the DOCX. The fill is
// reinforcement, never the carrier of the information (WCAG 1.4.1).
func TestAnnexTableEstadoHasTextNotOnlyColor(t *testing.T) {
	xml := BuildAnnexTableXML([]Recipient{
		{"a@x.com", "Envío de Datos"},
		{"b@x.com", "Clic al Enlace"},
		{"c@x.com", "Correo Abierto"},
		{"d@x.com", "Correo Ignorado"},
	})
	for _, label := range []string{"Envío de Datos", "Clic al Enlace", "Correo Abierto", "Correo Ignorado"} {
		if !strings.Contains(xml, `<w:t xml:space="preserve">`+label+`</w:t>`) {
			t.Fatalf("status %q is not present as TEXT:\n%s", label, xml)
		}
	}
	noColour := regexp.MustCompile(`<w:shd[^>]*/>`).ReplaceAllString(xml, "")
	for _, label := range []string{"Envío de Datos", "Correo Ignorado"} {
		if !strings.Contains(noColour, label) {
			t.Fatalf("without colour the status %q disappears — colour is carrying the meaning", label)
		}
	}
}

// TestAnnexTableColorsMatchExcelMapping is the anti-drift test: the Word table must
// not keep a private colour table. Both artefacts are painted from statusFillARGB
// through statusStyleIndex, so this fails the moment someone hardcodes a colour.
func TestAnnexTableColorsMatchExcelMapping(t *testing.T) {
	cases := []struct{ label, argb string }{
		{"Envío de Datos", "FFFF0000"},
		{"Clic al Enlace", "FFF4A020"},
		{"Correo Abierto", "FFFFFF00"},
		{"Correo Ignorado", "FF92D050"},
	}
	for _, c := range cases {
		// The Excel's own style table must hold this colour at this status's index.
		idx := statusStyleIndex(c.label)
		if statusFillARGB[idx] != c.argb {
			t.Fatalf("Excel mapping for %q is %s, expected %s", c.label, statusFillARGB[idx], c.argb)
		}
		// And the Word cell must be that same colour minus the alpha byte.
		if got := statusFillHex(c.label); got != c.argb[2:] {
			t.Fatalf("Word fill for %q is %s, Excel says %s", c.label, got, c.argb[2:])
		}
		xml := BuildAnnexTableXML([]Recipient{{"a@x.com", c.label}})
		if !strings.Contains(xml, `w:fill="`+c.argb[2:]+`"`) {
			t.Fatalf("rendered table does not use the Excel colour for %q:\n%s", c.label, xml)
		}
	}
	// A state with no fill in the Excel must have none in Word either.
	if got := statusFillHex("Error de envío"); got != "" {
		t.Fatalf("expected no fill for 'Error de envío', got %q", got)
	}
}

// TestAnnexTableHasThreeColumns pins the shape: N | Estado | Correo, in that order.
func TestAnnexTableHasThreeColumns(t *testing.T) {
	xml := BuildAnnexTableXML(submitters(3))
	if n := strings.Count(xml, "<w:gridCol"); n != 3 {
		t.Fatalf("expected a 3-column grid, got %d", n)
	}
	headers := regexp.MustCompile(`<w:b/></w:rPr><w:t xml:space="preserve">([^<]*)</w:t>`).FindAllStringSubmatch(xml, -1)
	var got []string
	for _, h := range headers {
		got = append(got, h[1])
	}
	want := []string{"N", "Estado", annexEmailHeader}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("headers are %v, want %v", got, want)
	}
	// Estado must come BEFORE the email in every data row.
	row := strings.Split(xml, "<w:tr>")[2]
	if strings.Index(row, "Envío de Datos") > strings.Index(row, "u00@x.com") {
		t.Fatalf("column order wrong: Estado must precede the email:\n%s", row)
	}
	for _, r := range strings.Split(xml, "<w:tr>")[1:] {
		if n := strings.Count(r, "<w:tc>"); n != 3 {
			t.Fatalf("row with %d cells, expected 3: %s", n, r)
		}
	}
}

func TestBuildAnnexTableXMLEscapes(t *testing.T) {
	xml := BuildAnnexTableXML([]Recipient{{`a<b>&"'@x.com`, "Envío de Datos"}})
	if strings.Contains(xml, "<b>") || !strings.Contains(xml, "&lt;b&gt;") {
		t.Fatalf("email not XML-escaped:\n%s", xml)
	}
}

// TestRenderReplacesAnnexTableMarker verifies the {{TABLA_ANEXO}} paragraph is
// swapped for the native table and no marker leaks into the output.
func TestRenderReplacesAnnexTableMarker(t *testing.T) {
	body := `<w:p><w:r><w:t>antes</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>{{TABLA_ANEXO}}</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>despues</w:t></w:r></w:p>`
	tmpl := buildDocxWithImage(t, body, "word/media/image1.png", []byte("x"))
	table := BuildAnnexTableXML([]Recipient{{"u@x.com", "Envío de Datos"}})
	out, _, err := Render(RenderInput{Template: tmpl, AnnexTableXML: table})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	doc := string(unzipEntry(t, out, "word/document.xml"))
	if strings.Contains(doc, "TABLA_ANEXO") {
		t.Fatalf("marker leaked into output:\n%s", doc)
	}
	if !strings.Contains(doc, "<w:tbl>") || !strings.Contains(doc, "u@x.com") {
		t.Fatalf("table not injected:\n%s", doc)
	}
	// Surrounding paragraphs preserved.
	if !strings.Contains(doc, "antes") || !strings.Contains(doc, "despues") {
		t.Fatalf("surrounding content lost:\n%s", doc)
	}
	// The marker's own paragraph must be gone (no empty <w:p></w:p> leftovers around the table).
	if regexp.MustCompile(`<w:p>\s*</w:p>`).MatchString(doc) {
		t.Fatalf("empty paragraph leftover:\n%s", doc)
	}
	// Output still a valid zip.
	if _, err := zip.NewReader(bytes.NewReader(out), int64(len(out))); err != nil {
		t.Fatalf("invalid docx: %v", err)
	}
}
