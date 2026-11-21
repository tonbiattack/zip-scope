// Directly instantiates the context menu handler and drives it, bypassing the
// Shell's handler discovery. This separates "the handler is broken" from "the
// registration is not being picked up".
#include <windows.h>
#include <shlobj.h>
#include <shlwapi.h>
#include <shobjidl.h>
#include <wrl/client.h>

#include <cstdio>

using Microsoft::WRL::ComPtr;

// {6F1B2A11-8C3D-4E57-9A21-5B7C0D3E8F41}
static const CLSID CLSID_TestContextMenu = {
    0x6f1b2a11, 0x8c3d, 0x4e57, {0x9a, 0x21, 0x5b, 0x7c, 0x0d, 0x3e, 0x8f, 0x41}};

int g_failures = 0;

void Check(bool cond, const char* what) {
    printf("  %s %s\n", cond ? "ok  " : "FAIL", what);
    if (!cond) g_failures++;
}

int wmain(int argc, wchar_t** argv) {
    // Unbuffered so a crash still shows how far the test got.
    setvbuf(stdout, nullptr, _IONBF, 0);

    if (argc < 2) {
        printf("usage: directtest <archive.zip>\n");
        return 2;
    }

    HRESULT hr = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(hr)) return 2;

    wprintf(L"\n%s\n", argv[1]);

    // 1. Can the class be created at all?
    ComPtr<IContextMenu> menu;
    hr = CoCreateInstance(CLSID_TestContextMenu, nullptr, CLSCTX_INPROC_SERVER,
                          IID_PPV_ARGS(&menu));
    printf("  CoCreateInstance: 0x%08lX\n", hr);
    Check(SUCCEEDED(hr), "context menu CLSID instantiates");
    if (FAILED(hr)) {
        CoUninitialize();
        return 1;
    }

    // 2. Build the IDataObject the handler expects.
    PIDLIST_ABSOLUTE pidl = ILCreateFromPathW(argv[1]);
    ComPtr<IShellFolder> parent;
    PCUITEMID_CHILD child = nullptr;
    hr = SHBindToParent(pidl, IID_PPV_ARGS(&parent), &child);
    if (FAILED(hr)) {
        printf("  FAIL SHBindToParent: 0x%08lX\n", hr);
        CoUninitialize();
        return 1;
    }

    ComPtr<IDataObject> dataObj;
    hr = parent->GetUIObjectOf(nullptr, 1, &child, IID_IDataObject, nullptr, &dataObj);
    Check(SUCCEEDED(hr), "IDataObject obtained");

    // 3. Initialize the handler.
    ComPtr<IShellExtInit> init;
    hr = menu.As(&init);
    Check(SUCCEEDED(hr), "IShellExtInit available");
    if (SUCCEEDED(hr)) {
        hr = init->Initialize(pidl, dataObj.Get(), nullptr);
        printf("  Initialize: 0x%08lX\n", hr);
        Check(SUCCEEDED(hr), "Initialize accepted a ZIP path");
    }

    // 4. Ask for the menu and look for our verb.
    HMENU hmenu = CreatePopupMenu();
    hr = menu->QueryContextMenu(hmenu, 0, 1, 0x7FFF, CMF_NORMAL);
    printf("  QueryContextMenu: 0x%08lX\n", hr);
    Check(SUCCEEDED(hr), "QueryContextMenu succeeded");

    int count = GetMenuItemCount(hmenu);
    printf("  menu items: %d\n", count);
    bool found = false;
    for (int i = 0; i < count; i++) {
        wchar_t text[256] = {};
        GetMenuStringW(hmenu, i, text, ARRAYSIZE(text), MF_BYPOSITION);
        wprintf(L"    [%d] %s\n", i, text);
        if (wcsstr(text, L"ZipScope")) found = true;
    }
    Check(found, "ZipScope verb present");

    DestroyMenu(hmenu);
    ILFree(pidl);

    // Release every COM object before CoUninitialize: the handler lives in our
    // own DLL, and letting a ComPtr release it after the apartment is gone
    // crashes on shutdown.
    init.Reset();
    dataObj.Reset();
    parent.Reset();
    menu.Reset();

    CoUninitialize();
    printf("\n%s\n", g_failures == 0 ? "ALL PASSED" : "FAILURES PRESENT");
    return g_failures == 0 ? 0 : 1;
}
