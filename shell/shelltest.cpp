// Standalone test harness for the shell extension's ZIP reader and inflater.
//
// Explorer loads the extension in-process, so the parsing code is exercised
// here against real archives before it ever runs inside explorer.exe. The
// harness links the same translation units the DLL uses.
#include <windows.h>

#include <cstdio>
#include <cstring>
#include <string>
#include <vector>

#include "decode.h"
#include "zip_reader.h"

namespace {

int g_failures = 0;

void Check(bool cond, const char* what) {
    if (cond) {
        printf("  ok   %s\n", what);
    } else {
        printf("  FAIL %s\n", what);
        g_failures++;
    }
}

// ReadFileBytes loads a whole file.
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

// TestArchive verifies that the first image can be extracted and decoded.
void TestArchive(const wchar_t* path) {
    wprintf(L"\n%s\n", path);
    std::vector<BYTE> archive;
    if (!ReadFileBytes(path, &archive)) {
        printf("  FAIL could not read the file\n");
        g_failures++;
        return;
    }
    printf("  archive: %zu bytes\n", archive.size());

    std::vector<BYTE> image;
    bool got = zipscope::ReadFirstImage(archive, 128ull * 1024 * 1024, 40ull * 1000 * 1000,
                                        &image);
    Check(got, "first image extracted");
    if (!got) return;
    printf("  image: %zu bytes\n", image.size());

    Check(zipscope::WithinPixelLimit(image, 40ull * 1000 * 1000), "within pixel limit");

    HBITMAP bmp = nullptr;
    bool decoded = zipscope::DecodeToHBITMAP(image, 256, &bmp);
    Check(decoded, "decoded to HBITMAP");
    if (decoded && bmp) {
        BITMAP bm = {};
        GetObject(bmp, sizeof(bm), &bm);
        printf("  bitmap: %ldx%ld\n", bm.bmWidth, bm.bmHeight);
        Check(bm.bmWidth <= 256 && bm.bmHeight <= 256, "scaled within bounds");
        Check(bm.bmWidth > 0 && bm.bmHeight > 0, "non-empty bitmap");
        DeleteObject(bmp);
    }
}

// TestRejects verifies that a non-archive is rejected rather than crashing.
void TestRejects(const wchar_t* path, const char* label) {
    printf("\n%s (%s)\n", path, label);
    std::vector<BYTE> data;
    if (!ReadFileBytes(path, &data)) {
        printf("  (file missing, skipped)\n");
        return;
    }
    std::vector<BYTE> image;
    bool got = zipscope::ReadFirstImage(data, 128ull * 1024 * 1024, 40ull * 1000 * 1000,
                                        &image);
    Check(!got, "rejected without crashing");
}

}  // namespace

int wmain(int argc, wchar_t** argv) {
    if (argc < 2) {
        printf("usage: shelltest <archive.zip> [more.zip ...]\n");
        return 2;
    }

    // WIC needs COM initialised.
    HRESULT hr = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(hr)) {
        printf("CoInitializeEx failed: 0x%08lX\n", hr);
        return 2;
    }

    for (int i = 1; i < argc; i++) {
        TestArchive(argv[i]);
    }

    CoUninitialize();
    printf("\n%s\n", g_failures == 0 ? "ALL PASSED" : "FAILURES PRESENT");
    return g_failures == 0 ? 0 : 1;
}
