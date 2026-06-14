#Requires -Version 5.1
# Local security checks aligned with CI (gitleaks + env validation).
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)

Write-Host '==> gitleaks'
if (Get-Command gitleaks -ErrorAction SilentlyContinue) {
    gitleaks detect --source . --config .gitleaks.toml --verbose --redact
} elseif (Get-Command pre-commit -ErrorAction SilentlyContinue) {
    pre-commit run gitleaks --all-files
} elseif (Get-Command wsl -ErrorAction SilentlyContinue) {
    wsl -e bash -lc "cd /mnt/c/Users/Aron/twitch-7tv-clone && pre-commit run gitleaks --all-files"
} else {
    Write-Error 'Install gitleaks, pre-commit, or WSL (make install-hooks)'
}

Write-Host '==> validate-env'
& (Join-Path $PSScriptRoot 'validate-env.ps1')

Write-Host '==> local debug instrumentation'
$debugPattern = '127\.0\.0\.1:7829|X-Debug-Session-Id|#region agent log'
if (Get-Command rg -ErrorAction SilentlyContinue) {
    & rg -n $debugPattern frontend internal cmd clipper deploy .github --glob '!frontend/node_modules/**' --glob '!frontend/dist/**' --glob '!**/testdata/rapid/**'
    if ($LASTEXITCODE -eq 0) {
        throw 'Local debug ingest instrumentation found; remove it before committing.'
    }
    if ($LASTEXITCODE -gt 1) {
        exit $LASTEXITCODE
    }
} else {
    $roots = @('frontend', 'internal', 'cmd', 'clipper', 'deploy', '.github')
    $matches = Get-ChildItem $roots -Recurse -File -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -notmatch '\\testdata\\rapid\\' -and $_.FullName -notmatch '\\frontend\\(node_modules|dist)\\' } |
        Select-String -Pattern $debugPattern -ErrorAction SilentlyContinue
    if ($matches) {
        $matches | ForEach-Object { Write-Host $_ }
        throw 'Local debug ingest instrumentation found; remove it before committing.'
    }
}

Write-Host '==> tracked artifact denylist'
$denyPatterns = @(
    '.kiro/settings/*',
    'deploy/cookies.txt',
    'runtime/clipper-twitch.env',
    'out.json',
    'test.md',
    'pw-*.png',
    'analytics',
    'tmp-vod-*',
    'tmp-metadata-*',
    'debug-*.log',
    '.cursor/debug-*.log',
    '*/testdata/rapid/*.fail'
)
$badPaths = @()
git ls-files | ForEach-Object {
    $path = $_
    foreach ($pattern in $denyPatterns) {
        if ($path -like $pattern) {
            $badPaths += $path
            break
        }
    }
}
if ($badPaths.Count -gt 0) {
    $badPaths | ForEach-Object { Write-Host $_ }
    throw 'Tracked local artifacts found; remove or untrack them before committing.'
}

Write-Host 'security-scan ok'
