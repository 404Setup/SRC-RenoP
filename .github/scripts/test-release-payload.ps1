<#
.SYNOPSIS
    Validate that a RenoP update artifact contains only executable packages and its manifest.

.PARAMETER DistDir
    Build output directory downloaded by the update-publishing job.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$DistDir
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if (-not (Test-Path -LiteralPath $DistDir -PathType Container)) {
    throw "Release payload directory not found: $DistDir"
}

$files = @(Get-ChildItem -LiteralPath $DistDir -File)
$packages = @($files | Where-Object { $_.Extension -eq '.br' } | Sort-Object Name)
$manifestPath = Join-Path $DistDir 'manifest.json'
$unexpected = @($files | Where-Object { $_.Name -ne 'manifest.json' -and $_.Extension -ne '.br' })

if ($packages.Count -eq 0) {
    throw "Release payload contains no Brotli packages: $DistDir"
}
if (-not (Test-Path -LiteralPath $manifestPath -PathType Leaf)) {
    throw "Release payload manifest not found: $manifestPath"
}
if ($unexpected.Count -gt 0) {
    throw "Release payload contains files that must not be sent to the update API: $($unexpected.Name -join ', ')"
}

$manifest = Get-Content -LiteralPath $manifestPath -Raw -Encoding utf8 | ConvertFrom-Json
$manifestFiles = @($manifest.targets | ForEach-Object { [string]$_.file } | Sort-Object -Unique)
$packageNames = @($packages.Name | Sort-Object -Unique)
if ($manifestFiles.Count -ne $packageNames.Count) {
    throw "Manifest target count $($manifestFiles.Count) does not match package count $($packageNames.Count)"
}
for ($index = 0; $index -lt $packageNames.Count; $index++) {
    if ($packageNames[$index] -ne $manifestFiles[$index]) {
        throw "Manifest target '$($manifestFiles[$index])' does not match package '$($packageNames[$index])'"
    }
}

Write-Host "Validated update payload: $($packages.Count) Brotli package(s) and manifest.json"
