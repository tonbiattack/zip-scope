// DLL entry point, lifetime counting and viewer launching.
#include "dllmain.h"

#include <shlwapi.h>
#include <vector>

#include "guids.h"

#pragma comment(lib, "ole32.lib")

namespace zipscope {

namespace {
// g_refCount tracks outstanding objects and server locks.
volatile LONG g_refCount = 0;
}  // namespace

// g_module is the DLL's own module handle, captured at load time. It lives at
// namespace scope so DllMain, which is outside the namespace, can set it.
HMODULE g_module = nullptr;

void DllAddRef() { InterlockedIncrement(&g_refCount); }
void DllRelease() { InterlockedDecrement(&g_refCount); }
LONG DllRefCount() { return g_refCount; }

std::wstring ModulePath() {
    wchar_t buf[MAX_PATH * 2] = {};
    if (!g_module) return std::wstring();
    DWORD n = GetModuleFileNameW(g_module, buf, ARRAYSIZE(buf));
    if (n == 0 || n >= ARRAYSIZE(buf)) return std::wstring();
    return std::wstring(buf, n);
}

std::wstring ClsidToString(REFCLSID clsid) {
    wchar_t buf[64] = {};
    if (StringFromGUID2(clsid, buf, ARRAYSIZE(buf)) == 0) return std::wstring();
    return std::wstring(buf);
}

std::wstring ZipScopeExePath() {
    // 1. Next to the DLL, which is how a portable install is laid out.
    std::wstring dll = ModulePath();
    if (!dll.empty()) {
        std::wstring dir = dll.substr(0, dll.find_last_of(L'\\') + 1);
        std::wstring candidate = dir + L"ZipScope.exe";
        if (PathFileExistsW(candidate.c_str())) return candidate;
    }

    // 2. The per-user install location the README documents.
    wchar_t localAppData[MAX_PATH * 2] = {};
    DWORD n = GetEnvironmentVariableW(L"LOCALAPPDATA", localAppData, ARRAYSIZE(localAppData));
    if (n > 0 && n < ARRAYSIZE(localAppData)) {
        std::wstring candidate = std::wstring(localAppData) +
                                 L"\\Programs\\ZipScope\\ZipScope.exe";
        if (PathFileExistsW(candidate.c_str())) return candidate;
    }

    // 3. Anywhere on PATH.
    wchar_t found[MAX_PATH * 2] = {};
    if (SearchPathW(nullptr, L"ZipScope.exe", nullptr, ARRAYSIZE(found), found, nullptr) > 0) {
        return std::wstring(found);
    }
    return std::wstring();
}

bool LaunchZipScope(const std::wstring& archivePath) {
    std::wstring exe = ZipScopeExePath();
    if (exe.empty()) {
        // Tell the user why nothing happened instead of failing silently.
        MessageBoxW(nullptr,
                    L"ZipScope.exe が見つかりません。\n"
                    L"ZipScope をインストールするか、この DLL と同じフォルダに配置してください。",
                    L"ZipScope", MB_OK | MB_ICONWARNING);
        return false;
    }

    // Quote the path so spaces and non-ASCII characters survive.
    std::wstring cmd = L"\"" + exe + L"\" \"" + archivePath + L"\"";
    std::vector<wchar_t> mutableCmd(cmd.begin(), cmd.end());
    mutableCmd.push_back(L'\0');

    STARTUPINFOW si = {};
    si.cb = sizeof(si);
    PROCESS_INFORMATION pi = {};
    // CREATE_UNICODE_ENVIRONMENT keeps the child's environment intact.
    BOOL ok = CreateProcessW(exe.c_str(), mutableCmd.data(), nullptr, nullptr, FALSE,
                             CREATE_UNICODE_ENVIRONMENT, nullptr, nullptr, &si, &pi);
    if (!ok) return false;
    CloseHandle(pi.hThread);
    CloseHandle(pi.hProcess);
    return true;
}

}  // namespace zipscope

BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    switch (reason) {
        case DLL_PROCESS_ATTACH:
            zipscope::g_module = instance;
            DisableThreadLibraryCalls(instance);
            break;
        case DLL_PROCESS_DETACH:
            zipscope::g_module = nullptr;
            break;
        default:
            break;
    }
    return TRUE;
}
