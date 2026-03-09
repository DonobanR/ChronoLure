package models

import (
	"strings"
	"testing"
)

func TestFormatFromHeader(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		// Non-ASCII names are transliterated to ASCII (no encoded-words, no UTF-8 bytes).
		{"Comunicaci\u00f3n Empresarial", "info@empresa.com",
			"Comunicacion Empresarial <info@empresa.com>"},
		{"Actualizaci\u00f3n 2026", "soporte@empresa.com",
			"Actualizacion 2026 <soporte@empresa.com>"},
		{"\u00c1rea de Soporte", "soporte@empresa.com",
			"Area de Soporte <soporte@empresa.com>"},
		{"Jos\u00e9 \u00d1o\u00f1o", "jose@empresa.com",
			"Jose Nono <jose@empresa.com>"},
		// Pure ASCII, no specials.
		{"Microsoft Teams", "noreply@teams.microsoft.com",
			"Microsoft Teams <noreply@teams.microsoft.com>"},
		// Contains RFC 5322 special (comma) → must be quoted.
		{"Soporte, Empresa", "soporte@empresa.com",
			`"Soporte, Empresa" <soporte@empresa.com>`},
		// Empty name → just the address.
		{"", "solo@empresa.com", "solo@empresa.com"},
	}

	for _, tt := range tests {
		got := formatFromHeader(tt.address, tt.name)

		// Must NEVER produce any RFC 2047 encoded-word (breaks old Outlook).
		if strings.Contains(got, "=?") {
			t.Errorf("formatFromHeader(%q): produced RFC-2047 encoded-word: %s", tt.name, got)
			continue
		}

		// Must NEVER contain non-ASCII bytes (breaks strict SMTP servers).
		for i, b := range []byte(got) {
			if b > 127 {
				t.Errorf("formatFromHeader(%q): non-ASCII byte 0x%02X at pos %d: %s", tt.name, b, i, got)
				break
			}
		}

		if got != tt.want {
			t.Errorf("formatFromHeader(%q):\n  got : %s\n  want: %s", tt.name, got, tt.want)
			continue
		}
		t.Logf("OK  %s", got)
	}
}

