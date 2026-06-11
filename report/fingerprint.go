package report

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
)

// Fingerprint computes a content fingerprint of a DOCX (ZIP) that is stable
// across compression method, entry write order and timestamps. It decompresses
// every entry, sorts entries by name, and hashes, for each entry, the
// length-prefixed name followed by the length-prefixed decompressed bytes.
//
// Two archives with the same logical content but different flate output, entry
// order or modtimes produce the SAME fingerprint; any real content change
// produces a different one. This makes it the right basis for a long-term
// reproducibility audit (it ignores toolchain noise but catches tampering or a
// genuinely changed renderer).
func Fingerprint(docx []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return "", err
	}

	names := make([]string, 0, len(zr.File))
	contents := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		b, err := readLimited(rc, maxEntryBytes)
		rc.Close()
		if err != nil {
			return "", err
		}
		names = append(names, f.Name)
		contents[f.Name] = b
	}
	return fingerprintEntries(names, contents), nil
}

// fingerprintEntries is the shared core of the fingerprint: it sorts the entry
// names and hashes, per entry, the length-prefixed name then the length-prefixed
// decompressed bytes. Both Fingerprint([]byte) (which unzips first) and Render
// (which already holds the decompressed parts) feed it the SAME inputs, so they
// produce identical fingerprints — letting Render avoid a second unzip.
func fingerprintEntries(names []string, contents map[string][]byte) string {
	sort.Strings(names)
	h := sha256.New()
	var lenbuf [8]byte
	writeChunk := func(b []byte) {
		binary.BigEndian.PutUint64(lenbuf[:], uint64(len(b)))
		h.Write(lenbuf[:])
		h.Write(b)
	}
	for _, n := range names {
		writeChunk([]byte(n))
		writeChunk(contents[n])
	}
	return hex.EncodeToString(h.Sum(nil))
}
