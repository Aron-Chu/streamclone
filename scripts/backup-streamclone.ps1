#Requires -Version 5.1
# Dump local Streamclone Postgres and print MinIO backup steps.
param(
    [string]$Root = '',
    [string]$OutDir = '',
    [switch]$SkipPostgres
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($Root)) {
    $Root = Split-Path -Parent $PSScriptRoot
}
. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')

$envPath = Join-Path $Root '.env'
if (-not (Test-Path $envPath)) {
    throw "Missing .env at $envPath — run scripts/setup.ps1 first."
}

$envValues = Read-EnvKeyValueFile -Path $envPath
$profile = [string]$envValues['STREAMCLONE_PROFILE']
if ([string]::IsNullOrWhiteSpace($profile)) { $profile = 'core' }
$useImages = ($envValues['STREAMCLONE_USE_IMAGES'] -eq '1') -or (-not [string]::IsNullOrWhiteSpace($envValues['IMAGE_TAG']))
$composeArgs = Get-StreamcloneComposeArgs -Root $Root -Profile $profile -UseImages:$useImages

if ([string]::IsNullOrWhiteSpace($OutDir)) {
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $OutDir = Join-Path $Root "backups\streamclone-$stamp"
}
New-Item -ItemType Directory -Path $OutDir -Force | Out-Null

$postgresDump = Join-Path $OutDir 'postgres.sql'
$minioNotes = Join-Path $OutDir 'minio-backup.txt'
$s3Bucket = if ([string]::IsNullOrWhiteSpace($envValues['S3_BUCKET'])) { 'emotes' } else { [string]$envValues['S3_BUCKET'] }
$s3Endpoint = if ([string]::IsNullOrWhiteSpace($envValues['S3_ENDPOINT'])) { 'http://localhost:9000' } else { [string]$envValues['S3_ENDPOINT'] }

Write-Host "Streamclone backup -> $OutDir"

if (-not $SkipPostgres) {
    Write-Host 'Dumping Postgres (analytics, emotes, local follows)...'
    $dumpArgs = $composeArgs + @('exec', '-T', 'postgres', 'pg_dump', '-U', 'app', '-d', 'streamclone', '--no-owner', '--no-acl')
    $result = Invoke-EnvDockerCaptured -Arguments $dumpArgs
    if ($result.ExitCode -ne 0) {
        $log = ($result.Output -join [Environment]::NewLine).Trim()
        throw "pg_dump failed (is the stack running?): $log"
    }
    [System.IO.File]::WriteAllText($postgresDump, ($result.Output -join [Environment]::NewLine), [System.Text.UTF8Encoding]::new($false))
    Write-Host "  wrote $postgresDump"
}

@(
    'MinIO object backup (rendered emotes + uploads)'
    ''
    'Streamclone stores emote assets in MinIO. Postgres dump above does NOT include object bytes.'
    ''
    'Option A — Docker mc mirror (recommended when stack is up):'
    "  docker run --rm --network container:streamclone-minio-1 -v `"${OutDir}:/backup`" minio/mc sh -c `""
    '    mc alias set local http://127.0.0.1:9000 minioadmin minioadmin &&'
    "    mc mirror --overwrite local/$s3Bucket /backup/minio-$s3Bucket"
    '  ""'
    ''
    'Option B — Host mc against published port:'
    "  mc alias set streamclone $s3Endpoint minioadmin minioadmin"
    "  mc mirror --overwrite streamclone/$s3Bucket `"$(Join-Path $OutDir "minio-$s3Bucket")`""
    ''
    'Restore Postgres:'
    "  Get-Content `"$postgresDump`" | docker compose ... exec -T postgres psql -U app -d streamclone"
    ''
    'Restore MinIO:'
    "  mc mirror --overwrite `"$(Join-Path $OutDir "minio-$s3Bucket")`" streamclone/$s3Bucket"
) | Set-Content -LiteralPath $minioNotes -Encoding UTF8

Write-Host "  wrote $minioNotes"
Write-Host 'Backup complete.'
