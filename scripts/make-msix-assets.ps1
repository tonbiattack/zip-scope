#!/usr/bin/env pwsh
# Generates the MSIX tile assets from the application icon.
#
# The Store requires a specific set of PNG sizes. Each is drawn from the same
# vector artwork the app icon uses, so the tiles match the executable's icon
# instead of being a separate design.

[CmdletBinding()]
param(
    [string]$SourceIcon = (Join-Path $PSScriptRoot '..\build\appicon.png'),
    [string]$OutDir = (Join-Path $PSScriptRoot '..\msix\Assets')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Add-Type -AssemblyName System.Drawing

$SourceIcon = [System.IO.Path]::GetFullPath($SourceIcon)
$OutDir = [System.IO.Path]::GetFullPath($OutDir)

if (-not (Test-Path $SourceIcon)) {
    throw "Source icon not found: $SourceIcon. Run scripts/make-icon.ps1 first."
}
if (-not (Test-Path $OutDir)) {
    New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
}

$source = [System.Drawing.Image]::FromFile($SourceIcon)
try {
    # Square tiles: the icon is drawn centred on a transparent canvas at the
    # size the manifest asks for.
    $square = @(
        @{ Name = 'Square44x44Logo.png'; Size = 44 },
        @{ Name = 'Square150x150Logo.png'; Size = 150 },
        @{ Name = 'StoreLogo.png'; Size = 50 },
        @{ Name = 'Square44x44Logo.targetsize-256.png'; Size = 256 },
        @{ Name = 'Square44x44Logo.altform-unplated_targetsize-256.png'; Size = 256 }
    )

    foreach ($item in $square) {
        $size = $item.Size
        $bmp = New-Object System.Drawing.Bitmap $size, $size
        $g = [System.Drawing.Graphics]::FromImage($bmp)
        $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
        $g.Clear([System.Drawing.Color]::Transparent)
        $g.DrawImage($source, 0, 0, $size, $size)
        $g.Dispose()
        $path = Join-Path $OutDir $item.Name
        $bmp.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
        $bmp.Dispose()
        Write-Host "wrote $($item.Name) ($size x $size)"
    }

    # Wide tile: the icon sits on the left, with the app name beside it.
    $wideW = 310
    $wideH = 150
    $wide = New-Object System.Drawing.Bitmap $wideW, $wideH
    $wg = [System.Drawing.Graphics]::FromImage($wide)
    $wg.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $wg.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $wg.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit
    $wg.Clear([System.Drawing.Color]::Transparent)

    $iconSize = 96
    $iconX = 24
    $iconY = ($wideH - $iconSize) / 2
    $wg.DrawImage($source, $iconX, $iconY, $iconSize, $iconSize)

    $font = New-Object System.Drawing.Font 'Segoe UI', 22, ([System.Drawing.FontStyle]::Bold)
    $brush = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::White)
    $textX = $iconX + $iconSize + 18
    $wg.DrawString('ZipScope', $font, $brush, $textX, ($wideH / 2 - 22))
    $font.Dispose()
    $brush.Dispose()
    $wg.Dispose()
    $widePath = Join-Path $OutDir 'Wide310x150Logo.png'
    $wide.Save($widePath, [System.Drawing.Imaging.ImageFormat]::Png)
    $wide.Dispose()
    Write-Host "wrote Wide310x150Logo.png ($wideW x $wideH)"

    # Splash screen: the icon centred on the app's dark background.
    $splashW = 620
    $splashH = 300
    $splash = New-Object System.Drawing.Bitmap $splashW, $splashH
    $sg = [System.Drawing.Graphics]::FromImage($splash)
    $sg.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $sg.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $sg.Clear([System.Drawing.Color]::FromArgb(255, 18, 18, 20))
    $splashIcon = 180
    $sg.DrawImage($source, ($splashW - $splashIcon) / 2, ($splashH - $splashIcon) / 2, $splashIcon, $splashIcon)
    $sg.Dispose()
    $splashPath = Join-Path $OutDir 'SplashScreen.png'
    $splash.Save($splashPath, [System.Drawing.Imaging.ImageFormat]::Png)
    $splash.Dispose()
    Write-Host "wrote SplashScreen.png ($splashW x $splashH)"
}
finally {
    $source.Dispose()
}

Write-Host ""
Write-Host "Assets written to $OutDir"
