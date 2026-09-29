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
  if command -v sudo >/dev/null 2>&1; then
    sudo pkill -INT -x tcpdump >/dev/null 2>&1 || true
  fi
  wait >/dev/null 2>&1 || true
  exit "$rc"
}
trap capture_exit EXIT

if command -v tcpdump >/dev/null 2>&1; then
  sudo tcpdump -l -nn -i any 'tcp port 631' >"$evidence/tcp631.log" 2>&1 &
fi

adb wait-for-device
adb install -r "$apk" | tee "$evidence/apk-install.txt"
adb shell getprop >"$evidence/getprop.txt"
adb shell dumpsys print >"$evidence/print-before-setup.txt" 2>&1 || true
adb shell logcat -c || true

{
  echo "== toybox nc help =="
  adb shell toybox nc --help || true
  echo "== ping host alias =="
  adb shell ping -c 1 -W 2 10.0.2.2 || true
  echo "== tcp 631 probe =="
  set +e
  adb shell 'toybox nc -w 3 10.0.2.2 631 </dev/null'
  probe_rc=$?
  set -e
  echo "tcp_probe_rc=$probe_rc"
} >"$evidence/android-host-reachability.txt" 2>&1

# The stock BIPS service is the printer bridge under test. Try its privileged
# add-printer activity directly first; if shell cannot launch that protected
# activity, navigate through the system Print Settings UI instead.
set +e
adb shell am start -W -n com.android.bips/.ui.AddPrintersActivity   >"$evidence/add-printer-direct.txt" 2>&1
direct_rc=$?
set -e

if [ "$direct_rc" -ne 0 ]; then
  adb shell am start -W -a android.settings.ACTION_PRINT_SETTINGS     >"$evidence/print-settings-launch.txt" 2>&1
  "${ui[@]}" tap contains "Default Print Service" --timeout 20
  sleep 2
  if ! "${ui[@]}" tap contains "Add printer" --timeout 8; then
    "${ui[@]}" tap desc "More options" --timeout 8 || true
    "${ui[@]}" tap contains "Add printer" --timeout 8
  fi
fi

sleep 2
if "${ui[@]}" wait contains "Add printer by IP address" --timeout 8; then
  "${ui[@]}" tap contains "Add printer by IP address" --timeout 5
fi

# BIPS renders 192.168.0.4 as the EditText hint. UIAutomator exposes that\n# hint through the node text attribute even though the field is actually empty,\n# so clearing it would target placeholder text rather than user input.\nadb shell input text '10.0.2.2'\n"${ui[@]}" wait text "10.0.2.2" --timeout 10\n"${ui[@]}" tap text "Add" --timeout 15

# BIPS probes standard IPP URIs on port 631. The host maps the real PAPPL
# candidate to that port for this qualification only.
"${ui[@]}" wait contains "FolioRelay" --timeout 45
"${ui[@]}" snapshot --label printer-added
adb shell dumpsys print >"$evidence/print-after-add.txt" 2>&1 || true

before="$(curl -fsS http://127.0.0.1:18081/v1/stats)"
printf '%s\n' "$before" >"$evidence/stats-before.json"

adb shell am force-stop org.sempersupra.foliorelay.printprobe || true
adb shell am start -W -n org.sempersupra.foliorelay.printprobe/.MainActivity   >"$evidence/probe-launch.txt" 2>&1

# If FolioRelay is not already the selected destination, use the real system
# print-spooler destination picker and select the BIPS-discovered printer.
if ! "${ui[@]}" wait contains "FolioRelay" --timeout 12; then
  "${ui[@]}" tap res "com.android.printspooler:id/destination_spinner" --timeout 15
  "${ui[@]}" tap contains "FolioRelay" --timeout 20
fi

"${ui[@]}" snapshot --label print-preview
"${ui[@]}" tap res "com.android.printspooler:id/print_button" --timeout 30

accepted=0
for _ in $(seq 1 60); do
  after="$(curl -fsS http://127.0.0.1:18081/v1/stats)"
  b="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["accepted"])' <<<"$before")"
  a="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["accepted"])' <<<"$after")"
  if [ "$a" -gt "$b" ]; then
    accepted=1
    break
  fi
  sleep 1
done

after="$(curl -fsS http://127.0.0.1:18081/v1/stats)"
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
