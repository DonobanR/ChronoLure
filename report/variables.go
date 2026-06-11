package report

import (
	"fmt"
	"strconv"
	"time"
)

// VarInput is the fully-resolved, snapshot-ready set of values needed to build
// the token map. The caller assembles it from the report's manual fields plus
// the metrics derived from a ReportSource; BuildVars itself is pure.
type VarInput struct {
	CompanyName     string
	PreparedBy      string
	ReportDate      time.Time
	ExecutedFrom    time.Time
	ExecutedTo      time.Time
	IntroExec       string
	TextPunto1      string
	Had2FA          bool
	TotalRecipients int64
	Funnel          FunnelMetrics
}

// BuildVars produces the complete token map consumed by Render. Every entry
// corresponds to a token in Vocabulary; numbers come from the frozen funnel and
// the manual 2FA count, never from live campaign state.
func BuildVars(in VarInput) map[string]string {
	f := in.Funnel
	// Número de la figura del anexo: la numeración debe ser consecutiva. Hay 7
	// figuras fijas (1..7); la 8 solo existe con 2FA; por tanto la figura del
	// anexo es la 9 (con 2FA) o la 8 (sin 2FA), cerrando el salto original 8->10.
	anexo := "8"
	if in.Had2FA {
		anexo = "9"
	}
	// Conclusión (Sección 8) coherente con el escenario real de 2FA:
	// - Con 2FA: se conserva la frase y el párrafo de cierre originales.
	// - Sin 2FA: la frase desaparece y el párrafo de cierre se reemplaza.
	fraseConclusion := ""
	parrafoCierre := "Ante la falta de controles adicionales como el 2FA, es fundamental reconocer que la ausencia de esta medida incrementa el nivel de riesgo global. Por ello, resulta imprescindible implementar este mecanismo y complementarlo con defensas adicionales, tales como:"
	if in.Had2FA {
		fraseConclusion = ", incluso pese a contar con configuraciones de doble factor de autenticación (2FA)"
		parrafoCierre = "Si bien es positivo que existan controles adicionales como el 2FA, es fundamental reconocer que ninguna medida por sí sola garantiza la protección absoluta. Por ello, resulta imprescindible complementar estos mecanismos con defensas adicionales, tales como:"
	}
	return map[string]string{
		"NUM_FIGURA_ANEXO":    anexo,
		"FRASE_2FA_CONCLUSION": fraseConclusion,
		"PARRAFO_CIERRE_2FA":   parrafoCierre,
		"EMPRESA":           in.CompanyName,
		"ELABORADO_POR":     in.PreparedBy,
		"FECHA_INFORME":     fechaES(in.ReportDate),
		"FECHA_EJECUCION":   rangoFechasES(in.ExecutedFrom, in.ExecutedTo),
		"RANGO_FECHAS":      rangoFechasES(in.ExecutedFrom, in.ExecutedTo),
		"N_USUARIOS":        itoa(in.TotalRecipients),
		"PARRAFO_EJECUCION": in.IntroExec,
		"TEXTO_PUNTO_1":     in.TextPunto1,
		"R_IGNORADO":        itoa(f.Ignorado),
		"R_ABIERTO":         itoa(f.Abierto),
		"R_CLIC":            itoa(f.Clic),
		"R_DATOS":           itoa(f.Datos),
		"R_TOTAL":           itoa(f.Total),
		"PCT_DATOS":         strconv.Itoa(f.PctDatos) + "%",
		"N_DATOS":           itoa(f.Datos),
		"N_ENVIO_DATOS":     itoa(f.Datos),
		"N_ENVIO_DATOS_PAD": fmt.Sprintf("%02d", f.Datos),
		"BLOQUE_2FA":        Bloque2FA(f.Datos, in.Had2FA),
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
