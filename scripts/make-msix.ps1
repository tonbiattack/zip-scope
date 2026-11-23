#!/usr/bin/env pwsh
# Builds the MSIX package for the Microsoft Store.
#
# The package is a full-trust desktop package: the Wails viewer and the
# Explorer COM DLL are copied in unchanged, and the manifest declares the
# Explorer integrations so the Store can register them without the app writing
# to HKEY_CLASSES_ROOT.
#
# Usage:
#   pwsh scripts/make-msix.ps1                 # build, unsigned
#   pwsh scripts/make-msix.ps1 -Sign           # build and sign with a test cert
#   pwsh scripts/make-msix.ps1 -Version 0.2.0.0

[CmdletBinding()]
param(
    [string]$Version = '0.1.0.0',
    [switch]$Sign,
    [string]$CertPath,
    [string]$CertPassword,
    [switch]$SkipBuild
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$stage = Join-Path $root 'build\msix\stage'
$outDir = Join-Path $root 'build\msix'
$msixPath = Join-Path $outDir "ZipScope_${Version}_x64.msix"

# Locate the Windows SDK tools. They are not on PATH by default.
function Find-SdkTool {
    param([string]$Name)

    $candidates = @()
    $kits = 'C:\Program Files (x86)\Windows Kits\10\bin'
    if (Test-Path $kits) {
        $candidates += Get-ChildItem $kits -Directory |
            Sort-Object Name -Descending |
            ForEach-Object { Join-Path $_.FullName "x64\$Name" }
    }
    $candidates += @(Get-Command $Name -ErrorAction SilentlyContinue | ForEach-Object { $_.Source })

    foreach ($c in $candidates) {
        if ($c -and (Test-Path $c)) { return $c }
    }
    throw "$Name not found. Install the Windows SDK."
}

$makeappx = Find-SdkTool 'makeappx.exe'
$signtool = Find-SdkTool 'signtool.exe'
Write-Host "makeappx: $makeappx"

# --- build the binaries ----------------------------------------------------

if (-not $SkipBuild) {
    Write-Host "Building the viewer..."
    Push-Location $root
    try {
        & wails build -platform windows/amd64
        if ($LASTEXITCODE -ne 0) { throw "wails build failed" }
    }
    finally {
        Pop-Location
    }

    Write-Host "Building the Explorer extension..."
    Push-Location (Join-Path $root 'shell')
    try {
        & cmd /c build.cmd
        if ($LASTEXITCODE -ne 0) { throw "shell build failed" }
    }
    finally {
        Pop-Location
    }
}

# --- stage the package contents -------------------------------------------

Write-Host "Staging package contents..."
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory -Path $stage -Force | Out-Null

Copy-Item (Join-Path $root 'build\bin\ZipScope.exe') $stage
Copy-Item (Join-Path $root 'build\shell\ZipScopeShell.dll') $stage
Copy-Item (Join-Path $root 'LICENSE') $stage
Copy-Item (Join-Path $root 'README.md') $stage

# Assets are generated from the app icon so the tiles match the executable.
$assetsSrc = Join-Path $root 'msix\Assets'
if (-not (Test-Path $assetsSrc)) {
    Write-Host "Generating tile assets..."
    & pwsh -NoProfile -File (Join-Path $root 'scripts\make-msix-assets.ps1')
    if ($LASTEXITCODE -ne 0) { throw "asset generation failed" }
}
Copy-Item $assetsSrc (Join-Path $stage 'Assets') -Recurse

# The manifest carries the version, so it is rewritten rather than copied.
$manifest = Get-Content (Join-Path $root 'msix\AppxManifest.xml') -Raw
$manifest = $manifest -replace 'Version="\d+\.\d+\.\d+\.\d+"', "Version=`"$Version`""
Set-Content -Path (Join-Path $stage 'AppxManifest.xml') -Value $manifest -Encoding UTF8

# --- pack ------------------------------------------------------------------

Write-Host "Packing $msixPath..."
if (-not (Test-Path $outDir)) { New-Item -ItemType Directory -Path $outDir -Force | Out-Null }
if (Test-Path $msixPath) { Remove-Item $msixPath -Force }

& $makeappx pack /d $stage /p $msixPath /o
if ($LASTEXITCODE -ne 0) { throw "makeappx pack failed" }

$size = [math]::Round((Get-Item $msixPath).Length / 1MB, 2)
Write-Host "Packed $msixPath ($size MB)"

# --- sign ------------------------------------------------------------------

if ($Sign) {
    if (-not $CertPath) {
        # A self-signed certificate is enough to install locally for testing.
        # The Store re-signs the package, so this is not the shipping signature.
        $CertPath = Join-Path $outDir 'ZipScope-Test.pfx'
        if (-not (Test-Path $CertPath)) {
            Write-Host "Creating a self-signed test certificate..."
            $cert = New-SelfSignedCertificate `
                -Type Custom `
                -Subject 'CN=tonbiattack' `
                -KeyUsage DigitalSignature `
                -FriendlyName 'ZipScope MSIX Test' `
                -CertStoreLocation 'Cert:\CurrentUser\My' `
                -TextExtension @('2.5.29.37={text}1.3.6.1.5.5.7.3.3', '2.5.29.19={text}')
            $pwd = ConvertTo-SecureString -String 'zipscope' -Force -AsPlainText
            Export-PfxCertificate -Cert $cert -FilePath $CertPath -Password $pwd | Out-Null
            Export-Certificate -Cert $cert -FilePath (Join-Path $outDir 'ZipScope-Test.cer') | Out-Null
            Write-Host "Wrote $CertPath (password: zipscope)"
            Write-Host "Wrote $(Join-Path $outDir 'ZipScope-Test.cer') - install it to Trusted People to sideload"
        }
        if (-not $CertPassword) { $CertPassword = 'zipscope' }
    }

    Write-Host "Signing..."
    & $signtool sign /fd SHA256 /a /f $CertPath /p $CertPassword $msixPath
    if ($LASTEXITCODE -ne 0) { throw "signtool sign failed" }

    & $signtool verify /pa $msixPath
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Signature verification failed. The test certificate is probably not trusted yet."
    }
}

Write-Host ""
Write-Host "Package: $msixPath"
Write-Host ""
Write-Host "To install locally (sideload):"
Write-Host "  1. Import the test certificate into Trusted People:"
Write-Host "     Import-Certificate -FilePath build\msix\ZipScope-Test.cer -CertStoreLocation Cert:\LocalMachine\TrustedPeople"
Write-Host "  2. Add-AppxPackage -Path $msixPath"
