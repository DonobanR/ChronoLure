package report

import (
	"strings"
	"testing"
	"time"
)

// date is a small helper for building fixed dates in tests.
func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestBloque2FAFound(t *testing.T) {
	got := Bloque2FA(2, true)
	if !strings.Contains(got, "los 2 casos") || !strings.Contains(got, "mitigada por la presencia de doble") {
		t.Fatalf("2FA found unexpected: %q", got)
	}
}

func TestBloque2FANotFound(t *testing.T) {
	if got := Bloque2FA(2, false); got != "" {
		t.Fatalf("no 2FA must produce no MFA block, got %q", got)
	}
}

func TestBloque2FASingularCaso(t *testing.T) {
	got := Bloque2FA(1, true)
	if !strings.Contains(got, "el 1 caso") {
		t.Fatalf("expected singular phrasing, got %q", got)
	}
}

func TestBloque2FADependsOnlyOnFlag(t *testing.T) {
	// El bloque depende del checkbox, no del conteo: con 2FA siempre aparece.
	if got := Bloque2FA(0, true); got == "" {
		t.Fatalf("2FA marcado debe producir el bloque aunque submitted=0")
	}
	if got := Bloque2FA(5, false); got != "" {
		t.Fatalf("sin 2FA debe ser vacío, got %q", got)
	}
}
