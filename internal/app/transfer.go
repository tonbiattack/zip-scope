package app

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Transfer errors. They are distinct so the caller can tell which stage failed
// and what state the files are in, instead of a single opaque failure.
var (
	// ErrCopyIncomplete reports that the copy stopped before finishing.
	ErrCopyIncomplete = errors.New("app: copy did not complete")
	// ErrCopyMismatch reports that the copied bytes differ from the source.
	ErrCopyMismatch = errors.New("app: copied file does not match the source")
	// ErrSourceKept reports that the destination was written but the source
	// could not be removed. Both files exist; the caller must tell the user.
	ErrSourceKept = errors.New("app: destination written but the source could not be removed")
)

// fileDigest returns the size and SHA-256 of path. It is used to prove that a
// copy is byte-identical before the original is removed.
func fileDigest(path string) (int64, [sha256.Size]byte, error) {
	var sum [sha256.Size]byte

	f, err := os.Open(path)
	if err != nil {
		return 0, sum, err
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, sum, err
	}
	copy(sum[:], h.Sum(nil))
	return n, sum, nil
}

// copyVerified copies src to dest without ever overwriting an existing file.
//
// The bytes are written to a temporary file in the destination directory, then
// compared against the source by size and SHA-256. Only after that check passes
// is the temporary file published at dest, and only after the publish succeeds
// is the source removed. A failure at any stage leaves the source untouched.
func copyVerified(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}

	srcInfo, err := in.Stat()
	if err != nil {
		in.Close()
		return err
	}

	// The temporary file lives beside the destination so the final publish is a
	// same-volume rename, which is atomic.
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".zipscope-move-*")
	if err != nil {
		in.Close()
		return err
	}
	tmpName := tmp.Name()
	// Remove the temporary file on every path that does not publish it.
	published := false
	defer func() {
		if !published {
			os.Remove(tmpName)
		}
	}()

	written, err := io.Copy(tmp, in)
	if err != nil {
		tmp.Close()
		in.Close()
		return fmt.Errorf("%w: %v", ErrCopyIncomplete, err)
	}
	if written != srcInfo.Size() {
		tmp.Close()
		in.Close()
		return fmt.Errorf("%w: wrote %d of %d bytes", ErrCopyIncomplete, written, srcInfo.Size())
	}
	// Flush to disk before the comparison, so the digest is taken from the same
	// bytes that will be published.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		in.Close()
		return fmt.Errorf("%w: sync: %v", ErrCopyIncomplete, err)
	}
	if err := tmp.Close(); err != nil {
		in.Close()
		return fmt.Errorf("%w: close: %v", ErrCopyIncomplete, err)
	}
	// Windows refuses to delete a file that still has an open handle, so the
	// source must be closed before it is removed at the end.
	if err := in.Close(); err != nil {
		return fmt.Errorf("%w: close source: %v", ErrCopyIncomplete, err)
	}

	// Prove the copy is identical before the original is at risk.
	srcSize, srcSum, err := fileDigest(src)
	if err != nil {
		return fmt.Errorf("%w: digest source: %v", ErrCopyIncomplete, err)
	}
	dstSize, dstSum, err := fileDigest(tmpName)
	if err != nil {
		return fmt.Errorf("%w: digest copy: %v", ErrCopyIncomplete, err)
	}
	if srcSize != dstSize || srcSum != dstSum {
		return fmt.Errorf("%w: source %d bytes, copy %d bytes", ErrCopyMismatch, srcSize, dstSize)
	}

	// Publish without replacing. This is the step that closes the window
	// between the earlier existence check and the final placement: if another
	// process created dest in the meantime, this fails instead of clobbering it.
	if err := publishNoReplace(tmpName, dest); err != nil {
		return err
	}
	published = true

	// The destination is now a verified copy. Removing the source is the last
	// step, and a failure here must not discard the copy: the user needs to
	// know both files exist so they can resolve it themselves.
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("%w: %v", ErrSourceKept, err)
	}
	return nil
}
