# Entry point for the storage-policy relocation test suites.
#
# Builds the frontend-embedded binary is NOT done here on purpose: the suites only
# exercise the backend, so the existing cloudreve.exe is reused. Run
# assets\build-frontend.ps1 + `go build -o cloudreve.exe .` first if the frontend
# changed.
#
# Usage (from the repository root):
#   powershell -NoProfile -ExecutionPolicy Bypass -File tests\relocate\run.ps1
#
# ASCII only (Windows PowerShell 5.1 reads .ps1 as ANSI when there is no BOM).
param(
  [string]$Exe = "",
  [int]$StartPort = 5410
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
if (-not $Exe) { $Exe = Join-Path $repo 'cloudreve.exe' }

if (-not (Test-Path $Exe)) {
  throw "cloudreve.exe not found at $Exe (build it first: go build -o cloudreve.exe .)"
}

Write-Host "repository: $repo"
Write-Host "binary    : $Exe"
Write-Host ""

$suites = @(
  @{ Name = 'relocate verify (paths, encryption, residue)'; Script = 'relocate-verify.ps1' },
  @{ Name = 'rollback (fault injection)'; Script = 'relocate-rollback.ps1' },
  @{ Name = 'plain -> encrypting policy'; Script = 'relocate-plain-to-encrypting.ps1' }
)

$summary = @()
$port = $StartPort

foreach ($suite in $suites) {
  $root = Join-Path ([System.IO.Path]::GetTempPath()) ("cr-test-" + [guid]::NewGuid().ToString('N').Substring(0, 8))
  New-Item -ItemType Directory -Force -Path (Join-Path $root 'data') | Out-Null
  Copy-Item $Exe (Join-Path $root 'cloudreve.exe') -Force
  # The suites drive the API over HTTP on a per-suite port.
  @"
[System]
Mode = master
Listen = 127.0.0.1:$port
LogLevel = warning

[Database]
Type = sqlite
DBFile = cloudreve.db
"@ | Set-Content -Encoding UTF8 (Join-Path $root 'data\conf.ini')

  Write-Host "=== $($suite.Name) (port $port) ==="
  $log = Join-Path $root 'suite.log'
  & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot $suite.Script) -Root $root -Port $port *> $log
  $code = $LASTEXITCODE
  Get-Content $log | ForEach-Object { Write-Host "  $_" }

  $result = (Get-Content $log | Select-String 'RESULT:' | Select-Object -Last 1).Line
  $summary += [pscustomobject]@{
    Suite   = $suite.Name
    Outcome = if ($code -eq 0) { 'PASS' } else { 'FAIL' }
    Detail  = if ($result) { $result.Trim() } else { "exit=$code" }
  }

  # Keep the instance directory when a suite fails, so its logs can be inspected.
  if ($code -eq 0) { Remove-Item $root -Recurse -Force -ErrorAction SilentlyContinue }
  else { Write-Host "  (instance kept for inspection: $root)" }

  $port++
  Write-Host ""
}

Write-Host "================ SUMMARY ================"
$summary | Format-Table -AutoSize

if (($summary | Where-Object { $_.Outcome -eq 'FAIL' }).Count -gt 0) { exit 1 }
Write-Host "all suites passed"
