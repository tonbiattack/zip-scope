#!/usr/bin/env pwsh
# Verifies that a built MSIX has the identity Partner Center expects.
#
# The Store rejects an upload whose Identity does not match the reservation, and
# the mismatch is easy to introduce: the Publisher is a GUID issued by Partner
# Center, not the certificate subject. This script unpacks the package, reads
# the manifest, and recomputes the Package Family Name the same way Windows
# does, so a wrong package is caught before it reaches the upload page.

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$MsixPath,

    [string]$ExpectedName = 'tonbiattack.ZipScope',
    [string]$ExpectedPublisher = 'CN=2F14A66E-02DF-43E2-93E0-5C5900B38D2E',
    [string]$ExpectedFamilyName = 'tonbiattack.ZipScope_e28ksxe3fc9w4'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$MsixPath = [System.IO.Path]::GetFullPath($MsixPath)
if (-not (Test-Path $MsixPath)) {
    throw "Package not found: $MsixPath"
}

# Locate makeappx.
$makeappx = $null
$kits = 'C:\Program Files (x86)\Windows Kits\10\bin'
if (Test-Path $kits) {
    $makeappx = Get-ChildItem $kits -Recurse -Filter makeappx.exe -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -match '\\x64\\' } |
        Sort-Object FullName -Descending |
        Select-Object -First 1
}
if (-not $makeappx) {
    $cmd = Get-Command makeappx.exe -ErrorAction SilentlyContinue
    if ($cmd) { $makeappx = Get-Item $cmd.Source }
}
if (-not $makeappx) {
    throw "makeappx.exe not found. Install the Windows SDK."
}

$temp = Join-Path ([System.IO.Path]::GetTempPath()) ("zipscope-pfn-" + [guid]::NewGuid().ToString('N'))
try {
    & $makeappx.FullName unpack /p $MsixPath /d $temp /o | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "makeappx unpack failed" }

    $manifestPath = Join-Path $temp 'AppxManifest.xml'
    if (-not (Test-Path $manifestPath)) { throw "AppxManifest.xml missing from the package" }

    [xml]$manifest = Get-Content $manifestPath -Raw
    $identity = $manifest.Package.Identity

    $name = $identity.Name
    $publisher = $identity.Publisher
    $version = $identity.Version
    $arch = $identity.ProcessorArchitecture

    Write-Host "Identity Name:      $name"
    Write-Host "Identity Publisher: $publisher"
    Write-Host "Version:            $version"
    Write-Host "Architecture:       $arch"

    # Package Family Name = <Name>_<13-char publisher hash>. The hash is the
    # first 13 characters of the base32 encoding of SHA-256 over the publisher
    # string encoded as UTF-16LE, using Windows' base32 alphabet.
    $bytes = [System.Text.Encoding]::Unicode.GetBytes($publisher)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try { $hash = $sha.ComputeHash($bytes) } finally { $sha.Dispose() }

    $alphabet = '0123456789abcdefghjkmnpqrstvwxyz'
    $bits = 0
    $value = 0
    $encoded = ''
    foreach ($b in $hash) {
        $value = ($value -shl 8) -bor $b
        $bits += 8
        while ($bits -ge 5) {
            $bits -= 5
            $encoded += $alphabet[($value -shr $bits) -band 0x1F]
            if ($encoded.Length -ge 13) { break }
        }
        if ($encoded.Length -ge 13) { break }
    }
    $familyName = "${name}_$encoded"
    Write-Host "Package Family Name: $familyName"

    $failures = @()
    if ($name -ne $ExpectedName) {
        $failures += "Name is '$name', expected '$ExpectedName'"
    }
    if ($publisher -ne $ExpectedPublisher) {
        $failures += "Publisher is '$publisher', expected '$ExpectedPublisher'"
    }
    if ($familyName -ne $ExpectedFamilyName) {
        $failures += "Package Family Name is '$familyName', expected '$ExpectedFamilyName'"
    }
    if ($version -notmatch '^\d+\.\d+\.\d+\.\d+$') {
        $failures += "Version '$version' is not four parts"
    }
    elseif ($version.Split('.')[3] -ne '0') {
        $failures += "Version '$version' must end in .0 for Store submission"
    }
    if ($arch -ne 'x64') {
        $failures += "Architecture is '$arch', expected 'x64'"
    }

    if ($failures.Count -gt 0) {
        Write-Host ""
        Write-Host "FAILED:" -ForegroundColor Red
        foreach ($f in $failures) { Write-Host "  - $f" -ForegroundColor Red }
        exit 1
    }

    Write-Host ""
    Write-Host "Identity matches Partner Center." -ForegroundColor Green
}
finally {
    if (Test-Path $temp) { Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue }
}
