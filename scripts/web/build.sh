#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/../.."

echo "[ContainerWay Web] A compilar..."
go build -o containerway-web ./cmd/containerway-web/

echo "[ContainerWay Web] OK: $(pwd)/containerway-web"
