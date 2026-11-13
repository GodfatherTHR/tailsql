param(
    [string]$Makensis = "",
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

if (-not $Version) {
    $wails = Get-Content (Join-Path $root "wails.json") -Raw | ConvertFrom-Json
    $Version = $wails.info.productVersion
}

$versionedExe = Join-Path $root "build\bin\HySQL-$Version.exe"
if (-not (Test-Path $versionedExe)) {
    $plainExe = Join-Path $root "build\bin\HySQL.exe"
    if (-not (Test-Path $plainExe)) {
        throw "Build first: neither $versionedExe nor $plainExe exists."
    }
    Copy-Item -Path $plainExe -Destination $versionedExe -Force
}

$h2 = Join-Path $root "build\bin\assets\h2.jar"
if (-not (Test-Path $h2)) {
    throw "Missing $h2. Run scripts/build.ps1 first."
}

if (-not $Makensis) {
    $command = Get-Command "makensis" -ErrorAction SilentlyContinue
    if ($command) {
        $Makensis = $command.Source
    } else {
        foreach ($candidate in @(
            "C:\Program Files (x86)\NSIS\makensis.exe",
            "C:\Program Files\NSIS\makensis.exe"
        )) {
            if (Test-Path $candidate) {
                $Makensis = $candidate
                break
            }
        }
    }
}

if (-not $Makensis -or -not (Test-Path $Makensis)) {
    throw "makensis was not found. Install NSIS, then run: scripts/build-installer.ps1 -Makensis <path-to-makensis.exe>"
}

New-Item -ItemType Directory -Path (Join-Path $root "dist") -Force | Out-Null
& $Makensis "/V2" "/DAPP_VERSION=$Version" (Join-Path $PSScriptRoot "installer.nsi")
if ($LASTEXITCODE -ne 0) {
    throw "makensis failed with exit code $LASTEXITCODE"
}

$setup = Join-Path $root "dist\HySQL-Setup-$Version.exe"
if (-not (Test-Path $setup)) {
    throw "Installer output was not created: $setup"
}
Write-Host "Installer: $setup"
