#include "debuglog.h"

#include <windows.h>

#include <cstdarg>
#include <cstdio>
#include <cstring>
#include <string>

namespace zipscope {

namespace {

// LogDirectory returns %LOCALAPPDATA%\ZipScope, creating it if needed.
//
// Only environment variables are used, never COM or the known-folder API: Log is
// called from DllMain, where the loader lock is held and COM is not initialised
// yet. prevhost.exe does not inherit LOCALAPPDATA, so USERPROFILE is the
// fallback.
std::wstring LogDirectory() {
    wchar_t dir[MAX_PATH] = {};

    DWORD n = GetEnvironmentVariableW(L"LOCALAPPDATA", dir, MAX_PATH);
    if (n > 0 && n < MAX_PATH) {
        std::wstring folder = std::wstring(dir) + L"\\ZipScope";
        CreateDirectoryW(folder.c_str(), nullptr);
        return folder;
    }

    n = GetEnvironmentVariableW(L"USERPROFILE", dir, MAX_PATH);
    if (n > 0 && n < MAX_PATH) {
        std::wstring folder = std::wstring(dir) + L"\\AppData\\Local\\ZipScope";
        // Create each level; the first may fail because it already exists.
        CreateDirectoryW((std::wstring(dir) + L"\\AppData\\Local").c_str(), nullptr);
        CreateDirectoryW(folder.c_str(), nullptr);
        return folder;
    }

    return std::wstring();
}

// TracesEnabled caches the marker check, but never caches failure: the first
// call can arrive from DllMain before the environment is usable, and caching
// "off" there would disable tracing for the life of the process.
bool TracesEnabled() {
    static int cached = -1;
    if (cached >= 0) return cached == 1;

    std::wstring folder = LogDirectory();
    if (folder.empty()) return false;

    std::wstring marker = folder + L"\\preview.debug";
    cached = GetFileAttributesW(marker.c_str()) != INVALID_FILE_ATTRIBUTES ? 1 : 0;
    return cached == 1;
}

}  // namespace

bool LogEnabled() { return TracesEnabled(); }

void Log(const char* format, ...) {
    if (!TracesEnabled()) return;

    std::wstring folder = LogDirectory();
    if (folder.empty()) return;
    std::wstring path = folder + L"\\preview.log";

    char message[1024] = {};
    va_list args;
    va_start(args, format);
    vsnprintf_s(message, sizeof(message), _TRUNCATE, format, args);
    va_end(args);

    SYSTEMTIME st = {};
    GetLocalTime(&st);

    // The module name makes it obvious whether the line came from explorer.exe
    // or from the surrogate host.
    wchar_t module[MAX_PATH] = {};
    GetModuleFileNameW(nullptr, module, MAX_PATH);
    const wchar_t* base = wcsrchr(module, L'\\');
    base = base ? base + 1 : module;

    char line[1400] = {};
    sprintf_s(line, sizeof(line), "%02d:%02d:%02d.%03d pid=%lu tid=%lu %ls %s\r\n", st.wHour,
              st.wMinute, st.wSecond, st.wMilliseconds, GetCurrentProcessId(),
              GetCurrentThreadId(), base, message);

    HANDLE f = CreateFileW(path.c_str(), FILE_APPEND_DATA, FILE_SHARE_READ | FILE_SHARE_WRITE,
                           nullptr, OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (f == INVALID_HANDLE_VALUE) return;
    DWORD written = 0;
    WriteFile(f, line, static_cast<DWORD>(strlen(line)), &written, nullptr);
    CloseHandle(f);
}

}  // namespace zipscope
