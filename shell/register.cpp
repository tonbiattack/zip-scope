// COM registration for the shell extension.
//
// Registration is per-user (HKEY_CURRENT_USER\Software\Classes), which is the
// same place Windows looks for user-level file associations. That means no
// administrator rights are needed and the extension can be installed by the
// same user who runs the viewer. A machine-wide install would use
// HKEY_LOCAL_MACHINE instead; the code is otherwise identical.
#include <windows.h>
#include <cwchar>
#include <shlwapi.h>
#include <shlobj.h>

#include <string>

#include "dllmain.h"
#include "guids.h"

#pragma comment(lib, "advapi32.lib")

namespace zipscope {

namespace {

// SetKey writes a string value, creating the key as needed.
bool SetKey(HKEY root, const std::wstring& subkey, const wchar_t* name,
            const std::wstring& value) {
    HKEY key = nullptr;
    LONG rc = RegCreateKeyExW(root, subkey.c_str(), 0, nullptr, 0, KEY_WRITE, nullptr,
                              &key, nullptr);
    if (rc != ERROR_SUCCESS) return false;
    rc = RegSetValueExW(key, name, 0, REG_SZ,
                        reinterpret_cast<const BYTE*>(value.c_str()),
                        static_cast<DWORD>((value.size() + 1) * sizeof(wchar_t)));
    RegCloseKey(key);
    return rc == ERROR_SUCCESS;
}

// DeleteTree removes a key and everything under it.
void DeleteTree(HKEY root, const std::wstring& subkey) {
    SHDeleteKeyW(root, subkey.c_str());
}

}  // namespace

// RegisterThumbnailHandler wires the thumbnail provider for the .zip and .cbz
// extensions. The handler is registered per-extension rather than per-ProgID so
// it applies regardless of which application owns the file association.
bool RegisterThumbnailHandler() {
    const wchar_t* exts[] = {L".zip", L".cbz"};
    for (const wchar_t* ext : exts) {
        std::wstring base = std::wstring(L"Software\\Classes\\") + ext +
                            L"\\ShellEx\\{E357FCCD-A995-4576-B01F-234630154E96}";
        if (!SetKey(HKEY_CURRENT_USER, base, nullptr, ClsidToString(CLSID_ZipScopeThumbnail))) {
            return false;
        }
    }
    return true;
}

// RegisterContextMenu wires the context menu handler.
//
// Context menu handlers are NOT read from extension keys (".zip\ShellEx\...");
// only the thumbnail handler uses that location. Explorer looks for menu
// handlers under a ProgID, or under AllFilesystemObjects for every file. The
// handler itself rejects anything that is not a ZIP/CBZ, so registering it for
// all files is safe and works for .cbz, which has no ProgID of its own.
bool RegisterContextMenu() {
    const wchar_t* bases[] = {
        L"Software\\Classes\\AllFilesystemObjects\\ShellEx\\ContextMenuHandlers\\ZipScope",
        L"Software\\Classes\\CompressedFolder\\ShellEx\\ContextMenuHandlers\\ZipScope",
    };
    for (const wchar_t* base : bases) {
        if (!SetKey(HKEY_CURRENT_USER, base, nullptr, ClsidToString(CLSID_ZipScopeContextMenu))) {
            return false;
        }
    }
    return true;
}

// RegisterComClass writes the CLSID entry that points at this DLL. Declared here
// because RegisterPreviewComClass below builds on it.
bool RegisterComClass(REFCLSID clsid, const wchar_t* name, const std::wstring& dllPath);

// ReadString returns a registry string value, or an empty string when the key or
// value does not exist. An absent value and an empty value are different things
// to this file, so callers that care check the key themselves.
std::wstring ReadString(HKEY root, const std::wstring& path, const wchar_t* name) {
    HKEY key = nullptr;
    if (RegOpenKeyExW(root, path.c_str(), 0, KEY_QUERY_VALUE, &key) != ERROR_SUCCESS) {
        return std::wstring();
    }
    wchar_t buf[512] = {};
    DWORD type = 0;
    DWORD size = sizeof(buf) - sizeof(wchar_t);
    LONG rc = RegQueryValueExW(key, name, nullptr, &type, reinterpret_cast<LPBYTE>(buf), &size);
    RegCloseKey(key);
    if (rc != ERROR_SUCCESS) return std::wstring();
    buf[size / sizeof(wchar_t)] = L'\0';
    return std::wstring(buf);
}
// kPreviewSurrogateAppId is the "Preview Handler Surrogate Host" AppID that
// Windows itself registers in HKLM, with DllSurrogate = prevhost.exe.
//
// A preview handler must reference it: Explorer activates preview handlers in
// prevhost.exe rather than loading them into explorer.exe, and without the AppID
// activation fails silently, which shows up as an empty pane.
constexpr const wchar_t* kPreviewSurrogateAppId =
    L"{6D2B5079-2F0B-48DD-AB7F-97CEC514D30B}";

// kPreviewHandlerList is where Windows keeps the list of preview handlers it is
// willing to use. A handler registered only under the extension is ignored; this
// list is what makes it appear.
constexpr const wchar_t* kPreviewHandlerList =
    L"Software\\Microsoft\\Windows\\CurrentVersion\\PreviewHandlers";

// kPreviewShellEx is the well-known ShellEx handler ID for preview handlers.
constexpr const wchar_t* kPreviewShellEx = L"{8895B1C6-B41F-4C1C-A562-0D564250836F}";

// kPreviewBackupRoot is where the app records the handler it replaced, so
// disabling can put it back. The Go side (internal/shellreg) uses the same path.
constexpr const wchar_t* kPreviewBackupRoot = L"Software\\ZipScope\\preview-backup";

// RegisterPreviewComClass writes the CLSID entry for the preview handler and
// points it at the system preview surrogate.
//
// The handler is NOT enabled here. Enabling is opt-in per extension and is done
// from the app's settings page, which writes the extension's ShellEx slot and the
// handler list. Registering the class up front just makes the later switch
// instantaneous.
bool RegisterPreviewComClass(const std::wstring& dllPath) {
    if (!RegisterComClass(CLSID_ZipScopePreviewHandler, L"ZipScope Preview Handler", dllPath)) {
        return false;
    }
    std::wstring base =
        L"Software\\Classes\\CLSID\\" + ClsidToString(CLSID_ZipScopePreviewHandler);
    return SetKey(HKEY_CURRENT_USER, base, L"AppID", kPreviewSurrogateAppId);
}

// RestorePreviewHandler puts back whatever handler an extension had before
// ZipScope took the slot, and removes the handler-list entry if no extension is
// using ZipScope any more.
void RestorePreviewHandler() {
    const wchar_t* exts[] = {L".zip", L".cbz"};
    for (const wchar_t* ext : exts) {
        std::wstring backupPath = std::wstring(kPreviewBackupRoot) + L"\\" + ext;
        std::wstring backup = ReadString(HKEY_CURRENT_USER, backupPath, nullptr);

        std::wstring slot = std::wstring(L"Software\\Classes\\") + ext + L"\\ShellEx\\" +
                            kPreviewShellEx;

        if (backup == L"<absent>") {
            // There was no handler before: remove the slot we created.
            DeleteTree(HKEY_CURRENT_USER, slot);
        }
        else if (!backup.empty()) {
            SetKey(HKEY_CURRENT_USER, slot, nullptr, backup);
        }
        else {
            // No backup recorded. Only remove the slot if it is still ours, so a
            // handler belonging to another application is never deleted.
            std::wstring current = ReadString(HKEY_CURRENT_USER, slot, nullptr);
            if (_wcsicmp(current.c_str(), ClsidToString(CLSID_ZipScopePreviewHandler).c_str()) == 0) {
                DeleteTree(HKEY_CURRENT_USER, slot);
            }
        }
        DeleteTree(HKEY_CURRENT_USER, backupPath);
    }

    // Drop the preview handler list entry; ZipScope no longer handles previews.
    std::wstring listClsid = ClsidToString(CLSID_ZipScopePreviewHandler);
    HKEY key = nullptr;
    if (RegOpenKeyExW(HKEY_CURRENT_USER, kPreviewHandlerList, 0, KEY_SET_VALUE, &key) ==
        ERROR_SUCCESS) {
        RegDeleteValueW(key, listClsid.c_str());
        RegCloseKey(key);
    }
}
// RegisterComClass writes the CLSID entry that points at this DLL.
bool RegisterComClass(REFCLSID clsid, const wchar_t* name, const std::wstring& dllPath) {
    std::wstring clsidStr = ClsidToString(clsid);
    std::wstring base = L"Software\\Classes\\CLSID\\" + clsidStr;

    if (!SetKey(HKEY_CURRENT_USER, base, nullptr, name)) return false;
    if (!SetKey(HKEY_CURRENT_USER, base + L"\\InprocServer32", nullptr, dllPath)) return false;
    if (!SetKey(HKEY_CURRENT_USER, base + L"\\InprocServer32", L"ThreadingModel", L"Apartment")) {
        return false;
    }
    return true;
}

}  // namespace zipscope

STDAPI DllRegisterServer() {
    try {
        std::wstring dll = zipscope::ModulePath();
        if (dll.empty()) return E_FAIL;

        if (!zipscope::RegisterComClass(CLSID_ZipScopeThumbnail,
                                        L"ZipScope Thumbnail Provider", dll)) {
            return E_FAIL;
        }
        if (!zipscope::RegisterComClass(CLSID_ZipScopeContextMenu,
                                        L"ZipScope Context Menu", dll)) {
            return E_FAIL;
        }
        if (!zipscope::RegisterPreviewComClass(dll)) return E_FAIL;
        if (!zipscope::RegisterThumbnailHandler()) return E_FAIL;
        if (!zipscope::RegisterContextMenu()) return E_FAIL;

        // Tell Explorer that the thumbnail cache is stale.
        SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, nullptr, nullptr);
        return S_OK;
    } catch (...) {
        return E_FAIL;
    }
}

STDAPI DllUnregisterServer() {
    try {
        zipscope::DeleteTree(HKEY_CURRENT_USER,
                             L"Software\\Classes\\CLSID\\" +
                                 zipscope::ClsidToString(CLSID_ZipScopeThumbnail));
        zipscope::DeleteTree(HKEY_CURRENT_USER,
                             L"Software\\Classes\\CLSID\\" +
                                 zipscope::ClsidToString(CLSID_ZipScopeContextMenu));
                zipscope::RestorePreviewHandler();

        zipscope::DeleteTree(HKEY_CURRENT_USER,
                             L"Software\\Classes\\AppID\\" +
                                 zipscope::ClsidToString(APPID_ZipScopePreviewHandler));

        const wchar_t* exts[] = {L".zip", L".cbz"};
        for (const wchar_t* ext : exts) {
            zipscope::DeleteTree(
                HKEY_CURRENT_USER,
                std::wstring(L"Software\\Classes\\") + ext +
                    L"\\ShellEx\\{E357FCCD-A995-4576-B01F-234630154E96}");
            zipscope::DeleteTree(
                HKEY_CURRENT_USER,
                std::wstring(L"Software\\Classes\\") + ext +
                    L"\\ShellEx\\{8895b1c6-b41f-4c1c-a562-0d564250836f}");
            // Older builds registered the menu handler per-extension; clean
            // that up so an upgrade does not leave a dead entry behind.
            zipscope::DeleteTree(
                HKEY_CURRENT_USER,
                std::wstring(L"Software\\Classes\\") + ext +
                    L"\\ShellEx\\ContextMenuHandlers\\ZipScope");
        }
        zipscope::DeleteTree(
            HKEY_CURRENT_USER,
            L"Software\\Classes\\AllFilesystemObjects\\ShellEx\\ContextMenuHandlers\\ZipScope");
        zipscope::DeleteTree(
            HKEY_CURRENT_USER,
            L"Software\\Classes\\CompressedFolder\\ShellEx\\ContextMenuHandlers\\ZipScope");

        SHChangeNotify(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, nullptr, nullptr);
        return S_OK;
    } catch (...) {
        return E_FAIL;
    }
}
