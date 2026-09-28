#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 OUTPUT_DIR" >&2
  exit 64
fi

out=$1
sdk_root=${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}
if [ -z "$sdk_root" ]; then
  echo "ANDROID_SDK_ROOT/ANDROID_HOME is not set" >&2
  exit 1
fi

platform="$sdk_root/platforms/android-35/android.jar"
test -f "$platform"
build_tools="$(find "$sdk_root/build-tools" -mindepth 1 -maxdepth 1 -type d -print | sort -V | tail -1)"
test -n "$build_tools"
test -x "$build_tools/aapt2"
test -x "$build_tools/d8"
test -x "$build_tools/zipalign"
test -x "$build_tools/apksigner"

rm -rf "$out"
mkdir -p "$out/classes" "$out/dex"
manifest="$PWD/experiments/mobile/android-print-probe/AndroidManifest.xml"
source="$PWD/experiments/mobile/android-print-probe/src/org/sempersupra/foliorelay/printprobe/MainActivity.java"

"$build_tools/aapt2" link   -I "$platform"   --manifest "$manifest"   --min-sdk-version 23   --target-sdk-version 35   -o "$out/base.apk"

javac --release 17   -classpath "$platform"   -d "$out/classes"   "$source"

mapfile -t class_files < <(find "$out/classes" -type f -name '*.class' -print | sort)
"$build_tools/d8"   --min-api 23   --lib "$platform"   --output "$out/dex"   "${class_files[@]}"

cp "$out/base.apk" "$out/unsigned.apk"
zip -q -j "$out/unsigned.apk" "$out/dex/classes.dex"
"$build_tools/zipalign" -f 4 "$out/unsigned.apk" "$out/aligned.apk"

keytool -genkeypair -noprompt   -keystore "$out/debug.keystore"   -storepass android   -keypass android   -alias androiddebugkey   -keyalg RSA   -keysize 2048   -validity 10000   -dname 'CN=FolioRelay Android Probe,O=SemperSupra,C=US' >/dev/null 2>&1

"$build_tools/apksigner" sign   --ks "$out/debug.keystore"   --ks-key-alias androiddebugkey   --ks-pass pass:android   --key-pass pass:android   --out "$out/FolioRelayPrintProbe.apk"   "$out/aligned.apk"

"$build_tools/apksigner" verify --verbose "$out/FolioRelayPrintProbe.apk"
