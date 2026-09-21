# Build the frontend and package it into application/statics/assets.zip so the Go
# backend can embed it (//go:embed assets.zip in application/statics/statics.go).
#
# Layout convention (see .gitmodules, .build/build-assets.sh and
# application/statics/statics.go): the frontend sources live in assets/, its build
# output is assets/build/, and the archive handed to the backend must keep that same
# "assets/build/..." prefix.
#
# The upstream flow is .build/build-assets.sh (bash + zip + yarn). If bash and zip are
# available you can run that instead; this PowerShell equivalent exists because this
# workflow runs on Windows without them.
#
# IMPORTANT: keep this file ASCII-only. Windows PowerShell 5.1 reads .ps1 files as
# ANSI/GBK when they have no UTF-8 BOM, which corrupts non-ASCII comments and can
# swallow the following lines into a comment (causing bogus syntax errors).
#
# Usage:
#   powershell -File assets\build-frontend.ps1
#   powershell -File assets\build-frontend.ps1 -BackendVersion 4.15.0
#   powershell -File assets\build-frontend.ps1 -SkipInstall
param(
  # Directory holding the frontend project, relative to the repository root.
  [string]$FrontendDir = "assets",
  # Value written into build/version.json. The backend validates it against
  # constants.BackendVersion at startup (application/statics/statics.go); keeping them
  # equal avoids the "Static resource version mismatch" error log.
  [string]$BackendVersion = "4.20.0",
  [switch]$SkipInstall
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path $PSScriptRoot -Parent
$frontendDir = Join-Path $repoRoot $FrontendDir
if (-not (Test-Path (Join-Path $frontendDir 'package.json'))) {
  throw "no frontend project (package.json) found at $frontendDir"
}

$staticDir = Join-Path $repoRoot 'application\statics'
$zipPath = Join-Path $staticDir 'assets.zip'
$buildDir = Join-Path $frontendDir 'build'

# Inherited npm_config_global/prefix variables make npm treat this as a global
# install and refuse to run.
foreach ($v in 'npm_config_global', 'npm_config_global_prefix', 'npm_config_prefix', 'npm_config_local_prefix', 'npm_config_globalconfig') {
  Remove-Item "env:$v" -ErrorAction SilentlyContinue
}

Push-Location $frontendDir
try {
  if (-not $SkipInstall) {
    Write-Host '==> Installing frontend dependencies...'
    # The committed package-lock.json is stale (it pins @mui/material 5.x while
    # package.json requires 6.x), so `npm ci` fails; yarn.lock is the authoritative
    # lockfile upstream. Adjust this line if you install with yarn instead.
    npm install --legacy-peer-deps --no-audit --no-fund
    if ($LASTEXITCODE -ne 0) { throw "dependency install failed with exit code $LASTEXITCODE" }
  }

  Write-Host '==> Building frontend (vite build)...'
  # Use the package.json script: it runs the local vite from node_modules/.bin. Do not
  # use `npm exec -- vite build` (it may try to fetch vite from the registry and block
  # on an interactive "Ok to proceed?" prompt), and do not use `build-prod` (it runs
  # `tsc` first, and this repository has hundreds of pre-existing type errors that are
  # unrelated to the build output).
  npm run build
  if ($LASTEXITCODE -ne 0) { throw "frontend build failed with exit code $LASTEXITCODE" }

  if (-not (Test-Path $buildDir)) { throw "build output not found at $buildDir" }

  # vite's writeBundle hook generates version.json from package.json name/version
  # ("cloudreve-frontend" / "4.0.0-next"); overwrite it with the backend version.
  $versionFile = Join-Path $buildDir 'version.json'
  $version = @{ name = 'cloudreve-frontend'; version = $BackendVersion } | ConvertTo-Json -Compress
  [System.IO.File]::WriteAllText($versionFile, $version, (New-Object System.Text.UTF8Encoding($false)))
  Write-Host "==> version.json => $version"

  Write-Host '==> Packaging assets.zip...'
  New-Item -ItemType Directory -Force -Path $staticDir | Out-Null
  if (Test-Path $zipPath) { Remove-Item $zipPath -Force }
  Add-Type -AssemblyName System.IO.Compression
  Add-Type -AssemblyName System.IO.Compression.FileSystem

  # Two hard constraints, otherwise the backend panics at startup:
  # 1. Entry paths must start with "assets/build/": the backend embeds this archive
  #    and resolves assets/build via fs.Sub (application/statics/statics.go).
  # 2. Entry names must use forward slashes. ZipFile::CreateFromDirectory writes
  #    backslash-separated names, which Go's fs.WalkDir treats as part of the file
  #    name, so every asset lookup fails.
  $zip = [System.IO.Compression.ZipFile]::Open($zipPath, [System.IO.Compression.ZipArchiveMode]::Create)
  try {
    $prefix = 'assets/build/'
    foreach ($dir in (Get-ChildItem $buildDir -Recurse -Directory)) {
      $rel = $dir.FullName.Substring($buildDir.Length + 1).Replace('\', '/')
      $null = $zip.CreateEntry($prefix + $rel + '/')
    }
    foreach ($file in (Get-ChildItem $buildDir -Recurse -File)) {
      $rel = $file.FullName.Substring($buildDir.Length + 1).Replace('\', '/')
      $entry = $zip.CreateEntry($prefix + $rel, [System.IO.Compression.CompressionLevel]::Optimal)
      $entryStream = $entry.Open()
      $fileStream = [System.IO.File]::OpenRead($file.FullName)
      try { $fileStream.CopyTo($entryStream) } finally { $entryStream.Dispose(); $fileStream.Dispose() }
    }
  }
  finally { $zip.Dispose() }

  $sizeMb = [math]::Round((Get-Item $zipPath).Length / 1MB, 2)
  Write-Host "==> Done: $zipPath ($sizeMb MB)"
  Write-Host '    Now build the backend: go build -o cloudreve.exe .'
}
finally {
  Pop-Location
}
