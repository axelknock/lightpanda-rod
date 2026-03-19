#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

binary_path="${LIGHTPANDA_BINARY:-$repo_root/.tmp/lightpanda}"
timeout_seconds="${LIGHTPANDA_TIMEOUT_SECONDS:-30}"
host="${LIGHTPANDA_HOST:-127.0.0.1}"
port="${LIGHTPANDA_PORT:-0}"

nightly_url() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"

  case "${os}/${arch}" in
    Linux/x86_64)
      printf '%s\n' "https://github.com/lightpanda-io/browser/releases/download/nightly/lightpanda-x86_64-linux"
      ;;
    Darwin/arm64)
      printf '%s\n' "https://github.com/lightpanda-io/browser/releases/download/nightly/lightpanda-aarch64-macos"
      ;;
    *)
      printf 'unsupported platform for nightly download: %s/%s\n' "$os" "$arch" >&2
      return 1
      ;;
  esac
}

ensure_binary() {
  if [[ -x "$binary_path" ]]; then
    return 0
  fi

  mkdir -p "$(dirname "$binary_path")"

  local url
  url="$(nightly_url)"

  printf 'downloading lightpanda nightly to %s\n' "$binary_path"
  curl -L --fail --output "$binary_path" "$url"
  chmod a+x "$binary_path"
}

ensure_binary

cd "$repo_root"
go run ./scripts/test-real-binary \
  -binary "$binary_path" \
  -host "$host" \
  -port "$port" \
  -timeout "${timeout_seconds}s"
