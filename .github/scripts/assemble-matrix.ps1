<#
.SYNOPSIS
    Validate the complete matrix and assemble the publishable release payload.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$PackageDir,
    [Parameter(Mandatory)][string]$DistDir,
    [Parameter(Mandatory)][string]$Version,
    [Parameter(Mandatory)][ValidateSet('true', 'false')][string]$Development,
    [Parameter(Mandatory)][ValidatePattern('^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$')][string]$Commit,
    [string]$PreviousCommit = ''
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$expected = (Import-PowerShellDataFile (Join-Path $repositoryRoot 'scripts/build-targets.psd1')).Targets
$source = (Resolve-Path -LiteralPath $PackageDir).Path
$displayVersion = if ($Version -match '^(?i:[0-9a-f]{40}|[0-9a-f]{64})$') { $Version.Substring(0, 7) } else { $Version }
$safeVersion = $displayVersion -replace '[^A-Za-z0-9._-]', '_'
$entries = @(Get-ChildItem -LiteralPath $source)
if ($entries.Count -ne $expected.Count * 2 -or @($entries | Where-Object PSIsContainer).Count -gt 0) {
    throw "Expected exactly $($expected.Count) packages and target descriptors."
}
$targets = foreach ($target in $expected) {
    $goos, $goarch = $target.GOOS, $target.GOARCH
    $result = Get-Content -LiteralPath (Join-Path $source "$goos-$goarch.json") -Raw | ConvertFrom-Json
    $filename = "renop-$safeVersion-$goos-$goarch.br"
    $executable = if ($goos -eq 'windows') { 'renop.exe' } else { 'renop' }
    if ($result.os -ne $goos -or $result.arch -ne $goarch -or $result.file -cne $filename -or
        $result.executable -cne $executable -or $result.format -ne 'brotli' -or
        $result.sha256 -notmatch '^[0-9a-f]{64}$' -or $result.size -le 0 -or $result.uncompressed_size -le 0) {
        throw "Invalid package descriptor for $goos/$goarch"
    }
    $package = Get-Item -LiteralPath (Join-Path $source $filename)
    if ($package.Length -ne $result.size -or (Get-FileHash -LiteralPath $package.FullName -Algorithm SHA256).Hash -ine $result.sha256) {
        throw "Package hash or size mismatch for $goos/$goarch"
    }
    [ordered]@{
        os = $goos
        arch = $goarch
        file = $filename
        sha256 = $result.sha256
        size = [int64]$result.size
        uncompressed_size = [int64]$result.uncompressed_size
        format = 'brotli'
        executable = $executable
    }
}
if (Test-Path -LiteralPath $DistDir) {
    if (@(Get-ChildItem -LiteralPath $DistDir -Force).Count -gt 0) { throw 'Release output directory must be empty.' }
} else {
    New-Item -ItemType Directory -Path $DistDir -Force | Out-Null
}
foreach ($target in $targets) {
    Copy-Item -LiteralPath (Join-Path $source $target.file) -Destination (Join-Path $DistDir $target.file)
}
[ordered]@{
    version = $displayVersion
    commit = $Commit
    previous_commit = $PreviousCommit
    development = ($Development -eq 'true')
    targets = @($targets)
} | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $DistDir 'manifest.json') -Encoding utf8
Write-Host "Assembled $($targets.Count) verified release targets."
