# Compilação Windows + Fyne (GLFW/OpenGL via CGO).
# Pré-requisito: GCC no PATH (ex.: MSYS2 — pacote mingw-w64-x86_64-gcc).
$ErrorActionPreference = "Stop"
$env:CGO_ENABLED = "1"
Set-Location $PSScriptRoot
go build -trimpath -ldflags="-s -w" -o containerway.exe ./cmd/containerway/
Write-Host "Gerado: $PSScriptRoot\containerway.exe"
