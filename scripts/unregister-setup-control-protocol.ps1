#Requires -Version 5.1
# Remove streamclone:// URL handler registration (Windows HKCU).
$ErrorActionPreference = 'Stop'

$protocolKey = 'HKCU:\Software\Classes\streamclone'
if (Test-Path $protocolKey) {
    Remove-Item -LiteralPath $protocolKey -Recurse -Force
    Write-Host 'Unregistered streamclone:// URL handler.'
}
