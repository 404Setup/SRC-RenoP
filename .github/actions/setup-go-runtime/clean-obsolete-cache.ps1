[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string]$CurrentTag,
    [Parameter(Mandatory)]
    [string]$RunnerOS,
    [Parameter(Mandatory)]
    [string]$RunnerArchitecture,
    [string]$ToolCache,
    [string]$GithubRepository,
    [string]$Token
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if (-not [string]::IsNullOrWhiteSpace($ToolCache)) {
    $baseGoDir = Join-Path $ToolCache '404setup-go'
    if (Test-Path -LiteralPath $baseGoDir -PathType Container) {
        try {
            $dirs = Get-ChildItem -LiteralPath $baseGoDir -Directory -ErrorAction SilentlyContinue
            foreach ($dir in $dirs) {
                if ($dir.Name -ne $CurrentTag) {
                    Write-Host "Removing obsolete local Go runtime directory: $($dir.FullName)"
                    Remove-Item -LiteralPath $dir.FullName -Recurse -Force -ErrorAction SilentlyContinue
                }
            }
        } catch {
            Write-Warning "Failed while cleaning local toolcache runtimes: $($_.Exception.Message)"
        }
    }
}

if (-not [string]::IsNullOrWhiteSpace($GithubRepository) -and -not [string]::IsNullOrWhiteSpace($Token)) {
    $headers = @{
        Accept                 = 'application/vnd.github+json'
        'X-GitHub-Api-Version' = '2022-11-28'
        'User-Agent'           = 'renop-setup-go-runtime'
        Authorization          = "Bearer $Token"
    }

    $runtimePrefix = "custom-go-runtime-$RunnerOS-$RunnerArchitecture-"
    $goCachePrefix = "go-cache-$RunnerOS-$RunnerArchitecture-"
    $tagPrefix = "$CurrentTag-"

    Write-Host "Checking GitHub Actions caches for obsolete Go version caches (Current tag: $CurrentTag)..."

    try {
        for ($page = 1; $page -le 20; $page++) {
            $uri = "https://api.github.com/repos/$GithubRepository/actions/caches?per_page=100&page=$page"
            $resp = $null
            try {
                $resp = Invoke-RestMethod -Uri $uri -Headers $headers -Method Get
            } catch {
                Write-Warning "GitHub Actions cache API query failed: $($_.Exception.Message)"
                break
            }

            if ($null -eq $resp -or $null -eq $resp.actions_caches -or $resp.actions_caches.Count -eq 0) {
                break
            }

            foreach ($cache in $resp.actions_caches) {
                $key = [string]$cache.key
                $shouldDelete = $false

                if ($key.StartsWith($runtimePrefix, [StringComparison]::OrdinalIgnoreCase)) {
                    $rest = $key.Substring($runtimePrefix.Length)
                    if (-not $rest.StartsWith($tagPrefix, [StringComparison]::OrdinalIgnoreCase)) {
                        $shouldDelete = $true
                    }
                } elseif ($key.StartsWith($goCachePrefix, [StringComparison]::OrdinalIgnoreCase)) {
                    $rest = $key.Substring($goCachePrefix.Length)
                    if (-not $rest.StartsWith($tagPrefix, [StringComparison]::OrdinalIgnoreCase)) {
                        $shouldDelete = $true
                    }
                }

                if ($shouldDelete) {
                    Write-Host "Deleting obsolete GitHub Actions cache: '$key' (ID $($cache.id))."
                    try {
                        $delUri = "https://api.github.com/repos/$GithubRepository/actions/caches/$($cache.id)"
                        Invoke-RestMethod -Uri $delUri -Headers $headers -Method Delete | Out-Null
                    } catch {
                        Write-Warning "Failed to delete cache '$key': $($_.Exception.Message)"
                    }
                }
            }

            if ($resp.actions_caches.Count -lt 100) {
                break
            }
        }
    } catch {
        Write-Warning "Failed to clean obsolete GitHub Actions caches: $($_.Exception.Message)"
    }
}
