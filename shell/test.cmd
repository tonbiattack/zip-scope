@echo off
REM Build and run the shell extension's test harnesses.
REM
REM These link the same parsing and decoding translation units the DLL uses, so a
REM bug in the reader shows up here instead of inside explorer.exe.
REM
REM   shelltest    ZIP index, extraction, decoding, rejecting bad input
REM   previewtest  the preview handler end to end, with a screenshot
REM   thumbtest    the thumbnail provider through COM (needs a real archive)
REM   menutest     the context menu through COM (needs a real archive)
REM
REM Passing an archive path forwards it to the harnesses that need one.

setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0
set OBJ_DIR=%SCRIPT_DIR%obj-test
set SRC_DIR=%~dp0.
set OUT_DIR=%SCRIPT_DIR%..\build\shell

REM Locate Visual Studio through vswhere, with fixed paths as a fallback.
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
call "%VCVARS%" >nul
if errorlevel 1 exit /b 1

if not exist "%OBJ_DIR%" mkdir "%OBJ_DIR%"
if not exist "%OUT_DIR%" mkdir "%OUT_DIR%"

set LIBS=shlwapi.lib ole32.lib oleaut32.lib uuid.lib windowscodecs.lib gdi32.lib user32.lib shell32.lib advapi32.lib psapi.lib

echo Compiling shelltest...
cl /nologo /EHsc /std:c++17 /O2 /utf-8 /DUNICODE /D_UNICODE ^
   /Fo"%OBJ_DIR%\\" /Fe"%OUT_DIR%\shelltest.exe" ^
   "%SCRIPT_DIR%shelltest.cpp" ^
   "%SCRIPT_DIR%zip_reader.cpp" ^
   "%SCRIPT_DIR%decode.cpp" ^
   "%SCRIPT_DIR%inflate.cpp" ^
   /link %LIBS%
if errorlevel 1 (
  echo ERROR: shelltest build failed.
  exit /b 1
)

echo Compiling previewtest...
cl /nologo /EHsc /std:c++17 /O2 /utf-8 /DUNICODE /D_UNICODE /I"%SRC_DIR%" ^
   /Fo"%OBJ_DIR%\\" /Fe"%OUT_DIR%\previewtest.exe" ^
   "%SCRIPT_DIR%previewtest.cpp" ^
   "%SCRIPT_DIR%preview_handler.cpp" ^
   "%SCRIPT_DIR%zip_reader.cpp" ^
   "%SCRIPT_DIR%decode.cpp" ^
   "%SCRIPT_DIR%inflate.cpp" ^
   "%SCRIPT_DIR%dllmain.cpp" ^
   /link %LIBS%
if errorlevel 1 (
  echo ERROR: previewtest build failed.
  exit /b 1
)

echo Compiling previewbench...
cl /nologo /EHsc /std:c++17 /O2 /utf-8 /DUNICODE /D_UNICODE /I"%SRC_DIR%" ^
   /Fo"%OBJ_DIR%\\" /Fe"%OUT_DIR%\previewbench.exe" ^
   "%SCRIPT_DIR%previewbench.cpp" ^
   /link ole32.lib uuid.lib psapi.lib user32.lib
if errorlevel 1 (
  echo ERROR: previewbench build failed.
  exit /b 1
)

echo.
echo Running tests...
"%OUT_DIR%\shelltest.exe" %*
endlocal
