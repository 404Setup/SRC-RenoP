<#
Copyright (c) 2026 404Setup. All rights reserved.
This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.

.SYNOPSIS
    Prepare the pinned disposable email domain snapshot before Go compilation.
#>
#requires -Version 7.0
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$dataDirectory = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../internal/mail/data'))
$outputPath = Join-Path $dataDirectory 'disposable_domains.txt'
$outputHash = 'e0e0c6ae2e120e0309da143091a2967f77676added3064e866c97c634fea2ead'
$sources = @(
    @{
        Repository = 'disposable/disposable-email-domains'
        Commit = 'e4f846fdc7470c8ad127b54867ce2790bd4f9a45'
        Path = 'domains.txt'
        SHA256 = 'c65415be8efc0802ba01d6382e0489df6df94ce1da674a3d00cf51284e6a1ed2'
    },
    @{
        Repository = 'disposable-email-domains/disposable-email-domains'
        Commit = '8d5b14f53dff80842e841ee0ae14de2962cee66c'
        Path = 'disposable_email_blocklist.conf'
        SHA256 = '9e94123b0f0ebd77ef9cd7c6e7dd39bf7d3b981f618a6d20632647e2de20649b'
    }
)

if ((Test-Path -LiteralPath $outputPath -PathType Leaf) -and
    (Get-FileHash -LiteralPath $outputPath -Algorithm SHA256).Hash -eq $outputHash) {
    return
}

$client = [Net.Http.HttpClient]::new()
$client.Timeout = [TimeSpan]::FromSeconds(45)
$client.MaxResponseContentBufferSize = 8MB
$hash = [Security.Cryptography.SHA256]::Create()
$encoding = [Text.UTF8Encoding]::new($false, $true)
$idn = [Globalization.IdnMapping]::new()
$idn.UseStd3AsciiRules = $true
$domains = [Collections.Generic.SortedSet[string]]::new([StringComparer]::Ordinal)

try {
    foreach ($source in $sources) {
        $url = "https://raw.githubusercontent.com/$($source.Repository)/$($source.Commit)/$($source.Path)"
        $data = $client.GetByteArrayAsync($url).GetAwaiter().GetResult()
        $digest = [BitConverter]::ToString($hash.ComputeHash($data)).Replace('-', '')
        if ($digest -ne $source.SHA256) { throw 'Disposable email source checksum mismatch' }
        foreach ($line in $encoding.GetString($data).Split("`n")) {
            if ([string]::IsNullOrWhiteSpace($line) -or $line.StartsWith('#')) { continue }
            $domain = $line.Trim().ToLowerInvariant()
            if ($domain.EndsWith('.')) { $domain = $domain.Substring(0, $domain.Length - 1) }
            $domain = $idn.GetAscii($domain)
            if ($domain.Length -gt 253 -or $domain -cnotmatch '^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$') {
                throw "Invalid disposable email domain: $line"
            }
            [void]$domains.Add($domain)
        }
    }

    $data = $encoding.GetBytes([string]::Join("`n", [string[]]$domains) + "`n")
    $digest = [BitConverter]::ToString($hash.ComputeHash($data)).Replace('-', '')
    if ($digest -ne $outputHash) { throw 'Generated disposable email snapshot checksum mismatch' }

    [void][IO.Directory]::CreateDirectory($dataDirectory)
    $temporaryPath = Join-Path $dataDirectory ('disposable_domains.' + [Guid]::NewGuid().ToString('N') + '.tmp')
    try {
        $file = [IO.File]::Open($temporaryPath, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
        try { $file.Write($data, 0, $data.Length) } finally { $file.Dispose() }
        [IO.File]::Move($temporaryPath, $outputPath, $true)
    } finally {
        if (Test-Path -LiteralPath $temporaryPath -PathType Leaf) { Remove-Item -LiteralPath $temporaryPath -Force }
    }
    Write-Host "Generated $($domains.Count) disposable email domains from $($sources.Count) pinned sources."
} finally {
    $hash.Dispose()
    $client.Dispose()
}
