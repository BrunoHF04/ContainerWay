# Compilação Windows + Fyne (GLFW/OpenGL via CGO).
# Pré-requisito: GCC ou clang no PATH (ex.: MSYS2 MinGW, ou winget: MartinStorsjo.LLVM-MinGW.UCRT).
$ErrorActionPreference = "Stop"
$env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
$env:CGO_ENABLED = "1"
Set-Location $PSScriptRoot
go build -trimpath -ldflags="-s -w -H=windowsgui" -o ContainerWay.exe ./cmd/containerway/
Write-Host "Gerado: $PSScriptRoot\ContainerWay.exe"