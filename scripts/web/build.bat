@echo off
setlocal EnableExtensions
cd /d "%~dp0..\.."

echo [ContainerWay Web] A compilar executavel (abre o browser ao iniciar)...
call "%~dp0..\build-web.ps1"
exit /b %ERRORLEVEL%
exit /b 0
