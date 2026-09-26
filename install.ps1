# PruneDocker Windows One-Liner Installer
# Usage: irm https://raw.githubusercontent.com/Minhaj009/PruneDocker/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$repo = "Minhaj009/PruneDocker"
$installDir = "$env:LOCALAPPDATA\Programs\prunedocker\bin"
$binaryPath = "$installDir\prunedocker.exe"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  PruneDocker: Windows One-Liner Installer" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Force -Path $installDir | Out-Null
}

$releaseUrl = "https://api.github.com/repos/$repo/releases/latest"
$latestRelease = $null
try {
    $resp = Invoke-RestMethod -Uri $releaseUrl -Headers @{ "User-Agent" = "PruneDocker-Installer" }
    $latestRelease = $resp.tag_name
} catch {
    # Fallback to source compilation if API rate limited or no release yet
}

$installed = $false
if ($latestRelease) {
    $cleanTag = $latestRelease.TrimStart("v")
    $zipName = "prunedocker_${cleanTag}_windows_amd64.zip"
    $downloadUrl = "https://github.com/$repo/releases/download/$latestRelease/$zipName"
    $tempZip = "$env:TEMP\$zipName"
    
    Write-Host "[INFO] Downloading release $latestRelease from GitHub..." -ForegroundColor Green
    try {
        Invoke-WebRequest -Uri $downloadUrl -OutFile $tempZip
        Expand-Archive -Path $tempZip -DestinationPath $installDir -Force
        Remove-Item -Path $tempZip -Force
        $installed = $true
    } catch {
        Write-Host "[WARN] Binary archive download failed, attempting build from source..." -ForegroundColor Yellow
    }
}

if (-not $installed) {
    if (Get-Command go -ErrorAction SilentlyContinue) {
        Write-Host "[INFO] Building latest prunedocker binary with Go..." -ForegroundColor Green
        $tempSource = "$env:TEMP\prunedocker_build"
        if (Test-Path $tempSource) { Remove-Item -Recurse -Force $tempSource }
        & git clone --depth 1 "https://github.com/$repo.git" $tempSource
        & go build -o $binaryPath "$tempSource\cmd\prunedocker"
        Remove-Item -Recurse -Force $tempSource
        $installed = $true
    } else {
        Write-Host "[ERROR] Could not download pre-compiled binary and Go is not installed." -ForegroundColor Red
        exit 1
    }
}

# Add to user PATH if not present
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    $env:Path = "$env:Path;$installDir"
    Write-Host "[INFO] Added $installDir to User PATH." -ForegroundColor Green
}

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "  PruneDocker installed successfully to $binaryPath!" -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "Run:"
Write-Host "  prunedocker analyze"
Write-Host "  prunedocker ui"
Write-Host "  prunedocker prune --dry-run"
