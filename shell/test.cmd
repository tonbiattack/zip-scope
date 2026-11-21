@echo off
REM Build and run the shell extension's ZIP reader test harness.
REM
REM This links the same parsing and decoding translation units the DLL uses, so
REM a bug in the reader shows up here instead of inside explorer.exe.

setlocal enabledelayedexpansion

set SCRIPT_DIR=%~dp0
set OBJ_DIR=%SCRIPT_DIR%obj-test

set VCVARS=
for %%P in (
  "C:\Program Files\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvars64.bat"
  "C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvars64.bat"
  "C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Auxiliary\Build\vcvars64.bat"
) do (
  if exist %%P (
    set VCVARS=%%~P
    goto :found
  )
)
echo ERROR: vcvars64.bat not found.
exit /b 1

:found
call "%VCVARS%" >nul
if errorlevel 1 exit /b 1

if not exist "%OBJ_DIR%" mkdir "%OBJ_DIR%"

echo Compiling test harness...
cl /nologo /EHsc /std:c++17 /O2 /utf-8 /DUNICODE /D_UNICODE ^
   /Fo"%OBJ_DIR%\\" /Fe"%OBJ_DIR%\shelltest.exe" ^
   "%SCRIPT_DIR%shelltest.cpp" ^
   "%SCRIPT_DIR%zip_reader.cpp" ^
   "%SCRIPT_DIR%decode.cpp" ^
   "%SCRIPT_DIR%inflate.cpp" ^
   /link shlwapi.lib ole32.lib oleaut32.lib uuid.lib windowscodecs.lib gdi32.lib user32.lib
if errorlevel 1 (
  echo ERROR: build failed.
  exit /b 1
)

echo.
echo Running tests...
"%OBJ_DIR%\shelltest.exe" %*
endlocal
