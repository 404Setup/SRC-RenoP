# Rebuild nightly metadata from the publishing commit's reachable Git history.
function Get-NightlyReleases {
    param(
        [Parameter(Mandatory)][object]$CurrentRelease,
        [object[]]$ExistingReleases = @()
    )

    $commit = [string]$CurrentRelease.commit
    if ($commit -notmatch '^[0-9a-f]{40}$') { throw 'Nightly publication requires a full commit SHA' }
    $history = [Collections.Generic.List[object]]::new()
    $offset = 0
    do {
        $raw = @(& git log --topo-order --max-count=100 --skip=$offset --format='%H%x1f%h%x1f%cI%x1f%s' `
            --extended-regexp --regexp-ignore-case --invert-grep `
            --grep='\[(skip ci|ci skip|no ci|skip actions|actions skip)\]' `
            --grep='^skip-checks: *true *$' $commit --)
        if ($LASTEXITCODE -ne 0) { throw 'Could not read nightly Git history' }
        foreach ($record in $raw) {
            $fields = $record -split [char]0x1f, 4
            if ($fields[3].Trim() -match '^\[(web|release)\]') { continue }
            $history.Add([ordered]@{
                version = $fields[1]
                commit = $fields[0]
                previous_commit = ''
                channel = 'nightly'
                development = $true
                published_at = $fields[2]
                changelog = $fields[3]
            })
            if ($history.Count -eq 100) { break }
        }
        $offset += 100
    } while ($history.Count -lt 100 -and $raw.Count -eq 100)
    if ($history.Count -eq 0 -or $history[0].commit -ne $commit) {
        throw 'The publishing commit must be a CI-eligible nightly commit with a full SHA'
    }

    $historyCommits = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($entry in $history) {
        $historyCommits.Add($entry.commit) | Out-Null
        $historyCommits.Add($entry.version) | Out-Null
    }
    $byCommit = @{}
    foreach ($release in $ExistingReleases) {
        $sha = [string]$release.commit
        if (-not $sha) { $sha = [string]$release.version }
        if ($sha -notmatch '^[0-9a-f]{7,40}$') { continue }
        if (-not $historyCommits.Contains($sha)) {
            & git merge-base --is-ancestor $commit $sha 2>$null
            if ($LASTEXITCODE -eq 0) { throw 'A newer nightly commit has already been published' }
        }
        $byCommit[$sha] = [ordered]@{
            version = [string]$release.version
            commit = [string]$release.commit
            previous_commit = [string]$release.previous_commit
            channel = 'nightly'
            development = $true
            published_at = [string]$release.published_at
            changelog = [string]$release.changelog
        }
    }
    $byCommit[$commit] = $CurrentRelease

    $sorted = [System.Collections.Generic.List[object]]::new()
    foreach ($entry in $history) {
        $release = $byCommit[$entry.commit]
        if (-not $release) { $release = $byCommit[$entry.version] }
        if ($release) {
            $release.commit = $entry.commit
            $sorted.Add($release)
        }
    }
    for ($i = 0; $i -lt $history.Count; $i++) {
        if ($i -ge $sorted.Count -or $sorted[$i].commit -ne $history[$i].commit) {
            $sorted.Insert($i, $history[$i])
        }
    }
    return $sorted.ToArray()
}
