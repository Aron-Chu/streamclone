param(
    [Parameter(Mandatory = $true)]
    [string]$Hook
)
$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "wsl-path.ps1")
$paths = Get-RepoWslPath
$WslRepo = $paths.WslRepo
$HookPath = ".cursor/hooks/$Hook"
$Inner = "cd '$WslRepo' && python3 '$HookPath'"
wsl bash -lc $Inner
