#Requires -Version 5.1
# Build deploy/installer/icon.ico from brand colors (used by Inno Setup).
$ErrorActionPreference = 'Stop'

$Root = Split-Path -Parent $PSScriptRoot
$OutDir = Join-Path $Root 'deploy\installer'
$OutPath = Join-Path $OutDir 'icon.ico'
New-Item -ItemType Directory -Path $OutDir -Force | Out-Null

Add-Type -AssemblyName System.Drawing

function New-BrandBitmap {
    param([int]$Size)
    $bmp = New-Object System.Drawing.Bitmap $Size, $Size
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $g.Clear([System.Drawing.Color]::FromArgb(255, 13, 13, 18))

    $rect = New-Object System.Drawing.Rectangle 1, 1, ($Size - 2), ($Size - 2)
    $radius = [int]($Size * 0.25)
    $path = New-Object System.Drawing.Drawing2D.GraphicsPath
    $path.AddArc($rect.X, $rect.Y, $radius, $radius, 180, 90)
    $path.AddArc($rect.Right - $radius, $rect.Y, $radius, $radius, 270, 90)
    $path.AddArc($rect.Right - $radius, $rect.Bottom - $radius, $radius, $radius, 0, 90)
    $path.AddArc($rect.X, $rect.Bottom - $radius, $radius, $radius, 90, 90)
    $path.CloseFigure()
    $g.FillPath([System.Drawing.Brushes]::Transparent, $path)

    $playBrush = New-Object System.Drawing.Drawing2D.LinearGradientBrush (
        (New-Object System.Drawing.Point ([int]($Size * 0.28), [int]($Size * 0.28))),
        (New-Object System.Drawing.Point ([int]($Size * 0.78), [int]($Size * 0.78))),
        [System.Drawing.Color]::FromArgb(255, 167, 139, 250),
        [System.Drawing.Color]::FromArgb(255, 34, 211, 238)
    )
    $play = @(
        (New-Object System.Drawing.PointF ([single]($Size * 0.30), [single]($Size * 0.28))),
        (New-Object System.Drawing.PointF ([single]($Size * 0.30), [single]($Size * 0.72))),
        (New-Object System.Drawing.PointF ([single]($Size * 0.72), [single]($Size * 0.50)))
    )
    $g.FillPolygon($playBrush, $play)

    $dot = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(255, 34, 211, 238))
    $dotSize = [int]($Size * 0.09)
    $g.FillEllipse($dot, [int]($Size * 0.68), [int]($Size * 0.24), $dotSize, $dotSize)

    $g.Dispose()
    return $bmp
}

$sizes = @(256, 48, 32, 16)
$bitmaps = foreach ($size in $sizes) { New-BrandBitmap -Size $size }
$icon = [System.Drawing.Icon]::FromHandle($bitmaps[0].GetHicon())
$stream = [System.IO.File]::Open($OutPath, [System.IO.FileMode]::Create)
try {
    $icon.Save($stream)
} finally {
    $stream.Close()
    foreach ($bmp in $bitmaps) { $bmp.Dispose() }
    $icon.Dispose()
}

Write-Host "Wrote $OutPath"
