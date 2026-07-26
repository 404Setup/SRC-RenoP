<#
.SYNOPSIS
    Publish RenoP platform packages to the official update host (mvnc.pkg.one).

.DESCRIPTION
    Layout (repo path on host):
      update/renop/{nightly|stable}/info.json
      update/renop/{nightly|stable}/{version}/{file}

    Auth: Authorization Bearer token from env RENOP_PUBLISH_TOKEN (or MVNC_TOKEN).

.PARAMETER Channel
    nightly or stable

.PARAMETER DistDir
    Directory containing platform zips and optional manifest.json

.PARAMETER Version
    Channel version directory name (short commit for nightly, release tag for stable)

.PARAMETER Commit
    Full git commit SHA embedded in info.json

.PARAMETER BaseUrl
    Update host origin (default https://mvnc.pkg.one)
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('nightly', 'stable')]
    [string]$Channel,

    [Parameter(Mandatory = $true)]
    [string]$DistDir,

    [Parameter(Mandatory = $true)]
    [string]$Version,

    [string]$Commit = '',

    [string]$BaseUrl = 'https://mvnc.pkg.one'
)

$ErrorActionPreference = 'Stop'

$token = $env:RENOP_PUBLISH_TOKEN
if ([string]::IsNullOrWhiteSpace($token)) {
    $token = $env:MVNC_TOKEN
}
if ([string]::IsNullOrWhiteSpace($token)) {
    throw 'RENOP_PUBLISH_TOKEN (or MVNC_TOKEN) is required'
}

$DistDir = (Resolve-Path -LiteralPath $DistDir).Path
$BaseUrl = $BaseUrl.TrimEnd('/')
$Version = $Version.Trim()
if ([string]::IsNullOrWhiteSpace($Version)) {
    throw 'Version must not be empty'
}

$channelRoot = "update/renop/$Channel"
$infoPath = "$channelRoot/info.json"
$infoUrl = "$BaseUrl/$infoPath"
$authHeaders = @{
    Authorization = "Bearer $token"
    'User-Agent'  = 'RenoP-Publish'
}

function Invoke-MvncRequest {
    param(
        [Parameter(Mandatory = $true)][string]$Method,
        [Parameter(Mandatory = $true)][string]$Uri,
        [hashtable]$Headers = @{},
        [string]$InFile,
        [byte[]]$BodyBytes,
        [string]$ContentType,
        [int[]]$OkStatus = @(200, 201, 204)
    )
    $merged = @{}
    foreach ($k in $authHeaders.Keys) { $merged[$k] = $authHeaders[$k] }
    foreach ($k in $Headers.Keys) { $merged[$k] = $Headers[$k] }

    $params = @{
        Method      = $Method
        Uri         = $Uri
        Headers     = $merged
        TimeoutSec  = 600
    }
    if ($ContentType) { $params['ContentType'] = $ContentType }
    if ($InFile) { $params['InFile'] = $InFile }
    if ($null -ne $BodyBytes) { $params['Body'] = $BodyBytes }

    try {
        $resp = Invoke-WebRequest @params -UseBasicParsing
        if ($OkStatus -notcontains [int]$resp.StatusCode) {
            throw "Unexpected status $([int]$resp.StatusCode) for $Method $Uri"
        }
        return $resp
    } catch {
        $status = $null
        if ($_.Exception.Response) {
            $status = [int]$_.Exception.Response.StatusCode
        }
        if ($null -ne $status -and $OkStatus -contains $status) {
            return $null
        }
        throw
    }
}

function Get-RemoteInfo {
    try {
        $resp = Invoke-WebRequest -Method GET -Uri $infoUrl -Headers @{ 'User-Agent' = 'RenoP-Publish' } -TimeoutSec 30 -UseBasicParsing
        if ([int]$resp.StatusCode -ne 200 -or [string]::IsNullOrWhiteSpace($resp.Content)) {
            return $null
        }
        return ($resp.Content | ConvertFrom-Json)
    } catch {
        $status = $null
        if ($_.Exception.Response) {
            $status = [int]$_.Exception.Response.StatusCode
        }
        if ($status -eq 404) { return $null }
        Write-Warning "Failed to read remote info.json: $($_.Exception.Message)"
        return $null
    }
}

function Remove-VersionTree {
    param([Parameter(Mandatory = $true)][string]$Ver)
    if ([string]::IsNullOrWhiteSpace($Ver)) { return }
    $dirUrl = "$BaseUrl/$channelRoot/$Ver"
    Write-Host "Deleting previous package tree: $dirUrl"
    try {
        Invoke-MvncRequest -Method DELETE -Uri $dirUrl -OkStatus @(200, 204, 404) | Out-Null
    } catch {
        Write-Warning "DELETE $dirUrl failed: $($_.Exception.Message)"
    }
}

$zipFiles = @(Get-ChildItem -LiteralPath $DistDir -Filter '*.zip' -File | Sort-Object Name)
if ($zipFiles.Count -eq 0) {
    throw "No .zip packages found in $DistDir"
}

$manifestPath = Join-Path $DistDir 'manifest.json'
$manifestTargets = @()
if (Test-Path -LiteralPath $manifestPath) {
    $manifest = Get-Content -LiteralPath $manifestPath -Raw -Encoding utf8 | ConvertFrom-Json
    if ($manifest.targets) {
        $manifestTargets = @($manifest.targets)
    }
    if ([string]::IsNullOrWhiteSpace($Commit) -and $manifest.commit) {
        $Commit = [string]$manifest.commit
    }
    if ([string]::IsNullOrWhiteSpace($Version) -and $manifest.version) {
        $Version = [string]$manifest.version
    }
}

if ([string]::IsNullOrWhiteSpace($Commit)) {
    try { $Commit = (& git rev-parse HEAD 2>$null).Trim() } catch { $Commit = '' }
}

$targets = [System.Collections.Generic.List[object]]::new()
foreach ($zip in $zipFiles) {
    $name = $zip.Name
    $os = ''
    $arch = ''
    $fromManifest = $manifestTargets | Where-Object { $_.file -eq $name } | Select-Object -First 1
    if ($fromManifest) {
        $os = [string]$fromManifest.os
        $arch = [string]$fromManifest.arch
        $sha = [string]$fromManifest.sha256
        $size = [int64]$fromManifest.size
    } else {
        if ($name -match 'renop-.+?-([a-z0-9]+)-([a-z0-9]+)\.zip$') {
            $os = $Matches[1]
            $arch = $Matches[2]
        }
        $sha = (Get-FileHash -LiteralPath $zip.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $size = [int64]$zip.Length
    }
    if ([string]::IsNullOrWhiteSpace($os) -or [string]::IsNullOrWhiteSpace($arch)) {
        throw "Cannot derive os/arch for package $name"
    }
    if ([string]::IsNullOrWhiteSpace($sha) -or $size -le 0) {
        $sha = (Get-FileHash -LiteralPath $zip.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $size = [int64]$zip.Length
    }
    $targets.Add([ordered]@{
        os     = $os
        arch   = $arch
        file   = $name
        sha256 = $sha
        size   = $size
        path   = $zip.FullName
    })
}

Write-Host "Publishing channel=$Channel version=$Version packages=$($targets.Count) base=$BaseUrl"

$remoteInfo = Get-RemoteInfo
if ($null -ne $remoteInfo) {
    $oldVersion = [string]$remoteInfo.version
    if (-not [string]::IsNullOrWhiteSpace($oldVersion)) {
        Remove-VersionTree -Ver $oldVersion
    }
}
Remove-VersionTree -Ver $Version

foreach ($t in $targets) {
    $dest = "$BaseUrl/$channelRoot/$Version/$($t.file)"
    Write-Host "PUT $($t.file) -> $dest ($($t.size) bytes)"
    Invoke-MvncRequest -Method PUT -Uri $dest -InFile $t.path -ContentType 'application/zip' -OkStatus @(200, 201, 204) | Out-Null
}

$publishedAt = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
$infoTargets = @(
    foreach ($t in $targets) {
        [ordered]@{
            os     = $t.os
            arch   = $t.arch
            file   = $t.file
            sha256 = $t.sha256
            size   = $t.size
        }
    }
)
$info = [ordered]@{
    version      = $Version
    commit       = $Commit
    channel      = $Channel
    development  = ($Channel -eq 'nightly')
    published_at = $publishedAt
    targets      = $infoTargets
}
$infoJson = $info | ConvertTo-Json -Depth 6
$infoLocal = Join-Path $DistDir 'info.json'
[System.IO.File]::WriteAllText($infoLocal, $infoJson, [System.Text.UTF8Encoding]::new($false))
Write-Host "PUT info.json -> $infoUrl"
$infoBytes = [System.Text.Encoding]::UTF8.GetBytes($infoJson)
Invoke-MvncRequest -Method PUT -Uri $infoUrl -BodyBytes $infoBytes -ContentType 'application/json' -OkStatus @(200, 201, 204) | Out-Null

Write-Host "Published $Channel $Version ($($targets.Count) targets) to $BaseUrl/$channelRoot/"
}
