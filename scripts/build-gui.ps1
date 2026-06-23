param(
    [string]$OutputDir = "dist"
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$targetDir = Join-Path $root $OutputDir
$exePath = Join-Path $targetDir "CodexSependMonitor.exe"
$goCommand = Get-Command go -ErrorAction SilentlyContinue
$goPath = $null
if ($goCommand) {
    $goPath = $goCommand.Source
}

if (-not $goPath) {
    $defaultGo = "C:\Program Files\Go\bin\go.exe"
    if (Test-Path -LiteralPath $defaultGo) {
        $goPath = $defaultGo
    }
}

if (-not $goPath) {
    throw "go executable not found. Install Go or add go.exe to PATH."
}

New-Item -ItemType Directory -Force -Path $targetDir | Out-Null

& $goPath build -ldflags="-H=windowsgui" -o $exePath ./cmd/codex-spend-monitor

Write-Host "Built $exePath"
