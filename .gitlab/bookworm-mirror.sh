#!/usr/bin/env bash
# Rewrite Debian bookworm apt sources to Aliyun (or DEBIAN_APT_MIRROR_*). Used by GitLab CI.
set -euo pipefail

deb="${DEBIAN_APT_MIRROR_DEB:-http://mirrors.aliyun.com/debian}"
sec="${DEBIAN_APT_MIRROR_SEC:-http://mirrors.aliyun.com/debian-security}"

rewrite_sources() {
  local f="$1"
  [ -f "$f" ] || return 0
  sed -i \
    -e "s|https://deb.debian.org/debian|${deb}|g" \
    -e "s|http://deb.debian.org/debian|${deb}|g" \
    -e "s|https://security.debian.org/debian-security|${sec}|g" \
    -e "s|http://security.debian.org/debian-security|${sec}|g" \
    "$f"
}

rewrite_sources /etc/apt/sources.list
if [ -d /etc/apt/sources.list.d ]; then
  for f in /etc/apt/sources.list.d/*; do
    [ -f "$f" ] || continue
    case "$f" in
      *.list|*.sources) rewrite_sources "$f" ;;
    esac
  done
fi
