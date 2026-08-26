<#
.SYNOPSIS
    Cross-build and package RenoP binaries.

.PARAMETER Mode
    Full matrix (default), s for mainstream targets, or c for the current
    Go platform. The positional forms `./build.ps1 s` and `./build.ps1 c` are
    supported as well as -s and -c. Add the nb suffix (for example
    `./build.ps1 c nb`) to write binaries directly to the invocation directory
    without creating raw Brotli packages.

.PARAMETER Version
    Version embedded into the binary with -ldflags. If omitted, the full
    current commit hash is used for a development build.

.PARAMETER Development
    Whether the binary is a development build. The value is a string so CI
    can pass either true or false explicitly.

.PARAMETER Commit
    Full source revision embedded in the binary and release manifest.

.PARAMETER PreviousCommit
    Full source revision of the preceding formal release.

.PARAMETER BuildConcurrency
    Maximum number of target compilation tasks running at once. The upper
    bound is four. Compilation slots are released before packaging begins.

.PARAMETER CompressionConcurrency
    Maximum number of asynchronous Brotli packaging tasks running at once.
    The upper bound is eight.
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
    [string]$Development,
    [string]$Commit,
    [string]$PreviousCommit,
    [ValidateRange(1, 4)]
    [int]$BuildConcurrency = 4,
    [ValidateRange(1, 8)]
    [int]$CompressionConcurrency = 8
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
$commitFull = $Commit.Trim()
if ([string]::IsNullOrWhiteSpace($commitFull)) {
    try { $commitFull = (& git rev-parse HEAD 2>$null).Trim() } catch { $commitFull = '' }
}
$previousCommitFull = $PreviousCommit.Trim()
if ($developmentValue -eq 'false' -and [string]::IsNullOrWhiteSpace($previousCommitFull) -and -not [string]::IsNullOrWhiteSpace($commitFull)) {
    try {
        $previousCommitFull = (& git log -1 --format='%H' -i --grep='^\[release\]' "${commitFull}^" 2>$null).Trim()
    } catch {
        $previousCommitFull = ''
    }
    if ([string]::IsNullOrWhiteSpace($previousCommitFull)) {
        try {
            $previousTag = (& git describe --tags --abbrev=0 "${commitFull}^" 2>$null).Trim()
            if (-not [string]::IsNullOrWhiteSpace($previousTag)) {
                $previousCommitFull = (& git rev-parse "$previousTag^{commit}" 2>$null).Trim()
            }
        } catch {
            $previousCommitFull = ''
        }
    }
}
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

function Install-BrotliPackTool {
    Write-Host 'Installing RenoP Brotli packaging CLI...'
    $toolGoos = $env:GOOS
    $toolGoarch = $env:GOARCH
    $toolGoamd64 = $env:GOAMD64
    try {
        $env:GOOS = (& go env GOHOSTOS).Trim()
        $env:GOARCH = (& go env GOHOSTARCH).Trim()
        Remove-Item Env:GOAMD64 -ErrorAction SilentlyContinue
        & go install ./cmd/renop-brotli
        if ($LASTEXITCODE -ne 0) {
            throw "go install ./cmd/renop-brotli failed with exit code $LASTEXITCODE."
        }
    } finally {
        if ($null -ne $toolGoos) { $env:GOOS = $toolGoos } else { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
        if ($null -ne $toolGoarch) { $env:GOARCH = $toolGoarch } else { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
        if ($null -ne $toolGoamd64) { $env:GOAMD64 = $toolGoamd64 } else { Remove-Item Env:GOAMD64 -ErrorAction SilentlyContinue }
    }
    $goBin = (& go env GOBIN).Trim()
    if ([string]::IsNullOrWhiteSpace($goBin)) {
        $goPath = (& go env GOPATH).Trim()
        if ([string]::IsNullOrWhiteSpace($goPath)) {
            throw 'Could not resolve the Go binary installation directory.'
        }
        $goBin = Join-Path (($goPath -split [IO.Path]::PathSeparator)[0]) 'bin'
    }
    $toolName = if ($IsWindows) { 'renop-brotli.exe' } else { 'renop-brotli' }
    $toolPath = Join-Path $goBin $toolName
    if (-not (Test-Path -LiteralPath $toolPath -PathType Leaf)) {
        throw "Installed Brotli packaging CLI not found at $toolPath"
    }
    if (($env:PATH -split [IO.Path]::PathSeparator) -notcontains $goBin) {
        $env:PATH = $goBin + [IO.Path]::PathSeparator + $env:PATH
    }
    return $toolPath
}

function ConvertTo-ProcessArgument {
    param([Parameter(Mandatory = $true)][string]$Value)
    return '"' + $Value.Replace('"', '\"') + '"'
}

function Start-TargetWorker {
    param(
        [Parameter(Mandatory = $true)]$Job,
        [Parameter(Mandatory = $true)][string]$WorkerScript,
        [Parameter(Mandatory = $true)][ValidateSet('compile', 'compress')][string]$Phase
    )
    $stdoutPath = if ($Phase -eq 'compile') { $Job.CompileStdoutPath } else { $Job.CompressStdoutPath }
    $stderrPath = if ($Phase -eq 'compile') { $Job.CompileStderrPath } else { $Job.CompressStderrPath }
    $pwshCommand = Get-Command pwsh -ErrorAction Stop
    $startParameters = @{
        FilePath = $pwshCommand.Source
        ArgumentList = @(
            '-NoLogo',
            '-NoProfile',
            '-NonInteractive',
            '-File',
            (ConvertTo-ProcessArgument $WorkerScript),
            '-SpecPath',
            (ConvertTo-ProcessArgument $Job.SpecPath)
        )
        WorkingDirectory = $repositoryRoot
        RedirectStandardOutput = $stdoutPath
        RedirectStandardError = $stderrPath
        PassThru = $true
    }
    if ($IsWindows) {
        $startParameters.WindowStyle = 'Hidden'
    }
    $process = Start-Process @startParameters
    return [pscustomobject]@{
        Job = $Job
        Process = $process
        Phase = $Phase
        StdoutPath = $stdoutPath
        StderrPath = $stderrPath
    }
}

function Write-TargetWorkerLogs {
    param([Parameter(Mandatory = $true)]$Worker)
    if (Test-Path -LiteralPath $Worker.StdoutPath -PathType Leaf) {
        Get-Content -LiteralPath $Worker.StdoutPath | ForEach-Object {
            if (-not [string]::IsNullOrWhiteSpace($_)) {
                Write-Host "[$($Worker.Job.Label)/$($Worker.Phase)] $_"
            }
        }
    }
    if (Test-Path -LiteralPath $Worker.StderrPath -PathType Leaf) {
        Get-Content -LiteralPath $Worker.StderrPath | ForEach-Object {
            if (-not [string]::IsNullOrWhiteSpace($_)) {
                Write-Warning "[$($Worker.Job.Label)/$($Worker.Phase)] $_"
            }
        }
    }
}

function Remove-BuildWorkspace {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path) -or -not (Test-Path -LiteralPath $Path)) {
        return
    }
    $resolvedPath = [IO.Path]::GetFullPath($Path).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd([IO.Path]::DirectorySeparatorChar)
    $expectedPrefix = $tempRoot + [IO.Path]::DirectorySeparatorChar
    $leaf = Split-Path -Leaf $resolvedPath
    if (-not $resolvedPath.StartsWith($expectedPrefix, [StringComparison]::OrdinalIgnoreCase) -or
        -not $leaf.StartsWith('renop-build-', [StringComparison]::Ordinal)) {
        throw "Refusing to remove unexpected build workspace: $resolvedPath"
    }
    Remove-Item -LiteralPath $resolvedPath -Recurse -Force
}

$buildWorkspace = $null
$activeCompileWorkers = [System.Collections.Generic.List[object]]::new()
$activeCompressionWorkers = [System.Collections.Generic.List[object]]::new()
try {
    Invoke-ProtobufGenerate
    Build-FrontendAssets

    $env:CGO_ENABLED = '0'

    $brotliPackTool = if ($noBundle) { $null } else { Install-BrotliPackTool }

    $buildWorkspace = Join-Path ([IO.Path]::GetTempPath()) ("renop-build-$PID-" + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $buildWorkspace -Force | Out-Null
    $workerScript = Join-Path $repositoryRoot 'scripts/build-target.ps1'
    if (-not (Test-Path -LiteralPath $workerScript -PathType Leaf)) {
        throw "Build target worker not found at $workerScript"
    }
    $compressionWorkerScript = Join-Path $repositoryRoot 'scripts/compress-target.ps1'
    if (-not $noBundle -and -not (Test-Path -LiteralPath $compressionWorkerScript -PathType Leaf)) {
        throw "Compression target worker not found at $compressionWorkerScript"
    }

    $jobs = [System.Collections.Generic.List[object]]::new()
    for ($targetIndex = 0; $targetIndex -lt $targets.Count; $targetIndex++) {
        $target = $targets[$targetIndex]
        $goos = $target.GOOS
        $goarch = $target.GOARCH
        $binaryExtension = if ($goos -eq 'windows') { '.exe' } else { '' }
        if ($noBundle) {
            $binaryName = if ($targets.Count -eq 1) {
                "renop$binaryExtension"
            } else {
                "renop-$goos-$goarch$binaryExtension"
            }
            $binaryPath = Join-Path $invocationDirectory $binaryName
            $archivePath = $null
        } else {
            $name = "renop-$safeVersion-$goos-$goarch"
            $stage = Join-Path $buildWorkspace "stage-$targetIndex"
            $binaryName = "renop$binaryExtension"
            $binaryPath = Join-Path $stage $binaryName
            $archivePath = Join-Path $dist "$name.br"
        }

        $actualGoarch = $goarch
        $goamd64 = $null
        if ($goarch -match '^(amd64)(v[1-4])?$') {
            $actualGoarch = 'amd64'
            $goamd64 = if ($Matches[2]) { $Matches[2] } else { 'v1' }
        }

        $ldflags = "-s -w -X=renop/internal/version.Version=$displayVersion -X=renop/internal/version.Development=$developmentValue -X=renop/internal/version.Commit=$commitFull -X=renop/internal/version.PreviousCommit=$previousCommitFull"
        if ($goos -eq 'linux') {
            # Apply before runtime initialization so even the first Go heap mapping
            # avoids transparent huge pages. A process-level GODEBUG still overrides it.
            $ldflags += ' -X=runtime.godebugDefault=disablethp=1'
        }
        $destinationDescription = if ($noBundle) { $binaryPath } else { $archivePath }
        $specPath = Join-Path $buildWorkspace "job-$targetIndex.json"
        $compileResultPath = Join-Path $buildWorkspace "compile-result-$targetIndex.json"
        $resultPath = Join-Path $buildWorkspace "result-$targetIndex.json"
        $jobSpec = [ordered]@{
            index = $targetIndex
            repository_root = $repositoryRoot
            goos = $goos
            goarch = $goarch
            actual_goarch = $actualGoarch
            goamd64 = $goamd64
            binary_path = $binaryPath
            binary_name = $binaryName
            archive_path = $archivePath
            brotli_tool = $brotliPackTool
            bundled = (-not $noBundle)
            ldflags = $ldflags
            compile_result_path = $compileResultPath
            result_path = $resultPath
        }
        $jobSpec | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $specPath -Encoding utf8
        $jobs.Add([pscustomobject]@{
            Index = $targetIndex
            Label = "$goos/$goarch"
            Destination = $destinationDescription
            BinaryPath = $binaryPath
            SpecPath = $specPath
            CompileResultPath = $compileResultPath
            ResultPath = $resultPath
            CompileStdoutPath = Join-Path $buildWorkspace "job-$targetIndex.compile.stdout.log"
            CompileStderrPath = Join-Path $buildWorkspace "job-$targetIndex.compile.stderr.log"
            CompressStdoutPath = Join-Path $buildWorkspace "job-$targetIndex.compress.stdout.log"
            CompressStderrPath = Join-Path $buildWorkspace "job-$targetIndex.compress.stderr.log"
        })
    }

    $results = @{}
    $failures = [System.Collections.Generic.List[string]]::new()
    $pendingCompression = [System.Collections.Generic.Queue[object]]::new()
    $nextJob = 0
    $stopLaunching = $false
    while ($activeCompileWorkers.Count -gt 0 -or $activeCompressionWorkers.Count -gt 0 -or
        (-not $stopLaunching -and ($nextJob -lt $jobs.Count -or $pendingCompression.Count -gt 0))) {
        while (-not $stopLaunching -and $activeCompileWorkers.Count -lt $BuildConcurrency -and $nextJob -lt $jobs.Count) {
            $job = $jobs[$nextJob]
            Write-Host "Starting compile $($job.Label) -> $($job.BinaryPath)"
            $activeCompileWorkers.Add((Start-TargetWorker -Job $job -WorkerScript $workerScript -Phase compile))
            $nextJob++
        }
        while (-not $stopLaunching -and -not $noBundle -and
            $activeCompressionWorkers.Count -lt $CompressionConcurrency -and $pendingCompression.Count -gt 0) {
            $job = $pendingCompression.Dequeue()
            Write-Host "Starting compression $($job.Label) -> $($job.Destination)"
            $activeCompressionWorkers.Add((Start-TargetWorker -Job $job -WorkerScript $compressionWorkerScript -Phase compress))
        }

        $completedWorker = $false
        for ($workerIndex = $activeCompileWorkers.Count - 1; $workerIndex -ge 0; $workerIndex--) {
            $worker = $activeCompileWorkers[$workerIndex]
            if (-not $worker.Process.HasExited) {
                continue
            }
            $completedWorker = $true
            $worker.Process.WaitForExit()
            Write-TargetWorkerLogs -Worker $worker
            if ($worker.Process.ExitCode -ne 0) {
                $failures.Add("$($worker.Job.Label) compile exited with code $($worker.Process.ExitCode)")
                $stopLaunching = $true
            } elseif (-not (Test-Path -LiteralPath $worker.Job.CompileResultPath -PathType Leaf)) {
                $failures.Add("$($worker.Job.Label) did not produce a compile result")
                $stopLaunching = $true
            } else {
                try {
                    $compileResult = Get-Content -LiteralPath $worker.Job.CompileResultPath -Raw | ConvertFrom-Json
                    if ([int]$compileResult.index -ne $worker.Job.Index -or
                        -not (Test-Path -LiteralPath ([string]$compileResult.binary) -PathType Leaf)) {
                        throw 'compile result does not match its job or binary'
                    }
                    if ($noBundle) {
                        $results[[int]$compileResult.index] = $compileResult
                        Write-Host "Completed $($worker.Job.Label)"
                    } else {
                        $pendingCompression.Enqueue($worker.Job)
                        Write-Host "Compiled $($worker.Job.Label); queued asynchronous compression"
                    }
                } catch {
                    $failures.Add("$($worker.Job.Label) produced an invalid compile result: $($_.Exception.Message)")
                    $stopLaunching = $true
                }
            }
            $worker.Process.Dispose()
            $activeCompileWorkers.RemoveAt($workerIndex)
        }

        for ($workerIndex = $activeCompressionWorkers.Count - 1; $workerIndex -ge 0; $workerIndex--) {
            $worker = $activeCompressionWorkers[$workerIndex]
            if (-not $worker.Process.HasExited) {
                continue
            }
            $completedWorker = $true
            $worker.Process.WaitForExit()
            Write-TargetWorkerLogs -Worker $worker
            if ($worker.Process.ExitCode -ne 0) {
                $failures.Add("$($worker.Job.Label) compression exited with code $($worker.Process.ExitCode)")
                $stopLaunching = $true
            } elseif (-not (Test-Path -LiteralPath $worker.Job.ResultPath -PathType Leaf)) {
                $failures.Add("$($worker.Job.Label) did not produce a compression result")
                $stopLaunching = $true
            } else {
                try {
                    $result = Get-Content -LiteralPath $worker.Job.ResultPath -Raw | ConvertFrom-Json
                    if ([int]$result.index -ne $worker.Job.Index) {
                        throw 'compression result does not match its job'
                    }
                    $results[[int]$result.index] = $result
                    Write-Host "Completed $($worker.Job.Label)"
                } catch {
                    $failures.Add("$($worker.Job.Label) produced an invalid compression result: $($_.Exception.Message)")
                    $stopLaunching = $true
                }
            }
            $worker.Process.Dispose()
            $activeCompressionWorkers.RemoveAt($workerIndex)
        }
        if ($stopLaunching -and $pendingCompression.Count -gt 0) {
            $pendingCompression.Clear()
        }
        if (-not $completedWorker -and
            ($activeCompileWorkers.Count -gt 0 -or $activeCompressionWorkers.Count -gt 0)) {
            Start-Sleep -Milliseconds 100
        }
    }

    if ($failures.Count -gt 0) {
        throw "Target build pipeline failed: $($failures -join '; ')"
    }
    if ($results.Count -ne $jobs.Count) {
        throw "Target build pipeline produced $($results.Count) result(s) for $($jobs.Count) job(s)."
    }

    if (-not $noBundle) {
        $manifestTargets = [System.Collections.Generic.List[object]]::new()
        for ($targetIndex = 0; $targetIndex -lt $jobs.Count; $targetIndex++) {
            $result = $results[$targetIndex]
            $archivePath = Join-Path $dist ([string]$result.file)
            if (-not (Test-Path -LiteralPath $archivePath -PathType Leaf)) {
                throw "Packaged target is missing: $archivePath"
            }
            $manifestTargets.Add([ordered]@{
                os = [string]$result.os
                arch = [string]$result.arch
                file = [string]$result.file
                sha256 = [string]$result.sha256
                size = [int64]$result.size
                uncompressed_size = [int64]$result.uncompressed_size
                format = [string]$result.format
                executable = [string]$result.executable
            })
        }
        $manifest = [ordered]@{
            version = $displayVersion
            commit = $commitFull
            previous_commit = $previousCommitFull
            development = ($developmentValue -eq 'true')
            targets = $manifestTargets
        }
        $manifest | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $dist 'manifest.json') -Encoding utf8
    }
}
finally {
    foreach ($worker in @($activeCompileWorkers) + @($activeCompressionWorkers)) {
        try {
            if (-not $worker.Process.HasExited) {
                $worker.Process.Kill($true)
                $worker.Process.WaitForExit()
            }
            $worker.Process.Dispose()
        } catch {
            Write-Warning "Failed to stop target worker $($worker.Job.Label): $($_.Exception.Message)"
        }
    }
    try {
        Remove-BuildWorkspace -Path $buildWorkspace
    } catch {
        Write-Warning $_
    }
    if ($hadCgo) { $env:CGO_ENABLED = $originalCgo } else { Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue }
    if ($hadGoos) { $env:GOOS = $originalGoos } else { Remove-Item Env:GOOS -ErrorAction SilentlyContinue }
    if ($hadGoarch) { $env:GOARCH = $originalGoarch } else { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue }
    if ($hadGoamd64) { $env:GOAMD64 = $originalGoamd64 } else { Remove-Item Env:GOAMD64 -ErrorAction SilentlyContinue }
}

$finalDirectory = if ($noBundle) { $invocationDirectory } else { $dist }
$packagingDescription = if ($noBundle) { 'without packaging' } else { 'with packages' }
Write-Host "Built $($targets.Count) target(s) into $finalDirectory $packagingDescription"
