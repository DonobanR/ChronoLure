package report

import "fmt"

// ChronoLure/GoPhish cannot determine whether a captured credential was
// protected by MFA, so 2FA is a single manual yes/no answer from the operator:
// "were any accounts protected by 2FA?". The number of cases shown in the
// narrative comes from the campaign metrics (submitted-data count); 2FA only
// selects which paragraph is used. No per-user findings are stored or inferred.

// Bloque2FA returns the conditional MFA paragraph for the credentials section
// and the conclusions. It is binary: if 2FA accounts were found, it returns the
// mitigation paragraph using the submitted-data count; otherwise it returns an
// empty string so the base paragraph stands alone. It never composes free
// narrative.
func Bloque2FA(submitted int64, had2FA bool) string {
	// El bloque depende exclusivamente de si se encontraron cuentas con 2FA
	// (el checkbox). X = cantidad de credenciales capturadas (submitted).
	if !had2FA {
		return ""
	}
	return fmt.Sprintf("En %s la autenticación fue mitigada por la presencia de doble "+
		"factor de autenticación (2FA), impidiendo el acceso directo a los servicios de "+
		"correo electrónico y a otras aplicaciones del ecosistema Microsoft 365.",
		casos(submitted))
}

// casos renders a grammatically-correct "el 1 caso" / "los N casos" fragment.
func casos(n int64) string {
	if n == 1 {
		return "el 1 caso"
	}
	return fmt.Sprintf("los %d casos", n)
}
