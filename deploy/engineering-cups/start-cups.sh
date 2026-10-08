#!/bin/sh
set -eu

instance_file=/var/lib/cups/foliorelay-substrate-instance
if [ ! -s "$instance_file" ]; then
  umask 077
  printf 'cups-%s\n' "$(cat /proc/sys/kernel/random/uuid)" >"$instance_file"
fi

export FOLIORELAY_SUBSTRATE_INSTANCE="$(cat "$instance_file")"

runtime_root=/etc/cups
# The active CUPS ServerRoot is a bounded ephemeral projection. Durable CUPS
# state remains under /var/lib/cups and spool state under /var/spool/cups.
mkdir -p \
  "$runtime_root/ppd" \
  /var/lib/cups/ssl \
  /var/spool/cups/tmp
chmod 0700 /var/lib/cups/ssl
chmod 0750 /var/spool/cups/tmp

if [ -n "${FOLIORELAY_IDENTITY_FILE:-}" ]; then
  wait_seconds="${FOLIORELAY_IDENTITY_WAIT_SECONDS:-60}"
  case "$wait_seconds" in
    ""|*[!0-9]*)
      echo "ERROR: invalid FOLIORELAY_IDENTITY_WAIT_SECONDS: $wait_seconds" >&2
      exit 64
      ;;
  esac
  waited=0
  while [ ! -s "$FOLIORELAY_IDENTITY_FILE" ]; do
    if [ "$waited" -ge "$wait_seconds" ]; then
      echo "ERROR: FolioRelay identity did not become available: $FOLIORELAY_IDENTITY_FILE" >&2
      exit 69
    fi
    sleep 1
    waited=$((waited + 1))
  done

  /usr/local/bin/foliorelay-cups-config \
    -identity-file "$FOLIORELAY_IDENTITY_FILE" \
    -cupsd-template /usr/share/foliorelay/cups/cupsd.conf.template \
    -printers-template /usr/share/foliorelay/cups/printers.conf.template \
    -ppd-source /usr/share/foliorelay/cups/FolioRelay.ppd \
    -output-root "$runtime_root"
  cp /usr/share/foliorelay/cups/cups-files.conf.template "$runtime_root/cups-files.conf"
  chmod 0600 "$runtime_root/cups-files.conf"
else
  # Compatibility path for isolated substrate qualification. Production
  # FolioRelay supplies the durable identity file and uses the branch above.
  public_host="${FOLIORELAY_PUBLIC_HOST:-localhost}"
  case "$public_host" in
    ""|*[!A-Za-z0-9.-]*|.*|*..*|*.)
      echo "ERROR: invalid FOLIORELAY_PUBLIC_HOST: $public_host" >&2
      exit 64
      ;;
  esac
  sed "s/__FOLIORELAY_PUBLIC_HOST__/$public_host/g" \
    /usr/share/foliorelay/cups/cupsd.conf.template >"$runtime_root/cupsd.conf"
  sed '/__FOLIORELAY_PRINTER_UUID__/d' \
    /usr/share/foliorelay/cups/printers.conf.template >"$runtime_root/printers.conf"
  cp /usr/share/foliorelay/cups/FolioRelay.ppd "$runtime_root/ppd/FolioRelay.ppd"
  cp /usr/share/foliorelay/cups/cups-files.conf.template "$runtime_root/cups-files.conf"
  chmod 0600 "$runtime_root/cupsd.conf" "$runtime_root/printers.conf" "$runtime_root/ppd/FolioRelay.ppd" "$runtime_root/cups-files.conf"
fi

/usr/sbin/cupsd -t -c "$runtime_root/cupsd.conf" -s "$runtime_root/cups-files.conf"
exec /usr/sbin/cupsd -f -c "$runtime_root/cupsd.conf" -s "$runtime_root/cups-files.conf"
