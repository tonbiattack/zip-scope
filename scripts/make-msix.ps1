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
    [switch]$SkipBuild,
    # Identity values from Partner Center. The defaults are the values assigned
    # to ZipScope on 2026-11-23; override them for a different reservation.
    [string]$IdentityName = 'tonbiattack.ZipScope',
    [string]$Publisher = 'CN=2F14A66E-02DF-43E2-93E0-5C5900B38D2E',
    [string]$PublisherDisplayName = 'tonbiattack'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$root = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$stage = Join-Path $root 'build\msix\stage'
$outDir = Join-Path $root 'build\msix'
$msixPath = Join-Path $outDir "ZipScope_${Version}_x64.msix"

# The Store requires a four-part version, and the last part must be 0 so the
# Store can append its own revision. Reject anything else before packing.
if ($Version -notmatch '^\d+\.\d+\.\d+\.\d+$') {
    throw "Version must have four parts, e.g. 0.1.0.0 (got '$Version')"
}
$parts = $Version.Split('.') | ForEach-Object { [int]$_ }
if ($parts[3] -ne 0) {
    throw "The fourth version part must be 0 for Store submission (got '$Version')"
}

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

# The manifest carries the identity and version, so it is rewritten rather than
# copied. The Store rejects a package whose Identity does not match the
# reservation exactly, so these values are substituted rather than hand-edited.
#
# The substitutions are anchored to the Identity element: a bare Version="..."
# pattern would also rewrite TargetDeviceFamily's MinVersion and produce an
# uninstallable package.
$manifest = Get-Content (Join-Path $root 'msix\AppxManifest.xml') -Raw
$manifest = $manifest -replace '(<Identity\s[^>]*?Version=")\d+\.\d+\.\d+\.\d+(")', "`${1}$Version`${2}"
$manifest = $manifest -replace '(<Identity\s[^>]*?Name=")[^"]*(")', "`${1}$IdentityName`${2}"
$manifest = $manifest -replace '(<Identity\s[^>]*?Publisher=")[^"]*(")', "`${1}$Publisher`${2}"
$manifest = $manifest -replace '<PublisherDisplayName>[^<]*</PublisherDisplayName>',
    "<PublisherDisplayName>$PublisherDisplayName</PublisherDisplayName>"
Set-Content -Path (Join-Path $stage 'AppxManifest.xml') -Value $manifest -Encoding UTF8

# Fail early if the substitution did not take, rather than at upload time.
$written = Get-Content (Join-Path $stage 'AppxManifest.xml') -Raw
foreach ($expected in @($IdentityName, $Publisher, $Version)) {
    if ($written -notmatch [regex]::Escape($expected)) {
        throw "manifest is missing the expected value: $expected"
    }
}
# The device family version must not have been touched.
if ($written -notmatch 'TargetDeviceFamily Name="Windows\.Desktop" MinVersion="10\.0\.') {
    throw "TargetDeviceFamily MinVersion was corrupted by the version substitution"
}
Write-Host "Identity: $IdentityName"
Write-Host "Publisher: $Publisher"
Write-Host "Version: $Version"

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
Write-Host "To install locally (sideload), run this from any directory:"
Write-Host "  pwsh -File `"$root\scripts\verify-msix-install.ps1`""
Write-Host ""
Write-Host "Or do it by hand:"
Write-Host "  1. Import the test certificate into Trusted People (as Administrator):"
Write-Host "     Import-Certificate -FilePath `"$outDir\ZipScope-Test.cer`" -CertStoreLocation Cert:\LocalMachine\TrustedPeople"
Write-Host "  2. Add-AppxPackage -Path `"$msixPath`""
