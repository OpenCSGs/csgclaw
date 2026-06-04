#!/usr/bin/env bash
exec "$(cd "$(dirname "$0")" && pwd)/prepare-docker-embed-dist.sh" "$@"
