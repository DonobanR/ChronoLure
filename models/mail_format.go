package models

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// formatFromHeader formats a "From" header value with maximum compatibility:
//
//   - Old Outlook (2007-2013): does NOT decode RFC 2047 encoded-words in the
//     From field, showing raw "=?utf-8?B?...?=" strings instead of the name.
//
//   - Strict SMTP servers (e.g. some Latin-American MTAs): reject raw UTF-8
//     bytes in headers with "554 Domain contains illegal character" when the
//     SMTPUTF8 extension is not negotiated.
//
// Solution: transliterate the display name to pure ASCII (e.g. "Comunicación"
// → "Comunicacion") before building the header.  This produces headers that
// are accepted by every SMTP server and rendered correctly by every email
// client without any decoding step.
//
// Usage:
//
//	msg.SetHeader("From", formatFromHeader(f.Address, f.Name))
func formatFromHeader(address, name string) string {
	if name == "" {
		return address
	}

	asciiName := toASCII(name)

	// If the name contains RFC 5322 special characters it must be wrapped in a
	// quoted-string.
	if hasMailSpecials(asciiName) {
		safe := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(asciiName)
		return `"` + safe + `" <` + address + `>`
	}
	return asciiName + " <" + address + ">"
}

// toASCII converts a UTF-8 string to its closest ASCII representation by:
//  1. Decomposing characters to NFD form (e.g. "ó" → "o" + combining accent)
//  2. Removing all combining diacritical marks (Unicode category Mn)
//  3. Dropping any remaining non-ASCII bytes
//
// Examples: "Comunicación!" → "Comunicacion!"
//
//	"José Ñoño"    → "Jose Nono"
func toASCII(s string) string {
	t := transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Mn)),
		norm.NFC,
	)
	result, _, err := transform.String(t, s)
	if err != nil {
		// Fallback: strip all non-ASCII bytes manually
		var b strings.Builder
		for _, r := range s {
			if r < 128 {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	// Remove any leftover non-ASCII bytes that NFD couldn't decompose
	var b strings.Builder
	for _, r := range result {
		if r < 128 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hasMailSpecials reports whether s contains any RFC 5322 "special" characters
// that require the display name to be wrapped in a quoted-string.
func hasMailSpecials(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', ')', '<', '>', '[', ']', ':', ';', '@', '\\', ',', '.', '"':
			return true
		}
	}
	return false
}
