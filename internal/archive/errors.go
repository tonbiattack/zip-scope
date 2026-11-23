// Package archive reads image entries out of ZIP/CBZ archives without
// extracting them to disk. Only the central directory is read up front; entry
// payloads are streamed on demand and never written to a path derived from the
// archive's internal names.
package archive

import (
	"errors"
	"fmt"
)

// Sentinel errors. Callers classify failures with errors.Is so that a single
// broken entry never aborts a whole listing.
var (
	// ErrEncrypted reports a password protected entry.
	ErrEncrypted = errors.New("archive: entry is encrypted")
	// ErrUnsupported reports an entry whose compression method is not
	// Store or Deflate.
	ErrUnsupported = errors.New("archive: unsupported compression method")
	// ErrCorrupt reports a structurally broken archive or entry.
	ErrCorrupt = errors.New("archive: corrupt data")
	// ErrChecksum reports a CRC32 mismatch.
	ErrChecksum = errors.New("archive: crc mismatch")
	// ErrTooLarge reports an entry that exceeds the configured byte limit.
	ErrTooLarge = errors.New("archive: entry exceeds size limit")
	// ErrTooManyPixels reports an image that exceeds the configured pixel limit.
	ErrTooManyPixels = errors.New("archive: image exceeds pixel limit")
	// ErrTooManyEntries reports an archive that exceeds the entry limit.
	ErrTooManyEntries = errors.New("archive: too many entries")
	// ErrNotImage reports an entry that is not a supported still image.
	ErrNotImage = errors.New("archive: entry is not a supported image")
	// ErrTruncated reports an entry whose data ends early.
	ErrTruncated = errors.New("archive: entry data truncated")
	// ErrNoImages reports an archive that contains no usable image entries.
	ErrNoImages = errors.New("archive: no image entries")
	// ErrArchiveChanged reports that the archive on disk no longer matches the
	// index this handle was built from. The caller must re-index before
	// reading, because the requested ordinal may now be a different image.
	ErrArchiveChanged = errors.New("archive: file changed since it was indexed")
)

// EntryError attaches the offending entry name to a sentinel error so the UI
// can show which image failed while still allowing errors.Is classification.
type EntryError struct {
	Name string
	Err  error
}

func (e *EntryError) Error() string {
	return fmt.Sprintf("%s: %v", e.Name, e.Err)
}

func (e *EntryError) Unwrap() error { return e.Err }

func entryErr(name string, err error) error {
	if err == nil {
		return nil
	}
	return &EntryError{Name: name, Err: err}
}
