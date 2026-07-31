<#
.SYNOPSIS
    Build release notes from git commits between the previous release and HEAD.

.DESCRIPTION
    Collects commit messages from the previous [release] commit (or tag) up to
    -Commit. Skips messages whose first line starts with [release], [ci skip],
    [skip ci], or [web]. Uses the first line of the first paragraph of each
    remaining message (text before the first blank line).

    Output shape (one entry per commit, newest first):

      feat: something
      fix: something else

.PARAMETER Commit
    End of the range (full or short SHA, or HEAD). Default: HEAD.

.PARAMETER OutFile
    Optional path to write the notes. If omitted, prints to stdout.

.PARAMETER Footer
    Optional text appended after the commit list (separated by a blank line).
#>
[CmdletBinding()]
param(
    [string]$Commit = 'HEAD',
    [string]$OutFile = '',
    [string]$Footer = ''
)

$ErrorActionPreference = 'Stop'

$Commit = (& git rev-parse $Commit).Trim()
if ([string]::IsNullOrWhiteSpace($Commit)) {
    throw 'Could not resolve Commit'
}

function Test-OmitSubject {
    param([Parameter(Mandatory = $true)][string]$Subject)
    $s = $Subject.Trim()
    if ([string]::IsNullOrWhiteSpace($s)) { return $true }
    $prefixes = @(
        '[release]',
        '[ci skip]',
        '[skip ci]',
        '[web]'
    )
    foreach ($p in $prefixes) {
        if ($s.StartsWith($p, [StringComparison]::OrdinalIgnoreCase)) {
            return $true
        }
    }
    return $false
}

function Get-FirstParagraphLine {
    param([Parameter(Mandatory = $true)][string]$Message)
    $msg = $Message -replace "`r`n", "`n" -replace "`r", "`n"
    $msg = $msg.Trim()
    if ([string]::IsNullOrWhiteSpace($msg)) { return '' }

    $para = ($msg -split "`n\s*`n", 2)[0].Trim()
    if ([string]::IsNullOrWhiteSpace($para)) { return '' }

    return (($para -split "`n", 2)[0]).Trim()
}

function Find-PreviousReleaseRef {
    param([Parameter(Mandatory = $true)][string]$Head)

    $prevRelease = (& git log -1 --format='%H' -i --grep='^\[release\]' "${Head}^" 2>$null)
    if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($prevRelease)) {
        return $prevRelease.Trim()
    }

    $prevTag = (& git describe --tags --abbrev=0 "${Head}^" 2>$null)
    if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($prevTag)) {
        return $prevTag.Trim()
    }

    return $null
}

function Get-GitLogRaw {
    param([Parameter(Mandatory = $true)][string[]]$GitArgs)
    $lines = & git @GitArgs
    if ($null -eq $lines) { return '' }
    if ($lines -is [System.Array]) {
        return ($lines -join "`n")
    }
    return [string]$lines
}

$headSubject = (& git log -1 --format='%s' $Commit 2>$null)
$isReleaseCommit = -not [string]::IsNullOrWhiteSpace($headSubject) -and $headSubject.Trim().StartsWith('[release]', [StringComparison]::OrdinalIgnoreCase)

if ($isReleaseCommit) {
    $prev = Find-PreviousReleaseRef -Head $Commit
    if ($prev) {
        Write-Host "Changelog range: ${prev}..${Commit}"
        $raw = Get-GitLogRaw -GitArgs @('log', '--format=%B%x1e', "${prev}..${Commit}")
    } else {
        Write-Host "Changelog range: full history through ${Commit} (no previous release found)"
        $raw = Get-GitLogRaw -GitArgs @('log', '--format=%B%x1e', $Commit)
    }
} else {
    Write-Host "Changelog range: single commit ${Commit} (nightly build)"
    $raw = Get-GitLogRaw -GitArgs @('log', '-1', '--format=%B%x1e', $Commit)
}

$entries = [System.Collections.Generic.List[string]]::new()
if (-not [string]::IsNullOrWhiteSpace($raw)) {
    $chunks = $raw -split [char]0x1e
    foreach ($chunk in $chunks) {
        $chunk = $chunk.Trim()
        if ([string]::IsNullOrWhiteSpace($chunk)) { continue }
        $line = Get-FirstParagraphLine -Message $chunk
        if (Test-OmitSubject -Subject $line) { continue }
        if (-not $entries.Contains($line)) {
            $entries.Add($line)
        }
    }
}

$body = ($entries -join "`n")
if (-not [string]::IsNullOrWhiteSpace($Footer)) {
    $footerText = $Footer.Trim()
    if ([string]::IsNullOrWhiteSpace($body)) {
        $body = $footerText
    } else {
        $body = $body + "`n`n" + $footerText
    }
}

if (-not [string]::IsNullOrWhiteSpace($OutFile)) {
    $full = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($OutFile)
    $parent = Split-Path -Parent $full
    if ($parent -and -not (Test-Path -LiteralPath $parent)) {
        New-Item -ItemType Directory -Path $parent -Force | Out-Null
    }
    [System.IO.File]::WriteAllText($full, $body, [System.Text.UTF8Encoding]::new($false))
    Write-Host "Wrote changelog ($($entries.Count) entries) to $full"
} else {
    Write-Output $body
}
