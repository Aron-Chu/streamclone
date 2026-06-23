# Run bearhost-corpus-only on the BearHost VPS from Windows.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
& (Join-Path $root 'scripts\bearhost-wsl-run.ps1') -Script 'bearhost-corpus-only-remote.sh'
