#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 EVIDENCE_DIR" >&2
  exit 64
fi

evidence=$1
mkdir -p "$evidence"

adb wait-for-device
adb shell getprop >"$evidence/getprop.txt"
adb shell pm list features >"$evidence/features.txt"
adb shell pm list packages >"$evidence/packages.txt"
adb shell dumpsys print >"$evidence/dumpsys-print.txt" 2>&1 || true
adb shell settings get secure enabled_print_services   >"$evidence/enabled-print-services.txt" 2>&1 || true
adb shell dumpsys package com.android.bips   >"$evidence/bips-package.txt" 2>&1 || true
adb shell dumpsys package com.android.printspooler   >"$evidence/printspooler-package.txt" 2>&1 || true

python3 - "$evidence" <<'PY'
import json
import pathlib
import re
import sys

p = pathlib.Path(sys.argv[1])
features = (p / "features.txt").read_text(errors="replace")
packages = (p / "packages.txt").read_text(errors="replace")
dump = (p / "dumpsys-print.txt").read_text(errors="replace")
enabled = (p / "enabled-print-services.txt").read_text(errors="replace").strip()
result = {
    "feature_printing": "android.software.print" in features,
    "print_spooler_present": "package:com.android.printspooler" in packages,
    "default_print_service_bips_present": "package:com.android.bips" in packages,
    "enabled_print_services": enabled,
    "dumpsys_print_available": bool(dump.strip()) and "Can't find service" not in dump,
    "android_release": "",
    "sdk": "",
}
props = (p / "getprop.txt").read_text(errors="replace")
for key, field in [
    ("ro.build.version.release", "android_release"),
    ("ro.build.version.sdk", "sdk"),
]:
    match = re.search(rf"\[{re.escape(key)}\]: \[(.*?)\]", props)
    if match:
        result[field] = match.group(1)

(p / "result.json").write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
print(json.dumps(result, sort_keys=True))
PY
