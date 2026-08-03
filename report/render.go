package report

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// RenderInput is everything the renderer needs. It is intentionally free of any
// HTTP, controller, user or request concept: render is a pure transformation
// from (template, variables, images) to DOCX bytes.
type RenderInput struct {
	// Template is the corporate DOCX template (provided by the caller from the
	// database; never embedded in the repository).
	Template []byte
	// Vars maps a token name (without braces, e.g. "EMPRESA") to its value.
	Vars map[string]string
	// Images maps an image slot key (e.g. "figura_1") to the replacement image
	// bytes. The image is swapped in place, preserving the drawing's anchor and
	// size, so pagination and figure numbering never change.
	Images map[string][]byte
	// ChartValues, if set, overwrites the numeric values of the native Word
	// results chart (word/charts/chart*.xml <c:val> numCache) in order, so the
	// chart reflects the SAME funnel data as the results table. Categories and
	// styling are left untouched.
	ChartValues []float64
	// Conditions drives block-level conditional content. A region of paragraphs
	// marked in the template with {{IF_NAME}} ... {{ENDIF_NAME}} (each in its own
	// paragraph) is removed entirely when Conditions[NAME] is false (figure,
	// caption, image and spacing all disappear); when true, only the marker
	// paragraphs are removed and the content is kept.
	Conditions map[string]bool
	// AnnexTableXML, if non-empty, replaces the paragraph carrying the
	// {{TABLA_ANEXO}} marker with this native <w:tbl> element (the S9 annex table,
	// CL-105). Empty leaves the marker paragraph in place (removed as an unknown
	// token by replaceTokens would not happen; see replaceAnnexTable).
	AnnexTableXML string
}

// Render performs a fixed-template mail-merge: it replaces text tokens and swaps
// image parts inside the template, leaving every other part (styles, numbering,
// TOC, headers/footers, captions) byte-for-byte unchanged. It never creates
// sections, paragraphs or styles. The returned bytes are a valid DOCX.
func Render(in RenderInput) ([]byte, string, error) {
	zr, err := zip.NewReader(bytes.NewReader(in.Template), int64(len(in.Template)))
	if err != nil {
		return nil, "", fmt.Errorf("invalid template docx: %w", err)
	}

	type part struct {
		name string
		data []byte
	}
	if len(zr.File) > maxEntries {
		return nil, "", fmt.Errorf("template has too many entries (%d > %d)", len(zr.File), maxEntries)
	}
	var parts []part
	byName := make(map[string][]byte)
	var total int64
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, "", err
		}
		b, err := readLimited(rc, maxEntryBytes)
		rc.Close()
		if err != nil {
			return nil, "", err
		}
		total += int64(len(b))
		if total > maxTotalBytes {
			return nil, "", fmt.Errorf("template decompresses beyond %d bytes (possible decompression bomb)", maxTotalBytes)
		}
		parts = append(parts, part{f.Name, b})
		byName[f.Name] = b
	}

	// Transform every part in a SINGLE pass, converting each part's bytes to a
	// string at most once (audit I-5):
	//   - text parts (document/headers/footers): drop conditional blocks
	//     ({{IF_X}}..{{ENDIF_X}} when X is false), replace tokens, then resolve
	//     image slots from the SAME final string;
	//   - chart parts: overwrite the values numCache so the chart matches the
	//     results table (same funnel source).
	// Slot resolution runs AFTER conditionals so a figure removed by a false
	// {{IF_X}} block is never swapped, and reads the post-token string so no
	// extra []byte<->string round-trip is needed.
	mediaReplacements := make(map[string][]byte)
	for i := range parts {
		switch {
		case isTextPart(parts[i].name):
			s := string(parts[i].data)
			if len(in.Conditions) > 0 {
				s = processConditionals(s, in.Conditions)
			}
			s = replaceTokens(s, in.Vars)
			if in.AnnexTableXML != "" {
				s = replaceAnnexTable(s, in.AnnexTableXML)
			}
			parts[i].data = []byte(s)
			if len(in.Images) > 0 {
				rels := byName[relsFor(parts[i].name)]
				for slot, mp := range slotMediaPaths(s, rels, parts[i].name, in.Images) {
					mediaReplacements[mp] = in.Images[slot]
				}
			}
		case isChartPart(parts[i].name) && len(in.ChartValues) > 0:
			parts[i].data = []byte(updateChartValues(string(parts[i].data), in.ChartValues))
		}
	}

	// 3) Write the output archive: replaced text, swapped media, everything
	//    else copied verbatim and in original order. Collect the final
	//    (decompressed) entries to compute the fingerprint without unzipping the
	//    result a second time (see fingerprintEntries / I-4).
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	names := make([]string, 0, len(parts))
	contents := make(map[string][]byte, len(parts))
	for _, p := range parts {
		data := p.data
		if repl, ok := mediaReplacements[p.name]; ok {
			data = repl
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate})
		if err != nil {
			return nil, "", err
		}
		if _, err := w.Write(data); err != nil {
			return nil, "", err
		}
		// Mirror Fingerprint(docx), which skips directory entries.
		if !strings.HasSuffix(p.name, "/") {
			names = append(names, p.name)
			contents[p.name] = data
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", err
	}
	return out.Bytes(), fingerprintEntries(names, contents), nil
}

// SlotIsImageBacked reports whether the template fills the given slot with an
// actual image (a drawing carrying r:embed), as opposed to a native Word chart
// (r:id to chart*.xml) or an absent slot. It reads only the textual parts and
// their relationships (never the media), so it is cheap. Used to skip generating
// the results-chart PNG when grafico_1 is a native chart and the PNG would never
// be consumed (I-1).
func SlotIsImageBacked(template []byte, slot string) (bool, error) {
	zr, err := zip.NewReader(bytes.NewReader(template), int64(len(template)))
	if err != nil {
		return false, err
	}
	byName := make(map[string][]byte)
	var textParts []string
	for _, f := range zr.File {
		n := f.Name
		if !strings.HasSuffix(n, ".xml") && !strings.HasSuffix(n, ".rels") {
			continue // skip media; only XML text parts + their rels are needed
		}
		rc, err := f.Open()
		if err != nil {
			return false, err
		}
		b, err := readLimited(rc, maxEntryBytes)
		rc.Close()
		if err != nil {
			return false, err
		}
		byName[n] = b
		if isTextPart(n) {
			textParts = append(textParts, n)
		}
	}
	want := map[string][]byte{slot: nil}
	for _, name := range textParts {
		if len(slotMediaPaths(string(byName[name]), byName[relsFor(name)], name, want)) > 0 {
			return true, nil
		}
	}
	return false, nil
}

var (
	tokenReplaceRe = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_]+)\s*\}\}`)
	paragraphRe    = regexp.MustCompile(`(?s)<w:p\b.*?</w:p>`)
	wtRe           = regexp.MustCompile(`(?s)(<w:t\b[^>]*>)(.*?)(</w:t>)`)
	embedRe        = regexp.MustCompile(`r:embed="(rId\d+)"`)
)

// replaceTokens substitutes tokens in two passes. Pass 1 replaces tokens that
// appear contiguously (the normal case). Pass 2 handles tokens Word fragmented
// across runs: only within paragraphs that still contain "{{", it merges the
// run texts, substitutes, and writes the result back into the first run
// (preserving that run's formatting) while emptying the others. Drawings and
// non-text nodes are never touched.
// replaceAnnexTable replaces the whole paragraph carrying the {{TABLA_ANEXO}}
// marker with the native <w:tbl> element. A table cannot live inside a paragraph,
// so the entire <w:p>…</w:p> is swapped out. Paragraphs never nest in OOXML, so
// the first </w:p> after the marker closes its paragraph. The marker text may be
// run-fragmented, but the token itself is emitted as a single run by the
// tokenizer, so a literal search suffices; if not found, the input is unchanged.
func replaceAnnexTable(xmlStr, tableXML string) string {
	marker := "{{" + AnnexTableToken + "}}"
	idx := strings.Index(xmlStr, marker)
	if idx < 0 {
		return xmlStr
	}
	// Start of the containing paragraph: the nearest "<w:p>" or "<w:p " before the
	// marker (never "<w:pPr>", which starts with "<w:pP").
	start := strings.LastIndex(xmlStr[:idx], "<w:p>")
	if s2 := strings.LastIndex(xmlStr[:idx], "<w:p "); s2 > start {
		start = s2
	}
	endRel := strings.Index(xmlStr[idx:], "</w:p>")
	if start < 0 || endRel < 0 {
		// No paragraph wrapper found; drop the marker so it never leaks to output.
		return strings.Replace(xmlStr, marker, tableXML, 1)
	}
	end := idx + endRel + len("</w:p>")
	return xmlStr[:start] + tableXML + xmlStr[end:]
}

func replaceTokens(xmlStr string, vars map[string]string) string {
	xmlStr = tokenReplaceRe.ReplaceAllStringFunc(xmlStr, func(m string) string {
		name := tokenReplaceRe.FindStringSubmatch(m)[1]
		if v, ok := vars[name]; ok {
			return xmlEscape(v)
		}
		return m
	})
	if !strings.Contains(xmlStr, "{{") {
		return xmlStr
	}
	return paragraphRe.ReplaceAllStringFunc(xmlStr, func(p string) string {
		if !strings.Contains(p, "{{") {
			return p
		}
		return mergeAndReplaceParagraph(p, vars)
	})
}

// mergeAndReplaceParagraph replaces tokens that Word fragmented across runs,
// touching only the MINIMAL span of runs each token spans (M6). Runs that hold
// no token text keep their exact formatting and content; only the runs a token
// actually occupies are rewritten. The replacement value is placed in the run
// where the token starts; runs fully inside the token become empty.
func mergeAndReplaceParagraph(p string, vars map[string]string) string {
	locs := wtRe.FindAllStringSubmatchIndex(p, -1)
	if len(locs) == 0 {
		return p
	}

	// Build the concatenated text S of all <w:t> inner contents, remembering
	// each segment's base offset in S so any S position maps back to a run.
	type segment struct {
		open, inner, close string
		fullStart, fullEnd int
	}
	segs := make([]segment, len(locs))
	base := make([]int, len(locs))
	var sb strings.Builder
	for i, l := range locs {
		base[i] = sb.Len()
		inner := p[l[4]:l[5]]
		sb.WriteString(inner)
		segs[i] = segment{open: p[l[2]:l[3]], inner: inner, close: p[l[6]:l[7]], fullStart: l[0], fullEnd: l[1]}
	}
	s := sb.String()

	// segAt maps a position in S to the run that owns it: the largest index i
	// with base[i] <= pos. base is sorted ascending, so a binary search is used
	// (audit M-9) — identical result to the former linear scan.
	segAt := func(pos int) int {
		i := sort.Search(len(base), func(k int) bool { return base[k] > pos })
		if i == 0 {
			return 0
		}
		return i - 1
	}

	matches := tokenReplaceRe.FindAllStringSubmatchIndex(s, -1)
	out := make([]strings.Builder, len(segs))
	changed := false
	i, mi := 0, 0
	for i < len(s) {
		if mi < len(matches) && i == matches[mi][0] {
			m := matches[mi]
			name := s[m[2]:m[3]]
			if v, ok := vars[name]; ok {
				out[segAt(m[0])].WriteString(xmlEscape(v)) // value into the start run
				changed = true
				i = m[1] // skip the token's characters across all runs it spans
				mi++
				continue
			}
			mi++ // unknown token: leave its characters literally
		}
		out[segAt(i)].WriteByte(s[i])
		i++
	}
	if !changed {
		return p // only unknown/unfilled tokens; leave untouched
	}

	var res strings.Builder
	last := 0
	for i, seg := range segs {
		res.WriteString(p[last:seg.fullStart])
		newInner := out[i].String()
		if newInner == seg.inner {
			res.WriteString(p[seg.fullStart:seg.fullEnd]) // unchanged run, verbatim
		} else {
			open := seg.open
			if newInner != "" {
				open = ensurePreserve(open)
			}
			// Three writes instead of a temporary concatenation (audit M-9).
			res.WriteString(open)
			res.WriteString(newInner)
			res.WriteString(seg.close)
		}
		last = seg.fullEnd
	}
	res.WriteString(p[last:])
	return res.String()
}

func ensurePreserve(openTag string) string {
	if strings.Contains(openTag, "xml:space") {
		return openTag
	}
	return strings.TrimSuffix(openTag, ">") + ` xml:space="preserve">`
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// slotMediaPaths resolves, in a SINGLE pass over the part's drawings, the media
// part backing each requested slot. For every <wp:docPr>/<pic:cNvPr> it reads
// the descr/name, and if that value is a wanted slot it follows the r:embed
// within the SAME <w:drawing> to its relationship target. Returns slot ->
// media path. This replaces the former per-slot mediaPathForSlot+regex-compile
// (audit I-5): one pass, package-level regexes, the relationships parsed once.
//
// Behavior is identical to calling mediaPathForSlot per slot: a slot resolves
// iff some drawing's docPr has descr==slot or name==slot with an r:embed; the
// r:embed search is bounded to the drawing so a native chart (r:id, no r:embed)
// never grabs the next figure's relationship.
func slotMediaPaths(xmlStr string, rels []byte, partName string, wanted map[string][]byte) map[string]string {
	out := make(map[string]string)
	if len(wanted) == 0 {
		return out
	}
	var relTargets map[string]string // parsed lazily, once, only if a slot matches
	dir := partName[:strings.LastIndex(partName, "/")+1]
	for _, loc := range docPrRe.FindAllStringIndex(xmlStr, -1) {
		el := xmlStr[loc[0]:loc[1]]
		// The drawing label identifies a slot by descr (preferred) or name. Fold
		// each to the canonical catalog key so authoring variations (case/spaces
		// in the Alt-Text) still resolve to the uploaded slot (CL-104).
		var slots []string
		if m := descrRe.FindStringSubmatch(el); m != nil {
			if canon, ok := CanonicalSlotKey(m[1]); ok {
				slots = append(slots, canon)
			}
		}
		if m := nameAtRe.FindStringSubmatch(el); m != nil {
			if canon, ok := CanonicalSlotKey(m[1]); ok {
				slots = append(slots, canon)
			}
		}
		matched := false
		for _, s := range slots {
			if _, ok := wanted[s]; ok {
				matched = true
			}
		}
		if !matched {
			continue
		}
		// r:embed within the SAME drawing only.
		rest := xmlStr[loc[1]:]
		if d := strings.Index(rest, "</w:drawing>"); d >= 0 {
			rest = rest[:d]
		}
		em := embedRe.FindStringSubmatch(rest)
		if em == nil {
			continue
		}
		if relTargets == nil {
			relTargets = parseRelTargets(rels)
		}
		target := relTargets[em[1]]
		if target == "" {
			continue
		}
		mp := path.Clean(dir + target)
		for _, s := range slots {
			if _, ok := wanted[s]; ok {
				out[s] = mp
			}
		}
	}
	return out
}

// parseRelTargets parses a .rels part once into rId -> Target.
func parseRelTargets(rels []byte) map[string]string {
	out := make(map[string]string)
	for _, el := range relationshipRe.FindAll(rels, -1) {
		id := relIDAttrRe.FindSubmatch(el)
		tgt := relTargetAttrRe.FindSubmatch(el)
		if id != nil && tgt != nil {
			out[string(id[1])] = string(tgt[1])
		}
	}
	return out
}

var (
	chartPartRe   = regexp.MustCompile(`^word/charts/chart\d+\.xml$`)
	chartValRe    = regexp.MustCompile(`(?s)<c:val>.*?</c:val>`)
	chartNumberRe = regexp.MustCompile(`(<c:v>)([^<]*)(</c:v>)`)

	// Relationship parsing (parseRelTargets) — compiled once.
	relationshipRe  = regexp.MustCompile(`(?s)<Relationship\b[^>]*>`)
	relIDAttrRe     = regexp.MustCompile(`\bId="([^"]+)"`)
	relTargetAttrRe = regexp.MustCompile(`\bTarget="([^"]+)"`)
)

func isChartPart(name string) bool { return chartPartRe.MatchString(name) }

var (
	ifMarkerRe    = regexp.MustCompile(`\{\{IF_([A-Za-z0-9_]+)\}\}`)
	endifMarkerRe = regexp.MustCompile(`\{\{ENDIF_[A-Za-z0-9_]+\}\}`)
)

// processConditionals walks the paragraphs of a part as a small state machine.
// A paragraph holding {{IF_NAME}} opens a region whose condition is
// Conditions[NAME]; {{ENDIF_NAME}} closes it. Marker paragraphs are always
// dropped; paragraphs inside a false region are dropped too. Paragraphs outside
// any region are kept verbatim.
func processConditionals(xmlStr string, conds map[string]bool) string {
	active := ""
	keep := true
	return paragraphRe.ReplaceAllStringFunc(xmlStr, func(p string) string {
		if m := ifMarkerRe.FindStringSubmatch(p); m != nil {
			active = m[1]
			keep = conds[active]
			return ""
		}
		if endifMarkerRe.MatchString(p) {
			active = ""
			keep = true
			return ""
		}
		if active != "" && !keep {
			return ""
		}
		return p
	})
}

// updateChartValues overwrites the numeric <c:v> values inside the <c:val>
// block (the series data) of a chart part, in order, with the provided values.
// The <c:cat> block (category labels) and styling are untouched.
func updateChartValues(xmlStr string, vals []float64) string {
	return chartValRe.ReplaceAllStringFunc(xmlStr, func(block string) string {
		i := 0
		return chartNumberRe.ReplaceAllStringFunc(block, func(m string) string {
			sub := chartNumberRe.FindStringSubmatch(m)
			if i < len(vals) {
				v := strconv.FormatFloat(vals[i], 'f', -1, 64)
				i++
				return sub[1] + v + sub[3]
			}
			return m
		})
	})
}

func isTextPart(name string) bool {
	if !strings.HasPrefix(name, "word/") || !strings.HasSuffix(name, ".xml") {
		return false
	}
	base := name[len("word/"):]
	return base == "document.xml" || strings.HasPrefix(base, "header") || strings.HasPrefix(base, "footer")
}

func relsFor(partName string) string {
	i := strings.LastIndex(partName, "/")
	return partName[:i+1] + "_rels/" + partName[i+1:] + ".rels"
}
