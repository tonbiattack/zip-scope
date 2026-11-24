#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Installs the ZipScope MSIX for verification, runs WACK, then uninstalls.

.DESCRIPTION
    This script does the whole verification in one run so it can be started from
    any working directory with a single command:

        pwsh -File C:\apps\zip-scope\scripts\verify-msix-install.ps1

    It resolves every path from its own location, so the caller's current
    directory does not matter. That was the failure mode of the manual steps:
    they were run from C:\Users\teni2 and every relative path missed.

    Steps:
      1. Trust the test certificate in LocalMachine\TrustedPeople.
      2. Install the package with Add-AppxPackage.
      3. Launch it and confirm the process starts.
      4. Run the Windows App Certification Kit against the package.
      5. Uninstall and confirm removal.

    Each step prints PASS or FAIL and the script exits non-zero if any failed.

.PARAMETER SkipWack
    Skip the certification kit. It takes several minutes and needs the app to
    be closed while it runs.

.PARAMETER KeepInstalled
    Leave the package installed at the end instead of uninstalling it.
#>
[CmdletBinding()]
param(
    [switch]$SkipWack,
    [switch]$KeepInstalled
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# --- paths, resolved from this script so the caller's cwd is irrelevant -----

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Split-Path -Parent $scriptDir
$msixDir = Join-Path $repoRoot 'build\msix'
$msixPath = Join-Path $msixDir 'ZipScope_0.1.1.0_x64.msix'
$certPath = Join-Path $msixDir 'ZipScope-Test.cer'
$wackReport = Join-Path $msixDir 'wack-report.xml'

$packageName = 'tonbiattack.ZipScope'
$packageFullName = 'tonbiattack.ZipScope_0.1.1.0_x64__e28ksxe3fc9w4'
$aumid = 'tonbiattack.ZipScope_e28ksxe3fc9w4!ZipScope'

$results = [System.Collections.Generic.List[object]]::new()

function Add-Result {
    param([string]$Step, [bool]$Passed, [string]$Detail = '')
    $results.Add([pscustomobject]@{ Step = $Step; Passed = $Passed; Detail = $Detail })
    $mark = if ($Passed) { 'PASS' } else { 'FAIL' }
    $colour = if ($Passed) { 'Green' } else { 'Red' }
    Write-Host ("[{0}] {1}" -f $mark, $Step) -ForegroundColor $colour
    if ($Detail) { Write-Host "       $Detail" -ForegroundColor DarkGray }
}

Write-Host ""
Write-Host "ZipScope MSIX verification" -ForegroundColor Cyan
Write-Host "  package: $msixPath"
Write-Host ""

# --- preconditions ---------------------------------------------------------

if (-not (Test-Path $msixPath)) {
    Write-Host "Package not found. Build it first:" -ForegroundColor Red
    Write-Host "  pwsh -File `"$repoRoot\scripts\make-msix.ps1`" -Version 0.1.1.0 -Sign"
    exit 1
}
if (-not (Test-Path $certPath)) {
    Write-Host "Test certificate not found. Build it with -Sign first." -ForegroundColor Red
    exit 1
}

$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "This script must run as Administrator." -ForegroundColor Red
    exit 1
}

# --- 1. trust the certificate ---------------------------------------------

try {
    $existing = Get-ChildItem Cert:\LocalMachine\TrustedPeople |
        Where-Object { $_.Subject -eq 'CN=tonbiattack' }
    if ($existing) {
        Add-Result 'Trust the test certificate' $true 'already trusted'
    }
    else {
        Import-Certificate -FilePath $certPath -CertStoreLocation Cert:\LocalMachine\TrustedPeople | Out-Null
        Add-Result 'Trust the test certificate' $true 'imported into LocalMachine\TrustedPeople'
    }
}
catch {
    Add-Result 'Trust the test certificate' $false $_.Exception.Message
    Write-Host "Cannot continue without a trusted certificate." -ForegroundColor Red
    exit 1
}

# --- 2. install ------------------------------------------------------------

$installed = $false
try {
    # Remove a previous copy so the install is a clean first-time install.
    $prior = Get-AppxPackage -Name $packageName -ErrorAction SilentlyContinue
    if ($prior) {
        Remove-AppxPackage -Package $prior.PackageFullName -ErrorAction SilentlyContinue
        Start-Sleep -Seconds 2
    }

    Add-AppxPackage -Path $msixPath -ErrorAction Stop
    $installed = $true
    $pkg = Get-AppxPackage -Name $packageName
    Add-Result 'Install the package' $true "installed $($pkg.PackageFullName)"
}
catch {
    Add-Result 'Install the package' $false $_.Exception.Message
    Write-Host ""
    Write-Host "Install failed. Recent deployment errors:" -ForegroundColor Yellow
    Get-WinEvent -LogName 'Microsoft-Windows-AppXDeploymentServer/Operational' -MaxEvents 5 -ErrorAction SilentlyContinue |
        Select-Object TimeCreated, Id, @{n = 'Message'; e = { $_.Message.Split("`n")[0] } } |
        Format-Table -AutoSize | Out-String | Write-Host
}

# --- 3. launch -------------------------------------------------------------

if ($installed) {
    try {
        Get-Process -Name 'ZipScope' -ErrorAction SilentlyContinue | Stop-Process -Force
        Start-Sleep -Seconds 1

        Start-Process "shell:AppsFolder\$aumid"
        $started = $false
        for ($i = 0; $i -lt 60; $i++) {
            if (Get-Process -Name 'ZipScope' -ErrorAction SilentlyContinue) { $started = $true; break }
            Start-Sleep -Milliseconds 500
        }
        if ($started) {
            $proc = Get-Process -Name 'ZipScope' | Select-Object -First 1
            Add-Result 'Launch the packaged app' $true "pid $($proc.Id), $([math]::Round($proc.WorkingSet64 / 1MB, 1)) MB"
            Start-Sleep -Seconds 5
            Get-Process -Name 'ZipScope' -ErrorAction SilentlyContinue | Stop-Process -Force
        }
        else {
            Add-Result 'Launch the packaged app' $false 'process did not start within 30s'
        }
    }
    catch {
        Add-Result 'Launch the packaged app' $false $_.Exception.Message
    }
}
else {
    Add-Result 'Launch the packaged app' $false 'skipped: install failed'
}

# --- 4. WACK ---------------------------------------------------------------

if (-not $SkipWack -and $installed) {
    $appcert = 'C:\Program Files (x86)\Windows Kits\10\App Certification Kit\appcert.exe'
    if (-not (Test-Path $appcert)) {
        Add-Result 'Windows App Certification Kit' $false 'appcert.exe not found; install the Windows SDK'
    }
    else {
        try {
            Write-Host "       running WACK (this takes several minutes)..." -ForegroundColor DarkGray
            Get-Process -Name 'ZipScope' -ErrorAction SilentlyContinue | Stop-Process -Force
            Start-Sleep -Seconds 2

            & $appcert test /appx $msixPath /reportoutput $wackReport 2>&1 | Out-Null
            $wackExit = $LASTEXITCODE

            if (Test-Path $wackReport) {
                [xml]$report = Get-Content $wackReport -Raw
                $requirements = $report.REPORT.REQUIREMENTS.REQUIREMENT
                $failed = @($requirements | Where-Object { $_.STATUS -ne 'PASS' })
                $total = @($requirements).Count
                if ($failed.Count -eq 0) {
                    Add-Result 'Windows App Certification Kit' $true "$total requirements, all PASS"
                }
                else {
                    $names = ($failed | ForEach-Object { $_.DESCRIPTION }) -join '; '
                    Add-Result 'Windows App Certification Kit' $false "$($failed.Count) of $total failed: $names"
                }
            }
            else {
                Add-Result 'Windows App Certification Kit' $false "no report produced (exit $wackExit)"
            }
        }
        catch {
            Add-Result 'Windows App Certification Kit' $false $_.Exception.Message
        }
    }
}
elseif ($SkipWack) {
    Add-Result 'Windows App Certification Kit' $true 'skipped by request'
}
else {
    Add-Result 'Windows App Certification Kit' $false 'skipped: install failed'
}

# --- 5. uninstall ----------------------------------------------------------

if ($installed -and -not $KeepInstalled) {
    try {
        Get-Process -Name 'ZipScope' -ErrorAction SilentlyContinue | Stop-Process -Force
        Start-Sleep -Seconds 2
        Remove-AppxPackage -Package $packageFullName -ErrorAction Stop
        Start-Sleep -Seconds 2
        $still = Get-AppxPackage -Name $packageName -ErrorAction SilentlyContinue
        if ($still) {
            Add-Result 'Uninstall the package' $false 'package is still registered'
        }
        else {
            Add-Result 'Uninstall the package' $true 'removed'
        }
    }
    catch {
        Add-Result 'Uninstall the package' $false $_.Exception.Message
    }
}
elseif ($KeepInstalled) {
    Add-Result 'Uninstall the package' $true 'skipped by request (left installed)'
}
else {
    Add-Result 'Uninstall the package' $false 'skipped: install failed'
}

# --- summary ---------------------------------------------------------------

Write-Host ""
Write-Host "Summary" -ForegroundColor Cyan
$results | Format-Table -AutoSize | Out-String | Write-Host

$failedCount = @($results | Where-Object { -not $_.Passed }).Count
if ($failedCount -eq 0) {
    Write-Host "All checks passed." -ForegroundColor Green
    exit 0
}
Write-Host "$failedCount check(s) failed." -ForegroundColor Red
exit 1
