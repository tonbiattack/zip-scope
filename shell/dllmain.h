// DLL lifetime and shared helpers.
#pragma once

#include <windows.h>

#include <string>

namespace zipscope {

// g_module is the DLL's own module handle, set by DllMain.
extern HMODULE g_module;

// DllAddRef and DllRelease maintain the module lock count so Explorer knows
// when the DLL can be unloaded.
void DllAddRef();
void DllRelease();
LONG DllRefCount();

// ModulePath returns the full path of this DLL.
std::wstring ModulePath();

// ClsidToString renders a CLSID in the {xxxxxxxx-...} form the registry uses.
std::wstring ClsidToString(REFCLSID clsid);

// ZipScopeExePath resolves the viewer executable. It looks next to the DLL
// first, then in %LOCALAPPDATA%\Programs\ZipScope, then on PATH.
std::wstring ZipScopeExePath();

// LaunchZipScope starts the viewer with the given archive path.
bool LaunchZipScope(const std::wstring& archivePath);

}  // namespace zipscope
