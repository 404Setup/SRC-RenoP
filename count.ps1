$excludeDirNames = [System.Collections.Generic.HashSet[string]]::new(
    [string[]]@(
        'node_modules',
        'dist',
        'data',
        'storage',
        '.git',
        '.idea',
        '.gocache',
        '.gomodcache',
        'target',
        'bin',
        'vendor'
    ),
    [System.StringComparer]::OrdinalIgnoreCase
)

function Get-SourceFiles {
    param(
        [string]$Root,
        [string]$Filter
    )

    $results = [System.Collections.Generic.List[System.IO.FileInfo]]::new()
    $queue = [System.Collections.Generic.Queue[string]]::new()
    $queue.Enqueue($Root)

    while ($queue.Count -gt 0) {
        $dir = $queue.Dequeue()
        try {
            foreach ($file in [System.IO.Directory]::EnumerateFiles($dir, $Filter)) {
                $results.Add([System.IO.FileInfo]::new($file))
            }
            foreach ($sub in [System.IO.Directory]::EnumerateDirectories($dir)) {
                $name = [System.IO.Path]::GetFileName($sub)
                if (-not $excludeDirNames.Contains($name)) {
                    $queue.Enqueue($sub)
                }
            }
        } catch {
            # Skip unreadable directories
        }
    }

    return $results
}

function Get-SourceLineCount {
    param([string]$Filter)

    $root = (Get-Location).Path
    $files = Get-SourceFiles -Root $root -Filter $Filter
    if ($files.Count -eq 0) {
        return 0
    }

    $lines = 0
    foreach ($file in $files) {
        try {
            $lines += [System.IO.File]::ReadAllLines($file.FullName).Length
        } catch {
            # Skip unreadable files
        }
    }
    return $lines
}

$go = Get-SourceLineCount -Filter '*.go'
$js = Get-SourceLineCount -Filter '*.js'
$css = Get-SourceLineCount -Filter '*.css'
$md = Get-SourceLineCount -Filter '*.md'
$total = $go + $js + $css + $md

Write-Host "Total: $total"
Write-Host "Go: $go"
Write-Host "JS: $js"
Write-Host "CSS: $css"
Write-Host "Markdown: $md"