package report

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// Recipient is one row of the per-recipient state listing (Excel annex):
// the email and the Spanish label of the furthest funnel stage it reached.
// The classification is the SAME one the results table uses (see rank); this
// file only formats it, it never invents or transforms states.
type Recipient struct {
	Email  string
	Status string
}

// recipientStatusLabel maps the furthest funnel rank reached by a recipient to
// the Spanish label used across the report. Ranks come from rank() in
// metrics.go, so the Excel mirrors the results table exactly.
func recipientStatusLabel(best int, hadError bool) string {
	switch best {
	case 4:
		return "Envío de Datos"
	case 3:
		return "Clic al Enlace"
	case 2:
		return "Correo Abierto"
	case 1:
		return "Correo Ignorado"
	default:
		if hadError {
			return "Error de envío"
		}
		return "Correo Ignorado"
	}
}

// statusStyleIndex returns the cell style index (defined in xlsxStyles) used to
// color the Estado cell: Correo Ignorado→verde, Correo Abierto→amarillo,
// Clic al Enlace→#F4A020, Envío de Datos→rojo. Any other state has no fill.
func statusStyleIndex(label string) int {
	switch label {
	case "Correo Ignorado":
		return 2 // verde
	case "Clic al Enlace":
		return 3 // #F4A020
	case "Envío de Datos":
		return 4 // rojo
	case "Correo Abierto":
		return 5 // amarillo
	default:
		return 0 // sin relleno
	}
}

// xlsxReplacer is built once (audit M-5) instead of per cell.
var xlsxReplacer = strings.NewReplacer(
	"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;",
)

func xlsxEsc(s string) string { return xlsxReplacer.Replace(s) }

// BuildRecipientsXLSX produces a minimal, valid .xlsx (SpreadsheetML) with one
// sheet: No. | Correo | Estado, the Estado cell colored by state.
func BuildRecipientsXLSX(rows []Recipient) ([]byte, error) {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	sb.WriteString(`<cols><col min="1" max="1" width="6" customWidth="1"/><col min="2" max="2" width="42" customWidth="1"/><col min="3" max="3" width="18" customWidth="1"/></cols>`)
	sb.WriteString(`<sheetData>`)
	sb.WriteString(`<row r="1">`)
	sb.WriteString(`<c r="A1" s="1" t="inlineStr"><is><t>No.</t></is></c>`)
	sb.WriteString(`<c r="B1" s="1" t="inlineStr"><is><t>Correo</t></is></c>`)
	sb.WriteString(`<c r="C1" s="1" t="inlineStr"><is><t>Estado</t></is></c>`)
	sb.WriteString(`</row>`)
	for i, rr := range rows {
		rn := i + 2
		fmt.Fprintf(&sb, `<row r="%d">`, rn)
		fmt.Fprintf(&sb, `<c r="A%d"><v>%d</v></c>`, rn, i+1)
		fmt.Fprintf(&sb, `<c r="B%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, rn, xlsxEsc(rr.Email))
		fmt.Fprintf(&sb, `<c r="C%d" s="%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, rn, statusStyleIndex(rr.Status), xlsxEsc(rr.Status))
		sb.WriteString(`</row>`)
	}
	sb.WriteString(`</sheetData></worksheet>`)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", xlsxContentTypes},
		{"_rels/.rels", xlsxRootRels},
		{"xl/workbook.xml", xlsxWorkbook},
		{"xl/_rels/workbook.xml.rels", xlsxWorkbookRels},
		{"xl/styles.xml", xlsxStyles},
		{"xl/worksheets/sheet1.xml", sb.String()},
	}
	for _, p := range parts {
		w, err := zw.Create(p.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

const xlsxContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
</Types>`

const xlsxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`

const xlsxWorkbook = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="Resultados" sheetId="1" r:id="rId1"/></sheets>
</workbook>`

const xlsxWorkbookRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`

// statusFillARGB is the ONE colour table for recipient states, shared by the Excel
// annex and by the Word annex table (CL-105). Its index is exactly what
// statusStyleIndex returns, which is also the xlsx fill id — so both artefacts are
// painted from this slice and a colour can never drift between them. Indexes 0/1 are
// reserved by the spec (none/gray125) and stay empty.
//
// Values are ARGB, as SpreadsheetML requires; the DOCX takes the RRGGBB tail.
var statusFillARGB = []string{
	2: "FF92D050", // Correo Ignorado — verde
	3: "FFF4A020", // Clic al Enlace
	4: "FFFF0000", // Envío de Datos — rojo
	5: "FFFFFF00", // Correo Abierto — amarillo
}

// xlsxStyles defines the fonts, fills and cell formats. The coloured fills are
// generated from statusFillARGB rather than written out by hand, so editing that
// slice repaints the Excel and the Word table together.
// cellXfs: 0=plain, 1=bold header, 2/3/4/5=colored.
var xlsxStyles = buildXlsxStyles()

func buildXlsxStyles() string {
	var fills strings.Builder
	fills.WriteString(`<fill><patternFill patternType="none"/></fill>` + "\n")
	fills.WriteString(`<fill><patternFill patternType="gray125"/></fill>` + "\n")
	for i := 2; i < len(statusFillARGB); i++ {
		fmt.Fprintf(&fills,
			`<fill><patternFill patternType="solid"><fgColor rgb="%s"/><bgColor indexed="64"/></patternFill></fill>`+"\n",
			statusFillARGB[i])
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>
<fills count="` + fmt.Sprint(len(statusFillARGB)) + `">
` + fills.String() + `</fills>
<borders count="1"><border/></borders>` + xlsxStylesTail
}

const xlsxStylesTail = `
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="6">
<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>
<xf numFmtId="0" fontId="0" fillId="2" borderId="0" xfId="0" applyFill="1"/>
<xf numFmtId="0" fontId="0" fillId="3" borderId="0" xfId="0" applyFill="1"/>
<xf numFmtId="0" fontId="0" fillId="4" borderId="0" xfId="0" applyFill="1"/>
<xf numFmtId="0" fontId="0" fillId="5" borderId="0" xfId="0" applyFill="1"/>
</cellXfs>
<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`
