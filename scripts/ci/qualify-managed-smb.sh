#!/usr/bin/env bash
# Disposable Ubuntu runner SMB qualification. No production paths, accounts,
# credentials, network shares or TrueNAS credentials are used.
set -euo pipefail
if [[ "$(id -u)" -eq 0 ]]; then echo "run as the unprivileged hosted runner" >&2; exit 78; fi
for bin in smbd smbclient smbpasswd testparm setfacl; do
  command -v "$bin" >/dev/null || { echo "missing dependency: $bin" >&2; exit 78; }
done
work="$(mktemp -d)"
server_pid=""
cleanup() {
  if [[ -n "$server_pid" ]]; then sudo kill "$server_pid" 2>/dev/null || true; fi
  sudo rm -rf -- "$work"
  sudo userdel folio_ci_rw1 2>/dev/null || true
  sudo userdel folio_ci_rw2 2>/dev/null || true
  sudo userdel folio_ci_denied 2>/dev/null || true
  sudo groupdel folio_ci_smb 2>/dev/null || true
}
trap cleanup EXIT
canon="$work/canonical"
exports="$work/exports"
mkdir -p -- "$canon" "$exports"
sudo groupadd folio_ci_smb
for user in folio_ci_rw1 folio_ci_rw2 folio_ci_denied; do
  sudo useradd -M -s /usr/sbin/nologin "$user"
done
sudo usermod -a -G folio_ci_smb folio_ci_rw1
sudo usermod -a -G folio_ci_smb folio_ci_rw2
# Export root group controls correspond to separate post-install filesystem
# grants. The denied account is explicitly excluded from both layers.
chmod 0711 -- "$work"
sudo chgrp folio_ci_smb "$exports"
chmod 2770 -- "$exports"
chmod 0700 -- "$canon"
export_name="$(go run ./tools/qualify-smb-output -canonical "$canon" -export "$exports")"
test -f "$exports/$export_name"
file_mode="$(stat -c '%a' "$exports/$export_name")"
test "$file_mode" = "660" || { echo "wrong exported mode $file_mode" >&2; exit 1; }
source_path="$(find "$canon/sha256" -type f -print -quit)"
canonical_before="$(sha256sum "$source_path" | cut -d' ' -f1)"
echo "Fixture file and canonical digest verified (no identity/secret output)"

# Samba requires a private 0700 passdb root and distinct traversable state
# directories; neither location is shared to SMB clients.
sudo mkdir -p "$work/private" "$work/lock" "$work/state" "$work/cache"
sudo chmod 0700 "$work/private"
sudo chmod 0755 "$work/lock" "$work/state" "$work/cache"
config="$work/smb.conf"
cat >"$config" <<EOF
[global]
    workgroup = WORKGROUP
    server role = standalone server
    map to guest = Never
    bind interfaces only = yes
    interfaces = lo 127.0.0.1
    log file = $work/samba-%m.log
    pid directory = $work
    lock directory = $work/lock
    state directory = $work/state
    cache directory = $work/cache
    private dir = $work/private
    server min protocol = SMB2
[foliorelay-fixture]
    path = $exports
    browseable = yes
    read only = no
    guest ok = no
    valid users = folio_ci_rw1 folio_ci_rw2
    force group = folio_ci_smb
EOF
testparm -s "$config" >/dev/null
# Use the same isolated passdb/config as smbd; no account password in argv.
secret="$(openssl rand -hex 20)"
for user in folio_ci_rw1 folio_ci_rw2 folio_ci_denied; do
  if ! printf '%s\n%s\n' "$secret" "$secret" | sudo smbpasswd -c "$config" -a -s "$user" >"$work/passdb-result.log" 2>&1; then
    echo "synthetic Samba passdb enrollment failed" >&2
    exit 1
  fi
  auth="$work/$user.auth"
  printf 'username = %s\npassword = %s\n' "$user" "$secret" > "$auth"
  chmod 0600 "$auth"
done
unset secret

# Launch in Samba's own daemon process group. Foreground/no-process-group
# can terminate the hosted Actions step if smbd tears down its inherited group.
# Store service diagnostics only in this disposable root.
sudo smbd --configfile="$config" --daemon >"$work/smbd-launch.log" 2>&1
server_pid="$(cat "$work/smbd.pid" 2>/dev/null || true)"
# Bounded local readiness polling; never reach a LAN or external host.
ready=0
for i in $(seq 1 30); do
  if smbclient //127.0.0.1/foliorelay-fixture \
      -A "$work/folio_ci_rw1.auth" -c 'ls' >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
if [[ "$ready" -ne 1 ]]; then
  echo "Samba fixture never became reachable" >&2
  sed -n '1,60p' "$work/smbd-launch.log" >&2 || true
  exit 1
fi

for user in folio_ci_rw1 folio_ci_rw2; do
  smbclient //127.0.0.1/foliorelay-fixture -A "$work/$user.auth" \
    -c "get $export_name $work/$user.download" >/dev/null
  cmp "$source_path" "$work/$user.download"
done

# Both allowed users have genuine SMB write/delete capability; shared output
# must never expose the canonical artifact directory or attached credentials.
printf 'synthetic SMB client write\n' > "$work/user-upload.txt"
smbclient //127.0.0.1/foliorelay-fixture -A "$work/folio_ci_rw1.auth" \
  -c "put $work/user-upload.txt user-upload.txt" >/dev/null
smbclient //127.0.0.1/foliorelay-fixture -A "$work/folio_ci_rw2.auth" \
  -c 'del user-upload.txt' >/dev/null

if smbclient //127.0.0.1/foliorelay-fixture -A "$work/folio_ci_denied.auth" \
  -c 'ls' >/dev/null 2>&1; then
  echo "UNAUTHORIZED SMB account was admitted" >&2
  exit 1
fi

smbclient //127.0.0.1/foliorelay-fixture -A "$work/folio_ci_rw1.auth" \
  -c "rename $export_name renamed-by-client.pdf; del renamed-by-client.pdf" >/dev/null
test ! -e "$exports/$export_name"
canonical_after="$(sha256sum "$source_path" | cut -d' ' -f1)"
test "$canonical_before" = "$canonical_after" || {
  echo "canonical artifact was changed by SMB mutation" >&2; exit 1;
}
test -f "$source_path"
echo "PASS: named-user SMB read/write/rename/delete, unrelated user DENIED, canonical unchanged"
