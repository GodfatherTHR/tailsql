param(
    [string]$Version = "2.3.232",
    [string]$Destination = "",
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($Destination)) {
    $Destination = Join-Path $root "assets"
}
New-Item -ItemType Directory -Path $Destination -Force | Out-Null
$target = Join-Path $Destination "h2.jar"

function Test-H2Archive([string]$Path) {
    if (-not (Test-Path $Path)) {
        return $false
    }
    try {
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $archive = [System.IO.Compression.ZipFile]::OpenRead($Path)
        try {
            $names = @($archive.Entries | ForEach-Object { $_.FullName })
            foreach ($required in @("org/h2/Driver.class", "org/h2/tools/Server.class", "org/h2/tools/RunScript.class")) {
                if ($names -notcontains $required) {
                    return $false
                }
            }
            return $true
        }
        finally {
            $archive.Dispose()
        }
    }
    catch {
        return $false
    }
}

if (-not $Force -and (Test-H2Archive $target)) {
    Write-Host "H2 $Version already present and valid at $target"
    exit 0
}

$url = "https://repo1.maven.org/maven2/com/h2database/h2/$Version/h2-$Version.jar"
$temporary = "$target.download"
if (Test-Path $temporary) {
    Remove-Item $temporary -Force
}

Write-Host "Downloading H2 $Version from Maven Central..."
Invoke-WebRequest -Uri $url -OutFile $temporary -UseBasicParsing
if (-not (Test-H2Archive $temporary)) {
    Remove-Item $temporary -Force -ErrorAction SilentlyContinue
    throw "Downloaded file is not a valid H2 jar."
}
Move-Item -Path $temporary -Destination $target -Force
$hash = (Get-FileHash -Algorithm SHA256 -Path $target).Hash.ToLowerInvariant()
Set-Content -Path "$target.sha256" -Value "$hash  h2.jar" -Encoding ASCII
Write-Host "Installed $target"
Write-Host "SHA-256: $hash"
