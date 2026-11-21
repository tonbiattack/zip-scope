@echo off
REM Register the ZipScope shell extension.
REM
REM Registration writes to HKEY_CLASSES_ROOT, so this must run elevated.
REM The DLL is copied next to ZipScope.exe so the extension and the viewer stay
REM together.

setlocal

set SCRIPT_DIR=%~dp0
set DLL=%SCRIPT_DIR%..\build\shell\ZipScopeShell.dll
set INSTALL_DIR=%LOCALAPPDATA%\Programs\ZipScope

if not exist "%DLL%" (
  echo ERROR: %DLL% not found. Run shell\build.cmd first.
  exit /b 1
)

REM Check for elevation.
net session >nul 2>&1
if errorlevel 1 (
  echo ERROR: this script must run as Administrator.
  echo Right-click and choose "Run as administrator".
  exit /b 1
)

if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"

echo Copying the extension to %INSTALL_DIR%
copy /y "%DLL%" "%INSTALL_DIR%\ZipScopeShell.dll" >nul
if errorlevel 1 (
  echo ERROR: could not copy the DLL.
  exit /b 1
)

echo Registering...
regsvr32 /s "%INSTALL_DIR%\ZipScopeShell.dll"
if errorlevel 1 (
  echo ERROR: regsvr32 failed.
  exit /b 1
)

echo.
echo Registered. Explorer will show ZIP/CBZ thumbnails and a "ZipScopeで開く"
echo context menu entry. If thumbnails do not appear immediately, sign out and
echo back in, or clear the thumbnail cache with:
echo   cleanmgr /sageset:1
endlocal
