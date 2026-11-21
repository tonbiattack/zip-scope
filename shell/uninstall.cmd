@echo off
REM Unregister the ZipScope shell extension and remove the installed DLL.

setlocal

set INSTALL_DIR=%LOCALAPPDATA%\Programs\ZipScope

net session >nul 2>&1
if errorlevel 1 (
  echo ERROR: this script must run as Administrator.
  exit /b 1
)

if exist "%INSTALL_DIR%\ZipScopeShell.dll" (
  echo Unregistering...
  regsvr32 /s /u "%INSTALL_DIR%\ZipScopeShell.dll"
  del /q "%INSTALL_DIR%\ZipScopeShell.dll"
)

echo.
echo Unregistered. Restart Explorer for the change to take effect:
echo   taskkill /f /im explorer.exe ^&^& start explorer.exe
endlocal
