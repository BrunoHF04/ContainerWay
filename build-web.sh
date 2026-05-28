#!/usr/bin/env sh
# Atalho na raiz — compila ContainerWay Web (ver docs/WEB_UI.md)
exec "$(dirname "$0")/scripts/web/build.sh" "$@"
