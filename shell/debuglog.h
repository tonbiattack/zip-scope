// Opt-in diagnostic tracing for the shell extension.
//
// Explorer hosts this DLL inside its own process or inside prevhost.exe, where
// there is no console to read output from. When something does not work, the only
// way to tell whether a call arrived is to write it down.
//
// Tracing is off unless the marker file exists, so a normal install writes
// nothing and pays nothing beyond one cached attribute check:
//
//   %LOCALAPPDATA%\ZipScope\preview.debug   create this file to enable tracing
//   %LOCALAPPDATA%\ZipScope\preview.log     the trace
//
// The check is deliberately cheap and cached in a function-local static, because
// Log is called from DllMain and from paint handlers.
#pragma once

namespace zipscope {

// Log appends one timestamped line to the trace file when tracing is enabled.
void Log(const char* format, ...);

// LogEnabled reports whether tracing is on, so callers can skip building
// expensive arguments.
bool LogEnabled();

}  // namespace zipscope
