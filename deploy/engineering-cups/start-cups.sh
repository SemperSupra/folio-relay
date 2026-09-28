#!/bin/sh
set -eu

instance_file=/var/lib/cups/foliorelay-substrate-instance
if [ ! -s "$instance_file" ]; then
  umask 077
  printf 'cups-%s\n' "$(cat /proc/sys/kernel/random/uuid)" >"$instance_file"
fi

export FOLIORELAY_SUBSTRATE_INSTANCE="$(cat "$instance_file")"

exec /usr/sbin/cupsd -f -c /etc/cups/cupsd.conf
