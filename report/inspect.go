package report

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// Bounds to defend against decompression ("zip") bombs on uploaded templates.
const (
	maxEntryBytes = 64 << 20  // 64 MiB decompressed per zip entry
	maxTotalBytes = 256 << 20 // 256 MiB total decompressed across read entries
	maxEntries    = 4096      // maximum number of entries in the archive
)

// Vocabulary is the set of text tokens the engine knows how to fill. A template
// that uses any token outside this set is rejected on upload, because the
// engine could not resolve it.
var Vocabulary = map[string]bool{
	"EMPRESA":           true,
	"ELABORADO_POR":     true,
	"FECHA_INFORME":     true,
	"FECHA_EJECUCION":   true,
	"RANGO_FECHAS":      true,
	"N_USUARIOS":        true,
	"PARRAFO_EJECUCION": true,
	"SUPLANTANDO":       true,
	"COMUNICADO":        true,
	"TEXTO_PUNTO_1":     true,
	"R_IGNORADO":        true,
	"R_ABIERTO":         true,
	"R_CLIC":            true,
	"R_DATOS":           true,
	"R_TOTAL":           true,
	"PCT_DATOS":         true,
	"N_DATOS":           true,
	"N_ENVIO_DATOS":     true,
	"N_ENVIO_DATOS_PAD": true,
	"BLOQUE_2FA":           true,
	"NUM_FIGURA_ANEXO":     true,
	"FRASE_2FA_CONCLUSION": true,
	"PARRAFO_CIERRE_2FA":   true,
}

// RequiredTokens must be present in every valid template.
var RequiredTokens = []string{
	"EMPRESA",
	"N_USUARIOS",
	"R_IGNORADO",
	"R_ABIERTO",
	"R_CLIC",
	"R_DATOS",
	"R_TOTAL",
	"PCT_DATOS",
	"N_DATOS",
}

// Inspection is the result of analyzing an uploaded template.
type Inspection struct {
	Tokens              []string `json:"tokens"`
	ImageSlots          []string `json:"image_slots"`
	MissingRequired     []string `json:"missing_required"`
	Unknown             []string `json:"unknown"`
	MissingRequiredSlot []string `json:"missing_required_slots"`
	DuplicateSlots      []string `json:"duplicate_slots"`
	Valid               bool     `json:"valid"`
}

var (
	tagRe    = regexp.MustCompile(`<[^>]+>`)
	tokenRe  = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)
	docPrRe  = regexp.MustCompile(`<(?:wp:docPr|pic:cNvPr)\b[^>]*>`)
	// Capture the full attribute value (not just [A-Za-z0-9_]) so Alt-Text with
	// spaces ("Figura 2") is seen; CanonicalSlotKey then folds case/separators.
	descrRe  = regexp.MustCompile(`\bdescr="([^"]*)"`)
	nameAtRe = regexp.MustCompile(`\bname="([^"]*)"`)
)

// Inspect analyzes the DOCX bytes and reports the tokens and image slots it
// uses, validating them against the engine's vocabulary and the slot catalog.
//
// Tokens are discovered by concatenating the textual content of document,
// header and footer parts after stripping XML tags, which reassembles tokens
// that Word may have fragmented across runs. Image slots are identified by the
// drawing's Alt-Text (descr) — or its name as a fallback — matched against the
// closed slot catalog. The engine never infers what an image represents: the
// template author assigns each slot explicitly via Alt-Text.
func Inspect(docx []byte) (Inspection, error) {
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return Inspection{}, err
	}
	if len(zr.File) > maxEntries {
		return Inspection{}, fmt.Errorf("template has too many entries (%d > %d)", len(zr.File), maxEntries)
	}

	var rawXML strings.Builder
	var total int64
	for _, f := range zr.File {
		n := f.Name
		if !strings.HasPrefix(n, "word/") || !strings.HasSuffix(n, ".xml") {
			continue
		}
		if !strings.Contains(n, "document") && !strings.Contains(n, "header") && !strings.Contains(n, "footer") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return Inspection{}, err
		}
		b, err := readLimited(rc, maxEntryBytes)
		rc.Close()
		if err != nil {
			return Inspection{}, err
		}
		total += int64(len(b))
		if total > maxTotalBytes {
			return Inspection{}, fmt.Errorf("template decompresses beyond %d bytes (possible decompression bomb)", maxTotalBytes)
		}
		rawXML.Write(b)
		rawXML.WriteByte('\n')
	}
	raw := rawXML.String()

	// Tokens: strip tags so run-fragmented placeholders reassemble.
	text := tagRe.ReplaceAllString(raw, "")
	tokens := uniqueSorted(captureGroup(tokenRe.FindAllStringSubmatch(text, -1)))

	// Image slots: Alt-Text (descr) or name on docPr/cNvPr matching the catalog.
	slotCounts := make(map[string]int)
	for _, el := range docPrRe.FindAllString(raw, -1) {
		key := ""
		// Prefer descr (Alt-Text), fall back to name; fold to the canonical
		// catalog key so authoring variations (case/spaces) still match (CL-104).
		if m := descrRe.FindStringSubmatch(el); m != nil {
			if canon, ok := CanonicalSlotKey(m[1]); ok {
				key = canon
			}
		}
		if key == "" {
			if m := nameAtRe.FindStringSubmatch(el); m != nil {
				if canon, ok := CanonicalSlotKey(m[1]); ok {
					key = canon
				}
			}
		}
		if key != "" {
			slotCounts[key]++
		}
	}
	slots := make([]string, 0, len(slotCounts))
	var duplicates []string
	for k, c := range slotCounts {
		slots = append(slots, k)
		if c > 1 {
			duplicates = append(duplicates, k)
		}
	}
	sort.Strings(slots)
	sort.Strings(duplicates)

	insp := Inspection{Tokens: tokens, ImageSlots: slots, DuplicateSlots: duplicates}

	tokenSet := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		tokenSet[t] = true
		// Conditional block markers ({{IF_X}}/{{ENDIF_X}}) and the annex-table
		// marker ({{TABLA_ANEXO}}) are structural, not fillable tokens; they are
		// valid and handled by the renderer.
		if strings.HasPrefix(t, "IF_") || strings.HasPrefix(t, "ENDIF_") || t == AnnexTableToken {
			continue
		}
		if !Vocabulary[t] {
			insp.Unknown = append(insp.Unknown, t)
		}
	}
	for _, r := range RequiredTokens {
		if !tokenSet[r] {
			insp.MissingRequired = append(insp.MissingRequired, r)
		}
	}
	for _, r := range RequiredSlots() {
		if slotCounts[r] == 0 {
			insp.MissingRequiredSlot = append(insp.MissingRequiredSlot, r)
		}
	}

	insp.Valid = len(insp.Unknown) == 0 &&
		len(insp.MissingRequired) == 0 &&
		len(insp.MissingRequiredSlot) == 0 &&
		len(insp.DuplicateSlots) == 0
	return insp, nil
}

// readLimited reads up to max bytes and errors if the source exceeds it.
func readLimited(r io.Reader, max int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("zip entry exceeds %d bytes (possible decompression bomb)", max)
	}
	return b, nil
}

func captureGroup(matches [][]string) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			out = append(out, m[1])
		}
	}
	return out
}

func uniqueSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
