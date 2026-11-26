// Package shellreg manages the Explorer preview handler registration.
//
// ZipScope's preview pane integration is opt-in per extension. Turning it on
// writes two things under HKCU:
//
//	Software\Classes\.<ext>\ShellEx\{8895b1c6-b41f-4c1c-a562-0d564250836f}
//	    (default) = the ZipScope preview handler CLSID
//
//	Software\Microsoft\Windows\CurrentVersion\PreviewHandlers
//	    <CLSID> = "ZipScope Preview Handler"
//
// The second one is the part that is easy to miss: Windows only offers preview
// handlers that appear in that list, so a handler registered solely under the
// extension is silently ignored. That is why the first version of this feature
// never ran inside Explorer.
//
// Nothing here touches the ZIP shell integration itself. Extraction, the
// "Extract All" verb, the folder handler and the default application are all
// registered elsewhere and are left alone.
package shellreg

import (
	"errors"
	"fmt"
	"strings"
)

// Supported extensions and the preview handler's identity.
const (
	extZip   = ".zip"
	extCbz   = ".cbz"
	previewCLSID  = "{6F1B2A12-8C3D-4E57-9A21-5B7C0D3E8F41}"
	previewName   = "ZipScope Preview Handler"

	// shellExPreview is the well-known ShellEx handler ID for preview handlers.
	shellExPreview = "{8895B1C6-B41F-4C1C-A562-0D564250836F}"
)

// ErrUnsupportedExt is returned for an extension ZipScope does not preview.
var ErrUnsupportedExt = errors.New("shellreg: unsupported extension")

// Normalize maps a caller-supplied extension to one ZipScope knows about.
// It accepts "zip", ".zip", ".ZIP" and so on.
func Normalize(ext string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(ext))
	if !strings.HasPrefix(e, ".") {
		e = "." + e
	}
	switch e {
	case extZip, extCbz:
		return e, nil
	}
	return "", fmt.Errorf("%w: %q", ErrUnsupportedExt, ext)
}

// Extensions lists the extensions this package manages, in display order.
func Extensions() []string { return []string{extZip, extCbz} }

// PreviewCLSID returns the preview handler CLSID as registered.
func PreviewCLSID() string { return previewCLSID }

// IsOwnHandler reports whether a CLSID string is ZipScope's preview handler.
func IsOwnHandler(clsid string) bool {
	return equalCLSID(clsid, previewCLSID)
}

// String implements fmt.Stringer for logging.
func (e ext) String() string { return string(e) }

type ext string

// backupSentinel marks "the key did not exist before we took it over", which is
// different from "it existed and held an empty value".
const backupSentinel = "<absent>"
