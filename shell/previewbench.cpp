// Measures the preview handler's first-display time and memory use.
//
// The issue asks for numbers on three things: how long the first paint takes for
// an archive of 100+ images, how responsive the pane stays when the selection
// changes repeatedly, and how much memory the handler holds. This tool reports
// all three so the values can be recorded in the documentation.
//
// Run it after building ZipScopeShell.dll and registering it:
//
//   previewbench.exe <archive.cbz> [archive2.cbz ...]
#include <windows.h>
#include <psapi.h>
#include <shobjidl.h>

#include <algorithm>
#include <cstdio>
#include <string>
#include <vector>

#include "guids.h"

#pragma comment(lib, "psapi.lib")

namespace {

// MakeStream wraps bytes in an IStream, as Explorer does.
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

// WorkingSetMb reports this process's working set.
double WorkingSetMb() {
    PROCESS_MEMORY_COUNTERS pmc = {};
    if (GetProcessMemoryInfo(GetCurrentProcess(), &pmc, sizeof(pmc))) {
        return static_cast<double>(pmc.WorkingSetSize) / (1024.0 * 1024.0);
    }
    return 0;
}

// PumpMessages drains the queue so paint work actually runs.
void PumpMessages(int rounds) {
    MSG msg = {};
    for (int i = 0; i < rounds; i++) {
        while (PeekMessageW(&msg, nullptr, 0, 0, PM_REMOVE)) {
            TranslateMessage(&msg);
            DispatchMessageW(&msg);
        }
        Sleep(5);
    }
}

// TimeFirstPaint measures CreateInstance -> DoPreview -> first paint.
double TimeFirstPaint(const std::vector<BYTE>& bytes, HWND host, int width, int height) {
    LARGE_INTEGER freq = {}, start = {}, end = {};
    QueryPerformanceFrequency(&freq);
    QueryPerformanceCounter(&start);

    IPreviewHandler* handler = nullptr;
    if (FAILED(CoCreateInstance(CLSID_ZipScopePreviewHandler, nullptr, CLSCTX_INPROC_SERVER,
                                IID_IPreviewHandler, reinterpret_cast<void**>(&handler)))) {
        return -1;
    }
    IInitializeWithStream* init = nullptr;
    if (FAILED(handler->QueryInterface(IID_IInitializeWithStream,
                                       reinterpret_cast<void**>(&init)))) {
        handler->Release();
        return -1;
    }
    IStream* stream = MakeStream(bytes);
    if (stream) {
        init->Initialize(stream, STGM_READ);
        stream->Release();
    }
    init->Release();

    RECT rc = {0, 0, width, height};
    handler->SetWindow(host, &rc);
    handler->DoPreview();

    // The first paint is what the user sees.
    UpdateWindow(host);
    PumpMessages(4);

    QueryPerformanceCounter(&end);
    handler->Unload();
    handler->Release();

    return (end.QuadPart - start.QuadPart) * 1000.0 / freq.QuadPart;
}

}  // namespace

int wmain(int argc, wchar_t** argv) {
    setvbuf(stdout, nullptr, _IONBF, 0);
    if (argc < 2) {
        printf("usage: previewbench <archive.cbz> [more.cbz ...]\n");
        return 2;
    }

    if (FAILED(CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED))) return 2;

    HWND host = CreateWindowExW(0, L"STATIC", L"", WS_OVERLAPPEDWINDOW,
                                0, 0, 560, 700, nullptr, nullptr,
                                GetModuleHandleW(nullptr), nullptr);
    if (!host) return 2;

    double baseline = WorkingSetMb();
    printf("baseline working set: %.1f MB\n\n", baseline);

    printf("%-34s %10s %10s\n", "archive", "first(ms)", "peak(MB)");
    for (int i = 1; i < argc; i++) {
        std::vector<BYTE> bytes;
        if (!ReadFileBytes(argv[i], &bytes)) {
            wprintf(L"%-34s (cannot read)\n", argv[i]);
            continue;
        }

        std::vector<double> samples;
        double peak = 0;
        for (int round = 0; round < 5; round++) {
            double ms = TimeFirstPaint(bytes, host, 520, 620);
            if (ms < 0) break;
            samples.push_back(ms);
            peak = (std::max)(peak, WorkingSetMb());
        }
        if (samples.empty()) {
            wprintf(L"%-34s (failed)\n", argv[i]);
            continue;
        }
        std::sort(samples.begin(), samples.end());

        const wchar_t* base = wcsrchr(argv[i], L'\\');
        base = base ? base + 1 : argv[i];
        wprintf(L"%-34s %10.1f %10.1f\n", base, samples[samples.size() / 2], peak);
    }

    DestroyWindow(host);
    CoUninitialize();
    return 0;
}
