# Build multiplataforma:
# - Windows: ContainerWay.exe
# - Linux  : .deb e .flatpak (via Docker)
#
# Pré-requisitos:
# - Build Windows: GCC/clang no PATH para CGO/Fyne.
# - Build Linux: Docker disponível e em execução.
param(
    [string]$Version,
    [switch]$SkipWindows,
    [switch]$SkipLinux
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

function Get-BuildVersion {
    param([string]$InputVersion)

    if ($InputVersion) {
        return $InputVersion
    }

    $gitVersion = ""
    try {
        $gitVersion = (git describe --tags --always 2>$null).Trim()
    } catch {
        $gitVersion = ""
    }

    if ($gitVersion) {
        return $gitVersion
    }

    return "0.1.0-$([DateTime]::UtcNow.ToString('yyyyMMddHHmm'))"
}

function Get-DebianVersion {
    param([string]$RawVersion)
    # Versão Debian: apenas caracteres permitidos.
    $sanitized = ($RawVersion -replace '[^0-9A-Za-z\.\+\:\~\-]', '.')
    if ($sanitized -notmatch '^[0-9]') {
        $sanitized = "0~$sanitized"
    }
    return $sanitized
}

function Test-CommandAvailable {
    param(
        [string]$Name,
        [string]$Hint
    )
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Comando '$Name' não encontrado. $Hint"
    }
}

$buildVersion = Get-BuildVersion -InputVersion $Version
$debVersion = Get-DebianVersion -RawVersion $buildVersion
$repoRoot = (Get-Location).Path
$distDir = Join-Path $repoRoot "dist"

if (-not (Test-Path $distDir)) {
    New-Item -ItemType Directory -Path $distDir | Out-Null
}

if (-not $SkipWindows) {
    Write-Host "==> Build Windows (.exe)"
    $env:Path = [System.Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "User")
    $env:CGO_ENABLED = "1"
    go build -trimpath -ldflags="-s -w -H=windowsgui" -o ContainerWay.exe ./cmd/containerway/
    Write-Host "Gerado: $repoRoot\ContainerWay.exe"
}

if (-not $SkipLinux) {
    Write-Host "==> Build Linux (.deb e .flatpak) via Docker"
    Test-CommandAvailable -Name "docker" -Hint "Instale o Docker Desktop e inicie o serviço."

    $linuxScript = @"
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends \
  build-essential pkg-config libgl1-mesa-dev xorg-dev \
  flatpak flatpak-builder appstream appstream-compose ca-certificates

cd /src
mkdir -p dist/linux dist/deb dist/flatpak

CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o dist/linux/containerway ./cmd/containerway/

rm -rf dist/deb/pkgroot
mkdir -p dist/deb/pkgroot/DEBIAN
mkdir -p dist/deb/pkgroot/usr/bin
mkdir -p dist/deb/pkgroot/usr/share/applications
mkdir -p dist/deb/pkgroot/usr/share/icons/hicolor/256x256/apps
mkdir -p dist/deb/pkgroot/usr/share/metainfo

cp dist/linux/containerway dist/deb/pkgroot/usr/bin/containerway
chmod 0755 dist/deb/pkgroot/usr/bin/containerway
cp packaging/linux/io.containerway.ContainerWay.desktop \
   dist/deb/pkgroot/usr/share/applications/io.containerway.ContainerWay.desktop
cp packaging/linux/io.containerway.ContainerWay.metainfo.xml \
   dist/deb/pkgroot/usr/share/metainfo/io.containerway.ContainerWay.metainfo.xml
cp assets/containerway-icon.png \
   dist/deb/pkgroot/usr/share/icons/hicolor/256x256/apps/io.containerway.ContainerWay.png

cat > dist/deb/pkgroot/DEBIAN/control <<'CONTROL'
Package: containerway
Version: __DEB_VERSION__
Section: utils
Priority: optional
Architecture: amd64
Maintainer: ContainerWay Team <noreply@containerway.local>
Depends: libgl1, libx11-6, libxcursor1, libxrandr2, libxi6, libxxf86vm1, libxinerama1
Description: ContainerWay - gestão remota de arquivos e contêineres
 Aplicativo desktop para administrar arquivos remotos por SSH/SFTP
 e operações em contêineres Docker.
CONTROL
sed -i "s/__DEB_VERSION__/__DEB_VERSION_VALUE__/g" dist/deb/pkgroot/DEBIAN/control

dpkg-deb --build dist/deb/pkgroot "dist/deb/containerway___DEB_VERSION_VALUE___amd64.deb"

flatpak --system remote-add --if-not-exists flathub https://flathub.org/repo/flathub.flatpakrepo
flatpak --system install -y flathub org.freedesktop.Platform//24.08 org.freedesktop.Sdk//24.08

rm -rf dist/flatpak/build dist/flatpak/repo
flatpak-builder --force-clean --default-branch=stable --disable-rofiles-fuse --repo=dist/flatpak/repo \
  dist/flatpak/build packaging/linux/io.containerway.ContainerWay.json
flatpak build-bundle dist/flatpak/repo \
  "dist/flatpak/containerway-__DEB_VERSION_VALUE__.flatpak" \
  io.containerway.ContainerWay stable
"@

    $linuxScript = $linuxScript.Replace("__DEB_VERSION_VALUE__", $debVersion)
    $linuxScript = $linuxScript -replace "`r`n", "`n"
    $linuxScriptPath = Join-Path $repoRoot "dist/build-linux.sh"
    [System.IO.File]::WriteAllText($linuxScriptPath, $linuxScript, [System.Text.UTF8Encoding]::new($false))

    docker run --rm `
        --privileged `
        -v "${repoRoot}:/src" `
        -w /src `
        golang:1.26-bookworm `
        bash /src/dist/build-linux.sh
    if ($LASTEXITCODE -ne 0) {
        throw "Falha no build Linux via Docker (exit code $LASTEXITCODE)."
    }

    Write-Host "Gerado: $repoRoot\dist\deb\containerway-$debVersion`_amd64.deb"
    Write-Host "Gerado: $repoRoot\dist\flatpak\containerway-$debVersion.flatpak"
}

Write-Host "Build finalizado (version=$buildVersion)"