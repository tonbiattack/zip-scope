#!/usr/bin/env pwsh
# Generates the ZipScope application icon.
#
# The artwork is drawn as vectors at every size the ICO needs, so 16px and 24px
# stay crisp instead of being downscaled from a large bitmap. Run this after
# changing the design, then rebuild the app:
#
#   pwsh scripts/make-icon.ps1
#   wails build -platform windows/amd64
#
# The design is a stack of photo cards on a rounded gradient tile: the front
# card carries a sun and two mountains, the back card peeks out to suggest that
# an archive holds many images.

[CmdletBinding()]
param(
    [string]$OutDir = (Join-Path $PSScriptRoot '..\build')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Add-Type -AssemblyName System.Drawing

# --- palette ---------------------------------------------------------------

$BgTop     = [System.Drawing.Color]::FromArgb(255, 107, 155, 255)
$BgBottom  = [System.Drawing.Color]::FromArgb(255, 47, 85, 200)
$CardBack  = [System.Drawing.Color]::FromArgb(110, 255, 255, 255)
$CardFront = [System.Drawing.Color]::FromArgb(255, 255, 255, 255)
$SunColor  = [System.Drawing.Color]::FromArgb(255, 255, 194, 77)
$PeakColor = [System.Drawing.Color]::FromArgb(255, 74, 124, 255)
$HillColor = [System.Drawing.Color]::FromArgb(255, 134, 173, 255)

# --- drawing helpers -------------------------------------------------------

function New-RoundedPath {
    param([double]$X, [double]$Y, [double]$W, [double]$H, [double]$R, [double]$Scale = 1.0)

    $path = [System.Drawing.Drawing2D.GraphicsPath]::new()
    $d = $R * 2
    if ($d -le 0.01) {
        $path.AddRectangle([System.Drawing.RectangleF]::new([float]$X, [float]$Y, [float]$W, [float]$H))
    }
    else {
        $path.AddArc([float]$X, [float]$Y, [float]$d, [float]$d, 180, 90)
        $path.AddArc([float]($X + $W - $d), [float]$Y, [float]$d, [float]$d, 270, 90)
        $path.AddArc([float]($X + $W - $d), [float]($Y + $H - $d), [float]$d, [float]$d, 0, 90)
        $path.AddArc([float]$X, [float]($Y + $H - $d), [float]$d, [float]$d, 90, 90)
        $path.CloseFigure()
    }

    # Scale the design-space geometry into device pixels.
    if ($Scale -ne 1.0) {
        $m = [System.Drawing.Drawing2D.Matrix]::new()
        $m.Scale([float]$Scale, [float]$Scale)
        $path.Transform($m)
        $m.Dispose()
    }
    return $path
}

# New-IconBitmap renders the icon at one size. All geometry is expressed in a
# 1024-unit design space and scaled, so every size is drawn rather than resized.
function New-IconBitmap {
    param([int]$Size)

    $bmp = [System.Drawing.Bitmap]::new($Size, $Size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $g.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
    $g.Clear([System.Drawing.Color]::Transparent)

    $s = $Size / 1024.0

    # Background tile.
    $bgPath = New-RoundedPath 0 0 1024 1024 200 $s
    $bgRect = [System.Drawing.RectangleF]::new(0, 0, [float]$Size, [float]$Size)
    $bgBrush = [System.Drawing.Drawing2D.LinearGradientBrush]::new(
        $bgRect, $BgTop, $BgBottom, [System.Drawing.Drawing2D.LinearGradientMode]::ForwardDiagonal)
    $g.FillPath($bgBrush, $bgPath)
    $bgBrush.Dispose()
    $bgPath.Dispose()

    # Back card, peeking above and to the right.
    $backPath = New-RoundedPath 340 225 440 360 34 $s
    $backBrush = [System.Drawing.SolidBrush]::new($CardBack)
    $g.FillPath($backBrush, $backPath)
    $backBrush.Dispose()
    $backPath.Dispose()

    # Front card.
    $frontPath = New-RoundedPath 235 315 545 430 42 $s
    $frontBrush = [System.Drawing.SolidBrush]::new($CardFront)
    $g.FillPath($frontBrush, $frontPath)
    $frontBrush.Dispose()

    # Everything below is clipped to the front card so the artwork cannot spill.
    $state = $g.Save()
    $g.SetClip($frontPath)

    # Sun.
    $sunBrush = [System.Drawing.SolidBrush]::new($SunColor)
    $g.FillEllipse($sunBrush,
        [float](311 * $s), [float](401 * $s), [float](88 * $s), [float](88 * $s))
    $sunBrush.Dispose()

    # Mountains. The arrays are built with New-Object so PowerShell passes them
    # as a single PointF[] argument rather than unrolling them.
    $peak = New-Object 'System.Drawing.PointF[]' 3
    $peak[0] = [System.Drawing.PointF]::new([float](250 * $s), [float](745 * $s))
    $peak[1] = [System.Drawing.PointF]::new([float](455 * $s), [float](520 * $s))
    $peak[2] = [System.Drawing.PointF]::new([float](655 * $s), [float](745 * $s))
    $peakBrush = [System.Drawing.SolidBrush]::new($PeakColor)
    $g.FillPolygon($peakBrush, $peak)
    $peakBrush.Dispose()

    $hill = New-Object 'System.Drawing.PointF[]' 3
    $hill[0] = [System.Drawing.PointF]::new([float](520 * $s), [float](745 * $s))
    $hill[1] = [System.Drawing.PointF]::new([float](655 * $s), [float](590 * $s))
    $hill[2] = [System.Drawing.PointF]::new([float](770 * $s), [float](745 * $s))
    $hillBrush = [System.Drawing.SolidBrush]::new($HillColor)
    $g.FillPolygon($hillBrush, $hill)
    $hillBrush.Dispose()

    $g.Restore($state)
    $frontPath.Dispose()
    $g.Dispose()
    return $bmp
}

# Get-IconDibBytes encodes a bitmap as the DIB an ICO entry expects: a
# BITMAPINFOHEADER, bottom-up BGRA pixels, then a zeroed AND mask. The mask is
# all zeros because the alpha channel carries the transparency.
function Get-IconDibBytes {
    param([System.Drawing.Bitmap]$Bitmap)

    $w = $Bitmap.Width
    $h = $Bitmap.Height
    $rect = [System.Drawing.Rectangle]::new(0, 0, $w, $h)
    $locked = $Bitmap.LockBits($rect,
        [System.Drawing.Imaging.ImageLockMode]::ReadOnly,
        [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    try {
        $stride = $locked.Stride
        $raw = [byte[]]::new($stride * $h)
        [System.Runtime.InteropServices.Marshal]::Copy($locked.Scan0, $raw, 0, $raw.Length)
    }
    finally {
        $Bitmap.UnlockBits($locked)
    }

    $ms = [System.IO.MemoryStream]::new()
    $bw = [System.IO.BinaryWriter]::new($ms)

    # BITMAPINFOHEADER
    $bw.Write([uint32]40)          # biSize
    $bw.Write([int32]$w)           # biWidth
    $bw.Write([int32]($h * 2))     # biHeight (XOR + AND)
    $bw.Write([uint16]1)           # biPlanes
    $bw.Write([uint16]32)          # biBitCount
    $bw.Write([uint32]0)           # biCompression = BI_RGB
    $bw.Write([uint32]($w * $h * 4))
    $bw.Write([int32]0)            # biXPelsPerMeter
    $bw.Write([int32]0)            # biYPelsPerMeter
    $bw.Write([uint32]0)           # biClrUsed
    $bw.Write([uint32]0)           # biClrImportant

    # XOR bitmap, bottom-up.
    for ($y = $h - 1; $y -ge 0; $y--) {
        $bw.Write($raw, $y * $stride, $w * 4)
    }

    # AND mask, rows padded to a 4-byte boundary.
    $maskStride = [int]([math]::Ceiling($w / 32.0) * 4)
    $mask = [byte[]]::new($maskStride * $h)
    $bw.Write($mask, 0, $mask.Length)

    $bw.Flush()
    return $ms.ToArray()
}

# --- generate --------------------------------------------------------------

$OutDir = [System.IO.Path]::GetFullPath($OutDir)
$windowsDir = Join-Path $OutDir 'windows'
foreach ($dir in @($OutDir, $windowsDir)) {
    if (-not (Test-Path $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
}

# The 1024px PNG is the master artwork Wails and the README use.
$master = New-IconBitmap 1024
$pngPath = Join-Path $OutDir 'appicon.png'
$master.Save($pngPath, [System.Drawing.Imaging.ImageFormat]::Png)
$master.Dispose()
Write-Host "wrote $pngPath"

# The ICO carries every size Windows asks for.
$sizes = @(16, 20, 24, 32, 40, 48, 64, 128, 256)
$images = @()
foreach ($size in $sizes) {
    $bmp = New-IconBitmap $size
    $images += [pscustomobject]@{
        Size = $size
        Data = (Get-IconDibBytes $bmp)
    }
    $bmp.Dispose()
}

$icoPath = Join-Path $windowsDir 'icon.ico'
$fs = [System.IO.File]::Create($icoPath)
$bw = [System.IO.BinaryWriter]::new($fs)
try {
    $bw.Write([uint16]0)              # reserved
    $bw.Write([uint16]1)              # type = icon
    $bw.Write([uint16]$images.Count)

    $offset = 6 + (16 * $images.Count)
    foreach ($img in $images) {
        # 256 is encoded as 0 in the directory entry.
        $dim = if ($img.Size -ge 256) { 0 } else { $img.Size }
        $bw.Write([byte]$dim)         # width
        $bw.Write([byte]$dim)         # height
        $bw.Write([byte]0)            # colour count
        $bw.Write([byte]0)            # reserved
        $bw.Write([uint16]1)          # planes
        $bw.Write([uint16]32)         # bits per pixel
        $bw.Write([uint32]$img.Data.Length)
        $bw.Write([uint32]$offset)
        $offset += $img.Data.Length
    }
    foreach ($img in $images) {
        $bw.Write($img.Data, 0, $img.Data.Length)
    }
}
finally {
    $bw.Dispose()
    $fs.Dispose()
}
Write-Host "wrote $icoPath ($($images.Count) sizes: $($sizes -join ', '))"

# Verify the ICO loads and reports the expected sizes.
$icon = [System.Drawing.Icon]::new($icoPath)
Write-Host "verified: default icon is $($icon.Width)x$($icon.Height)"
$icon.Dispose()
