#Requires -Version 5.1
# Dump local Streamclone Postgres and optionally upload gzip to Azure Blob archive.
param(
    [string]$Root = '',
    [string]$OutDir = '',
    [switch]$SkipPostgres,
    [switch]$SkipAzureUpload
)

$ErrorActionPreference = 'Stop'
if ([string]::IsNullOrWhiteSpace($Root)) {
    $Root = Split-Path -Parent $PSScriptRoot
}
. (Join-Path $PSScriptRoot 'lib\env.ps1')
. (Join-Path $PSScriptRoot 'lib\stack-progress.ps1')

function Get-MergedEnvValues {
    param([string]$Root)
    $values = @{}
    $paths = @(
        (Join-Path $Root '.env'),
        (Join-Path $Root '.env.local'),
        (Join-Path $Root 'deploy\env\profile-archive.env')
    )
    foreach ($path in $paths) {
        if (-not (Test-Path $path)) { continue }
        $fileValues = Read-EnvKeyValueFile -Path $path
        foreach ($key in $fileValues.Keys) {
            if (-not [string]::IsNullOrWhiteSpace([string]$fileValues[$key])) {
                $values[$key] = [string]$fileValues[$key]
            }
        }
    }
    return $values
}

function Get-ArchiveConnectionString {
    param([hashtable]$EnvValues, [string]$Root)
    $inline = [string]$EnvValues['ARCHIVE_AZURE_CONNECTION_STRING']
    if (-not [string]::IsNullOrWhiteSpace($inline)) {
        return $inline.Trim()
    }
    $connFile = [string]$EnvValues['ARCHIVE_AZURE_CONNECTION_STRING_FILE']
    if ([string]::IsNullOrWhiteSpace($connFile)) {
        $fallback = Join-Path $env:USERPROFILE '.streamclone\azure-archive-connection-string'
        if (Test-Path $fallback) {
            $connFile = $fallback
        }
    } elseif (-not [System.IO.Path]::IsPathRooted($connFile)) {
        $connFile = Join-Path $Root $connFile
    }
    if (-not [string]::IsNullOrWhiteSpace($connFile) -and (Test-Path -LiteralPath $connFile)) {
        return ((Get-Content -LiteralPath $connFile -Raw).Trim())
    }
    return ''
}

function Invoke-AzureArchiveUpload {
    param(
        [string]$ConnectionString,
        [string]$Container,
        [string]$BlobName,
        [string]$FilePath
    )
    $az = Get-Command az -ErrorAction SilentlyContinue
    if (-not $az) {
        throw 'Azure CLI (az) is required for archive upload. Install from https://aka.ms/installazurecliwindows'
    }
    $args = @(
        'storage', 'blob', 'upload',
        '--connection-string', $ConnectionString,
        '--container-name', $Container,
        '--name', $BlobName,
        '--file', $FilePath,
        '--overwrite'
    )
    $result = & az @args 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw ("Azure blob upload failed: " + ($result -join [Environment]::NewLine))
    }
    return ($result -join [Environment]::NewLine)
}

$envPath = Join-Path $Root '.env'
if (-not (Test-Path $envPath)) {
    throw "Missing .env at $envPath — run scripts/setup.ps1 first."
}

$envValues = Read-EnvKeyValueFile -Path $envPath
$mergedEnv = Get-MergedEnvValues -Root $Root
foreach ($key in $envValues.Keys) {
    if (-not [string]::IsNullOrWhiteSpace([string]$envValues[$key])) {
        $mergedEnv[$key] = [string]$envValues[$key]
    }
}
$envValues = $mergedEnv
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
$postgresDumpGz = Join-Path $OutDir 'postgres.sql.gz'
$minioNotes = Join-Path $OutDir 'minio-backup.txt'
$archiveNotes = Join-Path $OutDir 'azure-archive-backup.txt'
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
    $gzip = [System.IO.Compression.GzipStream]::new(
        [System.IO.File]::Create($postgresDumpGz),
        [System.IO.Compression.CompressionLevel]::Optimal
    )
    $bytes = [System.Text.Encoding]::UTF8.GetBytes((Get-Content -LiteralPath $postgresDump -Raw))
    $gzip.Write($bytes, 0, $bytes.Length)
    $gzip.Dispose()
    Write-Host "  wrote $postgresDumpGz"
}

$azureUploadStatus = 'skipped (no ARCHIVE_AZURE_CONNECTION_STRING or connection string file)'
$azureBlobName = ''
if (-not $SkipPostgres -and -not $SkipAzureUpload -and (Test-Path -LiteralPath $postgresDumpGz)) {
    $connStr = Get-ArchiveConnectionString -EnvValues $envValues -Root $Root
    if (-not [string]::IsNullOrWhiteSpace($connStr)) {
        $container = if ([string]::IsNullOrWhiteSpace([string]$envValues['ARCHIVE_AZURE_CONTAINER'])) { 'streamclone-archive' } else { [string]$envValues['ARCHIVE_AZURE_CONTAINER'] }
        $prefix = if ([string]::IsNullOrWhiteSpace([string]$envValues['ARCHIVE_AZURE_PREFIX'])) { 'streamclone' } else { [string]$envValues['ARCHIVE_AZURE_PREFIX'] }
        $prefix = $prefix.Trim('/')
        $date = Get-Date -Format 'yyyy-MM-dd'
        $azureBlobName = "$prefix/postgres/nightly/$date.sql.gz"
        Write-Host "Uploading Postgres gzip to Azure Blob archive..."
        try {
            $uploadOut = Invoke-AzureArchiveUpload -ConnectionString $connStr -Container $container -BlobName $azureBlobName -FilePath $postgresDumpGz
            $azureUploadStatus = "uploaded to $container/$azureBlobName"
            if (-not [string]::IsNullOrWhiteSpace($uploadOut)) {
                Write-Host "  $uploadOut"
            }
        } catch {
            $azureUploadStatus = "failed: $($_.Exception.Message)"
            Write-Warning $azureUploadStatus
        }
    }
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

@(
    'Azure Blob archive upload (optional — requires ARCHIVE_* env in .env.local)'
    ''
    'Gzip dump path for nightly archive:'
    "  $postgresDumpGz"
    ''
    "Last upload attempt: $azureUploadStatus"
    if ($azureBlobName) {
        "Blob key: $azureBlobName"
        ''
    }
    'Blob key convention (account e.g. ststreamclone3lf6tt):'
    '  streamclone/postgres/nightly/{yyyy-MM-dd}.sql.gz'
    ''
    'Upload with Azure CLI (connection string — no az login required):'
    '  $conn = Get-Content $env:USERPROFILE\.streamclone\azure-archive-connection-string -Raw'
    '  $date = Get-Date -Format yyyy-MM-dd'
    '  az storage blob upload --connection-string $conn --container-name streamclone-archive \'
    "    --name streamclone/postgres/nightly/$date.sql.gz --file `"$postgresDumpGz`" --overwrite"
    ''
    'This script auto-uploads when ARCHIVE_AZURE_CONNECTION_STRING(_FILE) is set; pass -SkipAzureUpload to disable.'
    ''
    'After upload, record manifest row via archive CLI or analytics export worker.'
    ''
    'Restore analytics stream rollups without full pg restore:'
    '  go run ./cmd/archive restore --stream-id <twitch-stream-id>'
) | Set-Content -LiteralPath $archiveNotes -Encoding UTF8

Write-Host "  wrote $minioNotes"
Write-Host "  wrote $archiveNotes"
Write-Host 'Backup complete.'
