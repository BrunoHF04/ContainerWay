# Compila ContainerWay Web — executável que abre o browser automaticamente.
# Sem CGO. Saída na raiz: "ContainerWay Web.exe" (Windows) ou containerway-web (Linux/macOS).
param(
    [string]$Version,
    [switch]$LinuxDebViaDocker
)

$ErrorActionPreference = "Stop"
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $repoRoot

$ldflags = "-s -w"
if ($IsWindows -or $env:OS -match "Windows") {
    $ldflags += " -H=windowsgui"
    $outName = "ContainerWay Web.exe"
} else {
    $outName = "containerway-web"
}

Write-Host "==> Build ContainerWay Web -> $outName"
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags=$ldflags -o $outName ./cmd/containerway-web/
Write-Host "Gerado: $repoRoot\$outName"
Write-Host "Duplo clique abre o browser em http://127.0.0.1:8765"

if ($LinuxDebViaDocker) {
    Write-Host "==> Pacote .deb (containerway-web) via Docker"
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        throw "Docker não encontrado."
    }
    $debVersion = "0.1.0"
    if ($Version) { $debVersion = $Version }
    $debVersion = ($debVersion -replace '[^0-9A-Za-z\.\+\:\~\-]', '.')
    if ($debVersion -notmatch '^[0-9]') { $debVersion = "0~$debVersion" }

    $sh = @"
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq dpkg-dev ca-certificates
cd /src
mkdir -p dist/linux dist/deb-web/pkgroot/DEBIAN
mkdir -p dist/deb-web/pkgroot/usr/bin
mkdir -p dist/deb-web/pkgroot/usr/share/applications
mkdir -p dist/deb-web/pkgroot/usr/share/icons/hicolor/256x256/apps
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/linux/containerway-web ./cmd/containerway-web/
cp dist/linux/containerway-web dist/deb-web/pkgroot/usr/bin/containerway-web
chmod 0755 dist/deb-web/pkgroot/usr/bin/containerway-web
cp packaging/linux/io.containerway.ContainerWay.Web.desktop dist/deb-web/pkgroot/usr/share/applications/
cp assets/containerway-icon.png dist/deb-web/pkgroot/usr/share/icons/hicolor/256x256/apps/io.containerway.ContainerWay.png
cat > dist/deb-web/pkgroot/DEBIAN/control <<CTRL
Package: containerway-web
Version: __VER__
Section: utils
Priority: optional
Architecture: amd64
Maintainer: ContainerWay Team <noreply@containerway.local>
Depends: ca-certificates, xdg-utils
Description: ContainerWay Web - gestão de ficheiros no browser
 Servidor local com interface web; abre o navegador ao iniciar.
CTRL
sed -i "s/__VER__/__VER__/g" dist/deb-web/pkgroot/DEBIAN/control
dpkg-deb --build dist/deb-web/pkgroot "dist/deb-web/containerway-web___VER___amd64.deb"
"@
    $sh = $sh.Replace("__VER__", $debVersion)
    $path = Join-Path $repoRoot "dist/build-web-deb.sh"
    New-Item -ItemType Directory -Force -Path (Join-Path $repoRoot "dist") | Out-Null
    [IO.File]::WriteAllText($path, ($sh -replace "`r`n", "`n"), [Text.UTF8Encoding]::new($false))
    docker run --rm -v "${repoRoot}:/src" -w /src golang:1.26-bookworm bash /src/dist/build-web-deb.sh
    Write-Host "Gerado: dist/deb-web/containerway-web_${debVersion}_amd64.deb"
}
