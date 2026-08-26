<#
.SYNOPSIS
    Brotli-compresses one compiled RenoP target for the parent build scheduler.

.PARAMETER SpecPath
    Path to the JSON job specification written by build.ps1.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$SpecPath
)

$ErrorActionPreference = 'Stop'

try {
    $resolvedSpec = (Resolve-Path -LiteralPath $SpecPath).Path
    $spec = Get-Content -LiteralPath $resolvedSpec -Raw | ConvertFrom-Json
    foreach ($required in @('index', 'goos', 'goarch', 'binary_path', 'binary_name',
            'archive_path', 'brotli_tool', 'result_path')) {
        if ($null -eq $spec.$required -or [string]::IsNullOrWhiteSpace([string]$spec.$required)) {
            throw "Compression job specification is missing '$required'."
        }
    }

    $binaryPath = [string]$spec.binary_path
    $archivePath = [string]$spec.archive_path
    $brotliTool = [string]$spec.brotli_tool
    if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
        throw "Compiled binary not found at $binaryPath"
    }
    if (-not (Test-Path -LiteralPath $brotliTool -PathType Leaf)) {
        throw "RenoP Brotli packaging CLI not found at $brotliTool"
    }

    $archiveDirectory = Split-Path -Parent $archivePath
    if (-not (Test-Path -LiteralPath $archiveDirectory -PathType Container)) {
        New-Item -ItemType Directory -Path $archiveDirectory -Force | Out-Null
    }
    Write-Host "Compressing $($spec.goos)/$($spec.goarch)"
    & $brotliTool -input $binaryPath -output $archivePath -quality 11
    if ($LASTEXITCODE -ne 0) {
        throw "renop-brotli failed for $($spec.goos)/$($spec.goarch) with exit code $LASTEXITCODE."
    }
    if (-not (Test-Path -LiteralPath $archivePath -PathType Leaf)) {
        throw "renop-brotli did not produce $archivePath"
    }

    $result = [ordered]@{
        index = [int]$spec.index
        os = [string]$spec.goos
        arch = [string]$spec.goarch
        binary = $binaryPath
        file = Split-Path -Leaf $archivePath
        sha256 = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
        size = (Get-Item -LiteralPath $archivePath).Length
        uncompressed_size = (Get-Item -LiteralPath $binaryPath).Length
        format = 'brotli'
        executable = [string]$spec.binary_name
    }

    $resultPath = [string]$spec.result_path
    $resultTempPath = $resultPath + '.tmp'
    $result | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $resultTempPath -Encoding utf8
    Move-Item -LiteralPath $resultTempPath -Destination $resultPath -Force
    exit 0
} catch {
    Write-Error $_
    exit 1
}
