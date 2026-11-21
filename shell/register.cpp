// COM registration for the shell extension.
//
// Registration is per-user (HKEY_CURRENT_USER\Software\Classes), which is the
// same place Windows looks for user-level file associations. That means no
// administrator rights are needed and the extension can be installed by the
// same user who runs the viewer. A machine-wide install would use
// HKEY_LOCAL_MACHINE instead; the code is otherwise identical.
#include <windows.h>
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

        const wchar_t* exts[] = {L".zip", L".cbz"};
        for (const wchar_t* ext : exts) {
            zipscope::DeleteTree(
                HKEY_CURRENT_USER,
                std::wstring(L"Software\\Classes\\") + ext +
                    L"\\ShellEx\\{E357FCCD-A995-4576-B01F-234630154E96}");
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
