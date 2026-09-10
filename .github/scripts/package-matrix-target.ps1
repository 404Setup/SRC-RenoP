<#
.SYNOPSIS
    Package one matrix executable using the shared Brotli worker.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Target,
    [Parameter(Mandatory)][string]$Version,
    [Parameter(Mandatory)][string]$BinaryDir,
    [Parameter(Mandatory)][string]$DistDir,
    [Parameter(Mandatory)][string]$BrotliTool
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$targets = (Import-PowerShellDataFile (Join-Path $repositoryRoot 'scripts/build-targets.psd1')).Targets
$selected = @($targets | Where-Object { "$($_.GOOS)/$($_.GOARCH)" -eq $Target })
if ($selected.Count -ne 1) { throw "Unknown release target: $Target" }
$goos, $goarch = $selected[0].GOOS, $selected[0].GOARCH
$binaryName = if ($goos -eq 'windows') { 'renop.exe' } else { 'renop' }
$binary = (Resolve-Path -LiteralPath (Join-Path $BinaryDir $binaryName)).Path
$tool = (Resolve-Path -LiteralPath $BrotliTool).Path
$displayVersion = if ($Version -match '^(?i:[0-9a-f]{40}|[0-9a-f]{64})$') { $Version.Substring(0, 7) } else { $Version }
$safeVersion = $displayVersion -replace '[^A-Za-z0-9._-]', '_'
New-Item -ItemType Directory -Path $DistDir -Force | Out-Null
$dist = (Resolve-Path -LiteralPath $DistDir).Path
$specPath = [IO.Path]::GetTempFileName()
try {
    [ordered]@{
        index = 0
        goos = $goos
        goarch = $goarch
        binary_path = $binary
        binary_name = $binaryName
        archive_path = Join-Path $dist "renop-$safeVersion-$goos-$goarch.br"
        brotli_tool = $tool
        result_path = Join-Path $dist "$goos-$goarch.json"
    } | ConvertTo-Json | Set-Content -LiteralPath $specPath -Encoding utf8
    & pwsh -NoProfile -File (Join-Path $repositoryRoot 'scripts/compress-target.ps1') -SpecPath $specPath
    if ($LASTEXITCODE -ne 0) { throw "Packaging failed for $Target" }
} finally {
    Remove-Item -LiteralPath $specPath -Force
}
