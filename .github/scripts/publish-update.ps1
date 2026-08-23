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

    [string]$Changelog = '',

    [string]$ChangelogFile = '',

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
    try {
        $req = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Get, $Url)
        $resp = $httpClient.SendAsync($req).GetAwaiter().GetResult()
        if ($resp.StatusCode -eq [System.Net.HttpStatusCode]::NotFound) {
            return $null
        }
        if (-not $resp.IsSuccessStatusCode) {
            Write-Warning "Failed to read remote info.json from $Url (HTTP $([int]$resp.StatusCode))."
            return $null
        }
        $jsonStr = $resp.Content.ReadAsStringAsync().GetAwaiter().GetResult()
        if ([string]::IsNullOrWhiteSpace($jsonStr)) {
            return $null
        }
        return ($jsonStr | ConvertFrom-Json)
    } catch {
        Write-Warning "Failed to read remote info.json: $($_.Exception.Message)"
        return $null
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
        os           = $os
        arch         = $arch
        file         = $name
        sha256       = $sha
        size         = $size
        download_url = "$BaseUrl/$channelRoot/$Version/$name"
        path         = $zip.FullName
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
            download_url = $t.download_url
        }
    }
)

$currentRelease = [ordered]@{
    version      = $Version
    commit       = $Commit
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
                    download_url = [string]$ot.download_url
                }
            }
        }
        $list.Add([ordered]@{
            version      = [string]$infoObj.version
            commit       = [string]$infoObj.commit
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
    # Retain up to 100 nightly releases in info.json
    if ($updatedReleases.Count -gt 100) {
        $updatedReleases = [System.Collections.Generic.List[object]]($updatedReleases.GetRange(0, 100))
    }
    # Older nightly builds do not provide downloads -> strip targets field completely
    for ($i = 1; $i -lt $updatedReleases.Count; $i++) {
        $rel = $updatedReleases[$i]
        $updatedReleases[$i] = [ordered]@{
            version      = [string]$rel.version
            commit       = [string]$rel.commit
            channel      = [string]$rel.channel
            development  = [bool]$rel.development
            published_at = [string]$rel.published_at
            changelog    = [string]$rel.changelog
        }
    }
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
                channel      = [string]$rel.channel
                development  = [bool]$rel.development
                published_at = [string]$rel.published_at
                changelog    = [string]$rel.changelog
            }
        }
    }
}

$allowedMvncVersions = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
if ($Channel -eq 'nightly') {
    $allowedMvncVersions.Add($Version) | Out-Null
} else {
    $allowedMvncVersions.Add($Version) | Out-Null
    if ($updatedReleases.Count -gt 1) {
        $allowedMvncVersions.Add([string]$updatedReleases[1].version) | Out-Null
    }
}

$candidatesToDelete = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)

foreach ($r in $existingReleases) {
    if ($r.version) { $candidatesToDelete.Add([string]$r.version) | Out-Null }
}
if ($remoteInfo -and $remoteInfo.version) {
    $candidatesToDelete.Add([string]$remoteInfo.version) | Out-Null
}

$freshRemoteInfo = Get-RemoteInfoJson -Url $infoUrl
if ($null -ne $freshRemoteInfo) {
    $freshReleases = Extract-ReleasesFromInfo -infoObj $freshRemoteInfo
    foreach ($fr in $freshReleases) {
        if ($fr.version) { $candidatesToDelete.Add([string]$fr.version) | Out-Null }
    }
    if ($freshRemoteInfo.version) {
        $candidatesToDelete.Add([string]$freshRemoteInfo.version) | Out-Null
    }
}

if ($Channel -eq 'nightly') {
    try {
        $recentShorts = & git log -n 100 --format=%h 2>$null
        foreach ($s in $recentShorts) {
            $trimmed = $s.Trim()
            if (-not [string]::IsNullOrWhiteSpace($trimmed)) {
                $candidatesToDelete.Add($trimmed) | Out-Null
            }
        }
        $recent7 = & git log -n 100 --abbrev=7 --format=%h 2>$null
        foreach ($s in $recent7) {
            $trimmed = $s.Trim()
            if (-not [string]::IsNullOrWhiteSpace($trimmed)) {
                $candidatesToDelete.Add($trimmed) | Out-Null
            }
        }
        $recentFulls = & git log -n 100 --format=%H 2>$null
        foreach ($f in $recentFulls) {
            $trimmed = $f.Trim()
            if (-not [string]::IsNullOrWhiteSpace($trimmed)) {
                $candidatesToDelete.Add($trimmed) | Out-Null
            }
        }
    } catch {
        Write-Warning "Could not inspect git commit log for old nightly versions: $($_.Exception.Message)"
    }
} else {
    try {
        $gitTags = & git tag -l 2>$null
        foreach ($t in $gitTags) {
            $trimmed = $t.Trim()
            if (-not [string]::IsNullOrWhiteSpace($trimmed)) {
                $candidatesToDelete.Add($trimmed) | Out-Null
            }
        }
    } catch {
        Write-Warning "Could not inspect git tags: $($_.Exception.Message)"
    }
}

$allDeletes = [System.Collections.Generic.List[string]]::new()
foreach ($c in $candidatesToDelete) {
    if (-not $allowedMvncVersions.Contains($c) -and $c -ne $Version -and -not [string]::IsNullOrWhiteSpace($c)) {
        $allDeletes.Add($c)
    }
}
if (-not [string]::IsNullOrWhiteSpace($Version) -and -not $allDeletes.Contains($Version)) {
    $allDeletes.Add($Version)
}

if ($allDeletes.Count -gt 0) {
    $allDeletes | ForEach-Object -Parallel {
        $oldVer = $_
        if ([string]::IsNullOrWhiteSpace($oldVer)) { return }
        $dirUrl = "$using:BaseUrl/$using:channelRoot/$oldVer"
        Write-Host "Deleting previous package tree from mvnc: $dirUrl"
        try {
            $client = $using:httpClient
            $req = [System.Net.Http.HttpRequestMessage]::new([System.Net.Http.HttpMethod]::Delete, $dirUrl)
            $resp = $client.SendAsync($req).GetAwaiter().GetResult()
            $code = [int]$resp.StatusCode
            if ($code -ne 200 -and $code -ne 204 -and $code -ne 404) {
                Write-Warning "DELETE $dirUrl returned unexpected status $code ($($resp.ReasonPhrase))"
            }
        } catch {
            Write-Warning "DELETE $dirUrl failed: $($_.Exception.Message)"
        }
    } -ThrottleLimit 16
}

$targets | ForEach-Object -Parallel {
    $t = $_
    $dest = "$using:BaseUrl/$using:channelRoot/$using:Version/$($t.file)"
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    Write-Host "PUT $($t.file) -> $dest ($($t.size) bytes)"
    $fs = [System.IO.File]::OpenRead($t.path)
    try {
        $client = $using:httpClient
        $content = [System.Net.Http.StreamContent]::new($fs, 131072)
        $content.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::Parse('application/zip')

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
