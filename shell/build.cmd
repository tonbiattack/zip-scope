@echo off
REM Build the ZipScope shell extension (x64).
REM
REM Requires the MSVC build tools and the Windows SDK. The script locates
REM vcvars64.bat itself so it can be run from a plain command prompt.

setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0
set OUT_DIR=%SCRIPT_DIR%..\build\shell
set OBJ_DIR=%SCRIPT_DIR%obj

REM Locate the Visual Studio build environment.
REM
REM vswhere is installed with every Visual Studio 2017+ edition and reports the
REM real installation path, so this works for BuildTools, Community,
REM Professional and Enterprise without hard-coding each layout. The fixed
REM paths below are a fallback for machines where vswhere is missing.
set VCVARS=
set VSWHERE=%ProgramFiles(x86)%\Microsoft Visual Studio\Installer\vswhere.exe
if exist "%VSWHERE%" (
  for /f "usebackq tokens=*" %%I in (`"%VSWHERE%" -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath`) do (
    if exist "%%I\VC\Auxiliary\Build\vcvars64.bat" (
      set VCVARS=%%I\VC\Auxiliary\Build\vcvars64.bat
      goto :found
    )
  )
)

for %%P in (
  "C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Auxiliary\Build\vcvars64.bat"
  "C:\Program Files\Microsoft Visual Studio\2022\Professional\VC\Auxiliary\Build\vcvars64.bat"
  "C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\vcvars64.bat"
  "C:\Program Files\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvars64.bat"
  "C:\Program Files (x86)\Microsoft Visual Studio\2022\Enterprise\VC\Auxiliary\Build\vcvars64.bat"
  "C:\Program Files (x86)\Microsoft Visual Studio\2022\Professional\VC\Auxiliary\Build\vcvars64.bat"
  "C:\Program Files (x86)\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\vcvars64.bat"
  "C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvars64.bat"
) do (
  if exist %%P (
    set VCVARS=%%~P
    goto :found
  )
)
echo ERROR: vcvars64.bat not found. Install the Visual Studio C++ build tools.
exit /b 1

:found
echo Using %VCVARS%
call "%VCVARS%" >nul
if errorlevel 1 (
  echo ERROR: failed to initialise the MSVC environment.
  exit /b 1
)

if not exist "%OUT_DIR%" mkdir "%OUT_DIR%"
if not exist "%OBJ_DIR%" mkdir "%OBJ_DIR%"

echo Compiling...
cl /nologo /c /EHsc /std:c++17 /O2 /W4 /WX- /utf-8 /DUNICODE /D_UNICODE ^
   /Fo"%OBJ_DIR%\\" ^
   "%SCRIPT_DIR%ZipScopeShell.cpp" ^
   "%SCRIPT_DIR%dllmain.cpp" ^
   "%SCRIPT_DIR%zip_reader.cpp" ^
   "%SCRIPT_DIR%decode.cpp" ^
   "%SCRIPT_DIR%inflate.cpp" ^
   "%SCRIPT_DIR%register.cpp"
if errorlevel 1 (
  echo ERROR: compilation failed.
  exit /b 1
)

echo Linking...
REM The resource file carries the icon and version info, so the DLL shows the
REM ZipScope icon in Explorer's handler list and in file properties.
rc /nologo /fo "%OBJ_DIR%\ZipScopeShell.res" "%SCRIPT_DIR%ZipScopeShell.rc"
if errorlevel 1 (
  echo ERROR: resource compilation failed.
  exit /b 1
)

link /nologo /DLL /OUT:"%OUT_DIR%\ZipScopeShell.dll" ^
     /DEF:"%SCRIPT_DIR%ZipScopeShell.def" ^
     /IMPLIB:"%OBJ_DIR%\ZipScopeShell.lib" ^
     "%OBJ_DIR%\ZipScopeShell.obj" ^
     "%OBJ_DIR%\dllmain.obj" ^
     "%OBJ_DIR%\zip_reader.obj" ^
     "%OBJ_DIR%\decode.obj" ^
     "%OBJ_DIR%\inflate.obj" ^
     "%OBJ_DIR%\register.obj" ^
     "%OBJ_DIR%\ZipScopeShell.res" ^
     shlwapi.lib ole32.lib oleaut32.lib uuid.lib windowscodecs.lib advapi32.lib shell32.lib user32.lib gdi32.lib
if errorlevel 1 (
  echo ERROR: link failed.
  exit /b 1
)

echo.
echo Built %OUT_DIR%\ZipScopeShell.dll
endlocal
