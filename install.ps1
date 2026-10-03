# Install cc-patcher on Windows.
$ErrorActionPreference = "Stop"
$repo = "aveekpatra/cc-patcher"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$url = "https://github.com/$repo/releases/latest/download/cc-patcher_windows_$arch.zip"
$dir = Join-Path $env:LOCALAPPDATA "cc-patcher"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$zip = Join-Path $env:TEMP "cc-patcher.zip"
Invoke-WebRequest -Uri $url -OutFile $zip
Expand-Archive -Path $zip -DestinationPath $dir -Force
Remove-Item $zip
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
  Write-Host "added $dir to PATH (restart your terminal)"
}
Write-Host "installed to $dir\cc-patcher.exe"

# Start it, unless asked not to.
if (-not $env:CC_PATCHER_NO_LAUNCH) { & (Join-Path $dir "cc-patcher.exe") }
