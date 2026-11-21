// Invokes the registered "ZipScopeで開く" verb through the Shell and checks that
// the viewer actually starts.
//
// This is the end-to-end check for the context menu: it goes through the
// registry, the handler, the executable lookup and CreateProcess, so a broken
// path or a missing App Paths entry shows up here.
#include <windows.h>
#include <shlobj.h>
#include <shlwapi.h>
#include <shobjidl.h>
#include <tlhelp32.h>
#include <wrl/client.h>

#include <cstdio>
#include <string>

using Microsoft::WRL::ComPtr;

namespace {

int g_failures = 0;

void Check(bool cond, const char* what) {
    printf("  %s %s\n", cond ? "ok  " : "FAIL", what);
    if (!cond) g_failures++;
}

// CountProcesses returns how many processes with the given image name exist.
int CountProcesses(const wchar_t* imageName) {
    int count = 0;
    HANDLE snap = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if (snap == INVALID_HANDLE_VALUE) return 0;
    PROCESSENTRY32W pe = {};
    pe.dwSize = sizeof(pe);
    if (Process32FirstW(snap, &pe)) {
        do {
            if (_wcsicmp(pe.szExeFile, imageName) == 0) count++;
        } while (Process32NextW(snap, &pe));
    }
    CloseHandle(snap);
    return count;
}

// InvokeVerb finds our menu item and invokes it.
bool InvokeVerb(const wchar_t* path) {
    PIDLIST_ABSOLUTE pidl = ILCreateFromPathW(path);
    if (!pidl) return false;

    ComPtr<IShellFolder> parent;
    PCUITEMID_CHILD child = nullptr;
    if (FAILED(SHBindToParent(pidl, IID_PPV_ARGS(&parent), &child))) {
        ILFree(pidl);
        return false;
    }

    ComPtr<IContextMenu> menu;
    if (FAILED(CDefFolderMenu_Create2(pidl, nullptr, 1, &child, parent.Get(), nullptr, 0,
                                      nullptr, &menu))) {
        ILFree(pidl);
        return false;
    }

    HMENU hmenu = CreatePopupMenu();
    if (FAILED(menu->QueryContextMenu(hmenu, 0, 1, 0x7FFF, CMF_NORMAL))) {
        DestroyMenu(hmenu);
        ILFree(pidl);
        return false;
    }

    // Find the command id of our verb.
    UINT cmdId = 0;
    int count = GetMenuItemCount(hmenu);
    for (int i = 0; i < count; i++) {
        wchar_t text[256] = {};
        GetMenuStringW(hmenu, i, text, ARRAYSIZE(text), MF_BYPOSITION);
        if (wcsstr(text, L"ZipScope")) {
            cmdId = GetMenuItemID(hmenu, i);
            break;
        }
    }
    DestroyMenu(hmenu);
    if (cmdId == 0) {
        ILFree(pidl);
        return false;
    }

    // Invoke it the way Explorer does.
    CMINVOKECOMMANDINFOEX info = {};
    info.cbSize = sizeof(info);
    info.fMask = CMIC_MASK_UNICODE | CMIC_MASK_FLAG_NO_UI;
    info.hwnd = nullptr;
    info.lpVerb = MAKEINTRESOURCEA(cmdId - 1);
    info.lpVerbW = MAKEINTRESOURCEW(cmdId - 1);
    info.nShow = SW_SHOWNORMAL;
    HRESULT hr = menu->InvokeCommand(reinterpret_cast<LPCMINVOKECOMMANDINFO>(&info));

    menu.Reset();
    parent.Reset();
    ILFree(pidl);
    return SUCCEEDED(hr);
}

}  // namespace

int wmain(int argc, wchar_t** argv) {
    setvbuf(stdout, nullptr, _IONBF, 0);
    if (argc < 2) {
        printf("usage: launchtest <archive.zip>\n");
        return 2;
    }

    HRESULT hr = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(hr)) return 2;

    wprintf(L"\n%s\n", argv[1]);

    int before = CountProcesses(L"ZipScope.exe");
    printf("  ZipScope processes before: %d\n", before);

    bool invoked = InvokeVerb(argv[1]);
    Check(invoked, "context menu verb invoked");

    // Give the viewer time to start.
    bool started = false;
    for (int i = 0; i < 100; i++) {
        if (CountProcesses(L"ZipScope.exe") > before) {
            started = true;
            break;
        }
        Sleep(100);
    }
    Check(started, "ZipScope.exe started");

    if (started) {
        printf("  ZipScope processes after: %d\n", CountProcesses(L"ZipScope.exe"));
    }

    CoUninitialize();
    printf("\n%s\n", g_failures == 0 ? "ALL PASSED" : "FAILURES PRESENT");
    return g_failures == 0 ? 0 : 1;
}
