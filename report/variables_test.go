package report

import "testing"

// TestBuildVarsDocumentValues checks the assembled token map reproduces the
// key values of the reference document.
func TestBuildVarsDocumentValues(t *testing.T) {
	f := Funnel(FunnelInput{Sent: 17, Opened: 16, Clicked: 16, Submitted: 2})
	vars := BuildVars(VarInput{
		CompanyName:     "Empresa Ejemplo, S.A.",
		TotalRecipients: 17,
		Had2FA:          true,
		Funnel:          f,
	})

	want := map[string]string{
		"EMPRESA":           "Empresa Ejemplo, S.A.",
		"N_USUARIOS":        "17",
		"R_IGNORADO":        "1",
		"R_ABIERTO":         "0",
		"R_CLIC":            "14",
		"R_DATOS":           "2",
		"R_TOTAL":           "17",
		"PCT_DATOS":         "12%",
		"N_DATOS":           "2",
		"N_ENVIO_DATOS_PAD": "02",
	}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("vars[%q] = %q, want %q", k, vars[k], v)
		}
	}
	if vars["BLOQUE_2FA"] == "" {
		t.Errorf("expected a 2FA block for all-protected case")
	}
}

// TestBuildVarsCoversVocabulary ensures every fillable token gets a value, so a
// template using any vocabulary token never renders an empty placeholder by
// omission.
func TestBuildVarsCoversVocabulary(t *testing.T) {
	vars := BuildVars(VarInput{Funnel: FunnelMetrics{}})
	for tok := range Vocabulary {
		if _, ok := vars[tok]; !ok {
			t.Errorf("BuildVars does not produce token %q", tok)
		}
	}
}
