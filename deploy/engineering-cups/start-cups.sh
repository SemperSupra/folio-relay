#!/bin/sh
set -eu

instance_file=/var/lib/cups/foliorelay-substrate-instance
if [ ! -s "$instance_file" ]; then
  umask 077
  printf 'cups-%s\n' "$(cat /proc/sys/kernel/random/uuid)" >"$instance_file"
fi

export FOLIORELAY_SUBSTRATE_INSTANCE="$(cat "$instance_file")"

public_host="${FOLIORELAY_PUBLIC_HOST:-localhost}"
case "$public_host" in
  ""|*[!A-Za-z0-9.-]*|.*|*..*|*.)
    echo "ERROR: invalid FOLIORELAY_PUBLIC_HOST: $public_host" >&2
    exit 64
    ;;
esac

runtime_conf=/var/lib/cups/foliorelay-cupsd.conf
sed "s/__FOLIORELAY_PUBLIC_HOST__/$public_host/g" /etc/cups/cupsd.conf >"$runtime_conf"
chmod 0600 "$runtime_conf"

exec /usr/sbin/cupsd -f -c "$runtime_conf" -s /etc/cups/cups-files.conf
