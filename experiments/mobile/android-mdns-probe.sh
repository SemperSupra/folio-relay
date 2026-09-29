#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 EVIDENCE_DIR SERVICE_NAME" >&2
  exit 64
fi

evidence=$1
service_name=$2
ui=(python3 scripts/mobile/android-ui.py --evidence "$evidence/ui")
mkdir -p "$evidence"

adb wait-for-device
adb shell getprop >"$evidence/getprop.txt"
adb shell dumpsys print >"$evidence/print-before.txt" 2>&1 || true
adb shell logcat -c || true

host_ipv4=${HOST_IPV4:-}
{
  echo "host_ipv4=$host_ipv4"
  adb shell ping -c 1 -W 2 10.0.2.2 || true
  if [ -n "$host_ipv4" ]; then
    adb shell ping -c 1 -W 2 "$host_ipv4" || true
  fi
} >"$evidence/reachability.txt" 2>&1

adb shell am start -W -a android.settings.ACTION_PRINT_SETTINGS   >"$evidence/print-settings-launch.txt" 2>&1
# Hosted emulator load has produced a known Pixel Launcher ANR modal while
# Print Settings is otherwise healthy. Dismiss only that exact system-noise
# dialog before evaluating the real BIPS discovery surface.
if "${ui[@]}" wait text "Pixel Launcher isn't responding" --timeout 3; then
  "${ui[@]}" tap text "Close app" --timeout 5
  sleep 1
fi
"${ui[@]}" tap contains "Default Print Service" --timeout 20
sleep 3
"${ui[@]}" snapshot --label default-print-service

discovered=false
if "${ui[@]}" wait contains "$service_name" --timeout 60; then
  discovered=true
  "${ui[@]}" snapshot --label mdns-discovered
else
  "${ui[@]}" snapshot --label mdns-not-discovered || true
fi

adb shell dumpsys print >"$evidence/print-after.txt" 2>&1 || true
adb shell logcat -d -v time >"$evidence/logcat.txt" 2>&1 || true

python3 - "$evidence/result.json" "$discovered" "$host_ipv4" <<'PY'
import json, pathlib, sys
out, discovered, host_ipv4 = sys.argv[1:]
result = {
    "platform": "android-15-api35",
    "native_discovery": "NsdManager via stock BIPS",
    "service": "external exact CUPS-style _ipp._tcp advertisement",
    "discovered": discovered == "true",
    "host_ipv4": host_ipv4 or None,
    "classification": "reachable" if discovered == "true" else "hosted-mdns-boundary-or-service-unseen",
    "candidate_qualification": "not-evaluated",
}
pathlib.Path(out).write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
print(json.dumps(result, sort_keys=True))
PY
