//go:build windows

package shellreg

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Keys and values this package touches. They are spelled out so the layout can
// be inspected with regedit and matched by the uninstaller.
const (
	backupRoot = `Software\ZipScope\preview-backup`
	previewList = `Software\Microsoft\Windows\CurrentVersion\PreviewHandlers`
)

// shellExKey returns the registry path for an extension's preview handler slot.
func shellExKey(ext string) string {
	return fmt.Sprintf(`Software\Classes\%s\ShellEx\%s`, ext, shellExPreview)
}

// backupKey returns where the previous handler for an extension is remembered.
func backupKey(ext string) string {
	return fmt.Sprintf(`%s\%s`, backupRoot, ext)
}

// HandlerCLSID reports the CLSID currently registered as an extension's preview
// handler. An empty string means no handler is registered.
func HandlerCLSID(ext string) (string, error) {
	e, err := Normalize(ext)
	if err != nil {
		return "", err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, shellExKey(e), registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return "", nil
		}
		return "", err
	}
	defer k.Close()

	value, _, err := k.GetStringValue("")
	if err != nil {
		if err == registry.ErrNotExist {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

// Enabled reports whether ZipScope is the registered preview handler.
func Enabled(ext string) (bool, error) {
	cls, err := HandlerCLSID(ext)
	if err != nil {
		return false, err
	}
	return equalCLSID(cls, previewCLSID), nil
}

// SetEnabled turns ZipScope's preview handler on or off for one extension.
//
// Enabling remembers whatever handler was registered before, so disabling can
// put it back. That matters because another archive tool may own the slot; the
// user asked for ZipScope to take over, not to lose the old handler for good.
func SetEnabled(ext string, on bool) error {
	e, err := Normalize(ext)
	if err != nil {
		return err
	}
	if on {
		return enable(e)
	}
	return disable(e)
}

func enable(e string) error {
	current, err := HandlerCLSID(e)
	if err != nil {
		return err
	}
	if equalCLSID(current, previewCLSID) {
		// Already ours; make sure the handler list entry exists too, since an
		// earlier version could have written only the extension key.
		return ensureListed()
	}

	// Remember what was there. An empty CLSID with an existing key is different
	// from no key at all, so the sentinel covers the latter.
	backup := current
	if backup == "" {
		backup = backupSentinel
	}
	if err := writeString(registry.CURRENT_USER, backupKey(e), "", backup); err != nil {
		return fmt.Errorf("remember the previous preview handler: %w", err)
	}

	if err := writeString(registry.CURRENT_USER, shellExKey(e), "", previewCLSID); err != nil {
		return fmt.Errorf("register the preview handler for %s: %w", e, err)
	}
	if err := ensureListed(); err != nil {
		return err
	}
	return NotifyShell()
}

func disable(e string) error {
	backup, err := readString(registry.CURRENT_USER, backupKey(e), "")
	if err != nil {
		return err
	}

	switch {
	case backup == "":
		// Nothing recorded: we never enabled this extension, or the backup was
		// lost. Only remove the value if it is ours, so a handler belonging to
		// another application is never deleted.
		current, cErr := HandlerCLSID(e)
		if cErr != nil {
			return cErr
		}
		if equalCLSID(current, previewCLSID) {
			if err := deleteKey(registry.CURRENT_USER, shellExKey(e)); err != nil {
				return err
			}
		}
	case backup == backupSentinel:
		if err := deleteKey(registry.CURRENT_USER, shellExKey(e)); err != nil {
			return err
		}
	default:
		if err := writeString(registry.CURRENT_USER, shellExKey(e), "", backup); err != nil {
			return fmt.Errorf("restore the previous preview handler for %s: %w", e, err)
		}
	}

	_ = deleteKey(registry.CURRENT_USER, backupKey(e))

	// Drop the handler list entry once no extension uses the handler, so the
	// preview pane's handler list does not keep a dead entry.
	anyLeft, err := AnyEnabled()
	if err != nil {
		return err
	}
	if !anyLeft {
		if err := removeListed(); err != nil {
			return err
		}
	}
	return NotifyShell()
}

// AnyEnabled reports whether ZipScope still handles previews for any extension.
func AnyEnabled() (bool, error) {
	for _, e := range Extensions() {
		on, err := Enabled(e)
		if err != nil {
			return false, err
		}
		if on {
			return true, nil
		}
	}
	return false, nil
}

// DisableAll restores every extension. It is used by uninstall.
func DisableAll() error {
	var firstErr error
	for _, e := range Extensions() {
		if err := SetEnabled(e, false); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func ensureListed() error {
	return writeString(registry.CURRENT_USER, previewList, previewCLSID, previewName)
}

func removeListed() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, previewList, registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(previewCLSID); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// equalCLSID compares two CLSID strings ignoring case and surrounding space.
func equalCLSID(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func writeString(root registry.Key, path, name, value string) error {
	k, _, err := registry.CreateKey(root, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if name == "" {
		return k.SetStringValue("", value)
	}
	return k.SetStringValue(name, value)
}

func readString(root registry.Key, path, name string) (string, error) {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return "", nil
		}
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		if err == registry.ErrNotExist {
			return "", nil
		}
		return "", err
	}
	return v, nil
}

func deleteKey(root registry.Key, path string) error {
	err := registry.DeleteKey(root, path)
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// NotifyShell tells Explorer that associations changed. Without this it keeps
// serving the handler list it cached at startup, so a freshly enabled handler
// does not appear until the user signs out.
func NotifyShell() error {
	const (
		shcneAssocChanged = 0x08000000
		shcnfIDList       = 0x0000
	)
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	proc := shell32.NewProc("SHChangeNotify")
	_, _, _ = proc.Call(uintptr(shcneAssocChanged), uintptr(shcnfIDList), 0, 0)
	return nil
}
