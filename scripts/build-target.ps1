<#
.SYNOPSIS
    Builds and optionally Brotli-compresses one RenoP release target.

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
    foreach ($required in @('index', 'repository_root', 'goos', 'goarch', 'actual_goarch',
            'binary_path', 'binary_name', 'ldflags', 'result_path')) {
        if ($null -eq $spec.$required -or [string]::IsNullOrWhiteSpace([string]$spec.$required)) {
            throw "Build job specification is missing '$required'."
        }
    }

    Set-Location ([string]$spec.repository_root)
    $env:CGO_ENABLED = '0'
    $env:GOOS = [string]$spec.goos
    $env:GOARCH = [string]$spec.actual_goarch
    if ([string]::IsNullOrWhiteSpace([string]$spec.goamd64)) {
        Remove-Item Env:GOAMD64 -ErrorAction SilentlyContinue
    } else {
        $env:GOAMD64 = [string]$spec.goamd64
    }

    $binaryPath = [string]$spec.binary_path
    $binaryDirectory = Split-Path -Parent $binaryPath
    if (-not (Test-Path -LiteralPath $binaryDirectory -PathType Container)) {
        New-Item -ItemType Directory -Path $binaryDirectory -Force | Out-Null
    }
    Write-Host "Compiling $($spec.goos)/$($spec.goarch)"
    & go build -ldflags ([string]$spec.ldflags) -o $binaryPath .
    if ($LASTEXITCODE -ne 0) {
        throw "go build failed for $($spec.goos)/$($spec.goarch) with exit code $LASTEXITCODE."
    }
    if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
        throw "go build did not produce $binaryPath"
    }

    $result = [ordered]@{
        index = [int]$spec.index
        os = [string]$spec.goos
        arch = [string]$spec.goarch
        binary = $binaryPath
    }
    if ([bool]$spec.bundled) {
        $archivePath = [string]$spec.archive_path
        $brotliTool = [string]$spec.brotli_tool
        if ([string]::IsNullOrWhiteSpace($archivePath) -or
            [string]::IsNullOrWhiteSpace($brotliTool) -or
            -not (Test-Path -LiteralPath $brotliTool -PathType Leaf)) {
            throw 'Bundled build job is missing its archive path or Brotli tool.'
        }
        Write-Host "Compressing $($spec.goos)/$($spec.goarch)"
        & $brotliTool -input $binaryPath -output $archivePath -quality 11
        if ($LASTEXITCODE -ne 0) {
            throw "renop-brotli failed for $($spec.goos)/$($spec.goarch) with exit code $LASTEXITCODE."
        }
        if (-not (Test-Path -LiteralPath $archivePath -PathType Leaf)) {
            throw "renop-brotli did not produce $archivePath"
        }
        $result.file = Split-Path -Leaf $archivePath
        $result.sha256 = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
        $result.size = (Get-Item -LiteralPath $archivePath).Length
        $result.uncompressed_size = (Get-Item -LiteralPath $binaryPath).Length
        $result.format = 'brotli'
        $result.executable = [string]$spec.binary_name
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
