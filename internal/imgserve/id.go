// Package imgserve answers image requests from the WebView.
//
// The browser never sends a filesystem path. It sends an opaque identifier that
// this package resolves against the archives the app itself has opened, so a
// crafted request cannot read an arbitrary file, and no external URL is ever
// fetched. Identifiers are validated by shape before any lookup happens.
package imgserve

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Kind selects what an identifier refers to.
type Kind int

const (
	// KindThumb is a cached thumbnail of one entry.
	KindThumb Kind = iota
	// KindFull is a full-size decode of one entry.
	KindFull
	// KindCover is an archive's representative image thumbnail.
	KindCover
)

// ID is a parsed image identifier.
type ID struct {
	Kind Kind
	// Archive is the archive's stable identifier assigned by the app.
	Archive int64
	// Ordinal is the entry's central-directory index.
	Ordinal int
	// Width is the requested thumbnail width, already snapped by the caller.
	Width int
}

// String renders the identifier in the form the frontend uses.
func (id ID) String() string {
	switch id.Kind {
	case KindThumb:
		return fmt.Sprintf("t%d/%d/%d", id.Archive, id.Ordinal, id.Width)
	case KindFull:
		return fmt.Sprintf("f%d/%d", id.Archive, id.Ordinal)
	case KindCover:
		return fmt.Sprintf("c%d/%d", id.Archive, id.Width)
	}
	return ""
}

// ErrBadID reports a malformed or out-of-range identifier.
var ErrBadID = errors.New("imgserve: invalid image id")

// Limits on the numeric fields. They exist so a hostile identifier cannot
// overflow an int or force a huge allocation.
const (
	maxArchiveID = 1 << 40
	maxOrdinal   = 1 << 24
	maxWidth     = 4096
)

var (
	thumbRe = regexp.MustCompile(`^t(\d{1,13})/(\d{1,8})/(\d{1,4})$`)
	fullRe  = regexp.MustCompile(`^f(\d{1,13})/(\d{1,8})$`)
	coverRe = regexp.MustCompile(`^c(\d{1,13})/(\d{1,4})$`)
)

// Parse validates and decodes an identifier. Anything that does not match one
// of the three exact shapes is rejected.
func Parse(s string) (ID, error) {
	if len(s) == 0 || len(s) > 40 {
		return ID{}, ErrBadID
	}
	if m := thumbRe.FindStringSubmatch(s); m != nil {
		id, err := buildID(KindThumb, m[1], m[2], m[3])
		return id, err
	}
	if m := fullRe.FindStringSubmatch(s); m != nil {
		id, err := buildID(KindFull, m[1], m[2], "")
		return id, err
	}
	if m := coverRe.FindStringSubmatch(s); m != nil {
		id, err := buildID(KindCover, m[1], "", m[2])
		return id, err
	}
	return ID{}, ErrBadID
}

func buildID(kind Kind, archive, ordinal, width string) (ID, error) {
	a, err := strconv.ParseInt(archive, 10, 64)
	if err != nil || a < 0 || a > maxArchiveID {
		return ID{}, ErrBadID
	}
	id := ID{Kind: kind, Archive: a}
	if ordinal != "" {
		o, err := strconv.Atoi(ordinal)
		if err != nil || o < 0 || o > maxOrdinal {
			return ID{}, ErrBadID
		}
		id.Ordinal = o
	}
	if width != "" {
		w, err := strconv.Atoi(width)
		if err != nil || w <= 0 || w > maxWidth {
			return ID{}, ErrBadID
		}
		id.Width = w
	}
	return id, nil
}

// ContentType maps a stored thumbnail format to a MIME type. Only the two
// formats the encoder produces are accepted, so a cache file can never be
// served as HTML or SVG.
func ContentType(format string) string {
	switch strings.ToLower(format) {
	case "png":
		return "image/png"
	case "jpeg", "jpg":
		return "image/jpeg"
	}
	return ""
}

// SniffContentType identifies the container from its magic bytes. It is used
// for full-size responses, where the bytes come straight from the archive.
func SniffContentType(data []byte) string {
	if len(data) < 12 {
		return ""
	}
	switch {
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return "image/png"
	case data[0] == 'G' && data[1] == 'I' && data[2] == 'F' && data[3] == '8':
		return "image/gif"
	case string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	}
	return ""
}
