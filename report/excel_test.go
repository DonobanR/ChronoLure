package report

import (
	"archive/zip"
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// TestBuildRecipientsXLSX checks the workbook is a valid zip with the expected
// parts, header and per-state cell coloring (verde/tomate/rojo).
func TestBuildRecipientsXLSX(t *testing.T) {
	rows := []Recipient{
		{Email: "ignorado@x.com", Status: "Correo Ignorado"},
		{Email: "clic@x.com", Status: "Clic al Enlace"},
		{Email: "datos@x.com", Status: "Envío de Datos"},
		{Email: "abierto@x.com", Status: "Correo Abierto"},
	}
	b, err := BuildRecipientsXLSX(rows)
	if err != nil {
		t.Fatalf("BuildRecipientsXLSX: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("not a valid zip: %v", err)
	}
	parts := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, e := rc.Read(buf)
			sb.Write(buf[:n])
			if e != nil {
				break
			}
		}
		rc.Close()
		parts[f.Name] = sb.String()
	}
	for _, want := range []string{"[Content_Types].xml", "xl/workbook.xml", "xl/styles.xml", "xl/worksheets/sheet1.xml"} {
		if _, ok := parts[want]; !ok {
			t.Fatalf("missing part %q", want)
		}
	}
	sheet := parts["xl/worksheets/sheet1.xml"]
	for _, h := range []string{"No.", "Correo", "Estado"} {
		if !strings.Contains(sheet, ">"+h+"<") {
			t.Errorf("header %q not found", h)
		}
	}
	// Each state's Estado cell must carry the right style index: 2=verde,
	// 3=tomate, 4=rojo. Correo Abierto carries no fill (style 0 / absent).
	type want struct{ status, style string }
	for _, w := range []want{
		{"Correo Ignorado", "2"},
		{"Clic al Enlace", "3"},
		{"Envío de Datos", "4"},
		{"Correo Abierto", "5"},
	} {
		re := regexp.MustCompile(`<c r="C\d+" s="` + w.style + `"[^>]*><is><t[^>]*>` + regexp.QuoteMeta(w.status) + `</t>`)
		if !re.MatchString(sheet) {
			t.Errorf("state %q expected style %s, not found", w.status, w.style)
		}
	}
	// 1 header + 4 data rows.
	if n := strings.Count(sheet, "<row "); n != 5 {
		t.Errorf("expected 5 rows, got %d", n)
	}
}

func TestRecipientStatusLabel(t *testing.T) {
	cases := []struct {
		best     int
		hadError bool
		want     string
	}{
		{4, false, "Envío de Datos"},
		{3, false, "Clic al Enlace"},
		{2, false, "Correo Abierto"},
		{1, false, "Correo Ignorado"},
		{0, true, "Error de envío"},
	}
	for _, c := range cases {
		if got := recipientStatusLabel(c.best, c.hadError); got != c.want {
			t.Errorf("recipientStatusLabel(%d,%v)=%q want %q", c.best, c.hadError, got, c.want)
		}
	}
}
