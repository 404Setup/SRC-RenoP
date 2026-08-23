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

$go = 0
$js = 0
$css = 0
$md = 0

$root = (Get-Location).Path
$queue = [System.Collections.Generic.Queue[System.IO.DirectoryInfo]]::new()
$queue.Enqueue([System.IO.DirectoryInfo]::new($root))

while ($queue.Count -gt 0) {
    $cur = $queue.Dequeue()
    try {
        foreach ($item in $cur.EnumerateFileSystemInfos()) {
            if ($item -is [System.IO.DirectoryInfo]) {
                if (-not $excludeDirNames.Contains($item.Name)) {
                    $queue.Enqueue($item)
                }
            } else {
                $ext = $item.Extension
                if ($ext.Equals('.go', [System.StringComparison]::OrdinalIgnoreCase)) {
                    $go += [System.Linq.Enumerable]::Count([System.IO.File]::ReadLines($item.FullName))
                } elseif ($ext.Equals('.js', [System.StringComparison]::OrdinalIgnoreCase)) {
                    $js += [System.Linq.Enumerable]::Count([System.IO.File]::ReadLines($item.FullName))
                } elseif ($ext.Equals('.css', [System.StringComparison]::OrdinalIgnoreCase)) {
                    $css += [System.Linq.Enumerable]::Count([System.IO.File]::ReadLines($item.FullName))
                } elseif ($ext.Equals('.md', [System.StringComparison]::OrdinalIgnoreCase)) {
                    $md += [System.Linq.Enumerable]::Count([System.IO.File]::ReadLines($item.FullName))
                }
            }
        }
    } catch {
        # Skip unreadable directories
    }
}

$total = $go + $js + $css + $md

Write-Host "Total: $total"
Write-Host "Go: $go"
Write-Host "JS: $js"
Write-Host "CSS: $css"
Write-Host "Markdown: $md"