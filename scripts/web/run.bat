@echo off
setlocal EnableExtensions
cd /d "%~dp0..\.."

set "ADDR=127.0.0.1:8765"
if not "%~1"=="" set "ADDR=%~1"

if exist "ContainerWay Web.exe" (
  echo [ContainerWay Web] A iniciar %ADDR% ^(ContainerWay Web.exe^)
  "ContainerWay Web.exe" -addr %ADDR%
  exit /b %ERRORLEVEL%
)

if exist "containerway-web.exe" (
  echo [ContainerWay Web] A iniciar %ADDR% ^(containerway-web.exe^)
  containerway-web.exe -addr %ADDR%
  exit /b %ERRORLEVEL%
)

where go >nul 2>&1
if errorlevel 1 (
  echo [ContainerWay Web] Go nao encontrado. Execute scripts\web\build.bat
  exit /b 1
)

echo [ContainerWay Web] Executavel nao encontrado; a usar go run...
echo Abra no browser: http://%ADDR%
echo.
go run ./cmd/containerway-web/ -addr %ADDR%
exit /b %ERRORLEVEL%
