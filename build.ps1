<#
.SYNOPSIS
    Cross-build and package RenoP binaries.

.PARAMETER Mode
    Full matrix (default), s for mainstream targets, or c for the current
    Go platform. The positional forms `./build.ps1 s` and `./build.ps1 c` are
    supported as well as -s and -c. Add the nb suffix (for example
    `./build.ps1 c nb`) to write binaries directly to the invocation directory
    without creating ZIP packages.

.PARAMETER Version
    Version embedded into the binary with -ldflags. If omitted, the full
    current commit hash is used for a development build.

.PARAMETER Development
    Whether the binary is a development build. The value is a string so CI
    can pass either true or false explicitly.
#>
[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('full', 's', 'c', 'nb')]
    [string]$Mode = 'full',
    [Parameter(Position = 1)]
    [ValidateSet('nb')]
    [string]$Suffix,
    [switch]$s,
    [switch]$c,
    [switch]$nb,
    [string]$Version,
    [string]$Development
)

$ErrorActionPreference = 'Stop'
$invocationDirectory = (Get-Location).Path
$repositoryRoot = (Resolve-Path $PSScriptRoot).Path
Set-Location $repositoryRoot

$noBundle = $nb -or $Suffix -eq 'nb' -or $Mode -eq 'nb'
if ($Mode -eq 'nb') { $Mode = 'full' }
if ($s -and $c) {
    throw 'Choose only one of -s or -c.'
}
if ($s) { $Mode = 's' }
if ($c) { $Mode = 'c' }

$gitVersion = $null
try {
    $gitVersion = (& git rev-parse --short HEAD 2>$null).Trim()
} catch {
    $gitVersion = 'unknown'
}
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = if ([string]::IsNullOrWhiteSpace($gitVersion)) { 'unknown' } else { $gitVersion }
    if ([string]::IsNullOrWhiteSpace($Development)) { $Development = 'true' }
} elseif ([string]::IsNullOrWhiteSpace($Development)) {
    $Development = 'false'
}

$developmentValue = if ($Development -match '^(?i:true|1|yes)$') { 'true' } else { 'false' }
$displayVersion = if ($Version -match '^(?i:[0-9a-f]{40}|[0-9a-f]{64})$') {
    $Version.Substring(0, 7)
} else {
    $Version
}
$safeVersion = $displayVersion -replace '[^A-Za-z0-9._-]', '_'

$allTargets = @(
    @{ GOOS = 'darwin'; GOARCH = 'amd64' },
    @{ GOOS = 'darwin'; GOARCH = 'amd64v2' },
    @{ GOOS = 'darwin'; GOARCH = 'amd64v3' },
    @{ GOOS = 'darwin'; GOARCH = 'amd64v4' },
    @{ GOOS = 'darwin'; GOARCH = 'arm64' },
    @{ GOOS = 'freebsd'; GOARCH = 'amd64' },
    @{ GOOS = 'freebsd'; GOARCH = 'amd64v2' },
    @{ GOOS = 'freebsd'; GOARCH = 'amd64v3' },
    @{ GOOS = 'freebsd'; GOARCH = 'amd64v4' },
    @{ GOOS = 'freebsd'; GOARCH = 'arm64' },
    @{ GOOS = 'linux'; GOARCH = 'amd64' },
    @{ GOOS = 'linux'; GOARCH = 'amd64v2' },
    @{ GOOS = 'linux'; GOARCH = 'amd64v3' },
    @{ GOOS = 'linux'; GOARCH = 'amd64v4' },
    @{ GOOS = 'linux'; GOARCH = 'arm64' },
    @{ GOOS = 'linux'; GOARCH = 'loong64' },
    @{ GOOS = 'linux'; GOARCH = 'riscv64' },
    @{ GOOS = 'netbsd'; GOARCH = 'amd64' },
    @{ GOOS = 'netbsd'; GOARCH = 'amd64v2' },
    @{ GOOS = 'netbsd'; GOARCH = 'amd64v3' },
    @{ GOOS = 'netbsd'; GOARCH = 'amd64v4' },
    @{ GOOS = 'openbsd'; GOARCH = 'amd64' },
    @{ GOOS = 'openbsd'; GOARCH = 'amd64v2' },
    @{ GOOS = 'openbsd'; GOARCH = 'amd64v3' },
    @{ GOOS = 'openbsd'; GOARCH = 'amd64v4' },
    @{ GOOS = 'openbsd'; GOARCH = 'arm64' },
    @{ GOOS = 'windows'; GOARCH = 'amd64' },
    @{ GOOS = 'windows'; GOARCH = 'amd64v2' },
    @{ GOOS = 'windows'; GOARCH = 'amd64v3' },
    @{ GOOS = 'windows'; GOARCH = 'amd64v4' },
    @{ GOOS = 'windows'; GOARCH = 'arm64' }
)

switch ($Mode) {
    's' {
        $targets = @(
            @{ GOOS = 'linux'; GOARCH = 'amd64' },
            @{ GOOS = 'linux'; GOARCH = 'amd64v4' },
            @{ GOOS = 'linux'; GOARCH = 'arm64' },
            @{ GOOS = 'windows'; GOARCH = 'amd64' },
            @{ GOOS = 'windows'; GOARCH = 'amd64v4' },
            @{ GOOS = 'windows'; GOARCH = 'arm64' }
        )
    }
    'c' {
        $currentGoos = (& go env GOHOSTOS).Trim()
        $currentGoarch = (& go env GOHOSTARCH).Trim()
        $targets = @(@{ GOOS = $currentGoos; GOARCH = $currentGoarch })
    }
    default { $targets = $allTargets }
}

$availablePlatforms = @(& go tool dist list | ForEach-Object { $_.Trim() })
$unsupportedTargets = @($targets | Where-Object {
    $baseArch = if ($_.GOARCH -match '^(amd64)(v[1-4])?$') { 'amd64' } else { $_.GOARCH }
    $platform = "$($_.GOOS)/$baseArch"
    $availablePlatforms -notcontains $platform
} | ForEach-Object { "$($_.GOOS)/$($_.GOARCH)" })
if ($unsupportedTargets.Count -gt 0) {
    throw "The installed Go toolchain does not support: $($unsupportedTargets -join ', '). No substitute binaries will be published."
}

$dist = Join-Path $repositoryRoot 'dist'
if (-not $noBundle) {
    if (Test-Path -LiteralPath $dist) {
        Remove-Item -LiteralPath $dist -Recurse -Force
    }
    New-Item -ItemType Directory -Path $dist -Force | Out-Null
}

$hadCgo = Test-Path Env:CGO_ENABLED
$originalCgo = $env:CGO_ENABLED
$hadGoos = Test-Path Env:GOOS
$originalGoos = $env:GOOS
$hadGoarch = Test-Path Env:GOARCH
$originalGoarch = $env:GOARCH
$hadGoamd64 = Test-Path Env:GOAMD64
$originalGoamd64 = $env:GOAMD64

function Invoke-ProtobufGenerate {
    Write-Host 'Generating protobuf (Go)...'
    $protoc = Get-Command protoc -ErrorAction SilentlyContinue
    if (-not $protoc) {
        throw 'protoc not found on PATH. Install Protocol Buffers compiler to build.'
    }
    $protocGenGo = Get-Command protoc-gen-go -ErrorAction SilentlyContinue
    if (-not $protocGenGo) {
        Write-Host 'Installing protoc-gen-go...'
        & go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
        if ($LASTEXITCODE -ne 0) {
            throw "go install protoc-gen-go failed with exit code $LASTEXITCODE."
        }
        $goBin = (& go env GOPATH).Trim()
        if ($goBin) {
            $env:PATH = (Join-Path $goBin 'bin') + [IO.Path]::PathSeparator + $env:PATH
        }
    }
    $protoFile = Join-Path $repositoryRoot 'proto/api/v1/api.proto'
    if (-not (Test-Path -LiteralPath $protoFile)) {
        throw "Proto schema not found at $protoFile"
    }
    & protoc -I (Join-Path $repositoryRoot 'proto') --go_out=$repositoryRoot --go_opt=module=renop $protoFile
    if ($LASTEXITCODE -ne 0) {
        throw "protoc (Go) failed with exit code $LASTEXITCODE."
    }
    $generated = Join-Path $repositoryRoot 'pkg/pb/api.pb.go'
    if (-not (Test-Path -LiteralPath $generated)) {
        throw "protoc did not produce $generated"
    }
}

function Build-FrontendAssets {
    $frontendDir = Join-Path $repositoryRoot 'internal/service/frontend/renop-html'
    if (-not (Test-Path -LiteralPath (Join-Path $frontendDir 'package.json'))) {
        throw "Frontend package.json not found at $frontendDir"
    }

    Write-Host 'Building frontend assets (protobuf + Rolldown JS + CSS)...'
    if (-not (Test-Path -LiteralPath (Join-Path $frontendDir 'node_modules'))) {
        Push-Location $repositoryRoot
        try {
            if (Test-Path -LiteralPath (Join-Path $repositoryRoot 'pnpm-lock.yaml')) {
                & pnpm install --frozen-lockfile
            } else {
                & pnpm install
            }
            if ($LASTEXITCODE -ne 0) {
                throw "pnpm install failed with exit code $LASTEXITCODE."
            }
        } finally {
            Pop-Location
        }
    }

    Push-Location $frontendDir
    try {
        & pnpm run build
        if ($LASTEXITCODE -ne 0) {
            throw "frontend pnpm run build failed with exit code $LASTEXITCODE."
        }
    } finally {
        Pop-Location
    }
}

try {
    Invoke-ProtobufGenerate
    Build-FrontendAssets

    $env:CGO_ENABLED = '0'

    $manifestTargets = [System.Collections.Generic.List[object]]::new()
    foreach ($target in $targets) {
        $goos = $target.GOOS
        $goarch = $target.GOARCH
        $binaryExtension = if ($goos -eq 'windows') { '.exe' } else { '' }
        if ($noBundle) {
            $binaryName = if ($targets.Count -eq 1) {
                "renop$binaryExtension"
            } else {
                "renop-$goos-$goarch$binaryExtension"
            }
            $stage = $null
            $binaryPath = Join-Path $invocationDirectory $binaryName
            $archivePath = $null
        } else {
            $name = "renop-$safeVersion-$goos-$goarch"
            $stage = Join-Path $dist ".stage-$goos-$goarch"
            $binaryName = "renop$binaryExtension"
            $binaryPath = Join-Path $stage $binaryName
            $archivePath = Join-Path $dist "$name.zip"
            New-Item -ItemType Directory -Path $stage -Force | Out-Null
        }

        $actualGoarch = $goarch
        $goamd64 = $null
        if ($goarch -match '^(amd64)(v[1-4])?$') {
            $actualGoarch = 'amd64'
            $goamd64 = if ($Matches[2]) { $Matches[2] } else { 'v1' }
        }

        $env:GOOS = $goos
        $env:GOARCH = $actualGoarch
        if ($goamd64) {
            $env:GOAMD64 = $goamd64
        } else {
            Remove-Item Env:GOAMD64 -ErrorAction SilentlyContinue
        }

        $ldflags = "-s -w -X=renop/internal/version.Version=$displayVersion -X=renop/internal/version.Development=$developmentValue"
        $destinationDescription = if ($noBundle) { $binaryPath } else { $archivePath }
        Write-Host "Building $goos/$goarch -> $destinationDescription"
        & go build -ldflags $ldflags -o $binaryPath .
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed for $goos/$goarch with exit code $LASTEXITCODE."
        }

        if ($noBundle) {
            continue
        }

        Copy-Item -LiteralPath (Join-Path $repositoryRoot 'LICENSE') -Destination $stage
        Copy-Item -LiteralPath (Join-Path $repositoryRoot 'README.md') -Destination $stage
        Copy-Item -LiteralPath (Join-Path $repositoryRoot 'THIRD_PARTY_NOTICES.md') -Destination $stage
        Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archivePath -CompressionLevel Optimal -Force
        $hash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
        $size = (Get-Item -LiteralPath $archivePath).Length
        $manifestTargets.Add([ordered]@{
            os = $goos
            arch = $goarch
            file = Split-Path -Leaf $archivePath
            sha256 = $hash
            size = $size
        })
        Remove-Item -LiteralPath $stage -Recurse -Force
    }

    if (-not $noBundle) {
        $commitFull = $null
        try {
            $commitFull = (& git rev-parse HEAD 2>$null).Trim()
        } catch {
            $commitFull = ''
        }
        $manifest = [ordered]@{
            version = $displayVersion
            commit = $commitFull
            development = ($developmentValue -eq 'true')
            targets = $manifestTargets
        }
        $manifest | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $dist 'manifest.json') -Encoding utf8
    }
}
finally {
    if ($hadCgo) { $env:CGO_ENABLED = $originalCgo } else { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
    if ($hadGoos) { $env:GOOS = $originalGoos } else { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
    if ($hadGoarch) { $env:GOARCH = $originalGoarch } else { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
    if ($hadGoamd64) { $env:GOAMD64 = $originalGoamd64 } else { Remove-Item Env:GOAMD64 -ErrorAction SilentlyContinue }
}

$finalDirectory = if ($noBundle) { $invocationDirectory } else { $dist }
$packagingDescription = if ($noBundle) { 'without packaging' } else { 'with packages' }
Write-Host "Built $($targets.Count) target(s) into $finalDirectory $packagingDescription"
