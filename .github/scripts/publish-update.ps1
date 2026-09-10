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
    Directory containing raw Brotli platform packages and optional manifest.json

.PARAMETER Version
    Channel version directory name (short commit for nightly, release tag for stable)

.PARAMETER Commit
    Full git commit SHA embedded in info.json

.PARAMETER PreviousCommit
    Full git commit SHA of the preceding stable release

.PARAMETER Changelog
    Release notes or commit messages content embedded in info.json

.PARAMETER ChangelogFile
    Path to file containing changelog text

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

    [string]$PreviousCommit = '',

    [string]$Changelog = '',

    [string]$ChangelogFile = '',

    [string]$BaseUrl = 'https://mvnc.pkg.one'
)

$ErrorActionPreference = 'Stop'
$nightlyPackageRetention = 9

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
if ($Version -notmatch '^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$') {
    throw 'Version must be a single safe directory name'
}

if ([string]::IsNullOrWhiteSpace($Changelog) -and -not [string]::IsNullOrWhiteSpace($ChangelogFile) -and (Test-Path -LiteralPath $ChangelogFile)) {
    $Changelog = Get-Content -LiteralPath $ChangelogFile -Raw -Encoding utf8
}
if ([string]::IsNullOrWhiteSpace($Changelog)) {
    $genScript = Join-Path $PSScriptRoot 'generate-changelog.ps1'
    if (Test-Path -LiteralPath $genScript) {
        try {
            $Changelog = & pwsh -NoProfile -File $genScript -Commit (if ($Commit) { $Commit } else { 'HEAD' })
        } catch {
            Write-Warning "Could not auto-generate changelog: $($_.Exception.Message)"
        }
    }
}
$Changelog = if ($null -ne $Changelog) { $Changelog.Trim() } else { '' }
if (-not [string]::IsNullOrWhiteSpace($Changelog) -and $Channel -eq 'stable') {
    $clLines = @($Changelog -split "`r?`n")
    $formattedLines = @(
        foreach ($cl in $clLines) {
            $trimmed = $cl.Trim()
            if ([string]::IsNullOrWhiteSpace($trimmed)) {
                $cl
            } elseif ($trimmed.StartsWith('- ') -or $trimmed.StartsWith('* ') -or $trimmed.StartsWith('#')) {
                $cl
            } else {
                "- $trimmed"
            }
        }
    )
    $Changelog = ($formattedLines -join "`n").Trim()
}

$channelRoot = "update/renop/$Channel"
$infoPath = "$channelRoot/info.json"
$infoUrl = "$BaseUrl/$infoPath"

$httpHandler = [System.Net.Http.SocketsHttpHandler]::new()
$httpHandler.PooledConnectionLifetime = [TimeSpan]::FromMinutes(5)
$httpHandler.PooledConnectionIdleTimeout = [TimeSpan]::FromSeconds(30)
$httpHandler.MaxConnectionsPerServer = 16
$httpHandler.EnableMultipleHttp2Connections = $true

$httpClient = [System.Net.Http.HttpClient]::new($httpHandler)
$httpClient.Timeout = [TimeSpan]::FromSeconds(600)
$httpClient.DefaultRequestHeaders.UserAgent.ParseAdd('RenoP-Publish')
$httpClient.DefaultRequestHeaders.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $token)

function Get-RemoteInfoJson {
    param([string]$Url)
    $req = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Get, $Url)
    $resp = $null
    try {
        $resp = $httpClient.SendAsync($req).GetAwaiter().GetResult()
        if ($resp.StatusCode -eq [System.Net.HttpStatusCode]::NotFound) { return $null }
        if (-not $resp.IsSuccessStatusCode) { throw "Read info.json returned HTTP $([int]$resp.StatusCode)" }
        $jsonStr = $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        if ([string]::IsNullOrWhiteSpace($jsonStr)) { throw 'Remote info.json is empty' }
        return ($jsonStr | ConvertFrom-Json -DateKind String)
    } finally {
        if ($null -ne $resp) { $resp.Dispose() }
        $req.Dispose()
    }
}

$packageFiles = @(Get-ChildItem -LiteralPath $DistDir -Filter '*.br' -File | Sort-Object Name)
if ($packageFiles.Count -eq 0) {
    throw "No .br packages found in $DistDir"
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
    if ([string]::IsNullOrWhiteSpace($PreviousCommit) -and $manifest.previous_commit) {
        $PreviousCommit = [string]$manifest.previous_commit
    }
    if ([string]::IsNullOrWhiteSpace($Version) -and $manifest.version) {
        $Version = [string]$manifest.version
    }
}

if ([string]::IsNullOrWhiteSpace($Commit)) {
    try { $Commit = (& git rev-parse HEAD 2>$null).Trim() } catch { $Commit = '' }
}
if ($Channel -eq 'stable' -and [string]::IsNullOrWhiteSpace($PreviousCommit) -and -not [string]::IsNullOrWhiteSpace($Commit)) {
    try { $PreviousCommit = (& git log -1 --format='%H' -i --grep='^\[release\]' "${Commit}^" 2>$null).Trim() } catch { $PreviousCommit = '' }
    if ([string]::IsNullOrWhiteSpace($PreviousCommit)) {
        try {
            $previousTag = (& git describe --tags --abbrev=0 "${Commit}^" 2>$null).Trim()
            if (-not [string]::IsNullOrWhiteSpace($previousTag)) {
                $PreviousCommit = (& git rev-parse "$previousTag^{commit}" 2>$null).Trim()
            }
        } catch { $PreviousCommit = '' }
    }
}

$targets = [System.Collections.Generic.List[object]]::new()
foreach ($packageFile in $packageFiles) {
    $name = $packageFile.Name
    $os = ''
    $arch = ''
    $fromManifest = $manifestTargets | Where-Object { $_.file -eq $name } | Select-Object -First 1
    if ($fromManifest) {
        $os = [string]$fromManifest.os
        $arch = [string]$fromManifest.arch
        $sha = [string]$fromManifest.sha256
        $size = [int64]$fromManifest.size
        $uncompressedSize = [int64]$fromManifest.uncompressed_size
        $format = if ($fromManifest.format) { [string]$fromManifest.format } else { 'brotli' }
        $executable = if ($fromManifest.executable) { [string]$fromManifest.executable } else { if ($os -eq 'windows') { 'renop.exe' } else { 'renop' } }
    } else {
        if ($name -match 'renop-.+?-([a-z0-9]+)-([a-z0-9]+)\.br$') {
            $os = $Matches[1]
            $arch = $Matches[2]
        }
        $sha = (Get-FileHash -LiteralPath $packageFile.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $size = [int64]$packageFile.Length
        $uncompressedSize = 0
        $format = 'brotli'
        $executable = if ($os -eq 'windows') { 'renop.exe' } else { 'renop' }
    }
    if ([string]::IsNullOrWhiteSpace($os) -or [string]::IsNullOrWhiteSpace($arch)) {
        throw "Cannot derive os/arch for package $name"
    }
    if ([string]::IsNullOrWhiteSpace($sha) -or $size -le 0) {
        $sha = (Get-FileHash -LiteralPath $packageFile.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
        $size = [int64]$packageFile.Length
    }
    $targets.Add([ordered]@{
        os           = $os
        arch         = $arch
        file         = $name
        sha256       = $sha
        size         = $size
        uncompressed_size = $uncompressedSize
        format       = $format
        executable   = $executable
        download_url = "$BaseUrl/$channelRoot/$Version/$name"
        path         = $packageFile.FullName
    })
}

Write-Host "Publishing channel=$Channel version=$Version packages=$($targets.Count) base=$BaseUrl"

$publishedAt = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')

$currentReleaseTargets = @(
    foreach ($t in $targets) {
        [ordered]@{
            os           = $t.os
            arch         = $t.arch
            file         = $t.file
            sha256       = $t.sha256
            size         = $t.size
            uncompressed_size = $t.uncompressed_size
            format       = $t.format
            executable   = $t.executable
            download_url = $t.download_url
        }
    }
)

$currentRelease = [ordered]@{
    version      = $Version
    commit       = $Commit
    previous_commit = if ($Channel -eq 'stable') { $PreviousCommit } else { '' }
    channel      = $Channel
    development  = ($Channel -eq 'nightly')
    published_at = $publishedAt
    changelog    = $Changelog
    targets      = $currentReleaseTargets
}

$remoteInfo = Get-RemoteInfoJson -Url $infoUrl

function Extract-ReleasesFromInfo {
    param([object]$infoObj)
    $list = [System.Collections.Generic.List[object]]::new()
    if ($null -eq $infoObj) { return $list }

    if ($infoObj.releases -and $infoObj.releases.Count -gt 0) {
        foreach ($r in $infoObj.releases) {
            $list.Add($r)
        }
    } elseif ($infoObj.version) {
        $oldTargets = @()
        if ($infoObj.targets) {
            foreach ($ot in $infoObj.targets) {
                $oldTargets += [ordered]@{
                    os           = [string]$ot.os
                    arch         = [string]$ot.arch
                    file         = [string]$ot.file
                    sha256       = [string]$ot.sha256
                    size         = [int64]$ot.size
                    uncompressed_size = [int64]$ot.uncompressed_size
                    format       = [string]$ot.format
                    executable   = [string]$ot.executable
                    download_url = [string]$ot.download_url
                }
            }
        }
        $list.Add([ordered]@{
            version      = [string]$infoObj.version
            commit       = [string]$infoObj.commit
            previous_commit = [string]$infoObj.previous_commit
            channel      = [string]$infoObj.channel
            development  = [bool]$infoObj.development
            published_at = [string]$infoObj.published_at
            changelog    = [string]$infoObj.changelog
            targets      = $oldTargets
        })
    }
    return $list
}

$existingReleases = Extract-ReleasesFromInfo -infoObj $remoteInfo

$seenVersions = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
$seenVersions.Add($Version) | Out-Null

$updatedReleases = [System.Collections.Generic.List[object]]::new()
$updatedReleases.Add($currentRelease)

foreach ($r in $existingReleases) {
    $v = [string]$r.version
    if (-not [string]::IsNullOrWhiteSpace($v) -and $seenVersions.Add($v)) {
        $updatedReleases.Add($r)
    }
}

if ($Channel -eq 'nightly') {
    . (Join-Path $PSScriptRoot 'nightly-info.ps1')
    $updatedReleases = @(Get-NightlyReleases -CurrentRelease $currentRelease -ExistingReleases @($existingReleases))
} else {
    for ($i = 0; $i -lt $updatedReleases.Count; $i++) {
        $rel = $updatedReleases[$i]
        $ver = [string]$rel.version
        $tag = if ($ver.StartsWith('v')) { $ver } else { "v$ver" }
        $relTargets = [System.Collections.Generic.List[object]]::new()
        if ($rel.targets) {
            foreach ($t in $rel.targets) {
                $file = [string]$t.file
                $dl = if ($i -le 1) {
                    "$BaseUrl/$channelRoot/$ver/$file"
                } else {
                    "https://github.com/404Setup/SRC-RenoP/releases/download/$tag/$file"
                }
                $relTargets.Add([ordered]@{
                    os           = [string]$t.os
                    arch         = [string]$t.arch
                    file         = $file
                    sha256       = [string]$t.sha256
                    size         = [int64]$t.size
                    uncompressed_size = [int64]$t.uncompressed_size
                    format       = [string]$t.format
                    executable   = [string]$t.executable
                    download_url = $dl
                })
            }
        }
        if ($relTargets.Count -gt 0) {
            $rel.targets = $relTargets
        } else {
            $updatedReleases[$i] = [ordered]@{
                version      = [string]$rel.version
                commit       = [string]$rel.commit
                previous_commit = [string]$rel.previous_commit
                channel      = [string]$rel.channel
                development  = [bool]$rel.development
                published_at = [string]$rel.published_at
                changelog    = [string]$rel.changelog
            }
        }
    }
}

$retention = if ($Channel -eq 'nightly') { $nightlyPackageRetention } else { 2 }
$allowedMvncVersions = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
$allowedMvncVersions.Add($Version) | Out-Null
foreach ($release in ($updatedReleases | Select-Object -First $retention)) {
    $allowedMvncVersions.Add([string]$release.version) | Out-Null
}
$candidatesToDelete = @(
    $updatedReleases | Select-Object -Skip $retention -First 100 | ForEach-Object { [string]$_.version } |
        Where-Object { $_ -match '^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$' -and -not $allowedMvncVersions.Contains($_) } |
        Select-Object -Unique
)

$targets | ForEach-Object -Parallel {
    $t = $_
    $dest = "$using:BaseUrl/$using:channelRoot/$using:Version/$($t.file)"
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    Write-Host "PUT $($t.file) -> $dest ($($t.size) bytes)"
    $fs = [System.IO.File]::OpenRead($t.path)
    try {
        $client = $using:httpClient
        $content = [System.Net.Http.StreamContent]::new($fs, 131072)
        $content.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse('application/x-brotli')

        $req = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Put, $dest)
        $req.Content = $content

        $resp = $client.SendAsync($req).GetAwaiter().GetResult()
        $code = [int]$resp.StatusCode
        if ($code -ne 200 -and $code -ne 201 -and $code -ne 204) {
            throw "PUT $dest returned unexpected status $code ($($resp.ReasonPhrase))"
        }
        $sw.Stop()
        $elapsedSec = [Math]::Max($sw.Elapsed.TotalSeconds, 0.001)
        $speedMBs = [Math]::Round(($t.size / 1048576) / $elapsedSec, 2)
        Write-Host "Uploaded $($t.file) in $($sw.ElapsedMilliseconds)ms ($speedMBs MB/s)"
    } finally {
        $fs.Dispose()
    }
} -ThrottleLimit 8

$info = [ordered]@{
    releases = $updatedReleases
}
$infoJson = $info | ConvertTo-Json -Depth 8
$infoLocal = Join-Path $DistDir 'info.json'
[System.IO.File]::WriteAllText($infoLocal, $infoJson, [System.Text.UTF8Encoding]::new($false))

Write-Host "PUT info.json -> $infoUrl"
$infoBytes = [System.Text.Encoding]::UTF8.GetBytes($infoJson)
$infoContent = [System.Net.Http.ByteArrayContent]::new($infoBytes)
$infoContent.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse('application/json')
$infoReq = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Put, $infoUrl)
$infoReq.Content = $infoContent

$infoResp = $httpClient.SendAsync($infoReq).GetAwaiter().GetResult()
$infoCode = [int]$infoResp.StatusCode
if ($infoCode -ne 200 -and $infoCode -ne 201 -and $infoCode -ne 204) {
    throw "PUT $infoUrl returned unexpected status $infoCode ($($infoResp.ReasonPhrase))"
}

Write-Host "Published $Channel $Version ($($targets.Count) targets) to $BaseUrl/$channelRoot/"

# Retire obsolete trees only after the new packages and metadata are durable.
$deleted = 0
foreach ($oldVersion in $candidatesToDelete) {
    $dirUrl = "$BaseUrl/$channelRoot/$oldVersion"
    $request = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Delete, $dirUrl)
    $response = $null
    try {
        $response = $httpClient.SendAsync($request).GetAwaiter().GetResult()
        $code = [int]$response.StatusCode
        if ($code -eq 404) { continue }
        if ($code -notin @(200, 204)) { throw "DELETE $dirUrl returned HTTP $code" }
        Write-Host "Deleted obsolete package tree: $dirUrl"
        $deleted++
        if ($deleted -eq 5) { break }
    } finally {
        if ($null -ne $response) { $response.Dispose() }
        $request.Dispose()
    }
}
Write-Host "Cleaned $deleted obsolete package tree(s)."
$httpClient.Dispose()
