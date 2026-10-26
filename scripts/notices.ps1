# Writes web/public/LICENSE.txt and web/public/THIRD_PARTY_NOTICES.txt (D-071).
# With -Check it regenerates in memory and exits 1 naming any file that differs.
# Assumes `npm ci` has been run in web/ so node_modules holds the npm components.
param([switch]$Check)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$pub = Join-Path $root 'web/public'

function ReadLf([string]$path) { ([IO.File]::ReadAllText($path)) -replace "`r`n", "`n" }

# Culture-independent sort, so output matches on every platform.
function OrdinalSort([scriptblock]$key) {
    $items = @($input)
    $keys = [string[]]@($items | ForEach-Object { & $key })
    [Array]::Sort($keys, [object[]]$items, [StringComparer]::Ordinal)
    $items
}

# Licence and notice files in a component's root directory, in name order.
function NoticeFiles([string]$dir) {
    Get-ChildItem -LiteralPath $dir -File |
        Where-Object { $_.Name -match '^(LICEN[CS]E|COPYING|NOTICE)' } |
        ForEach-Object { $_ } | OrdinalSort { $_.Name }
}

$sections = @{}
function AddSection([string]$name, [string]$version, [string]$source, [string]$dir) {
    $files = @(NoticeFiles $dir)
    if ($files.Count -eq 0) { throw "no licence file for $name in $dir" }
    $body = "$name $version`n$source`n"
    foreach ($f in $files) { $body += "`n--- $($f.Name) ---`n" + (ReadLf $f.FullName).TrimEnd() + "`n" }
    $sections["$name $version"] = $body
}

Push-Location $root
try {
    $lines = go list -e -tags ui -deps -f '{{with .Module}}{{.Path}} {{.Version}} {{.Dir}}{{end}}' ./cmd/ceems
    if ($LASTEXITCODE -ne 0) { throw 'go list failed' }
    $mods = $lines | Where-Object { $_ } | Sort-Object -Unique | ForEach-Object {
        $p = $_ -split ' ', 3
        [pscustomobject]@{ Path = $p[0]; Version = $p[1]; Dir = $p[2] }
    } | Where-Object { $_.Version }   # the main module has no version
    foreach ($m in $mods) { AddSection $m.Path $m.Version "https://pkg.go.dev/$($m.Path)@$($m.Version)" $m.Dir }
    $goroot = (go env GOROOT).Trim()
    $sections['Go standard library'] = "Go standard library`nhttps://go.dev`n`n--- LICENSE ---`n" + (ReadLf (Join-Path $goroot 'LICENSE')).TrimEnd() + "`n"
} finally { Pop-Location }

# npm: vue and vue-router plus the closure of their dependencies, resolved as Node does.
$web = Join-Path $root 'web'
$seen = @{}
function AddNpm([string]$name, [string]$from) {
    $d = $from
    while (-not (Test-Path -LiteralPath (Join-Path $d "node_modules/$name/package.json"))) {
        if ($d -eq $web) { throw "npm package $name not found; run npm ci in web/" }
        $d = Split-Path -Parent $d
    }
    $c = Join-Path $d "node_modules/$name"
    if ($seen.ContainsKey($c)) { return }
    $seen[$c] = $true
    $pkg = Get-Content -Raw -LiteralPath (Join-Path $c 'package.json') | ConvertFrom-Json
    AddSection "npm:$name" $pkg.version "https://www.npmjs.com/package/$name/v/$($pkg.version)" $c
    if ($pkg.dependencies) { foreach ($dep in $pkg.dependencies.PSObject.Properties.Name) { AddNpm $dep $c } }
}
foreach ($n in 'vue', 'vue-router') { AddNpm $n $web }
# Vite's modulepreload polyfill ships in the bundle; its own dependencies are build tooling and do not.
$vite = Join-Path $web 'node_modules/vite'
$viteVersion = (Get-Content -Raw -LiteralPath (Join-Path $vite 'package.json') | ConvertFrom-Json).version
AddSection 'npm:vite' $viteVersion "https://www.npmjs.com/package/vite/v/$viteVersion" $vite

# Fonts: one section per heading of the fonts' own licence record.
$fonts = ReadLf (Join-Path $root 'web/src/assets/fonts/LICENSE.md')
foreach ($b in ($fonts -split '(?m)^## ' | Select-Object -Skip 1)) {
    $name, $rest = $b -split "`n", 2
    $sections["font:$name"] = "$name`n$($rest.TrimEnd())`n"
}

$license = ReadLf (Join-Path $root 'LICENSE')
$nl = [string]::new('=', 78)
$out = "CeeMS, Copyright $([char]0xA9) 2026 Conor Morgan and CeeMS contributors, AGPL-3.0-or-later (see LICENSE.txt).`n" +
    "CeeMS includes the following third-party components.`n"
foreach ($k in ($sections.Keys | OrdinalSort { $_ })) { $out += "`n$nl`n$($sections[$k])" }

$files = @{ 'LICENSE.txt' = $license; 'THIRD_PARTY_NOTICES.txt' = $out }
$utf8 = New-Object Text.UTF8Encoding($false)
$bad = @()
foreach ($n in $files.Keys) {
    $path = Join-Path $pub $n
    if ($Check) {
        if (-not (Test-Path -LiteralPath $path) -or (ReadLf $path) -ne $files[$n]) { $bad += "web/public/$n" }
    } else {
        New-Item -ItemType Directory -Force $pub | Out-Null
        [IO.File]::WriteAllText($path, $files[$n], $utf8)
    }
}
if ($bad) { Write-Warning "out of date, run scripts/notices.ps1: $($bad -join ', ')"; exit 1 }
Write-Host "$($sections.Count) notice sections"
