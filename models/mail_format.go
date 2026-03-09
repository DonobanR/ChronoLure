package models

import (
	"strings"
)

// formatFromHeader formats a "From" header value with maximum compatibility for
// ALL email clients, including old Outlook (2007/2010/2013).
//
// Root cause of the bug: gomail encodes display names that contain non-ASCII
// characters using RFC 2047 encoded-words (=?utf-8?B?...?= or =?utf-8?q?...?=).
// Old Outlook versions do NOT decode those encoded-words in the From field and
// show the raw string instead of the actual sender name.
//
// Fix: send the display name as raw UTF-8 bytes (RFC 6532 / SMTPUTF8).
// Every modern SMTP relay (Exchange, Gmail, SendGrid, etc.) and every mail
// client — including old Outlook — renders raw UTF-8 display names correctly
// without any decoding step.
//
// Usage:
//
//	msg.SetHeader("From", formatFromHeader(f.Address, f.Name))
func formatFromHeader(address, name string) string {
	if name == "" {
		return address
	}

	// If the name contains RFC 5322 special characters it must be wrapped in a
	// quoted-string.  All other names (including UTF-8) are sent as plain text —
	// no RFC 2047 encoding applied.
	if hasMailSpecials(name) {
		safe := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(name)
		return `"` + safe + `" <` + address + `>`
	}
	return name + " <" + address + ">"
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
