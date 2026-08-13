# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

$ErrorActionPreference = "Stop"
$repo = "larksuite/cli"
$version = $env:LARK_CLI_VERSION
$installDir = $env:LARK_CLI_INSTALL_DIR
if ([string]::IsNullOrWhiteSpace($installDir)) {
  $installDir = Join-Path $env:LOCALAPPDATA "Programs\lark-cli\bin"
}

$arch = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
  "X64" { "amd64" }
  "Arm64" { "arm64" }
  default { throw "Unsupported architecture: $_" }
}

if ([string]::IsNullOrWhiteSpace($version)) {
  $latest = Invoke-RestMethod `
    -Headers @{ "User-Agent" = "lark-cli-extended-installer" } `
    -Uri "https://api.github.com/repos/$repo/releases/latest"
  $version = [string]$latest.tag_name
  $version = $version.TrimStart("v")
}
if ($version -notmatch '^[0-9A-Za-z.+-]+$') {
  throw "Invalid lark-cli version: $version"
}

$archive = "lark-cli-extended-$version-windows-$arch.zip"
$base = "https://github.com/$repo/releases/download/v$version"
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("lark-cli-extended-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmpDir | Out-Null

try {
  $checksumsPath = Join-Path $tmpDir "checksums.txt"
  $archivePath = Join-Path $tmpDir $archive
  Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile $checksumsPath
  Invoke-WebRequest -UseBasicParsing -Uri "$base/$archive" -OutFile $archivePath

  $checksumLine = Get-Content $checksumsPath | Where-Object {
    $_ -match ('^[0-9a-fA-F]{64}\s+\*?' + [regex]::Escape($archive) + '$')
  } | Select-Object -First 1
  if (-not $checksumLine) {
    throw "Checksum entry not found for $archive"
  }
  $expected = ($checksumLine -split '\s+')[0].ToLowerInvariant()
  $actual = (Get-FileHash -Algorithm SHA256 $archivePath).Hash.ToLowerInvariant()
  if ($actual -ne $expected) {
    throw "Checksum verification failed for $archive"
  }

  Expand-Archive -LiteralPath $archivePath -DestinationPath $tmpDir -Force
  $candidate = Join-Path $tmpDir "lark-cli.exe"
  $metadata = & $candidate version --json | ConvertFrom-Json
  if ($metadata.edition -ne "extended" -or $metadata.version.TrimStart("v") -ne $version.TrimStart("v")) {
    throw "Downloaded binary is not the requested lark-cli Extended version"
  }

  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  Copy-Item -Force $candidate (Join-Path $installDir "lark-cli.exe")

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $pathEntries = @($userPath -split ';' | Where-Object { $_ })
  if ($pathEntries -notcontains $installDir) {
    $newPath = (($pathEntries + $installDir) -join ';')
    [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
    $env:Path = "$env:Path;$installDir"
  }
  Write-Host "lark-cli Extended $version installed at $installDir\lark-cli.exe"
} finally {
  Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $tmpDir
}
