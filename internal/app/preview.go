package app

import (
	"github.com/tonbiattack/zip-scope/internal/shellreg"
)

// PreviewSetting reports whether ZipScope handles the preview pane for one
// extension, plus the CLSID of whatever handler is registered right now.
//
// The CLSID is returned so the UI can say "another application's handler is
// installed" rather than implying the slot is empty.
type PreviewSetting struct {
	// Ext is the normalised extension, for example ".zip".
	Ext string `json:"ext"`
	// Enabled reports whether ZipScope's handler owns the slot.
	Enabled bool `json:"enabled"`
	// CurrentHandler is the CLSID registered for the extension, empty if none.
	CurrentHandler string `json:"currentHandler"`
	// IsZipScope reports whether CurrentHandler is ZipScope's own CLSID.
	IsZipScope bool `json:"isZipScope"`
}

// GetPreviewSettings lists the preview integration state for .zip and .cbz.
func (a *App) GetPreviewSettings() ([]PreviewSetting, error) {
	out := make([]PreviewSetting, 0, len(shellreg.Extensions()))
	for _, ext := range shellreg.Extensions() {
		cls, err := shellreg.HandlerCLSID(ext)
		if err != nil {
			return nil, err
		}
		ours := shellreg.IsOwnHandler(cls)
		out = append(out, PreviewSetting{
			Ext:            ext,
			Enabled:        ours,
			CurrentHandler: cls,
			IsZipScope:     ours,
		})
	}
	return out, nil
}

// SetPreviewEnabled turns ZipScope's preview pane integration on or off for one
// extension. Turning it off puts back whatever handler was registered before.
func (a *App) SetPreviewEnabled(ext string, enabled bool) ([]PreviewSetting, error) {
	if err := shellreg.SetEnabled(ext, enabled); err != nil {
		return nil, err
	}
	return a.GetPreviewSettings()
}

// DisablePreviewAll restores every extension. It is what uninstall should call.
func (a *App) DisablePreviewAll() error {
	return shellreg.DisableAll()
}
