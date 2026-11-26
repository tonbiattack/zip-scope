// Exercises the preview handler the way Explorer's preview pane does.
//
// It creates the handler through COM, hands it a stream, gives it a window and a
// rectangle, and lets it paint. A screenshot of the resulting window is saved so
// the layout can be inspected without opening Explorer.
//
// This is the check that matters for the preview pane: if the handler can be
// created, initialised, given a window and painted here, it will work in
// Explorer too. What cannot be checked here is Explorer's own activation path
// and the surrogate host, which the registration test covers separately.
#include <windows.h>
#include <shobjidl.h>
#include <wincodec.h>
#include <wrl/client.h>

#include <cstdio>
#include <string>
#include <vector>

#include "guids.h"

using Microsoft::WRL::ComPtr;

namespace {

int g_failures = 0;

void Check(bool cond, const char* what) {
    printf("  %s %s\n", cond ? "ok  " : "FAIL", what);
    if (!cond) g_failures++;
}

// ReadFileBytes loads a file into memory, which is what the handler receives
// from Explorer as a stream.
bool ReadFileBytes(const wchar_t* path, std::vector<BYTE>* out) {
    HANDLE h = CreateFileW(path, GENERIC_READ, FILE_SHARE_READ, nullptr, OPEN_EXISTING,
                           FILE_ATTRIBUTE_NORMAL, nullptr);
    if (h == INVALID_HANDLE_VALUE) return false;
    LARGE_INTEGER size = {};
    if (!GetFileSizeEx(h, &size)) {
        CloseHandle(h);
        return false;
    }
    out->resize(static_cast<size_t>(size.QuadPart));
    DWORD read = 0;
    BOOL ok = ReadFile(h, out->data(), static_cast<DWORD>(out->size()), &read, nullptr);
    CloseHandle(h);
    return ok && read == out->size();
}

// MakeStream wraps bytes in an IStream, which is what Initialize expects.
IStream* MakeStream(const std::vector<BYTE>& bytes) {
    HGLOBAL mem = GlobalAlloc(GMEM_MOVEABLE, bytes.size());
    if (!mem) return nullptr;
    void* p = GlobalLock(mem);
    if (!p) {
        GlobalFree(mem);
        return nullptr;
    }
    memcpy(p, bytes.data(), bytes.size());
    GlobalUnlock(mem);

    IStream* stream = nullptr;
    if (FAILED(CreateStreamOnHGlobal(mem, TRUE, &stream))) {
        GlobalFree(mem);
        return nullptr;
    }
    return stream;
}

// SaveWindowPng captures a window with PrintWindow into a PNG.
bool SaveWindowPng(HWND hwnd, const wchar_t* path) {
    RECT rc = {};
    if (!GetWindowRect(hwnd, &rc)) return false;
    int w = rc.right - rc.left;
    int h = rc.bottom - rc.top;
    if (w <= 0 || h <= 0) return false;

    HDC screen = GetDC(nullptr);
    HDC mem = CreateCompatibleDC(screen);
    HBITMAP bmp = CreateCompatibleBitmap(screen, w, h);
    HGDIOBJ old = SelectObject(mem, bmp);

    BOOL ok = PrintWindow(hwnd, mem, 2 /* PW_RENDERFULLCONTENT */);

    // Encode with WIC so the file is a normal PNG the reviewer can open.
    ComPtr<IWICImagingFactory> factory;
    bool saved = false;
    if (SUCCEEDED(CoCreateInstance(CLSID_WICImagingFactory, nullptr, CLSCTX_INPROC_SERVER,
                                   IID_PPV_ARGS(&factory)))) {
        ComPtr<IWICBitmap> wicBmp;
        if (SUCCEEDED(factory->CreateBitmapFromHBITMAP(bmp, nullptr, WICBitmapIgnoreAlpha,
                                                       &wicBmp))) {
            ComPtr<IWICStream> stream;
            ComPtr<IWICBitmapEncoder> encoder;
            ComPtr<IWICBitmapFrameEncode> frame;
            if (SUCCEEDED(factory->CreateStream(&stream)) &&
                SUCCEEDED(stream->InitializeFromFilename(path, GENERIC_WRITE)) &&
                SUCCEEDED(factory->CreateEncoder(GUID_ContainerFormatPng, nullptr, &encoder)) &&
                SUCCEEDED(encoder->Initialize(stream.Get(), WICBitmapEncoderNoCache)) &&
                SUCCEEDED(encoder->CreateNewFrame(&frame, nullptr)) &&
                SUCCEEDED(frame->Initialize(nullptr)) &&
                SUCCEEDED(frame->SetSize(w, h)) &&
                SUCCEEDED(frame->WriteSource(wicBmp.Get(), nullptr)) &&
                SUCCEEDED(frame->Commit()) && SUCCEEDED(encoder->Commit())) {
                saved = true;
            }
        }
    }

    SelectObject(mem, old);
    DeleteObject(bmp);
    DeleteDC(mem);
    ReleaseDC(nullptr, screen);
    return ok && saved;
}

// TestPreview creates the handler, previews the file and captures the pane.
void TestPreview(const wchar_t* path, int width, int height, const wchar_t* shot) {
    wprintf(L"\n%s (%dx%d)\n", path, width, height);

    std::vector<BYTE> bytes;
    if (!ReadFileBytes(path, &bytes)) {
        printf("  FAIL cannot read the file\n");
        g_failures++;
        return;
    }

    ComPtr<IPreviewHandler> handler;
    HRESULT hr = CoCreateInstance(CLSID_ZipScopePreviewHandler, nullptr, CLSCTX_INPROC_SERVER,
                                  IID_PPV_ARGS(&handler));
    printf("  CoCreateInstance: 0x%08lX\n", hr);
    Check(SUCCEEDED(hr), "preview handler CLSID instantiates");
    if (FAILED(hr)) return;

    // Explorer initialises with a stream.
    ComPtr<IInitializeWithStream> init;
    hr = handler.As(&init);
    Check(SUCCEEDED(hr), "IInitializeWithStream available");
    if (SUCCEEDED(hr)) {
        IStream* stream = MakeStream(bytes);
        Check(stream != nullptr, "stream created from the file");
        if (stream) {
            hr = init->Initialize(stream, STGM_READ);
            printf("  Initialize: 0x%08lX\n", hr);
            Check(SUCCEEDED(hr), "Initialize accepted the stream");
            stream->Release();
        }
    }

    // Explorer gives the handler a host window and a rectangle inside it.
    HWND host = CreateWindowExW(0, L"STATIC", L"", WS_OVERLAPPEDWINDOW | WS_VISIBLE,
                                CW_USEDEFAULT, CW_USEDEFAULT, width + 32, height + 56,
                                nullptr, nullptr, GetModuleHandleW(nullptr), nullptr);
    Check(host != nullptr, "host window created");
    if (!host) return;

    RECT rc = {0, 0, width, height};
    hr = handler->SetWindow(host, &rc);
    printf("  SetWindow: 0x%08lX\n", hr);
    Check(SUCCEEDED(hr), "SetWindow accepted the rectangle");

    hr = handler->DoPreview();
    printf("  DoPreview: 0x%08lX\n", hr);
    Check(SUCCEEDED(hr), "DoPreview succeeded");

    // Let the window paint. The first paint does the decoding, so a few
    // hundred milliseconds is normal for a large archive; the extra passes let
    // the result settle before the screenshot.
    MSG msg = {};
    for (int i = 0; i < 60; i++) {
        while (PeekMessageW(&msg, nullptr, 0, 0, PM_REMOVE)) {
            TranslateMessage(&msg);
            DispatchMessageW(&msg);
        }
        Sleep(20);
    }
    // Force a full repaint of the host and its child before capturing.
    RedrawWindow(host, nullptr, nullptr,
                 RDW_INVALIDATE | RDW_UPDATENOW | RDW_ALLCHILDREN);
    UpdateWindow(host);
    for (int i = 0; i < 10; i++) {
        while (PeekMessageW(&msg, nullptr, 0, 0, PM_REMOVE)) {
            TranslateMessage(&msg);
            DispatchMessageW(&msg);
        }
        Sleep(20);
    }

    if (shot) {
        bool saved = SaveWindowPng(host, shot);
        Check(saved, "screenshot captured");
        if (saved) wprintf(L"  wrote %s\n", shot);
    }

    hr = handler->Unload();
    Check(SUCCEEDED(hr), "Unload succeeded");
    DestroyWindow(host);
}

// TestRejects hands the handler a file that is not a ZIP.
void TestRejects(const wchar_t* path) {
    wprintf(L"\n%s (not an archive)\n", path);

    std::vector<BYTE> bytes;
    if (!ReadFileBytes(path, &bytes)) {
        printf("  (file missing, skipped)\n");
        return;
    }

    ComPtr<IPreviewHandler> handler;
    HRESULT hr = CoCreateInstance(CLSID_ZipScopePreviewHandler, nullptr, CLSCTX_INPROC_SERVER,
                                  IID_PPV_ARGS(&handler));
    if (FAILED(hr)) {
        printf("  FAIL CoCreateInstance: 0x%08lX\n", hr);
        g_failures++;
        return;
    }
    ComPtr<IInitializeWithStream> init;
    if (SUCCEEDED(handler.As(&init))) {
        IStream* stream = MakeStream(bytes);
        if (stream) {
            init->Initialize(stream, STGM_READ);
            stream->Release();
        }
    }

    HWND host = CreateWindowExW(0, L"STATIC", L"", WS_OVERLAPPEDWINDOW,
                                0, 0, 300, 300, nullptr, nullptr,
                                GetModuleHandleW(nullptr), nullptr);
    RECT rc = {0, 0, 260, 260};
    handler->SetWindow(host, &rc);
    // DoPreview must succeed and show an error state; it must not crash or hang.
    hr = handler->DoPreview();
    Check(SUCCEEDED(hr), "DoPreview reported an error state instead of failing");
    handler->Unload();
    DestroyWindow(host);
}

}  // namespace

int wmain(int argc, wchar_t** argv) {
    setvbuf(stdout, nullptr, _IONBF, 0);
    if (argc < 3) {
        printf("usage: previewtest <archive.zip> <screenshot.png> [more.zip ...]\n");
        return 2;
    }

    HRESULT hr = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(hr)) return 2;

    // Wide pane and a narrow one, to check the layout adapts.
    TestPreview(argv[1], 520, 620, argv[2]);
    if (argc > 3) {
        TestPreview(argv[3], 300, 620, nullptr);
    }
    if (argc > 4) {
        TestRejects(argv[4]);
    }

    CoUninitialize();
    printf("\n%s\n", g_failures == 0 ? "ALL PASSED" : "FAILURES PRESENT");
    return g_failures == 0 ? 0 : 1;
}
