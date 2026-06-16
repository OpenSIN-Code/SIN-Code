# SIN-Code one-line installer — Windows / PowerShell (issue #170).
# Mirrors the install.sh shim: downloads the single sin-code.exe
# from the latest GitHub release and re-execs it in --auto mode.
#
# Usage (from an elevated PowerShell):
#   irm https://raw.githubusercontent.com/OpenSIN-Code/SIN-Code/main/install.ps1 | iex
#   $env:SIN_CODE_BIN_DIR = "$env:USERPROFILE\my-tools"; .\install.ps1
#   .\install.ps1 -Release "v3.17.0"
[CmdletBinding()]
param(
    [string]$Release = "latest",
    [string]$BinDir = if ($env:SIN_CODE_BIN_DIR) { $env:SIN_CODE_BIN_DIR } else { Join-Path $env:LOCALAPPDATA "sin-code\bin" }
)
$ErrorActionPreference = "Stop"
$Repo = "OpenSIN-Code/SIN-Code"
$Arch = if ([System.Environment]::Is64BitOperatingSystem) { "amd64" } else { throw "32-bit Windows not supported" }
$Asset = "sin-code-windows-$Arch.zip"
$Url = if ($Release -eq "latest") {
    "https://github.com/$Repo/releases/latest/download/$Asset"
} else {
    "https://github.com/$Repo/releases/download/$Release/$Asset"
}
$Tmp = Join-Path $env:TEMP ([System.Guid]::NewGuid().ToString("N").Substring(0, 8))
Write-Host "[install.ps1] downloading $Url"
New-Item -ItemType Directory -Path $Tmp -Force | Out-Null
New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
Invoke-WebRequest -Uri $Url -OutFile (Join-Path $Tmp "sin-code.zip") -UseBasicParsing
Add-Type -AssemblyName System.IO.Compression.FileSystem
[System.IO.Compression.ZipFile]::ExtractToDirectory((Join-Path $Tmp "sin-code.zip"), $Tmp)
$Exe = Get-ChildItem -Path $Tmp -Recurse -Filter "sin-code.exe" | Select-Object -First 1
if (-not $Exe) { Write-Host "[install.ps1] ERROR: sin-code.exe not found in archive" -ForegroundColor Red; exit 1 }
Copy-Item $Exe.FullName (Join-Path $BinDir "sin-code.exe") -Force
Write-Host "[install.ps1] installed: $(Join-Path $BinDir 'sin-code.exe')"
Remove-Item -Recurse -Force $Tmp
& (Join-Path $BinDir "sin-code.exe") install --auto @args
