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
export_base="$work/export-base"
exports="$export_base/shared"
mkdir -p -- "$canon" "$exports"
# Protected staging stays in the non-group-writable parent, outside the
# actual Samba-exported shared/ subtree.
chmod 0711 "$export_base"
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
# The CI runner is not itself in the permitted group. Only a privileged
# chmod can retain setgid on this group-owned fixture directory.
sudo chmod 2770 -- "$exports"
test "$(stat -c '%a' "$exports")" = "2770" || {
  echo "fixture setgid/group inheritance failed" >&2; exit 1;
}
chmod 0700 -- "$canon"
# The FolioRelay exporter must have an admitted primary share group.
# Exercise that exact synthetic identity without broadening host membership.
go build -trimpath -o "$work/qualify-smb-output" ./tools/qualify-smb-output
sudo usermod -a -G folio_ci_smb "$(id -un)"
# Launch a new, unprivileged process with the explicit synthetic group.
# Ubuntu runner sudoers grants root but not arbitrary -u/-g; sg consults the
# newly admitted /etc/group membership and does not require escalation.
export_name="$(sg folio_ci_smb -c "$work/qualify-smb-output -canonical $canon -export $export_base")"
case "$export_name" in
  shared/Prints/2026/*.pdf) ;;
  *) echo "unexpected managed export destination" >&2; exit 1 ;;
esac
smb_name="${export_name#shared/}"
test -d "$export_base/.foliorelay-private-stage"
test "$(stat -c '%a' "$export_base/.foliorelay-private-stage")" = "700"
for p in "$exports/Prints" "$exports/Prints/2026"; do
  test "$(stat -c '%a' "$p")" = "2770" || {
    echo "nested export folder lacks admitted setgid/group access" >&2
    stat -c '%a %U %G %n' "$p" >&2
    exit 1
  }
done
test -f "$export_base/$export_name"
file_mode="$(stat -c '%a' "$export_base/$export_name")"
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
echo "Samba fixture configuration validated"
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
echo "Samba synthetic passdb enrollment complete"

# Launch in Samba's own daemon process group. Foreground/no-process-group
# can terminate the hosted Actions step if smbd tears down its inherited group.
# Store service diagnostics only in this disposable root.
if ! sudo smbd --configfile="$config" --daemon >"$work/smbd-launch.log" 2>&1; then
  echo "Samba fixture daemon launch failed" >&2
  sed -n '1,60p' "$work/smbd-launch.log" >&2 || true
  exit 1
fi
echo "Samba fixture daemon launched"
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
echo "SMB authenticated fixture listing passed"

for user in folio_ci_rw1 folio_ci_rw2; do
  if ! smbclient //127.0.0.1/foliorelay-fixture -A "$work/$user.auth" \
    -c "get $smb_name $work/$user.download" >"$work/smb-download-result.log" 2>&1; then
    echo "SMB named-user read failed: $user" >&2
    sed -n '1,50p' "$work/smb-download-result.log" >&2 || true
    stat -c '%a %U %G %n' "$exports" "$export_base/$export_name" >&2 || true
    exit 1
  fi
  cmp "$source_path" "$work/$user.download"
done
echo "SMB two named-user reads passed"

# Both allowed users have genuine SMB write/delete capability; shared output
# must never expose the canonical artifact directory or attached credentials.
printf 'synthetic SMB client write\n' > "$work/user-upload.txt"
smbclient //127.0.0.1/foliorelay-fixture -A "$work/folio_ci_rw1.auth" \
  -c "put $work/user-upload.txt Prints/2026/user-upload.txt" >/dev/null
smbclient //127.0.0.1/foliorelay-fixture -A "$work/folio_ci_rw2.auth" \
  -c 'del Prints/2026/user-upload.txt' >/dev/null
echo "SMB two named-user writes passed"

if smbclient //127.0.0.1/foliorelay-fixture -A "$work/folio_ci_denied.auth" \
  -c 'ls' >/dev/null 2>&1; then
  echo "UNAUTHORIZED SMB account was admitted" >&2
  exit 1
fi

smbclient //127.0.0.1/foliorelay-fixture -A "$work/folio_ci_rw1.auth" \
  -c "rename $smb_name Prints/2026/renamed-by-client.pdf; del Prints/2026/renamed-by-client.pdf" >/dev/null
test ! -e "$export_base/$export_name"
canonical_after="$(sha256sum "$source_path" | cut -d' ' -f1)"
test "$canonical_before" = "$canonical_after" || {
  echo "canonical artifact was changed by SMB mutation" >&2; exit 1;
}
test -f "$source_path"
echo "PASS: named-user SMB read/write/rename/delete, unrelated user DENIED, canonical unchanged"
