# Starts or stops a throwaway CeeMS server with a disposable admin, for agent UI checks (D-041, D-042).
#   agent-server.ps1 start [-Seed <dir of .tti files>]
#   agent-server.ps1 stop -RunDir <path>
param(
    [Parameter(Mandatory = $true, Position = 0)][ValidateSet('start', 'stop')][string]$Action,
    [string]$Seed,
    [string]$RunDir
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\')

function Test-Under([string]$path, [string]$parent) {
    $full = [IO.Path]::GetFullPath($path).TrimEnd('\')
    return $full.StartsWith($parent.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase)
}

function Stop-ByPid([string]$pidFile) {
    if (-not (Test-Path -LiteralPath $pidFile)) { return }
    $id = [int](Get-Content -LiteralPath $pidFile -Raw).Trim()
    $p = Get-Process -Id $id -ErrorAction SilentlyContinue
    if ($p) { Stop-Process -Id $id -Force; $p.WaitForExit(5000) | Out-Null }
}

if ($Action -eq 'stop') {
    if (-not $RunDir) { Write-Error 'stop needs -RunDir'; exit 1 }
    $full = [IO.Path]::GetFullPath($RunDir).TrimEnd('\')
    if (-not (Test-Under $full $tempRoot) -or (Split-Path -Leaf $full) -notlike 'ceems-agent-*' -or -not (Test-Path -LiteralPath $full -PathType Container)) {
        Write-Error "refusing: $RunDir is not a ceems-agent-* directory under the temp path"
        exit 1
    }
    Stop-ByPid (Join-Path $full 'pid')
    Remove-Item -LiteralPath $full -Recurse -Force
    'stopped'
    exit 0
}

$run = Join-Path $tempRoot ('ceems-agent-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Path $run | Out-Null
$serverPid = $null
try {
    Push-Location (Join-Path $root 'web')
    try {
        if (-not (Test-Path 'node_modules')) {
            npm ci
            if ($LASTEXITCODE -ne 0) { throw 'npm ci failed' }
        }
        npm run build
        if ($LASTEXITCODE -ne 0) { throw 'npm run build failed' }
    } finally { Pop-Location }

    $exe = Join-Path $run 'ceems.exe'
    Push-Location $root
    try {
        go build -tags "ui agentdev" -o $exe ./cmd/ceems
        if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
    } finally { Pop-Location }

    $listener = New-Object Net.Sockets.TcpListener([Net.IPAddress]::Loopback, 0)
    $listener.Start()
    $port = $listener.LocalEndpoint.Port
    $listener.Stop()

    $dataDir = Join-Path $run 'data'
    $pagesDir = Join-Path $run 'pages'
    $cfg = Join-Path $run 'ceems.json'
    # Nothing touches data until both directories are proven to be inside the run directory.
    if (-not (Test-Under $run $tempRoot) -or -not (Test-Under $dataDir $run) -or -not (Test-Under $pagesDir $run)) {
        throw 'refusing: dataDir or pagesDir is outside the run directory'
    }

    $json = [ordered]@{ listen = "127.0.0.1:$port"; dataDir = $dataDir; pagesDir = $pagesDir; logLevel = 'warn' } | ConvertTo-Json
    [IO.File]::WriteAllText($cfg, $json, (New-Object Text.UTF8Encoding($false)))
    New-Item -ItemType Directory -Path $dataDir, $pagesDir | Out-Null

    if ($Seed) {
        & $exe import -config $cfg $Seed
        if ($LASTEXITCODE -ne 0) { throw 'import failed' }
    }

    # Native stderr must not become a terminating error while the password line is held in $out.
    $ErrorActionPreference = 'Continue'
    $out = & $exe admin create -config $cfg agent 2>&1 | ForEach-Object { "$_" }
    $code = $LASTEXITCODE
    $ErrorActionPreference = 'Stop'
    # The password is never used: the agentdev build logs in at /api/v1/agent-login (D-042).
    if ($code -ne 0) {
        $safe = ($out | Where-Object { $_ -notlike 'password: *' }) -join "`n"
        throw "admin create failed: $safe"
    }

    # ShellExecute starts the server without inheriting this script's stdout pipe, so a caller that
    # captures our output is not held open by the long-lived server. cmd does the log redirection.
    $psi = New-Object Diagnostics.ProcessStartInfo
    $psi.FileName = $env:ComSpec
    $psi.Arguments = "/c `"`"$exe`" serve -config `"$cfg`" >`"$(Join-Path $run 'server.log')`" 2>`"$(Join-Path $run 'server.err.log')`"`""
    $psi.UseShellExecute = $true
    $psi.WindowStyle = 'Hidden'
    $cmd = [Diagnostics.Process]::Start($psi)
    for ($i = 0; $i -lt 20 -and -not $serverPid; $i++) {
        $child = Get-CimInstance Win32_Process -Filter "ParentProcessId = $($cmd.Id)" | Where-Object { $_.ExecutablePath -eq $exe } | Select-Object -First 1
        if ($child) { $serverPid = $child.ProcessId } else { Start-Sleep -Milliseconds 250 }
    }
    if (-not $serverPid) { Stop-Process -Id $cmd.Id -Force -ErrorAction SilentlyContinue; throw 'server process did not start' }
    Set-Content -LiteralPath (Join-Path $run 'pid') -Value $serverPid -NoNewline
    $url = "http://127.0.0.1:$port/"
    $ready = $false
    for ($i = 0; $i -lt 30 -and -not $ready; $i++) {
        try { Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 2 | Out-Null; $ready = $true }
        catch {
            if ($_.Exception.Response) { $ready = $true } else { Start-Sleep -Milliseconds 500 }
        }
    }
    if (-not $ready) { throw 'server did not answer within 15 s' }

    "url: $url"
    'user: agent'
    "login url: http://127.0.0.1:$port/api/v1/agent-login"
    "run dir: $run"
    "pid: $serverPid"
} catch {
    if ($serverPid) { Stop-Process -Id $serverPid -Force -ErrorAction SilentlyContinue }
    Remove-Item -LiteralPath $run -Recurse -Force -ErrorAction SilentlyContinue
    Write-Error $_.Exception.Message
    exit 1
}
