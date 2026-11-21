@echo off
REM Install ZipScope and its Explorer integration for the current user.
REM
REM Everything goes under %LOCALAPPDATA%\Programs\ZipScope, which needs no
REM administrator rights. Registration is per-user (HKCU), so the extension is
REM only active for the user who ran this script.

setlocal

set SCRIPT_DIR=%~dp0
set DLL=%SCRIPT_DIR%..\build\shell\ZipScopeShell.dll
set EXE=%SCRIPT_DIR%..\build\bin\ZipScope.exe
set INSTALL_DIR=%LOCALAPPDATA%\Programs\ZipScope

if not exist "%DLL%" (
  echo ERROR: %DLL% not found.
  echo Build it first:  shell\build.cmd
  exit /b 1
)
if not exist "%EXE%" (
  echo ERROR: %EXE% not found.
  echo Build it first:  wails build -platform windows/amd64
  exit /b 1
)

if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"

echo Installing to %INSTALL_DIR%
copy /y "%EXE%" "%INSTALL_DIR%\ZipScope.exe" >nul
if errorlevel 1 (
  echo ERROR: could not copy ZipScope.exe
  exit /b 1
)
copy /y "%DLL%" "%INSTALL_DIR%\ZipScopeShell.dll" >nul
if errorlevel 1 (
  echo ERROR: could not copy ZipScopeShell.dll
  exit /b 1
)

REM App Paths lets the extension find the viewer no matter where it was placed,
REM and makes "ZipScope" work from the Run dialog.
echo Registering App Paths...
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\App Paths\ZipScope.exe" ^
    /ve /t REG_SZ /d "%INSTALL_DIR%\ZipScope.exe" /f >nul
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\App Paths\ZipScope.exe" ^
    /v Path /t REG_SZ /d "%INSTALL_DIR%" /f >nul

echo Registering the shell extension...
regsvr32 /s "%INSTALL_DIR%\ZipScopeShell.dll"
if errorlevel 1 (
  echo ERROR: regsvr32 failed.
  exit /b 1
)

echo.
echo Installed.
echo   Viewer:      %INSTALL_DIR%\ZipScope.exe
echo   Extension:   %INSTALL_DIR%\ZipScopeShell.dll
echo.
echo Explorer now shows ZIP/CBZ cover thumbnails, and the right-click menu has
echo "ZipScope???" (under "Show more options" on Windows 11).
echo.
echo If thumbnails do not appear, clear the cache and restart Explorer:
echo   del /q "%LOCALAPPDATA%\Microsoft\Windows\Explorer\thumbcache_*.db"
echo   taskkill /f /im explorer.exe ^&^& start explorer.exe
endlocal
