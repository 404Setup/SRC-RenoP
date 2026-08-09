[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$AssetName,
    [Parameter(Mandatory)]
    [uri]$AssetUrl,
    [Parameter(Mandatory)]
    [string]$InstallDirectory,
    [Parameter(Mandatory)]
    [ValidatePattern('^[0-9a-fA-F]{64}$')]
    [string]$Sha256,
    [string]$Token,
    [Parameter(Mandatory)]
    [string]$EnvironmentFile,
    [Parameter(Mandatory)]
    [string]$PathFile,
    [Parameter(Mandatory)]
    [string]$OutputFile
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$goRoot = Join-Path $InstallDirectory 'go'
$goExecutable = Join-Path $goRoot $(if ($IsWindows) { 'bin/go.exe' } else { 'bin/go' })
if (-not (Test-Path -LiteralPath $goExecutable -PathType Leaf)) {
    $installParent = Split-Path -Parent $InstallDirectory
    New-Item -ItemType Directory -Path $installParent -Force | Out-Null
    if (Test-Path -LiteralPath $InstallDirectory) {
        Remove-Item -LiteralPath $InstallDirectory -Recurse -Force
    }
    New-Item -ItemType Directory -Path $InstallDirectory -Force | Out-Null

    $downloadDirectory = Join-Path $env:RUNNER_TEMP "setup-go-$([guid]::NewGuid().ToString('N'))"
    $archivePath = Join-Path $downloadDirectory $AssetName
    New-Item -ItemType Directory -Path $downloadDirectory -Force | Out-Null
    try {
        $headers = @{ 'User-Agent' = 'renop-setup-go-runtime' }
        if (-not [string]::IsNullOrWhiteSpace($Token)) {
            $headers.Authorization = "Bearer $Token"
        }
        Write-Host "Downloading $AssetName."
        Invoke-WebRequest -Uri $AssetUrl -Headers $headers -OutFile $archivePath -MaximumRetryCount 3 -RetryIntervalSec 2

        $actualSha256 = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($actualSha256 -ne $Sha256.ToLowerInvariant()) {
            throw "SHA-256 mismatch for '$AssetName': expected $Sha256, got $actualSha256."
        }

        if ($AssetName.EndsWith('.zip', [StringComparison]::OrdinalIgnoreCase)) {
            Expand-Archive -LiteralPath $archivePath -DestinationPath $InstallDirectory -Force
        } elseif ($AssetName.EndsWith('.tar.gz', [StringComparison]::OrdinalIgnoreCase)) {
            & tar -xzf $archivePath -C $InstallDirectory
            if ($LASTEXITCODE -ne 0) {
                throw "Unable to extract '$AssetName' (tar exit code $LASTEXITCODE)."
            }
        } else {
            throw "Unsupported Go runtime archive: '$AssetName'."
        }
    } finally {
        if (Test-Path -LiteralPath $downloadDirectory) {
            Remove-Item -LiteralPath $downloadDirectory -Recurse -Force
        }
    }
}

if (-not (Test-Path -LiteralPath $goExecutable -PathType Leaf)) {
    throw "The extracted runtime does not contain '$goExecutable'."
}

$env:GOROOT = $goRoot
$env:GOTOOLCHAIN = 'local'
$goVersion = (& $goExecutable version).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($goVersion)) {
    throw "The installed Go executable failed validation with exit code $LASTEXITCODE."
}
$goPath = (& $goExecutable env GOPATH).Trim()
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($goPath)) {
    throw "Unable to determine GOPATH with the installed Go runtime (exit code $LASTEXITCODE)."
}

"GOROOT=$goRoot" >> $EnvironmentFile
"GOTOOLCHAIN=local" >> $EnvironmentFile
(Join-Path $goRoot 'bin') >> $PathFile
(Join-Path $goPath 'bin') >> $PathFile
"goroot=$goRoot" >> $OutputFile

Write-Host "Activated $goVersion from $goRoot."

