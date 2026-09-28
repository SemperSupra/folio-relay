#!/bin/sh
# Qualification-only CUPS backend. Production, if CUPS wins the bake-off, will
# use a role-specific static Go backend with the same contract.
set -eu

if [ "$#" -eq 0 ]; then
  printf 'direct foliorelay:/ "FolioRelay" "FolioRelay PDF ingress" "" ""\n'
  exit 0
fi

if [ "$#" -lt 5 ]; then
  echo "ERROR: invalid CUPS backend invocation" >&2
  exit 1
fi

job_id="$1"
copies="$4"
tmp=""
if [ "$#" -ge 6 ] && [ -n "$6" ]; then
  source="$6"
else
  tmp="$(mktemp "${TMPDIR:-/tmp}/foliorelay-cups.XXXXXX")"
  trap 'rm -f "$tmp"' EXIT HUP INT TERM
  cat >"$tmp"
  source="$tmp"
fi

: "${FOLIORELAY_INGEST_BIN:?FOLIORELAY_INGEST_BIN is required}"
export FOLIORELAY_SUBSTRATE=cups
export FOLIORELAY_SOURCE_JOB_ID="$job_id"
export FOLIORELAY_MEDIA_TYPE=application/pdf
export FOLIORELAY_COPIES="$copies"

exec "$FOLIORELAY_INGEST_BIN" "$source"
