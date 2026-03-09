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
		// Non-ASCII names must be raw UTF-8 — NO encoded-words whatsoever.
		{"Comunicaci\u00f3n Empresarial!", "banrural@comunicacion-empresarial.com",
			"Comunicaci\u00f3n Empresarial! <banrural@comunicacion-empresarial.com>"},
		{"Actualizaci\u00f3n 2026", "soporte@empresa.com",
			"Actualizaci\u00f3n 2026 <soporte@empresa.com>"},
		{"\u00c1rea de Soporte", "soporte@empresa.com",
			"\u00c1rea de Soporte <soporte@empresa.com>"},
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
			t.Errorf("formatFromHeader(%q): produced RFC-2047 encoded-word (breaks old Outlook): %s", tt.name, got)
			continue
		}

		if got != tt.want {
			t.Errorf("formatFromHeader(%q):\n  got : %s\n  want: %s", tt.name, got, tt.want)
			continue
		}
		t.Logf("OK  %s", got)
	}
}
