#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 EVIDENCE_DIR APK" >&2
  exit 64
fi

evidence=$1
apk=$2
ui=(python3 scripts/mobile/android-ui.py --evidence "$evidence/ui")
mkdir -p "$evidence"

capture_exit() {
  rc=$?
  set +e
  adb shell dumpsys print >"$evidence/print-on-exit.txt" 2>&1
  adb shell logcat -d -v time >"$evidence/logcat-on-exit.txt" 2>&1
  exit "$rc"
}
trap capture_exit EXIT

adb wait-for-device
adb install -r "$apk" | tee "$evidence/apk-install.txt"
adb shell getprop >"$evidence/getprop.txt"
adb shell logcat -c || true

# Do not evaluate discovery before the furnished emulator network is usable.
ready=0
for i in $(seq 1 60); do
  {
    echo "attempt=$i"
    adb shell ip route || true
    adb shell ping -c 1 -W 2 10.0.2.2 || true
    if [ -n "${HOST_IPV4:-}" ]; then
      adb shell ping -c 1 -W 2 "$HOST_IPV4" || true
    fi
  } >>"$evidence/network-readiness.txt" 2>&1
  if adb shell ping -c 1 -W 2 10.0.2.2 >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
test "$ready" -eq 1

# Exercise stock BIPS discovery first. The workflow publishes the exact CUPS
# queue using its authoritative IPP printer-uuid and rp path.
adb shell am start -W -a android.settings.ACTION_PRINT_SETTINGS \
  >"$evidence/print-settings-launch.txt" 2>&1
"${ui[@]}" tap contains "Default Print Service" --timeout 20
"${ui[@]}" wait contains "FolioRelay" --timeout 60
"${ui[@]}" snapshot --label cups-discovered
adb shell dumpsys print >"$evidence/print-after-discovery.txt" 2>&1 || true

before="$(curl -fsS http://127.0.0.1:18080/v1/stats)"
printf '%s\n' "$before" >"$evidence/stats-before.json"

# Launch a real app PrintManager job and select the discovered printer through
# the system PrintSpooler registry.
adb shell am force-stop org.sempersupra.foliorelay.printprobe || true
adb shell am start -W -n org.sempersupra.foliorelay.printprobe/.MainActivity \
  >"$evidence/probe-launch.txt" 2>&1

"${ui[@]}" wait res "com.android.printspooler:id/destination_spinner" --timeout 20
"${ui[@]}" tap res "com.android.printspooler:id/destination_spinner" --timeout 10
"${ui[@]}" tap contains "All printers" --timeout 15
"${ui[@]}" wait contains "FolioRelay" --timeout 45
"${ui[@]}" tap contains "FolioRelay" --timeout 15
adb shell dumpsys print >"$evidence/print-after-selection.txt" 2>&1 || true

"${ui[@]}" wait contains "FolioRelay" --timeout 20
"${ui[@]}" wait res "com.android.printspooler:id/print_button" --timeout 15
"${ui[@]}" snapshot --label print-preview-live
"${ui[@]}" tap res "com.android.printspooler:id/print_button" --timeout 10

if "${ui[@]}" wait contains "Use Default Print Service?" --timeout 5; then
  "${ui[@]}" snapshot --label default-print-service-confirmation
  "${ui[@]}" tap text "OK" --timeout 5
fi

accepted=0
for i in $(seq 1 60); do
  after="$(curl -fsS http://127.0.0.1:18080/v1/stats)"
  b="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["accepted"])' <<<"$before")"
  a="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["accepted"])' <<<"$after")"
  adb shell dumpsys print >"$evidence/print-submit-state-$(printf '%02d' "$i").txt" 2>&1 || true
  if [ "$a" -gt "$b" ]; then
    accepted=1
    break
  fi
  sleep 1
done

after="$(curl -fsS http://127.0.0.1:18080/v1/stats)"
printf '%s\n' "$after" >"$evidence/stats-after.json"
adb shell dumpsys print >"$evidence/print-after-job.txt" 2>&1 || true
adb shell logcat -d -v time >"$evidence/logcat.txt" 2>&1 || true
"${ui[@]}" snapshot --label final || true

python3 - "$before" "$after" "$accepted" <<'PY'
import json, sys
before=json.loads(sys.argv[1])
after=json.loads(sys.argv[2])
assert sys.argv[3] == "1", (before, after)
assert after["accepted"] > before["accepted"], (before, after)
assert after["conflicts"] == before["conflicts"], (before, after)
PY
