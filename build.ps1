$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
New-Item -ItemType Directory -Force -Path dist | Out-Null
go build -ldflags="-H windowsgui -s -w" -o dist/padslogic.exe .
Write-Host "Built dist/padslogic.exe"
