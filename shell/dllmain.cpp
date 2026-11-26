// DLL entry point, lifetime counting and viewer launching.
#include "dllmain.h"

#include <shlwapi.h>
#include <vector>

#include "debuglog.h"
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

namespace {

// Exists reports whether path is an existing file.
bool Exists(const std::wstring& path) {
    return !path.empty() && PathFileExistsW(path.c_str()) != FALSE;
}

// ReadAppPath looks up an executable registered under App Paths, which is how
// Windows records where an installed program lives.
std::wstring ReadAppPath(HKEY root, const wchar_t* subkey) {
    HKEY key = nullptr;
    if (RegOpenKeyExW(root, subkey, 0, KEY_READ, &key) != ERROR_SUCCESS) {
        return std::wstring();
    }
    wchar_t buf[MAX_PATH * 2] = {};
    DWORD size = sizeof(buf);
    DWORD type = 0;
    LONG rc = RegQueryValueExW(key, nullptr, nullptr, &type,
                               reinterpret_cast<BYTE*>(buf), &size);
    RegCloseKey(key);
    if (rc != ERROR_SUCCESS || (type != REG_SZ && type != REG_EXPAND_SZ)) {
        return std::wstring();
    }
    // An App Paths value may be quoted.
    std::wstring value(buf);
    if (value.size() >= 2 && value.front() == L'"' && value.back() == L'"') {
        value = value.substr(1, value.size() - 2);
    }
    return value;
}

}  // namespace

std::wstring ZipScopeExePath() {
    // 1. Next to the DLL, which is how the installer lays it out.
    std::wstring dll = ModulePath();
    if (!dll.empty()) {
        std::wstring dir = dll.substr(0, dll.find_last_of(L'\\') + 1);
        std::wstring candidate = dir + L"ZipScope.exe";
        if (Exists(candidate)) return candidate;

        // 2. A development checkout keeps the DLL in build\shell and the
        //    executable in build\bin, so look one level across as well.
        std::wstring parent = dir;
        if (!parent.empty() && parent.back() == L'\\') parent.pop_back();
        size_t slash = parent.find_last_of(L'\\');
        if (slash != std::wstring::npos) {
            std::wstring up = parent.substr(0, slash + 1);
            candidate = up + L"bin\\ZipScope.exe";
            if (Exists(candidate)) return candidate;
        }
    }

    // 3. The per-user install location the README documents.
    wchar_t localAppData[MAX_PATH * 2] = {};
    DWORD n = GetEnvironmentVariableW(L"LOCALAPPDATA", localAppData, ARRAYSIZE(localAppData));
    if (n > 0 && n < ARRAYSIZE(localAppData)) {
        std::wstring candidate = std::wstring(localAppData) +
                                 L"\\Programs\\ZipScope\\ZipScope.exe";
        if (Exists(candidate)) return candidate;
    }

    // 4. App Paths, which the installer writes. This is the same lookup the
    //    Run dialog uses, so it works regardless of where the app was placed.
    const wchar_t* appPathKey = L"Software\\Microsoft\\Windows\\CurrentVersion\\App Paths\\ZipScope.exe";
    if (std::wstring p = ReadAppPath(HKEY_CURRENT_USER, appPathKey); Exists(p)) return p;
    if (std::wstring p = ReadAppPath(HKEY_LOCAL_MACHINE, appPathKey); Exists(p)) return p;

    // 5. Anywhere on PATH.
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
            // Tracing is off unless the marker file exists. Recording the load
            // is what makes it possible to tell whether Explorer or prevhost
            // ever picked the DLL up at all.
            zipscope::Log("DLL loaded");
            break;
        case DLL_PROCESS_DETACH:
            zipscope::Log("DLL unloaded");
            zipscope::g_module = nullptr;
            break;
        default:
            break;
    }
    return TRUE;
}
