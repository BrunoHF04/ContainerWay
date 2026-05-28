#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/../.."

echo "[ContainerWay Web] A compilar..."
go build -trimpath -ldflags="-s -w" -o containerway-web ./cmd/containerway-web/
echo "[ContainerWay Web] OK: $(pwd)/containerway-web"
echo "Execute ./containerway-web ou instale o .deb (scripts/build.ps1 -SkipWindows)"
