# Wrapper so -File callers pass profile list correctly (comma strings break string[] binding).
param(
    [int]$MaxAttemptsPerProbe = 3,
    [switch]$RotateSessionOnFail,
    [switch]$RecreateScraperOnRetry,
    [switch]$UseFlameApi,
    [double]$MinGbRemaining = 0.5,
    [string]$OutFile = ''
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
Set-Location $repoRoot

$profiles = @('direct', 'premium', 'budget', 'api_premium', 'api_budget')
$params = @{
    Profiles            = $profiles
    MaxAttemptsPerProbe = $MaxAttemptsPerProbe
    MinGbRemaining      = $MinGbRemaining
}
if ($RotateSessionOnFail) { $params.RotateSessionOnFail = $true }
if ($RecreateScraperOnRetry) { $params.RecreateScraperOnRetry = $true }
if ($UseFlameApi) { $params.UseFlameApi = $true }
if ($OutFile) { $params.OutFile = $OutFile }

& (Join-Path $PSScriptRoot 'scraper-proxy-benchmark.ps1') @params
