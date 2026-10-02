# Install claude_patcher on Windows.
$ErrorActionPreference = "Stop"
$repo = "aveekpatra/claude_patcher"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
$url = "https://github.com/$repo/releases/latest/download/claude_patcher_windows_$arch.zip"
$dir = Join-Path $env:LOCALAPPDATA "claude_patcher"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$zip = Join-Path $env:TEMP "claude_patcher.zip"
Invoke-WebRequest -Uri $url -OutFile $zip
Expand-Archive -Path $zip -DestinationPath $dir -Force
Remove-Item $zip
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
  Write-Host "added $dir to PATH (restart your terminal)"
}
Write-Host "installed to $dir\claude_patcher.exe"
