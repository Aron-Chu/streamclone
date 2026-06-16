#Requires -Version 5.1
# Capture README showcase screenshots, WebM source clips, and trimmed GIFs at 1920x1080.
param(
    [string]$BaseUrl = 'http://localhost:8090',
    [string]$PulseUrl = 'http://localhost:3000/d/streamclone-emote-pulse/emote-pulse?from=now-24h&to=now&orgId=1&timezone=browser&refresh=30s',
    [string]$ChannelPath = '/c/xqc',
    [string]$AnalyticsPath = '/analytics/xqc/2026-06-14',
    [int]$ClipSeconds = 7,
    [int]$GifFps = 30,
    [int]$GifSeconds = 6,
    [string[]]$Scenes = @(),
    [switch]$SkipGifs
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot

function Convert-ToGif {
    param(
        [Parameter(Mandatory = $true)][string]$InputPath,
        [Parameter(Mandatory = $true)][string]$OutputPath,
        [Parameter(Mandatory = $true)][int]$Fps,
        [Parameter(Mandatory = $true)][int]$StartSeconds,
        [Parameter(Mandatory = $true)][int]$DurationSeconds,
        [Parameter(Mandatory = $true)][string]$RepoRoot
    )

    $filter = "fps=$Fps,scale=1920:1080:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=192[p];[s1][p]paletteuse=dither=bayer:bayer_scale=3"
    if (Get-Command ffmpeg -ErrorAction SilentlyContinue) {
        ffmpeg -y -ss $StartSeconds -t $DurationSeconds -i $InputPath -vf $filter -loop 0 $OutputPath | Out-Host
        if ($LASTEXITCODE -ne 0) { throw "ffmpeg failed for $InputPath with exit $LASTEXITCODE" }
        return
    }

    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw 'ffmpeg was not found and Docker is unavailable for ffmpeg fallback.'
    }

    $rootPath = (Resolve-Path -LiteralPath $RepoRoot).Path
    $inputRelative = (Resolve-Path -LiteralPath $InputPath).Path.Substring($rootPath.Length).TrimStart('\') -replace '\\', '/'
    $outputRelative = $OutputPath.Substring($rootPath.Length).TrimStart('\') -replace '\\', '/'
    docker run --rm -v "${RepoRoot}:/work" jrottenberg/ffmpeg:6.1-alpine -y -ss $StartSeconds -t $DurationSeconds -i "/work/$inputRelative" -vf $filter -loop 0 "/work/$outputRelative" | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "docker ffmpeg failed for $InputPath with exit $LASTEXITCODE" }
}

if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    throw 'Node.js is required for Playwright capture.'
}
if (-not (Test-Path 'frontend\node_modules\playwright\package.json')) {
    Push-Location frontend
    try {
        npm install
        if ($LASTEXITCODE -ne 0) { throw "npm install failed with exit $LASTEXITCODE" }
    } finally {
        Pop-Location
    }
}

Push-Location frontend
try {
    npx playwright install chromium | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "playwright install chromium failed with exit $LASTEXITCODE" }
} finally {
    Pop-Location
}

$env:DOCS_PLAYWRIGHT_REQUIRE = Join-Path $repoRoot 'frontend\node_modules\playwright\package.json'
$env:DOCS_BASE_URL = $BaseUrl
$env:DOCS_PULSE_URL = $PulseUrl
$env:DOCS_CHANNEL_PATH = $ChannelPath
$env:DOCS_ANALYTICS_PATH = $AnalyticsPath
$env:DOCS_CLIP_SECONDS = [string]$ClipSeconds
$env:DOCS_VIEWPORT_WIDTH = '1920'
$env:DOCS_VIEWPORT_HEIGHT = '1080'
if ($Scenes.Count -gt 0) {
    $env:DOCS_SCENES = ($Scenes -join ',')
} else {
    Remove-Item Env:DOCS_SCENES -ErrorAction SilentlyContinue
}

Write-Host 'Capturing 1920x1080 showcase screenshots and WebM clips...'
node (Join-Path $PSScriptRoot 'capture-showcase-media.mjs')
if ($LASTEXITCODE -ne 0) { throw "showcase capture failed with exit $LASTEXITCODE" }

if (-not $SkipGifs) {
    $mediaDir = Join-Path $repoRoot 'docs\media'
    $imagesDir = Join-Path $repoRoot 'docs\images'
    $gifStarts = @{
        directory = 5
        channel = 12
        analytics = 6
        pulse = 10
    }
    $gifDurations = @{
        directory = $GifSeconds
        channel = [Math]::Min($GifSeconds, 3)
        analytics = $GifSeconds
        pulse = $GifSeconds
    }
    $videos = Get-ChildItem -Path $mediaDir -Filter '*.webm' -ErrorAction SilentlyContinue
    if ($Scenes.Count -gt 0) {
        $sceneNames = @($Scenes | ForEach-Object { $_.ToLowerInvariant() })
        $videos = @($videos | Where-Object { $sceneNames -contains $_.BaseName.ToLowerInvariant() })
    }
    foreach ($video in $videos) {
        $gif = Join-Path $imagesDir ($video.BaseName + '.gif')
        $start = if ($gifStarts.ContainsKey($video.BaseName)) { $gifStarts[$video.BaseName] } else { 0 }
        $duration = if ($gifDurations.ContainsKey($video.BaseName)) { $gifDurations[$video.BaseName] } else { $GifSeconds }
        Write-Host "Converting $($video.Name) -> docs/images/$($video.BaseName).gif at ${GifFps}fps, ${duration}s from +${start}s..."
        Convert-ToGif -InputPath $video.FullName -OutputPath $gif -Fps $GifFps -StartSeconds $start -DurationSeconds $duration -RepoRoot $repoRoot
    }
}

Write-Host ''
Write-Host 'Saved showcase media:'
Write-Host '  docs/images/directory.png, channel.png, analytics.png, pulse.png'
Write-Host '  docs/images/directory.gif, channel.gif, analytics.gif, pulse.gif'
Write-Host '  docs/media/*.webm source clips and capture-summary.json'
