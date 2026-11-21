@echo off
REM Remove the ZipScope Explorer integration and the installed files.

setlocal

set INSTALL_DIR=%LOCALAPPDATA%\Programs\ZipScope

if exist "%INSTALL_DIR%\ZipScopeShell.dll" (
  echo Unregistering the shell extension...
  regsvr32 /s /u "%INSTALL_DIR%\ZipScopeShell.dll"
)

echo Removing App Paths...
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\App Paths\ZipScope.exe" /f >nul 2>&1

echo Removing files...
del /q "%INSTALL_DIR%\ZipScopeShell.dll" >nul 2>&1
del /q "%INSTALL_DIR%\ZipScope.exe" >nul 2>&1
rmdir "%INSTALL_DIR%" >nul 2>&1

echo.
echo Uninstalled. Restart Explorer for the change to take effect:
echo   taskkill /f /im explorer.exe ^&^& start explorer.exe
echo.
echo The cache in %LOCALAPPDATA%\ZipScope was left in place.
echo Delete that folder too if you want a clean removal.
endlocal
