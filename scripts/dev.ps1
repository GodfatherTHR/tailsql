param()

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

& (Join-Path $PSScriptRoot "fetch-h2.ps1")
& (Join-Path $PSScriptRoot "build-bridge.ps1")

Push-Location (Join-Path $root "frontend")
try {
    if (-not (Test-Path "node_modules")) {
        npm install --include=dev
        if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
    }
}
finally {
    Pop-Location
}

$wails = Get-Command wails -ErrorAction SilentlyContinue
if (-not $wails) {
    throw "Wails CLI not found. Install it with: go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1"
}

$outputAssets = Join-Path $root "build/bin/assets"
New-Item -ItemType Directory -Path $outputAssets -Force | Out-Null
Copy-Item -Path (Join-Path $root "assets/h2.jar") -Destination $outputAssets -Force

Push-Location $root
try {
    wails dev
}
finally {
    Pop-Location
}
