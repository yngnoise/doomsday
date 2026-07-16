param(
    [string]$EnvPath = (Join-Path $PSScriptRoot "..\.env"),
    [switch]$DisableSmtp
)

$ErrorActionPreference = "Stop"

function New-RandomSecret([int]$ByteCount) {
    $bytes = [byte[]]::new($ByteCount)
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($bytes)
    } finally {
        $rng.Dispose()
    }
    return [Convert]::ToBase64String($bytes)
}

$replacements = [ordered]@{
    JWT_SECRET     = New-RandomSecret 48
    ADMIN_PASSWORD = New-RandomSecret 32
}

if ($DisableSmtp) {
    $replacements.SMTP_PASS = ""
}

$lines = if (Test-Path -LiteralPath $EnvPath) {
    [System.IO.File]::ReadAllLines((Resolve-Path -LiteralPath $EnvPath))
} else {
    [string[]]@()
}

$seen = [System.Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
$updated = foreach ($line in $lines) {
    if ($line -match '^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=') {
        $key = $Matches[1]
        if ($replacements.Contains($key)) {
            [void]$seen.Add($key)
            "$key=$($replacements[$key])"
            continue
        }
    }
    $line
}

foreach ($entry in $replacements.GetEnumerator()) {
    if (-not $seen.Contains($entry.Key)) {
        $updated += "$($entry.Key)=$($entry.Value)"
    }
}

$absolutePath = [System.IO.Path]::GetFullPath($EnvPath)
[System.IO.File]::WriteAllLines($absolutePath, $updated, [System.Text.UTF8Encoding]::new($false))

$changed = @('JWT_SECRET', 'ADMIN_PASSWORD')
if ($DisableSmtp) {
    $changed += 'SMTP_PASS (disabled locally)'
}
Write-Output ("Updated {0}: {1}" -f $absolutePath, ($changed -join ', '))
