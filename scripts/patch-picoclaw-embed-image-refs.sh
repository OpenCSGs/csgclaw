#!/usr/bin/env bash
exec "$(cd "$(dirname "$0")" && pwd)/patch-docker-embed-image-refs.sh" "$@"
