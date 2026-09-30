#!/bin/sh
# AirPrint/URF protocol experiment. Keep the submitted source format/bytes
# intact and hand them to the same FolioRelay ingest adapter.
set -eu

if [ "$#" -eq 0 ]; then
  printf 'direct foliorelay:/ "FolioRelay" "FolioRelay virtual printer" "" ""\n'
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

media_type="${FINAL_CONTENT_TYPE:-${CONTENT_TYPE:-}}"
case "$media_type" in
  application/pdf|image/urf)
    ;;
  *)
    echo "ERROR: unsupported final content type: ${media_type:-unset}" >&2
    exit 1
    ;;
esac

: "${FOLIORELAY_INGEST_BIN:?FOLIORELAY_INGEST_BIN is required}"
export FOLIORELAY_SUBSTRATE=cups
export FOLIORELAY_SOURCE_JOB_ID="$job_id"
export FOLIORELAY_MEDIA_TYPE="$media_type"
export FOLIORELAY_COPIES="$copies"

exec "$FOLIORELAY_INGEST_BIN" "$source"
