package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// allowMissingPlantillaEnv is the ONLY way to run without the master template.
// It must be set deliberately (and visibly, in CI config), because a guard that
// disables itself when its subject is absent is not a guard: the template is
// gitignored, so "skip if missing" meant the protection existed only on the
// machine that was already making the change.
const allowMissingPlantillaEnv = "CHRONOLURE_ALLOW_MISSING_PLANTILLA"

// requirePlantilla returns the template bytes, FAILING when it is absent unless the
// opt-out env var is set (in which case it skips, having said so out loud).
func requirePlantilla(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err == nil {
		return raw
	}
	if os.Getenv(allowMissingPlantillaEnv) != "" {
		t.Skipf("plantilla ausente y %s activado: protección DESACTIVADA a propósito (%v)",
			allowMissingPlantillaEnv, err)
	}
	t.Fatalf(`falta la plantilla maestra y la protección NO puede verificarse: %v

%s es gitignored (contenido corporativo), así que este test no puede confiar en
que esté presente: si simplemente hiciera skip, el guardián solo existiría en la
máquina de quien está haciendo el cambio.

Opciones:
  - provee la plantilla en el entorno (secreto/artefacto de CI), o
  - desactívalo EXPLÍCITAMENTE con %s=1 (queda registrado en la config de CI).`,
		err, path, allowMissingPlantillaEnv)
	return nil
}

// plantillaFingerprints mirrors report/testdata/plantilla_fingerprints.json.
type plantillaFingerprints struct {
	Current struct {
		File     string `json:"file"`
		SHA256   string `json:"sha256"`
		State    string `json:"state"`
		Recorded string `json:"recorded"`
	} `json:"current"`
	History []struct {
		File     string `json:"file"`
		SHA256   string `json:"sha256"`
		State    string `json:"state"`
		Recorded string `json:"recorded"`
	} `json:"history"`
}

// TestPlantillaReinaFingerprintIsDeclared guards the master template against
// UNDECLARED edits.
//
// Why this exists: the C1 reproducibility test (TestGenerateAndRegenerateReproducible)
// builds a SYNTHETIC template in-memory, so it never covered plantilla_reina.docx.
// That template was edited twice (CL-103 and CL-105) without a single test noticing.
// This test does not forbid editing it — it forces the edit to be DELIBERATE: change
// the template, then record the new hash in report/testdata/plantilla_fingerprints.json
// together with what changed and why.
//
// The template is gitignored (corporate content). This test therefore FAILS when it
// is absent, unless CHRONOLURE_ALLOW_MISSING_PLANTILLA is set: a guard that skips
// itself when its subject is missing would only ever protect the machine that is
// already making the change.
func TestPlantillaReinaFingerprintIsDeclared(t *testing.T) {
	const tmplPath = "../plantilla_reina.docx"
	raw := requirePlantilla(t, tmplPath)

	recPath := filepath.Join("testdata", "plantilla_fingerprints.json")
	recRaw, err := os.ReadFile(recPath)
	if err != nil {
		t.Fatalf("no se pudo leer %s: %v", recPath, err)
	}
	var rec plantillaFingerprints
	if err := json.Unmarshal(recRaw, &rec); err != nil {
		t.Fatalf("%s no es JSON válido: %v", recPath, err)
	}

	sum := sha256.Sum256(raw)
	got := hex.EncodeToString(sum[:])
	if got != rec.Current.SHA256 {
		t.Fatalf(`plantilla_reina.docx CAMBIÓ y el cambio no está declarado.

  sha256 actual   : %s
  sha256 declarado: %s   (estado: %s, registrado: %s)

Si el cambio es DELIBERADO:
  1. Copia el estado anterior con una etiqueta (p. ej. plantilla_reina.<ticket>.docx).
  2. Actualiza "current" en %s con el sha256 nuevo, el estado y la fecha,
     y mueve el anterior a "history".
Si NO lo esperabas: la plantilla se editó sin registro — revísalo antes de generar informes.`,
			got, rec.Current.SHA256, rec.Current.State, rec.Current.Recorded, recPath)
	}

	// The engine must still consider the declared template valid: a fingerprint that
	// matches a broken template would be a false sense of safety.
	insp, err := Inspect(raw)
	if err != nil {
		t.Fatalf("Inspect sobre la plantilla declarada falló: %v", err)
	}
	if !insp.Valid {
		t.Fatalf("la plantilla declarada NO es válida para el motor: unknown=%v missingReq=%v missingSlot=%v dup=%v",
			insp.Unknown, insp.MissingRequired, insp.MissingRequiredSlot, insp.DuplicateSlots)
	}
}

// TestPlantillaHistoryCopiesAreIntact verifies that the labelled copies recorded in
// history still hash to what was recorded, so the rollback points remain trustworthy
// (a corrupted backup discovered during a rollback is worse than no backup).
func TestPlantillaHistoryCopiesAreIntact(t *testing.T) {
	recRaw, err := os.ReadFile(filepath.Join("testdata", "plantilla_fingerprints.json"))
	if err != nil {
		t.Fatalf("no se pudo leer el registro: %v", err)
	}
	var rec plantillaFingerprints
	if err := json.Unmarshal(recRaw, &rec); err != nil {
		t.Fatalf("registro inválido: %v", err)
	}
	checked := 0
	missing := []string{}
	for _, h := range rec.History {
		raw, err := os.ReadFile(filepath.Join("..", h.File))
		if err != nil {
			missing = append(missing, h.File)
			continue
		}
		sum := sha256.Sum256(raw)
		if got := hex.EncodeToString(sum[:]); got != h.SHA256 {
			t.Errorf("la copia %q (%s) ya no coincide con su registro: %s != %s",
				h.File, h.State, got, h.SHA256)
		}
		checked++
	}
	if len(missing) > 0 {
		if os.Getenv(allowMissingPlantillaEnv) != "" {
			t.Skipf("copias de rollback ausentes %v y %s activado: verificación DESACTIVADA a propósito",
				missing, allowMissingPlantillaEnv)
		}
		t.Fatalf(`faltan copias de rollback declaradas: %v

Un punto de rollback que nadie verifica no es un punto de rollback. Provéelas en el
entorno o desactiva la verificación explícitamente con %s=1.`, missing, allowMissingPlantillaEnv)
	}
	if checked == 0 {
		t.Fatalf("el registro no declara ninguna copia histórica: no hay punto de rollback verificable")
	}
}
