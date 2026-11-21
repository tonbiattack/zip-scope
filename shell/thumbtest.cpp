// Verifies that the registered thumbnail provider works by asking the Shell for
// a thumbnail exactly the way Explorer does.
//
// This is the end-to-end check: it goes through the registry, the COM class
// factory and the provider, so a registration mistake or a broken provider
// shows up here rather than as a missing icon in Explorer.
#include <windows.h>
#include <shobjidl.h>
#include <shlwapi.h>
#include <thumbcache.h>
#include <wrl/client.h>

#include <cstdio>

using Microsoft::WRL::ComPtr;

namespace {

int g_failures = 0;

void Check(bool cond, const char* what) {
    printf("  %s %s\n", cond ? "ok  " : "FAIL", what);
    if (!cond) g_failures++;
}

// TestThumbnail asks the Shell for a thumbnail of path and reports the result.
void TestThumbnail(const wchar_t* path) {
    wprintf(L"\n%s\n", path);

    ComPtr<IShellItem> item;
    HRESULT hr = SHCreateItemFromParsingName(path, nullptr, IID_PPV_ARGS(&item));
    if (FAILED(hr)) {
        printf("  FAIL SHCreateItemFromParsingName: 0x%08lX\n", hr);
        g_failures++;
        return;
    }

    ComPtr<IShellItemImageFactory> factory;
    hr = item.As(&factory);
    if (FAILED(hr)) {
        printf("  FAIL IShellItemImageFactory: 0x%08lX\n", hr);
        g_failures++;
        return;
    }

    // SIIGBF_THUMBNAILONLY forces the thumbnail path, which is what the
    // extension provides. Without it the Shell might fall back to the icon.
    SIZE size = {256, 256};
    HBITMAP bmp = nullptr;
    hr = factory->GetImage(size, SIIGBF_THUMBNAILONLY, &bmp);
    if (FAILED(hr) || !bmp) {
        printf("  FAIL GetImage: 0x%08lX (no thumbnail produced)\n", hr);
        g_failures++;
        return;
    }

    BITMAP bm = {};
    GetObject(bmp, sizeof(bm), &bm);
    printf("  thumbnail: %ldx%ld\n", bm.bmWidth, bm.bmHeight);
    Check(bm.bmWidth > 0 && bm.bmHeight > 0, "thumbnail has pixels");
    Check(bm.bmWidth <= 256 && bm.bmHeight <= 256, "thumbnail within requested size");

    // A generic archive icon would be a flat 32-bit image; a real thumbnail of
    // our gradient test data has many distinct colours. Sampling a few pixels
    // distinguishes the two.
    BITMAPINFO bmi = {};
    bmi.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    bmi.bmiHeader.biWidth = bm.bmWidth;
    bmi.bmiHeader.biHeight = -bm.bmHeight;
    bmi.bmiHeader.biPlanes = 1;
    bmi.bmiHeader.biBitCount = 32;
    bmi.bmiHeader.biCompression = BI_RGB;

    HDC dc = GetDC(nullptr);
    void* bits = nullptr;
    HBITMAP dib = CreateDIBSection(dc, &bmi, DIB_RGB_COLORS, &bits, nullptr, 0);
    if (dib && bits) {
        HDC src = CreateCompatibleDC(dc);
        HGDIOBJ old = SelectObject(src, bmp);
        BitBlt(static_cast<HDC>(CreateCompatibleDC(dc)), 0, 0, 0, 0, nullptr, 0, 0, 0);
        // Copy through GDI so the DIB section receives the pixels.
        HDC dst = CreateCompatibleDC(dc);
        HGDIOBJ oldDst = SelectObject(dst, dib);
        BitBlt(dst, 0, 0, bm.bmWidth, bm.bmHeight, src, 0, 0, SRCCOPY);
        SelectObject(dst, oldDst);
        DeleteDC(dst);
        SelectObject(src, old);
        DeleteDC(src);

        const DWORD* px = static_cast<const DWORD*>(bits);
        DWORD first = px[0];
        bool varied = false;
        for (LONG i = 0; i < bm.bmWidth * bm.bmHeight; i++) {
            if (px[i] != first) {
                varied = true;
                break;
            }
        }
        Check(varied, "thumbnail is a real image, not a flat icon");
        DeleteObject(dib);
    }
    ReleaseDC(nullptr, dc);
    DeleteObject(bmp);
}

}  // namespace

int wmain(int argc, wchar_t** argv) {
    if (argc < 2) {
        printf("usage: thumbtest <archive.zip> [...]\n");
        return 2;
    }

    HRESULT hr = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(hr)) {
        printf("CoInitializeEx failed: 0x%08lX\n", hr);
        return 2;
    }

    for (int i = 1; i < argc; i++) {
        TestThumbnail(argv[i]);
    }

    CoUninitialize();
    printf("\n%s\n", g_failures == 0 ? "ALL PASSED" : "FAILURES PRESENT");
    return g_failures == 0 ? 0 : 1;
}
