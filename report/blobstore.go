// Package report implements a DOCX mail-merge engine: it takes a fixed
// corporate template (stored, by the caller, outside this repository) and only
// substitutes a bounded set of text tokens and image slots plus an
// auto-generated chart. It never builds documents, creates sections or
// renumbers figures. The template is the absolute source of truth.
package report

// BlobStore abstracts content-addressed binary storage so the engine never
// depends on where bytes live. The default implementation (models.DBBlobStore)
// keeps blobs in the database; a filesystem or external implementation can be
// swapped in later without touching the rest of the module.
type BlobStore interface {
	// Put stores content and returns its sha256 hex digest. Implementations
	// must deduplicate identical content.
	Put(content []byte) (sha256 string, err error)
	// Get returns the content for a previously stored digest.
	Get(sha256 string) ([]byte, error)
	// Exists reports whether a digest is already stored.
	Exists(sha256 string) (bool, error)
}
