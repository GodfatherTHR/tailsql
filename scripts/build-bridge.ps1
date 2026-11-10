param()

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$jar = Join-Path $root "assets/h2.jar"
if (-not (Test-Path $jar)) {
    Write-Host "H2 jar missing; fetching it first."
    & (Join-Path $PSScriptRoot "fetch-h2.ps1")
}

$javac = Get-Command javac -ErrorAction SilentlyContinue
if (-not $javac) {
    throw "javac was not found. Install a JDK 11+ to rebuild the embedded JDBC bridge."
}

$source = Join-Path $root "internal/jdbc/JdbcBridge.java"
$output = Join-Path $root "internal/jdbc"
Write-Host "Compiling JdbcBridge with --release 11..."
& javac --release 11 -cp $jar -d $output $source
if ($LASTEXITCODE -ne 0) {
    throw "javac failed with exit code $LASTEXITCODE"
}
Write-Host "Bridge classes written to $output"
