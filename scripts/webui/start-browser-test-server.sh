#!/usr/bin/env bash
set -euo pipefail

root="${FOLIORELAY_WEBUI_TEST_ROOT:-${RUNNER_TEMP:-/tmp}/foliorelay-webui-test}"
listen="${FOLIORELAY_WEBUI_LISTEN:-127.0.0.1:18080}"
token="${FOLIORELAY_TEST_TOKEN:-0123456789abcdef0123456789abcdef}"

rm -rf "$root"
mkdir -p "$root/state" "$root/artifacts"
printf '%s\n' "$token" >"$root/token"
chmod 0600 "$root/token"

exec go run ./cmd/foliorelayd \
  -listen "$listen" \
  -journal "$root/state/journal.frj" \
  -artifact-store "$root/artifacts" \
  -token-file "$root/token" \
  -identity-file "$root/printer.json" \
  -printer-uri "ipp://foliorelay.test:8634/printers/FolioRelay" \
  -printer-name "FolioRelay" \
  -printer-location "Browser qualification fixture" \
  -airprint
