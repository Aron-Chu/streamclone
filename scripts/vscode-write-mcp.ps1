# Write Streamclone MCP configs for VS Code (workspace + user profile).
param(
    [string]$RepoWin = "",
    [string]$RepoWsl = ""
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "wsl-path.ps1")

if (-not $RepoWin) {
    $paths = Get-RepoWslPath
    $RepoWin = $paths.Repo
    $RepoWsl = $paths.WslRepo
}

function New-StreamcloneMcpObject {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WslRepoPath
    )
    [ordered]@{
        servers = [ordered]@{
            streamcloneCodegraph = [ordered]@{
                type    = "stdio"
                command = "wsl.exe"
                args    = @("--cd", $WslRepoPath, "bash", "scripts/codegraph-mcp.sh")
            }
            streamcloneStack = [ordered]@{
                type    = "stdio"
                command = "wsl.exe"
                args    = @("--cd", $WslRepoPath, "bash", "scripts/stack-mcp.sh")
            }
            streamcloneData = [ordered]@{
                type    = "stdio"
                command = "wsl.exe"
                args    = @("--cd", $WslRepoPath, "bash", "scripts/data-mcp.sh")
            }
            playwright = [ordered]@{
                type    = "stdio"
                command = "npx"
                args    = @("-y", "@playwright/mcp@latest")
            }
        }
    }
}

function New-StreamcloneMcpObjectForWorkspaceFolder {
    param(
        [Parameter(Mandatory = $true)]
        [string]$WorkspaceFolderVar
    )
    [ordered]@{
        servers = [ordered]@{
            streamcloneCodegraph = [ordered]@{
                type    = "stdio"
                command = "wsl.exe"
                args    = @("--cd", $WorkspaceFolderVar, "bash", "scripts/codegraph-mcp.sh")
            }
            streamcloneStack = [ordered]@{
                type    = "stdio"
                command = "wsl.exe"
                args    = @("--cd", $WorkspaceFolderVar, "bash", "scripts/stack-mcp.sh")
            }
            streamcloneData = [ordered]@{
                type    = "stdio"
                command = "wsl.exe"
                args    = @("--cd", $WorkspaceFolderVar, "bash", "scripts/data-mcp.sh")
            }
            playwright = [ordered]@{
                type    = "stdio"
                command = "npx"
                args    = @("-y", "@playwright/mcp@latest")
            }
        }
    }
}

function Write-JsonFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [object]$Object
    )
    $dir = Split-Path $Path -Parent
    if ($dir -and -not (Test-Path $dir)) {
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
    $json = ($Object | ConvertTo-Json -Depth 8)
    Set-Content -Path $Path -Value $json -Encoding utf8
}

function Install-StreamcloneVsCodeMcp {
    param(
        [string]$RepoWinPath = $RepoWin,
        [string]$RepoWslPath = $RepoWsl
    )

    $streamcloneMcp = Join-Path $RepoWinPath ".vscode\mcp.json"
    Write-JsonFile -Path $streamcloneMcp -Object (New-StreamcloneMcpObjectForWorkspaceFolder '${workspaceFolder}')

    $example = Join-Path $RepoWinPath ".vscode\mcp.json.example"
    Write-JsonFile -Path $example -Object (New-StreamcloneMcpObjectForWorkspaceFolder '${workspaceFolder}')

    $pulseRoot = Join-Path (Split-Path $RepoWinPath -Parent) "streamclone-pulse"
    if (Test-Path $pulseRoot) {
        $pulseMcp = Join-Path $pulseRoot ".vscode\mcp.json"
        Write-JsonFile -Path $pulseMcp -Object (New-StreamcloneMcpObjectForWorkspaceFolder '${workspaceFolder:streamclone}')
        Write-Host "wrote $pulseMcp for multi-root workspace folder streamclone"
    }

    $userMcp = Join-Path $env:APPDATA "Code\User\mcp.json"
    Write-JsonFile -Path $userMcp -Object (New-StreamcloneMcpObject $RepoWslPath)
    Write-Host "wrote $userMcp with absolute WSL repo path"
}

if ($MyInvocation.InvocationName -ne '.') {
    Install-StreamcloneVsCodeMcp
}
