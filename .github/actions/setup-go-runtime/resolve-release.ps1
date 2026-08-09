[CmdletBinding()]
param(
    [string]$Version,
    [string]$VersionFile = 'go.mod',
    [Parameter(Mandatory)]
    [string]$Workspace,
    [Parameter(Mandatory)]
    [string]$Repository,
    [Parameter(Mandatory)]
    [string]$RunnerOS,
    [Parameter(Mandatory)]
    [string]$RunnerArchitecture,
    [string]$Token,
    [Parameter(Mandatory)]
    [string]$ToolCache,
    [Parameter(Mandatory)]
    [string]$OutputFile
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$normalizedVersion = if ($null -eq $Version) { '' } else { $Version.Trim() }
if ([string]::IsNullOrWhiteSpace($normalizedVersion)) {
    if ([string]::IsNullOrWhiteSpace($VersionFile)) {
        throw 'Either Version or VersionFile must be provided.'
    }
    $versionFilePath = if ([IO.Path]::IsPathRooted($VersionFile)) {
        $VersionFile
    } else {
        Join-Path $Workspace $VersionFile
    }
    if (-not (Test-Path -LiteralPath $versionFilePath -PathType Leaf)) {
        throw "Go version file not found: '$versionFilePath'."
    }

    $goDirective = Get-Content -LiteralPath $versionFilePath | Where-Object {
        $_ -match '^\s*go\s+([0-9]+(?:\.[0-9]+){1,2})\s*(?://.*)?$'
    } | Select-Object -First 1
    if ($null -eq $goDirective) {
        throw "No valid Go version directive was found in '$versionFilePath'."
    }
    $null = $goDirective -match '^\s*go\s+([0-9]+(?:\.[0-9]+){1,2})'
    $normalizedVersion = $Matches[1]
    Write-Host "Read Go version $normalizedVersion from $versionFilePath."
}
if ($normalizedVersion.StartsWith('go', [StringComparison]::OrdinalIgnoreCase)) {
    $normalizedVersion = $normalizedVersion.Substring(2)
}
if ($normalizedVersion -notmatch '^[0-9][0-9A-Za-z._+-]*$') {
    throw "Invalid Go version prefix: '$Version'."
}
if ($Repository -notmatch '^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$') {
    throw "Invalid GitHub repository: '$Repository'."
}
if ([string]::IsNullOrWhiteSpace($ToolCache)) {
    throw 'RUNNER_TOOL_CACHE is not set.'
}

$platform = switch ($RunnerOS.ToLowerInvariant()) {
    'linux' { 'linux' }
    'macos' { 'darwin' }
    'darwin' { 'darwin' }
    'windows' { 'windows' }
    default { throw "Unsupported runner operating system: '$RunnerOS'." }
}
$architecture = switch ($RunnerArchitecture.ToLowerInvariant()) {
    'x64' { 'amd64' }
    'amd64' { 'amd64' }
    'arm64' { 'arm64' }
    'x86' { '386' }
    '386' { '386' }
    default { throw "Unsupported runner architecture: '$RunnerArchitecture'." }
}
$archiveExtension = if ($platform -eq 'windows') { '.zip' } else { '.tar.gz' }
$tagPrefix = "go$normalizedVersion"

$headers = @{
    Accept = 'application/vnd.github+json'
    'X-GitHub-Api-Version' = '2022-11-28'
    'User-Agent' = 'renop-setup-go-runtime'
}
if (-not [string]::IsNullOrWhiteSpace($Token)) {
    $headers.Authorization = "Bearer $Token"
}

$candidates = [System.Collections.Generic.List[object]]::new()
$matchingTags = [System.Collections.Generic.List[string]]::new()
for ($page = 1; $page -le 100; $page++) {
    $uri = "https://api.github.com/repos/$Repository/releases?per_page=100&page=$page"
    try {
        $response = Invoke-RestMethod -Uri $uri -Headers $headers -Method Get
        $releases = @($response)
    } catch {
        throw "Unable to query releases from '$Repository': $($_.Exception.Message)"
    }

    foreach ($release in $releases) {
        $tag = [string]$release.tag_name
        if (-not $tag.StartsWith($tagPrefix, [StringComparison]::OrdinalIgnoreCase)) {
            continue
        }

        # Avoid treating go1.280 as a match for a requested go1.28 prefix.
        $suffix = $tag.Substring($tagPrefix.Length)
        if ($suffix -and $suffix -notmatch '^(?:[._+-]|rc[0-9]|beta[0-9])') {
            continue
        }

        $matchingTags.Add($tag)
        $expectedAssetName = "$tag.$platform-$architecture$archiveExtension"
        $asset = @($release.assets | Where-Object {
            [string]::Equals([string]$_.name, $expectedAssetName, [StringComparison]::OrdinalIgnoreCase)
        }) | Select-Object -First 1
        if ($null -ne $asset) {
            $publishedAt = if ($null -ne $release.published_at) {
                [DateTimeOffset]::Parse([string]$release.published_at, [Globalization.CultureInfo]::InvariantCulture)
            } else {
                [DateTimeOffset]::MinValue
            }
            $createdAt = if ($null -ne $release.created_at) {
                [DateTimeOffset]::Parse([string]$release.created_at, [Globalization.CultureInfo]::InvariantCulture)
            } else {
                [DateTimeOffset]::MinValue
            }
            $candidates.Add([pscustomobject]@{
                Release = $release
                Asset = $asset
                PublishedAt = $publishedAt
                CreatedAt = $createdAt
                Id = [long]$release.id
            })
        }
    }

    if ($releases.Count -lt 100) {
        break
    }
}

if ($candidates.Count -eq 0) {
    $details = if ($matchingTags.Count -gt 0) {
        " Matching tags without a $platform-$architecture asset: $($matchingTags -join ', ')."
    } else {
        ''
    }
    throw "No release asset matching '$tagPrefix' for $platform-$architecture was found in '$Repository'.$details"
}

$resolved = $candidates | Sort-Object -Property @(
    @{ Expression = { $_.PublishedAt }; Descending = $true },
    @{ Expression = { $_.CreatedAt }; Descending = $true },
    @{ Expression = { $_.Id }; Descending = $true }
) | Select-Object -First 1
$resolvedRelease = $resolved.Release
$resolvedAsset = $resolved.Asset

$digest = [string]$resolvedAsset.digest
if ($digest -notmatch '^sha256:([0-9a-fA-F]{64})$') {
    throw "Release asset '$($resolvedAsset.name)' does not provide a valid SHA-256 digest."
}
$sha256 = $Matches[1].ToLowerInvariant()
$tag = [string]$resolvedRelease.tag_name
$installDirectory = Join-Path $ToolCache "404setup-go/$tag/$platform-$architecture"

"tag=$tag" >> $OutputFile
"asset-name=$($resolvedAsset.name)" >> $OutputFile
"asset-url=$($resolvedAsset.browser_download_url)" >> $OutputFile
"sha256=$sha256" >> $OutputFile
"platform=$platform" >> $OutputFile
"architecture=$architecture" >> $OutputFile
"install-directory=$installDirectory" >> $OutputFile
"requested-version=$normalizedVersion" >> $OutputFile

Write-Host "Resolved Go runtime $tag for $platform-$architecture ($($resolvedAsset.name))."
