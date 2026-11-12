param(
    [Parameter(Mandatory = $true)][string]$Source,
    [string]$Root = ""
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing
Add-Type -Namespace NativeIcon -Name Methods -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll", SetLastError=true)]
public static extern bool DestroyIcon(System.IntPtr hIcon);
'@

if (-not $Root) {
    $Root = Split-Path -Parent $PSScriptRoot
}
if (-not (Test-Path $Source)) {
    throw "Source image not found: $Source"
}

function Save-Png {
    param(
        [System.Drawing.Image]$Image,
        [int]$Size,
        [string]$Path
    )
    $bitmap = New-Object System.Drawing.Bitmap($Size, $Size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    try {
        $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
        $graphics.DrawImage($Image, 0, 0, $Size, $Size)
    }
    finally {
        $graphics.Dispose()
    }
    $bitmap.Save($Path, [System.Drawing.Imaging.ImageFormat]::Png)
    $bitmap.Dispose()
}

$sourceImage = [System.Drawing.Image]::FromFile((Resolve-Path $Source))
try {
    $docsAssets = Join-Path $Root "docs/assets"
    $frontendAssets = Join-Path $Root "frontend/src/assets"
    $windowsAssets = Join-Path $Root "build/windows"
    New-Item -ItemType Directory -Path $docsAssets -Force | Out-Null
    New-Item -ItemType Directory -Path $frontendAssets -Force | Out-Null
    New-Item -ItemType Directory -Path $windowsAssets -Force | Out-Null

    # Full square brand image for README and website.
    Save-Png -Image $sourceImage -Size 512 -Path (Join-Path $docsAssets "logo.png")

    # The mark is the gem/database artwork without the wordmark. The source is
    # 2048x2048; this crop keeps the icon legible at small sizes.
    $cropSize = 1480
    $mark = New-Object System.Drawing.Bitmap($cropSize, $cropSize)
    $graphics = [System.Drawing.Graphics]::FromImage($mark)
    try {
        $graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $graphics.DrawImage(
            $sourceImage,
            (New-Object System.Drawing.Rectangle(0, 0, $cropSize, $cropSize)),
            (New-Object System.Drawing.Rectangle(320, 30, $cropSize, $cropSize)),
            [System.Drawing.GraphicsUnit]::Pixel
        )
    }
    finally {
        $graphics.Dispose()
    }

    try {
        Save-Png -Image $mark -Size 128 -Path (Join-Path $docsAssets "logo-mark.png")
        Save-Png -Image $mark -Size 180 -Path (Join-Path $docsAssets "appicon.png")
        Save-Png -Image $mark -Size 64 -Path (Join-Path $docsAssets "favicon.png")
        Save-Png -Image $mark -Size 256 -Path (Join-Path $frontendAssets "logo-mark.png")

        $payloads = @()
        foreach ($size in @(16, 24, 32, 48, 64, 128, 256)) {
            $bitmap = New-Object System.Drawing.Bitmap($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
            $bitmapGraphics = [System.Drawing.Graphics]::FromImage($bitmap)
            try {
                $bitmapGraphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
                $bitmapGraphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
                $bitmapGraphics.DrawImage($mark, 0, 0, $size, $size)
            }
            finally {
                $bitmapGraphics.Dispose()
            }

            $handle = $bitmap.GetHicon()
            $icon = [System.Drawing.Icon]::FromHandle($handle)
            $stream = New-Object System.IO.MemoryStream
            try {
                $icon.Save($stream)
                $bytes = $stream.ToArray()
            }
            finally {
                $stream.Dispose()
                $icon.Dispose()
                [NativeIcon.Methods]::DestroyIcon($handle) | Out-Null
                $bitmap.Dispose()
            }

            $count = [BitConverter]::ToUInt16($bytes, 4)
            if ($count -ne 1) {
                throw "Unexpected icon count $count for size $size"
            }
            $dataSize = [BitConverter]::ToUInt32($bytes, 6 + 8)
            $dataOffset = [BitConverter]::ToUInt32($bytes, 6 + 12)
            $payload = New-Object byte[] $dataSize
            [Array]::Copy($bytes, $dataOffset, $payload, 0, $dataSize)
            $payloads += [pscustomobject]@{ Size = $size; Data = $payload }
        }

        $iconPath = Join-Path $windowsAssets "icon.ico"
        $stream = [System.IO.File]::Create($iconPath)
        $writer = New-Object System.IO.BinaryWriter($stream)
        try {
            $writer.Write([UInt16]0)
            $writer.Write([UInt16]1)
            $writer.Write([UInt16]$payloads.Count)
            $offset = 6 + 16 * $payloads.Count
            foreach ($entry in $payloads) {
                $dimension = 0
                if ($entry.Size -lt 256) {
                    $dimension = $entry.Size
                }
                $writer.Write([byte]$dimension)
                $writer.Write([byte]$dimension)
                $writer.Write([byte]0)
                $writer.Write([byte]0)
                $writer.Write([UInt16]1)
                $writer.Write([UInt16]32)
                $writer.Write([UInt32]$entry.Data.Length)
                $writer.Write([UInt32]$offset)
                $offset += $entry.Data.Length
            }
            foreach ($entry in $payloads) {
                $writer.Write($entry.Data)
            }
        }
        finally {
            $writer.Close()
        }
    }
    finally {
        $mark.Dispose()
    }
}
finally {
    $sourceImage.Dispose()
}

Write-Host "Brand assets written under $Root"
Write-Host "  docs/assets/logo.png (512), logo-mark.png (128), appicon.png (180), favicon.png (64)"
Write-Host "  frontend/src/assets/logo-mark.png (256)"
Write-Host "  build/windows/icon.ico (16-256 multi-size)"
