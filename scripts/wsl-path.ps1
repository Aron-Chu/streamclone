function Get-WslLinuxPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WindowsPath
    )
    $resolved = (Resolve-Path $WindowsPath).Path
    $forward = $resolved -replace '\\', '/'
    $wsl = (wsl wslpath -a $forward 2>$null)
    if ($LASTEXITCODE -eq 0 -and $wsl) {
        return $wsl.Trim()
    }
    if ($forward -match '^([A-Za-z]):/(.*)$') {
        return "/mnt/$($Matches[1].ToLower())/$($Matches[2])"
    }
    throw "Could not convert path to WSL: $resolved"
}

function Get-RepoWslPath {
    param(
        [string]$StartPath = $PSScriptRoot
    )
    $repo = (Resolve-Path (Join-Path $StartPath "..")).Path
    return @{
        Repo = $repo
        WslRepo = (Get-WslLinuxPath $repo)
    }
}
