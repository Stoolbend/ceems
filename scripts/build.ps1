# Builds the Vue app, then embeds it in windows/amd64, linux/arm64,
# linux/amd64 and linux/armv6 (Raspberry Pi 1 and Zero) binaries. -Version is
# shown in the footer; the default is the git describe output.
param([string]$Version = (git describe --tags --always --dirty))
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Push-Location (Join-Path $root 'web')
try {
    $env:VITE_CEEMS_VERSION = $Version
    npm ci
    if ($LASTEXITCODE -ne 0) { exit 1 }
    npm run build
    if ($LASTEXITCODE -ne 0) { exit 1 }
} finally {
    Remove-Item Env:VITE_CEEMS_VERSION -ErrorAction SilentlyContinue
    Pop-Location
}

Push-Location $root
try {
    $env:CGO_ENABLED = '0'
    $targets = @(
        @{ os = 'windows'; arch = 'amd64'; out = 'dist/ceems-windows-amd64.exe' },
        @{ os = 'linux'; arch = 'arm64'; out = 'dist/ceems-linux-arm64' },
        @{ os = 'linux'; arch = 'amd64'; out = 'dist/ceems-linux-amd64' },
        @{ os = 'linux'; arch = 'arm'; arm = '6'; out = 'dist/ceems-linux-armv6' }
    )
    foreach ($t in $targets) {
        $env:GOOS = $t.os
        $env:GOARCH = $t.arch
        if ($t.arm) { $env:GOARM = $t.arm } else { Remove-Item Env:GOARM -ErrorAction SilentlyContinue }
        go build -tags ui -trimpath -ldflags "-s -w" -o $t.out ./cmd/ceems
        if ($LASTEXITCODE -ne 0) { exit 1 }
    }
} finally {
    Remove-Item Env:GOOS, Env:GOARCH, Env:GOARM, Env:CGO_ENABLED -ErrorAction SilentlyContinue
    Pop-Location
}
