@echo off
setlocal EnableExtensions
cd /d "%~dp0..\.."

echo [ContainerWay Web] A compilar...
go build -o containerway-web.exe ./cmd/containerway-web/
if errorlevel 1 (
  echo [ContainerWay Web] Falha na compilacao.
  exit /b 1
)

echo [ContainerWay Web] OK: %CD%\containerway-web.exe
exit /b 0
