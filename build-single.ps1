$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
New-Item -ItemType Directory -Force -Path out_single | Out-Null
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build -ldflags="-H windowsgui -s -w" -o out_single/padslogic.exe .
Write-Host "Built out_single/padslogic.exe"
