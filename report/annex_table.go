package report

import (
	"fmt"
	"sort"
	"strings"
)

// AnnexTableToken is the template marker (its own paragraph) that the render
// engine replaces with the native S9 annex table (CL-105). It supersedes the
// former figura_10 image slot.
const AnnexTableToken = "TABLA_ANEXO"

// annexTopN caps the annex table at the most severe recipients.
const annexTopN = 10

// annexEmailHeader is the third column's heading, accented: this is text delivered
// to a client and the rest of the document accents (Envío de Datos, Clic al Enlace).
//
// It deliberately does NOT match the Excel annex, whose heading is "Correo". That
// divergence is a recorded decision, not an oversight: the Excel is the full listing
// and keeps its own wording, so changing it would alter every future spreadsheet for
// a cosmetic gain.
const annexEmailHeader = "Correo Electrónico"

// Column widths in twips. The Estado column is narrow and fixed-width because its
// contents are four known labels; the email column takes the rest.
const (
	colWidthNum    = "700"
	colWidthEstado = "2400"
	colWidthEmail  = "5600"
)

// recipientSeverity ranks a recipient's Spanish status label by funnel severity so
// the annex lists the worst cases first: Envío de Datos > Clic al Enlace > Correo
// Abierto > Correo Ignorado. The annex shows the ten most exposed recipients, not
// only those who submitted: with few submitters the table still has to say who came
// closest. Mirrors recipientStatusLabel / statusStyleIndex so table, Excel and
// funnel classify identically.
func recipientSeverity(label string) int {
	switch label {
	case "Envío de Datos":
		return 4
	case "Clic al Enlace":
		return 3
	case "Correo Abierto":
		return 2
	case "Correo Ignorado":
		return 1
	default: // "Error de envío" and anything else sort last
		return 0
	}
}

// statusFillHex returns the Word cell shading (RRGGBB, no alpha) for a status label.
// It does NOT keep its own colour table: it reads statusFillARGB through the very
// same statusStyleIndex the Excel uses, so a colour changed for the spreadsheet is
// changed for the document in the same edit. Empty means no fill.
func statusFillHex(label string) string {
	i := statusStyleIndex(label)
	if i < 0 || i >= len(statusFillARGB) || statusFillARGB[i] == "" {
		return ""
	}
	return statusFillARGB[i][2:] // drop the ARGB alpha byte
}

// TopRecipientsBySeverity returns a stable copy of rows sorted by descending
// severity (worst first), then by email ascending, capped at n.
//
// The tie-break is not decoration: within one status the order must be reproducible,
// or two renders of the same campaign would list the same ten people differently and
// the fingerprint re-audit would flag a change that never happened.
//
// The rows come from the SAME per-recipient universe as the funnel and the Excel
// annex, which already excludes soft-deleted recipients (CL-102R).
func TopRecipientsBySeverity(rows []Recipient, n int) []Recipient {
	out := make([]Recipient, len(rows))
	copy(out, rows)
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := recipientSeverity(out[i].Status), recipientSeverity(out[j].Status)
		if si != sj {
			return si > sj
		}
		return out[i].Email < out[j].Email
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

var docxCellReplacer = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
)

func docxEsc(s string) string { return docxCellReplacer.Replace(s) }

// BuildAnnexTableXML builds the native Word table (<w:tbl>) for the S9 annex:
// three columns N | Estado | Correo Electrónico. The Estado cell always carries the
// status TEXT and the colour is only reinforcement — colour alone would lose the
// information in greyscale printing, for a colour-blind reader, or when the DOCX is
// converted to plain text. The colours come from statusFillARGB, the Excel's own
// table, so the two artefacts can never disagree about what red means.
//
// It lists at most annexTopN rows and nothing else: no overflow row, no "and N
// more". The annex is a sample of the most exposed recipients, and the complete
// listing is the attached Excel — which the surrounding paragraph already points to.
//
// Returns a self-contained <w:tbl> that replaces the {{TABLA_ANEXO}} marker
// paragraph; the w: namespace is declared on the document root.
func BuildAnnexTableXML(rows []Recipient) string {
	top := TopRecipientsBySeverity(rows, annexTopN)

	var sb strings.Builder
	sb.WriteString(`<w:tbl>`)
	sb.WriteString(`<w:tblPr><w:tblStyle w:val="TableGrid"/><w:tblW w:w="0" w:type="auto"/>`)
	sb.WriteString(`<w:tblBorders>`)
	for _, edge := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		fmt.Fprintf(&sb, `<w:%s w:val="single" w:sz="4" w:space="0" w:color="auto"/>`, edge)
	}
	sb.WriteString(`</w:tblBorders></w:tblPr>`)
	sb.WriteString(`<w:tblGrid><w:gridCol w:w="` + colWidthNum + `"/><w:gridCol w:w="` + colWidthEstado +
		`"/><w:gridCol w:w="` + colWidthEmail + `"/></w:tblGrid>`)

	// Header row (bold). Order and wording are fixed: N | Estado | Correo.
	sb.WriteString(`<w:tr>`)
	sb.WriteString(headerCell(colWidthNum, "N"))
	sb.WriteString(headerCell(colWidthEstado, "Estado"))
	sb.WriteString(headerCell(colWidthEmail, annexEmailHeader))
	sb.WriteString(`</w:tr>`)

	if len(top) == 0 {
		// Explicit empty state: an empty grid under a "Figura" caption reads as a
		// rendering failure, so the table says out loud that there is nothing to show.
		sb.WriteString(`<w:tr>`)
		sb.WriteString(dataCell(colWidthNum, "—", ""))
		sb.WriteString(dataCell(colWidthEstado, "—", ""))
		sb.WriteString(dataCell(colWidthEmail, "Sin destinatarios que mostrar", ""))
		sb.WriteString(`</w:tr>`)
	}
	for i, r := range top {
		sb.WriteString(`<w:tr>`)
		sb.WriteString(dataCell(colWidthNum, fmt.Sprintf("%d", i+1), ""))
		sb.WriteString(dataCell(colWidthEstado, r.Status, statusFillHex(r.Status)))
		sb.WriteString(dataCell(colWidthEmail, r.Email, ""))
		sb.WriteString(`</w:tr>`)
	}
	sb.WriteString(`</w:tbl>`)
	return sb.String()
}

func headerCell(width, text string) string {
	return fmt.Sprintf(
		`<w:tc><w:tcPr><w:tcW w:w="%s" w:type="dxa"/><w:shd w:val="clear" w:color="auto" w:fill="D9D9D9"/></w:tcPr>`+
			`<w:p><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p></w:tc>`,
		width, docxEsc(text))
}

func dataCell(width, text, fillHex string) string {
	shd := `<w:shd w:val="clear" w:color="auto" w:fill="auto"/>`
	if fillHex != "" {
		shd = fmt.Sprintf(`<w:shd w:val="clear" w:color="auto" w:fill="%s"/>`, fillHex)
	}
	return fmt.Sprintf(
		`<w:tc><w:tcPr><w:tcW w:w="%s" w:type="dxa"/>%s</w:tcPr>`+
			`<w:p><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p></w:tc>`,
		width, shd, docxEsc(text))
}
