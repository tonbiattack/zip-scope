// Verifies that the registered context menu handler appears for a ZIP file.
//
// It asks the Shell for the context menu of a file and checks that our verb is
// present, which is exactly what Explorer does when the user right-clicks.
#include <windows.h>
#include <shlobj.h>
#include <shlwapi.h>
#include <shobjidl.h>
#include <wrl/client.h>

#include <cstdio>
#include <string>
#include <vector>

using Microsoft::WRL::ComPtr;

namespace {

int g_failures = 0;

void Check(bool cond, const char* what) {
    printf("  %s %s\n", cond ? "ok  " : "FAIL", what);
    if (!cond) g_failures++;
}

// TestContextMenu enumerates the context menu verbs for path.
void TestContextMenu(const wchar_t* path) {
    wprintf(L"\n%s\n", path);

    // Bind directly to the file's parent folder and get its child PIDL.
    PIDLIST_ABSOLUTE filePidl = ILCreateFromPathW(path);
    if (!filePidl) {
        printf("  FAIL ILCreateFromPathW\n");
        g_failures++;
        return;
    }

    ComPtr<IShellFolder> parent;
    PCUITEMID_CHILD childRel = nullptr;
    HRESULT hr = SHBindToParent(filePidl, IID_PPV_ARGS(&parent), &childRel);
    if (FAILED(hr) || !childRel) {
        printf("  FAIL SHBindToParent: 0x%08lX\n", hr);
        g_failures++;
        ILFree(filePidl);
        return;
    }

    // GetUIObjectOf returns only the folder's own menu. The registered ShellEx
    // handlers are merged in by CDefFolderMenu_Create2, which is what Explorer
    // actually calls, so the test must use it too.
    ComPtr<IContextMenu> menu;
    hr = CDefFolderMenu_Create2(filePidl, nullptr, 1, &childRel, parent.Get(), nullptr, 0,
                                nullptr, &menu);
    if (FAILED(hr)) {
        printf("  FAIL CDefFolderMenu_Create2: 0x%08lX\n", hr);
        g_failures++;
        ILFree(filePidl);
        return;
    }

    HMENU hmenu = CreatePopupMenu();
    hr = menu->QueryContextMenu(hmenu, 0, 1, 0x7FFF, CMF_NORMAL);
    if (FAILED(hr)) {
        printf("  FAIL QueryContextMenu: 0x%08lX\n", hr);
        g_failures++;
        DestroyMenu(hmenu);
        ILFree(filePidl);
        return;
    }

    // Look for our verb in the menu.
    int count = GetMenuItemCount(hmenu);
    printf("  menu items: %d\n", count);
    bool found = false;
    std::wstring foundText;
    for (int i = 0; i < count; i++) {
        wchar_t text[256] = {};
        GetMenuStringW(hmenu, i, text, ARRAYSIZE(text), MF_BYPOSITION);
        if (wcsstr(text, L"ZipScope")) {
            found = true;
            foundText = text;
        }
    }
    Check(found, "ZipScope verb present in the context menu");

    // The console code page cannot represent Japanese, so the label is written
    // to a UTF-8 file for verification instead of being printed.
    if (found) {
        FILE* f = nullptr;
        if (_wfopen_s(&f, L"menu-label.txt", L"wb") == 0 && f) {
            char utf8[512] = {};
            int n = WideCharToMultiByte(CP_UTF8, 0, foundText.c_str(), -1, utf8,
                                        sizeof(utf8), nullptr, nullptr);
            if (n > 0) fwrite(utf8, 1, n - 1, f);
            fclose(f);
        }
    }

    DestroyMenu(hmenu);
    ILFree(filePidl);
}

}  // namespace

int wmain(int argc, wchar_t** argv) {
    if (argc < 2) {
        printf("usage: menutest <archive.zip> [...]\n");
        return 2;
    }

    HRESULT hr = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(hr)) {
        printf("CoInitializeEx failed: 0x%08lX\n", hr);
        return 2;
    }

    for (int i = 1; i < argc; i++) {
        TestContextMenu(argv[i]);
    }

    CoUninitialize();
    printf("\n%s\n", g_failures == 0 ? "ALL PASSED" : "FAILURES PRESENT");
    return g_failures == 0 ? 0 : 1;
}
