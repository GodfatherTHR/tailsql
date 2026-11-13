param(
    [switch]$SkipFrontend
)

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
    if (-not $SkipFrontend) {
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "frontend build failed" }
    }
}
finally {
    Pop-Location
}

$wails = Get-Command wails -ErrorAction SilentlyContinue
if (-not $wails) {
    throw "Wails CLI not found. Install it with: go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1"
}

Push-Location $root
try {
    wails build
    if ($LASTEXITCODE -ne 0) { throw "wails build failed" }

    $assets = Join-Path $root "assets"
    $outputAssets = Join-Path $root "build/bin/assets"
    New-Item -ItemType Directory -Path $outputAssets -Force | Out-Null
    Copy-Item -Path (Join-Path $assets "h2.jar") -Destination $outputAssets -Force
    if (Test-Path (Join-Path $assets "h2.jar.sha256")) {
        Copy-Item -Path (Join-Path $assets "h2.jar.sha256") -Destination $outputAssets -Force
    }

    $wailsConfig = Get-Content (Join-Path $root "wails.json") -Raw | ConvertFrom-Json
    $version = $wailsConfig.info.productVersion
    $versionedExe = Join-Path $root "build/bin/HySQL-$version.exe"
    Copy-Item -Path (Join-Path $root "build/bin/HySQL.exe") -Destination $versionedExe -Force

    $mcpBridge = Join-Path $root "build/bin/hysql-mcp.exe"
    & go build -o $mcpBridge ./cmd/hysql-mcp
    if ($LASTEXITCODE -ne 0) { throw "hysql-mcp build failed" }

    Write-Host "Build complete: $root/build/bin/HySQL.exe"
    Write-Host "Versioned executable: $versionedExe"
    Write-Host "MCP stdio bridge: $mcpBridge"
    Write-Host "H2 asset copied next to the executable."
}
finally {
    Pop-Location
}
