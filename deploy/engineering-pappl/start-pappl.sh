#!/bin/sh
set -eu

instance_file=/var/lib/pappl/foliorelay-substrate-instance
if [ ! -s "$instance_file" ]; then
  umask 077
  printf 'pappl-%s\n' "$(cat /proc/sys/kernel/random/uuid)" >"$instance_file"
fi

export FOLIORELAY_SUBSTRATE_INSTANCE="$(cat "$instance_file")"

exec /usr/local/bin/foliorelay-pappl-ingress
