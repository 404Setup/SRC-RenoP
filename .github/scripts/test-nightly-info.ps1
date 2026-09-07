$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'nightly-info.ps1')

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}

function New-TestCommit {
    param([string]$Message)
    & git -c user.name=Test -c user.email=test@example.invalid -c commit.gpgsign=false `
        commit --quiet --allow-empty -m $Message
    if ($LASTEXITCODE -ne 0) { throw 'Could not create test commit' }
    return (& git rev-parse HEAD).Trim()
}

$temporaryRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath())
$testRoot = Join-Path $temporaryRoot ('renop-nightly-test-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot | Out-Null
Push-Location -LiteralPath $testRoot
try {
    & git init --quiet
    if ($LASTEXITCODE -ne 0) { throw 'Could not initialize test repository' }
    $commits = @(
        for ($i = 0; $i -lt 105; $i++) { New-TestCommit "feat: history $i" }
    )
    foreach ($message in @(
        '[web] website', '[SKIP CI] maintenance', 'maintenance [ci skip]',
        '[no ci] maintenance', '[skip actions] maintenance', '[actions skip] maintenance',
        "chore: trailer`n`n`nskip-checks: true", '[release] v1.0.0'
    )) { New-TestCommit $message | Out-Null }
    $missing = New-TestCommit "fix: previously missed`n`n[web] in the body does not skip a build"
    $head = New-TestCommit 'fix: current build'
    $headVersion = (& git rev-parse --short HEAD).Trim()
    $current = [ordered]@{
        version = $headVersion; commit = $head; previous_commit = ''; channel = 'nightly'
        development = $true; published_at = '2026-09-07T00:00:00Z'; changelog = 'Current build'
        targets = @(@{file = 'fresh.br'; sha256 = 'fresh'})
    }
    $legacyVersion = (& git rev-parse --short $commits[104]).Trim()
    $existing = @(
        @{version = $commits[5].Substring(0, 7); commit = $commits[5]; targets = @(@{file = 'stale.br'})},
        @{version = $legacyVersion; changelog = 'Preserved legacy notes'; targets = @(@{file = 'stale.br'})},
        @{version = $headVersion; commit = $head; targets = @(@{file = 'old-current.br'})},
        @{version = 'unknown'; commit = ('a' * 40); targets = @(@{file = 'invalid.br'})}
    )
    $result = @(Get-NightlyReleases -CurrentRelease $current -ExistingReleases $existing)
    Assert-True ($result.Count -eq 100) 'Nightly history must retain 100 eligible commits'
    Assert-True ($result[0].commit -eq $head) 'Current build must be first'
    Assert-True ($result[1].commit -eq $missing) 'Missing eligible commit must be inserted in Git order'
    Assert-True ($result[1].changelog -eq 'fix: previously missed') 'Backfill must include the commit subject'
    Assert-True ($result[1].published_at -ne '') 'Backfill must include the Git commit timestamp'
    Assert-True ($result[2].changelog -eq 'Preserved legacy notes') 'Existing notes must survive sorting'
    Assert-True ($result[0].targets[0].file -eq 'fresh.br') 'Current targets must replace old targets'
    for ($i = 1; $i -lt $result.Count; $i++) {
        Assert-True (-not $result[$i].Contains('targets')) 'Historical targets must be removed entirely'
    }
    for ($i = 2; $i -lt $result.Count; $i++) {
        Assert-True ($result[$i].commit -eq $commits[106 - $i]) 'History must exclude non-nightly commits'
    }
    Assert-True ($existing[0].targets[0].file -eq 'stale.br') 'Rebuilding must not mutate remote input'
    $serialized = $result | ConvertTo-Json -Depth 8
    $repeated = @(Get-NightlyReleases -CurrentRelease $current -ExistingReleases ($serialized | ConvertFrom-Json -DateKind String))
    Assert-True (($repeated | ConvertTo-Json -Depth 8) -eq $serialized) 'Rebuilding must be idempotent'
    $empty = @(Get-NightlyReleases -CurrentRelease $current)
    Assert-True ($empty.Count -eq 100 -and $empty[0].targets[0].file -eq 'fresh.br') 'Missing remote metadata must be rebuilt'

    $old = [ordered]@{version = $legacyVersion; commit = $commits[104]; targets = @()}
    $rejected = $false
    try { Get-NightlyReleases -CurrentRelease $old -ExistingReleases @($current) | Out-Null }
    catch { $rejected = $_.Exception.Message -eq 'A newer nightly commit has already been published' }
    Assert-True $rejected 'An older build must not replace a published descendant'
    $rejected = $false
    try { Get-NightlyReleases -CurrentRelease @{commit = ('b' * 40)} | Out-Null }
    catch { $rejected = $true }
    Assert-True $rejected 'Git history failures must abort generation'
    Write-Host 'Nightly metadata regression checks passed.'
} finally {
    Pop-Location
    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    if (-not $resolvedTestRoot.StartsWith($temporaryRoot, [StringComparison]::OrdinalIgnoreCase) -or
        [IO.Path]::GetFileName($resolvedTestRoot) -notmatch '^renop-nightly-test-[0-9a-f]{32}$') {
        throw 'Refusing to remove an unexpected test directory'
    }
    Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force
}
