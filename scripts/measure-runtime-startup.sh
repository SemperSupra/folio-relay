#!/bin/sh
set -eu

if [ "$#" -lt 4 ] || [ "$#" -gt 5 ]; then
  echo "usage: $0 COMPOSE_FILE SERVICE IPP_URI OUTPUT_DIR [REPS]" >&2
  exit 64
fi

compose_file=$1
service=$2
ipp_uri=$3
output_dir=$4
reps=${5:-3}

mkdir -p "$output_dir"
testfile="${FOLIORELAY_IPP_READINESS_TEST:-}"
if [ -z "$testfile" ]; then
  testfile="$(find /usr/share/cups -type f -name get-printer-attributes.test -print -quit)"
fi
test -n "$testfile"
test -f "$testfile"

: >"$output_dir/startup-latency-ms.tsv"

rep=1
while [ "$rep" -le "$reps" ]; do
  docker compose -f "$compose_file" stop "$service" >/dev/null

  start_ns="$(date +%s%N)"
  docker compose -f "$compose_file" start "$service" >/dev/null

  ready=0
  attempt=1
  while [ "$attempt" -le 300 ]; do
    if ipptool -q "$ipp_uri" "$testfile" >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 0.1
    attempt=$((attempt + 1))
  done
  end_ns="$(date +%s%N)"

  if [ "$ready" -ne 1 ]; then
    echo "service $service did not become IPP-ready during startup rep $rep" >&2
    exit 1
  fi

  elapsed_ms=$(( (end_ns - start_ns) / 1000000 ))
  printf '%s\t%s\n' "$rep" "$elapsed_ms" \
    | tee -a "$output_dir/startup-latency-ms.tsv"
  rep=$((rep + 1))
done

python3 - "$output_dir/startup-latency-ms.tsv" "$output_dir/startup-latency.json" <<'PY'
import json
import pathlib
import statistics
import sys

rows = []
for line in pathlib.Path(sys.argv[1]).read_text().splitlines():
    if not line:
        continue
    rep, ms = line.split("\t", 1)
    rows.append({"rep": int(rep), "milliseconds": int(ms)})

values = [row["milliseconds"] for row in rows]
summary = {
    "repetitions": rows,
    "min_ms": min(values),
    "median_ms": statistics.median(values),
    "max_ms": max(values),
}
pathlib.Path(sys.argv[2]).write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")
print(json.dumps(summary, sort_keys=True))
PY
