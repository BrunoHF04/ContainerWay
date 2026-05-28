#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/../.."

ADDR="${1:-127.0.0.1:8765}"
URL="http://$ADDR"

if [ -x "./containerway-web" ]; then
  echo "[ContainerWay Web] A iniciar $ADDR (executável)"
  echo "Abra no browser: $URL"
  echo
  exec ./containerway-web -addr "$ADDR"
fi

if ! command -v go >/dev/null 2>&1; then
  echo "[ContainerWay Web] Go não encontrado. Execute ./build-web.sh ou scripts/web/build.sh" >&2
  exit 1
fi

echo "[ContainerWay Web] Executável não encontrado; a usar go run..."
echo "Abra no browser: $URL"
echo
exec go run ./cmd/containerway-web/ -addr "$ADDR"
