# Runs every check. Assumes `npm ci` has already been run in web/.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

Push-Location $root
try {
    go vet ./...
    if ($LASTEXITCODE -ne 0) { exit 1 }
    go vet -tags agentdev ./...
    if ($LASTEXITCODE -ne 0) { exit 1 }
    go test ./...
    if ($LASTEXITCODE -ne 0) { exit 1 }
    go test -tags agentdev ./...
    if ($LASTEXITCODE -ne 0) { exit 1 }
} finally { Pop-Location }

Push-Location (Join-Path $root 'web')
try {
    npm test
    if ($LASTEXITCODE -ne 0) { exit 1 }
    npm run typecheck
    if ($LASTEXITCODE -ne 0) { exit 1 }
} finally { Pop-Location }

& (Join-Path $PSScriptRoot 'notices.ps1') -Check
if ($LASTEXITCODE -ne 0) { exit 1 }
