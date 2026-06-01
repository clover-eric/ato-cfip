param(
    [string]$InstallDir = "$HOME\ato-cfip",
    [string]$RepoUrl = "https://github.com/clover-eric/ato-cfip.git",
    [int]$WebPort = 8080
)

$ErrorActionPreference = "Stop"

function Require-Command($Name, $Hint) {
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "$Name is required. $Hint"
    }
}

Write-Host "==> ATO-CFIP one-click installer"
Write-Host "==> Install dir: $InstallDir"

Require-Command git "Install Git for Windows first."
Require-Command docker "Install Docker Desktop or NAS Container Manager first."

if (Test-Path (Join-Path $InstallDir ".git")) {
    Write-Host "==> Updating existing checkout"
    git -C $InstallDir pull --ff-only
} else {
    Write-Host "==> Cloning repository"
    git clone $RepoUrl $InstallDir
}

Set-Location $InstallDir

if (-not (Test-Path ".env")) {
    Copy-Item ".env.example" ".env"
    (Get-Content ".env") -replace '^CFST_WEB_PORT=.*', "CFST_WEB_PORT=$WebPort" | Set-Content ".env"
}

if (-not (Test-Path "config.yaml")) {
    Copy-Item "config.example.yaml" "config.yaml"
    Write-Host "==> Created config.yaml"
    Write-Host "==> Edit config.yaml to set your domain and Cloudflare API token when ready."
}

New-Item -ItemType Directory -Force -Path "data" | Out-Null

Write-Host "==> Starting service"
docker compose up -d --build

Write-Host ""
Write-Host "ATO-CFIP is running."
Write-Host "Open: http://YOUR_NAS_IP:$WebPort"
Write-Host "Config: $InstallDir\config.yaml"

