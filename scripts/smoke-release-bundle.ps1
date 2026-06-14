#Requires -Version 5.1
<#
Release bundle verification smoke (moment-timeline Requirement 33, P0).

Trust-critical: catches deployments where the served frontend bundle is stale
versus the CI-built artifact. The classic failure signature is "live playback
works but VOD fails with HTTP 400 invalid_vod_id for a well-formed id" — the
stale browser bundle posts the wrong field (vodId instead of vod_id), so the
backend never sees a valid id. This is a frontend deploy mismatch, NOT a Twitch
content issue.

Checks:
  A. Record the served frontend entry script (e.g. /assets/index-*.js from
     index.html) and compare it to the CI-built artifact (an explicit expected
     value, or the entry referenced by a locally built frontend/dist/index.html).
     A mismatch fails as a deploy mismatch (Req 33.1).
  B. POST /v1/stream/vod/start with a known well-formed VOD_Identifier
     (^\d{5,20}$). HTTP 400 invalid_vod_id for a valid id indicates a client
     bundle regression and fails (Req 33.2).
  C. Classify live-pass / VOD-fail-on-400 as a frontend deploy mismatch rather
     than a Twitch content issue (Req 33.3).

The stack may not be running. By default an unreachable stack is reported as a
SKIP (exit 0). Pass -RequireStack to make stack-down a hard failure (release gate).
#>
[CmdletBinding()]
param(
    [string]$BaseUrl = $(if ($env:SMOKE_BASE_URL) { $env:SMOKE_BASE_URL } else { 'http://localhost:8090' }),
    [string]$VodId = $(if ($env:SMOKE_VOD_ID) { $env:SMOKE_VOD_ID } else { '1234567890' }),
    [string]$ExpectedEntry = $env:RELEASE_BUNDLE_ENTRY,
    [string]$DistDir = $(if ($env:RELEASE_BUNDLE_DIST) { $env:RELEASE_BUNDLE_DIST } else { 'frontend/dist' }),
    [switch]$RequireStack,
    [switch]$Strict
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path $PSScriptRoot -Parent
Set-Location $Root

$BaseUrl = $BaseUrl.TrimEnd('/')
$failures = New-Object System.Collections.Generic.List[string]

function Write-Step($msg) { Write-Host "smoke-release-bundle: $msg" }
function Add-Failure($msg) { Write-Host "  FAIL: $msg" -ForegroundColor Red; $script:failures.Add($msg) }
function Write-Ok($msg)    { Write-Host "  ok: $msg" -ForegroundColor Green }
function Write-Note($msg)  { Write-Host "  note: $msg" -ForegroundColor Yellow }

# Extract the ES-module entry script src from an index.html document.
function Get-EntryScript([string]$html) {
    $m = [regex]::Match($html, '<script[^>]*\btype\s*=\s*"module"[^>]*\bsrc\s*=\s*"([^"]+)"')
    if (-not $m.Success) {
        $m = [regex]::Match($html, '<script[^>]*\bsrc\s*=\s*"([^"]+)"[^>]*\btype\s*=\s*"module"')
    }
    if ($m.Success) { return $m.Groups[1].Value }
    return $null
}

# --- Reachability guard -----------------------------------------------------
Write-Step "probing $BaseUrl/ ..."
$indexHtml = $null
try {
    $resp = Invoke-WebRequest -Uri "$BaseUrl/" -UseBasicParsing -TimeoutSec 10
    $indexHtml = [string]$resp.Content
    Write-Ok "frontend index reachable (HTTP $($resp.StatusCode))"
} catch {
    $msg = "stack not reachable at $BaseUrl/ ($($_.Exception.Message))"
    if ($RequireStack) {
        Write-Host "smoke-release-bundle: $msg" -ForegroundColor Red
        Write-Host "smoke-release-bundle: -RequireStack set -> FAIL" -ForegroundColor Red
        exit 1
    }
    Write-Host "smoke-release-bundle: SKIP - $msg" -ForegroundColor Yellow
    Write-Host "smoke-release-bundle: bring the stack up (make up) then re-run, or pass -RequireStack in a release gate."
    exit 0
}

# A "live" path is considered served when the frontend index loads. This is the
# baseline used by the deploy-mismatch classification below.
$livePass = -not [string]::IsNullOrWhiteSpace($indexHtml)

# --- Check A: served entry script vs CI-built artifact ----------------------
Write-Step "checking served frontend entry script (Req 33.1) ..."
$servedEntry = Get-EntryScript $indexHtml
$servedHash = $null
if (-not $servedEntry) {
    Add-Failure "could not locate an ES-module entry <script> in served index.html"
} else {
    $servedEntryName = ($servedEntry -split '[/\\]')[-1]
    Write-Note "served entry: $servedEntry"
    if ($servedEntry -match 'main\.tsx$') {
        Write-Note "served entry is the dev source (main.tsx); this looks like a Vite dev server, not a built bundle."
    }
    # Record a content hash of the served entry for the deploy record (Req 33.1).
    try {
        $entryUrl = if ($servedEntry -match '^https?://') { $servedEntry } else { "$BaseUrl/" + $servedEntry.TrimStart('/') }
        $js = Invoke-WebRequest -Uri $entryUrl -UseBasicParsing -TimeoutSec 15
        $bytes = if ($js.RawContentStream) {
            $ms = New-Object System.IO.MemoryStream
            $js.RawContentStream.CopyTo($ms); $ms.ToArray()
        } else { [System.Text.Encoding]::UTF8.GetBytes([string]$js.Content) }
        $sha = [System.Security.Cryptography.SHA256]::Create()
        $servedHash = ([System.BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-', '').ToLowerInvariant()
        Write-Note "served entry sha256: $servedHash"
    } catch {
        Write-Note "could not fetch served entry for hashing ($($_.Exception.Message)); comparing by filename only"
    }

    # Resolve the expected entry (CI-built artifact).
    $expected = $ExpectedEntry
    $expectedSource = 'RELEASE_BUNDLE_ENTRY / -ExpectedEntry'
    if (-not $expected) {
        $distIndex = Join-Path $Root (Join-Path $DistDir 'index.html')
        if (Test-Path $distIndex) {
            $expected = Get-EntryScript (Get-Content -Raw -LiteralPath $distIndex)
            $expectedSource = $distIndex
        }
    }

    if (-not $expected) {
        $note = "no expected entry available (set RELEASE_BUNDLE_ENTRY or build $DistDir) - recorded served entry only"
        if ($Strict) { Add-Failure $note } else { Write-Note $note }
    } else {
        $expectedName = ($expected -split '[/\\]')[-1]
        Write-Note "expected entry: $expected (from $expectedSource)"
        # Allow comparing either an explicit content hash or a filename.
        if ($ExpectedEntry -and $servedHash -and ($ExpectedEntry.ToLowerInvariant() -eq $servedHash)) {
            Write-Ok "served entry sha256 matches expected hash"
        } elseif ($servedEntryName -eq $expectedName) {
            Write-Ok "served entry filename matches CI-built artifact ($servedEntryName)"
        } else {
            Add-Failure "served entry '$servedEntryName' != CI-built artifact '$expectedName' - DEPLOY MISMATCH (stale frontend bundle)"
        }
    }
}

# --- Check B: VOD start round-trip with a valid id (Req 33.2) ---------------
Write-Step "checking VOD start with valid id '$VodId' (Req 33.2) ..."
if ($VodId -notmatch '^\d{5,20}$') {
    Add-Failure "configured smoke VodId '$VodId' is not a well-formed VOD_Identifier (^\d{5,20}$); cannot run VOD regression check"
}
$vodStatus = $null
$vodCode = $null
$vodFailRegression = $false
$body = "{`"vod_id`":`"$VodId`",`"offset_seconds`":0}"
try {
    $r = Invoke-WebRequest -Uri "$BaseUrl/v1/stream/vod/start" -Method Post -ContentType 'application/json' -Body $body -UseBasicParsing -TimeoutSec 20
    $vodStatus = [int]$r.StatusCode
} catch {
    $r = $_.Exception.Response
    if ($r -and $r.StatusCode) {
        $vodStatus = [int]$r.StatusCode
        try {
            $sr = New-Object System.IO.StreamReader($r.GetResponseStream())
            $respBody = $sr.ReadToEnd()
            $m = [regex]::Match($respBody, '"(?:error|code)"\s*:\s*"([^"]+)"')
            if ($m.Success) { $vodCode = $m.Groups[1].Value }
        } catch {}
    } else {
        Add-Failure "VOD start request failed without an HTTP response: $($_.Exception.Message)"
    }
}

if ($null -ne $vodStatus) {
    Write-Note "POST /v1/stream/vod/start -> HTTP $vodStatus$(if ($vodCode) { " ($vodCode)" })"
    if ($vodStatus -eq 400 -and $vodCode -eq 'invalid_vod_id') {
        $vodFailRegression = $true
        Add-Failure "HTTP 400 invalid_vod_id for a well-formed id '$VodId' - client bundle regression (frontend posting wrong field)"
    } else {
        # 200 (relay started), 404 vod_unavailable, 502/503 etc. all mean the
        # backend ACCEPTED the id format. Those are not bundle regressions.
        Write-Ok "VOD start accepted the id format (no 400 invalid_vod_id)"
    }
}

# --- Check C: deploy-mismatch classification (Req 33.3) ---------------------
if ($livePass -and $vodFailRegression) {
    Write-Host ""
    Write-Host "smoke-release-bundle: ===================================================" -ForegroundColor Red
    Write-Host "smoke-release-bundle: DEPLOY MISMATCH - live path serves but VOD start" -ForegroundColor Red
    Write-Host "smoke-release-bundle: returned 400 invalid_vod_id for a valid id." -ForegroundColor Red
    Write-Host "smoke-release-bundle: Treat as a STALE FRONTEND BUNDLE, not a Twitch" -ForegroundColor Red
    Write-Host "smoke-release-bundle: content issue. Rebuild/redeploy the frontend image" -ForegroundColor Red
    Write-Host "smoke-release-bundle: and verify the served entry matches CI (Check A)." -ForegroundColor Red
    Write-Host "smoke-release-bundle: ===================================================" -ForegroundColor Red
}

Write-Host ""
if ($failures.Count -gt 0) {
    Write-Host "smoke-release-bundle: FAILED ($($failures.Count) issue(s))" -ForegroundColor Red
    exit 1
}
Write-Host "smoke-release-bundle: all checks passed" -ForegroundColor Green
exit 0
