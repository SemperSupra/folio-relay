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

# Capture the durable baseline before launching the real Android PrintManager
# client. Printer setup itself must never advance FolioRelay durable state.
before="$(curl -fsS http://127.0.0.1:18081/v1/stats)"
printf '%s\n' "$before" >"$evidence/stats-before.json"

# Start the probe first so PrintSpooler owns the active discovery/selection
# session while the manual printer is added. This mirrors the user journey:
# Print -> All printers -> Add printer -> Default Print Service -> manual IP.
adb shell am force-stop org.sempersupra.foliorelay.printprobe || true
adb shell am start -W -n org.sempersupra.foliorelay.printprobe/.MainActivity \
  >"$evidence/probe-launch.txt" 2>&1

"${ui[@]}" wait res "com.android.printspooler:id/destination_spinner" --timeout 20
"${ui[@]}" tap res "com.android.printspooler:id/destination_spinner" --timeout 10
"${ui[@]}" tap contains "All printers" --timeout 15
"${ui[@]}" snapshot --label all-printers-before-add
adb shell dumpsys print >"$evidence/print-all-printers-before-add.txt" 2>&1 || true

# SelectPrinterActivity exposes an Add Printer action for enabled print
# services with a declared add-printers activity. Prefer its accessibility
# description and retain a text fallback for platform rendering differences.
if ! "${ui[@]}" tap desc "Add printer" --timeout 15; then
  "${ui[@]}" tap contains "Add printer" --timeout 8
fi
"${ui[@]}" wait contains "Choose print service" --timeout 10 || true
"${ui[@]}" tap contains "Default Print Service" --timeout 15

# BIPS is now launched by PrintSpooler itself, not by a privileged shell or
# through Settings. Add the candidate while the printer-selection workflow is
# already active.
"${ui[@]}" wait contains "Add printer by IP address" --timeout 15
"${ui[@]}" tap contains "Add printer by IP address" --timeout 8

# BIPS renders 192.168.0.4 as the EditText hint. UIAutomator exposes that
# hint through the node text attribute even though the field is actually empty.
# Whole-string shell input proved lossy under hosted-emulator load, so type
# paced key events and require exact text before allowing capability probing.
"${ui[@]}" wait res "com.android.bips:id/hostname" --timeout 10
sleep 1

type_emulator_host() {
  local key
  for key in \
    KEYCODE_1 KEYCODE_0 KEYCODE_PERIOD KEYCODE_0 \
    KEYCODE_PERIOD KEYCODE_2 KEYCODE_PERIOD KEYCODE_2
  do
    adb shell input keyevent "$key"
    sleep 0.20
  done
}

type_emulator_host
if ! "${ui[@]}" wait text "10.0.2.2" --timeout 4; then
  adb shell input keyevent KEYCODE_MOVE_END
  for _ in $(seq 1 16); do
    adb shell input keyevent KEYCODE_DEL
    sleep 0.05
  done
  sleep 0.5
  type_emulator_host
  "${ui[@]}" wait text "10.0.2.2" --timeout 10
fi
"${ui[@]}" tap text "Add" --timeout 15

# A successful BIPS Add is authoritative when the real print service reports
# the printer and capabilities. Android 15 may immediately return from BIPS to
# PrintSpooler's Add-printer service chooser, so foreground UI text is not a
# stable success oracle. Poll dumpsys instead and retain every attempt.
bips_added=0
for i in $(seq 1 45); do
  receipt="$evidence/print-after-bips-add-attempt-$(printf '%02d' "$i").txt"
  adb shell dumpsys print >"$receipt" 2>&1 || true
  if grep -q 'name=FolioRelay' "$receipt" && grep -q 'status=1' "$receipt"; then
    cp "$receipt" "$evidence/print-after-bips-add.txt"
    bips_added=1
    break
  fi
  sleep 1
done
if [ "$bips_added" -ne 1 ]; then
  echo 'BIPS never reported FolioRelay IDLE after Add' >&2
  exit 1
fi
"${ui[@]}" snapshot --label bips-add-return-state

# BIPS can either remain foreground or return automatically. If its activity
# is still foreground, go back once; otherwise preserve the current chooser.
if "${ui[@]}" wait contains "Add printer by IP address" --timeout 2; then
  adb shell input keyevent KEYCODE_BACK
  sleep 1
fi
# Returning from BIPS can reveal the PrintSpooler Add printer service chooser
# that launched it. On the hosted Android 15 flow that chooser is a modal over
# the still-live All Printers activity, so its window hides the actionable
# FolioRelay row from UIAutomator even though the row is already present behind
# it. Dismiss only that chooser if it is still active; do not blindly send a
# second BACK that could leave All Printers when the chooser already closed.
if "${ui[@]}" wait contains "Default Print Service" --timeout 3; then
  "${ui[@]}" snapshot --label add-printer-chooser-returned
  adb shell input keyevent KEYCODE_BACK
  sleep 1
fi
"${ui[@]}" wait contains "All printers" --timeout 10

# The selection row must be actionable. android-ui.py tap filters disabled
# nodes, so this refuses to select a stale STATUS_UNAVAILABLE printer.
"${ui[@]}" snapshot --label all-printers-after-add
"${ui[@]}" tap contains "FolioRelay" --timeout 30
adb shell dumpsys print >"$evidence/print-after-live-selection.txt" 2>&1 || true

# Back in PrintActivity, wait for the live destination and the real print
# control. Preserve intermediate dumpsys receipts so a discovery regression is
# classifiable without guessing from a missing button.
"${ui[@]}" wait contains "FolioRelay" --timeout 20
for i in $(seq 1 15); do
  adb shell dumpsys print >"$evidence/print-preview-state-$(printf '%02d' "$i").txt" 2>&1 || true
  if "${ui[@]}" wait res "com.android.printspooler:id/print_button" --timeout 1; then
    break
  fi
  sleep 1
done

"${ui[@]}" snapshot --label print-preview-live
if ! "${ui[@]}" tap res "com.android.printspooler:id/print_button" --timeout 10; then
  # Accessibility text is the semantic fallback if this Android build renders
  # the control under a different resource id.
  "${ui[@]}" tap desc "Print" --timeout 5
fi

# On a clean Android profile, the first native submission through the newly
# enabled Default Print Service can require an explicit framework confirmation:
# "Use Default Print Service?". This is system PrintSpooler policy UI, not a
# candidate prompt. Acknowledge it only when that exact dialog is present.
if "${ui[@]}" wait contains "Use Default Print Service?" --timeout 5; then
  "${ui[@]}" snapshot --label default-print-service-confirmation
  "${ui[@]}" tap text "OK" --timeout 5
  adb shell dumpsys print >"$evidence/print-after-service-confirmation.txt" 2>&1 || true
fi

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
