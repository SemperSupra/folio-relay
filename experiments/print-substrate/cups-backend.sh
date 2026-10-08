#!/bin/sh
# FolioRelay CUPS backend. Preserve the submitted source artifact and pass the
# final CUPS MIME type into the FolioRelay ingest contract.
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

: "${FOLIORELAY_INGEST_BIN:?FOLIORELAY_INGEST_BIN is required}"
media_type="${FINAL_CONTENT_TYPE:-${CONTENT_TYPE:-application/octet-stream}}"
case "$media_type" in
  application/pdf|image/urf) ;;
  *)
    echo "ERROR: unsupported FolioRelay media type: $media_type" >&2
    exit 1
    ;;
esac

export FOLIORELAY_SUBSTRATE=cups
export FOLIORELAY_SOURCE_JOB_ID="$job_id"
export FOLIORELAY_MEDIA_TYPE="$media_type"
export FOLIORELAY_COPIES="$copies"

exec "$FOLIORELAY_INGEST_BIN" "$source"
