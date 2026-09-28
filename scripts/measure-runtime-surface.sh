#!/bin/sh
set -eu

if [ "$#" -lt 4 ]; then
  echo "usage: $0 COMPOSE_FILE SERVICE OUTPUT_DIR BINARY [BINARY ...]" >&2
  exit 64
fi

compose_file=$1
service=$2
output_dir=$3
shift 3

mkdir -p "$output_dir"

cid="$(docker compose -f "$compose_file" ps -q "$service")"
test -n "$cid"
image="$(docker inspect -f '{{.Image}}' "$cid")"

docker inspect "$cid" >"$output_dir/container-inspect.json"
docker image inspect "$image" >"$output_dir/image-inspect.json"
docker image history --no-trunc --format '{{json .}}' "$image" \
  >"$output_dir/image-history.jsonl"

image_size_bytes="$(docker image inspect -f '{{.Size}}' "$image")"
image_layer_count="$(docker image inspect -f '{{len .RootFS.Layers}}' "$image")"

docker compose -f "$compose_file" exec -T "$service" \
  sh -c 'find / -xdev -type f -printf "%p\t%s\n" 2>/dev/null | sort' \
  >"$output_dir/rootfs-files.tsv"

docker compose -f "$compose_file" exec -T "$service" \
  sh -c 'find / -xdev -type f -perm /111 -printf "%p\t%s\n" 2>/dev/null | sort' \
  >"$output_dir/executables.tsv"

if docker compose -f "$compose_file" exec -T "$service" \
    sh -c 'command -v dpkg-query >/dev/null 2>&1'; then
  docker compose -f "$compose_file" exec -T "$service" \
    dpkg-query -W '-f=${binary:Package}\t${Version}\t${Installed-Size}\n' \
    | sort >"$output_dir/packages.tsv"
else
  : >"$output_dir/packages.tsv"
fi

for binary in "$@"; do
  safe="$(printf '%s' "$binary" | sed 's#[^A-Za-z0-9_.-]#_#g')"
  docker compose -f "$compose_file" exec -T "$service" \
    stat -c '%n\t%s' "$binary" >"$output_dir/binary-$safe.tsv"
  docker compose -f "$compose_file" exec -T "$service" \
    ldd "$binary" >"$output_dir/ldd-$safe.txt" 2>&1 || true
done

# Runtime/process characterization. These observations are comparison metrics,
# not hard resource limits.
docker compose -f "$compose_file" exec -T "$service" \
  sh -c 'cat /proc/1/status' >"$output_dir/process-status.txt"
docker compose -f "$compose_file" exec -T "$service" \
  sh -c 'tr "\000" " " </proc/1/cmdline; echo' >"$output_dir/process-cmdline.txt"
docker compose -f "$compose_file" exec -T "$service" \
  sh -c 'readlink /proc/1/exe' >"$output_dir/process-exe.txt" 2>/dev/null || true
docker compose -f "$compose_file" exec -T "$service" \
  sh -c 'find /proc/1/fd -mindepth 1 -maxdepth 1 -print 2>/dev/null | wc -l' \
  >"$output_dir/process-open-fds.txt"

docker compose -f "$compose_file" exec -T "$service" sh -c '
  if [ -r /sys/fs/cgroup/memory.current ]; then
    printf "memory_current_bytes\t%s\n" "$(cat /sys/fs/cgroup/memory.current)"
  elif [ -r /sys/fs/cgroup/memory/memory.usage_in_bytes ]; then
    printf "memory_current_bytes\t%s\n" "$(cat /sys/fs/cgroup/memory/memory.usage_in_bytes)"
  fi
  if [ -r /sys/fs/cgroup/memory.peak ]; then
    printf "memory_peak_bytes\t%s\n" "$(cat /sys/fs/cgroup/memory.peak)"
  elif [ -r /sys/fs/cgroup/memory/memory.max_usage_in_bytes ]; then
    printf "memory_peak_bytes\t%s\n" "$(cat /sys/fs/cgroup/memory/memory.max_usage_in_bytes)"
  fi
  if [ -r /sys/fs/cgroup/pids.current ]; then
    printf "pids_current\t%s\n" "$(cat /sys/fs/cgroup/pids.current)"
  elif [ -r /sys/fs/cgroup/pids/pids.current ]; then
    printf "pids_current\t%s\n" "$(cat /sys/fs/cgroup/pids/pids.current)"
  fi
' >"$output_dir/cgroup.tsv"

docker compose -f "$compose_file" exec -T "$service" \
  sh -c 'cat /proc/net/tcp 2>/dev/null || true' >"$output_dir/proc-net-tcp.txt"
docker compose -f "$compose_file" exec -T "$service" \
  sh -c 'cat /proc/net/tcp6 2>/dev/null || true' >"$output_dir/proc-net-tcp6.txt"

docker inspect -f '{{json .Mounts}}' "$cid" >"$output_dir/mounts.json"
docker inspect -f '{{json .HostConfig.Tmpfs}}' "$cid" >"$output_dir/tmpfs.json"
: >"$output_dir/mount-usage.tsv"
for destination in $(docker inspect -f '{{range .Mounts}}{{println .Destination}}{{end}}' "$cid"); do
  bytes="$(docker compose -f "$compose_file" exec -T "$service" \
    sh -c 'du -sb "$1" 2>/dev/null | cut -f1' sh "$destination" \
    | tr -d '\r' | tail -1)"
  [ -n "$bytes" ] || bytes=0
  printf '%s\t%s\n' "$destination" "$bytes" >>"$output_dir/mount-usage.tsv"
done

python3 - "$output_dir" "$image_size_bytes" "$image_layer_count" "$service" <<'PY'
import json
import pathlib
import re
import sys

out = pathlib.Path(sys.argv[1])

def aggregate_tsv(path):
    count = 0
    total = 0
    for line in path.read_text().splitlines():
        if not line:
            continue
        parts = line.rsplit("\t", 1)
        if len(parts) != 2:
            continue
        count += 1
        try:
            total += int(parts[1])
        except ValueError:
            pass
    return count, total

def parse_proc_status(path):
    result = {}
    for line in path.read_text(errors="replace").splitlines():
        if ":" not in line:
            continue
        key, value = line.split(":", 1)
        fields = value.strip().split()
        if not fields:
            continue
        if fields[0].isdigit():
            result[key] = int(fields[0])
        else:
            result[key] = value.strip()
    return result

def parse_key_value_tsv(path):
    result = {}
    if not path.exists():
        return result
    for line in path.read_text().splitlines():
        if not line:
            continue
        parts = line.split("\t", 1)
        if len(parts) != 2:
            continue
        try:
            result[parts[0]] = int(parts[1])
        except ValueError:
            result[parts[0]] = parts[1]
    return result

def parse_listeners(path):
    listeners = []
    if not path.exists():
        return listeners
    for line in path.read_text().splitlines()[1:]:
        fields = line.split()
        if len(fields) < 4 or fields[3] != "0A":
            continue
        local = fields[1]
        if ":" not in local:
            continue
        _, port_hex = local.rsplit(":", 1)
        try:
            listeners.append(int(port_hex, 16))
        except ValueError:
            pass
    return listeners

rootfs_count, rootfs_bytes = aggregate_tsv(out / "rootfs-files.tsv")
exec_count, exec_bytes = aggregate_tsv(out / "executables.tsv")

packages = []
package_kib = 0
for line in (out / "packages.tsv").read_text().splitlines():
    parts = line.split("\t")
    if len(parts) >= 3:
        packages.append(parts[0])
        try:
            package_kib += int(parts[2])
        except ValueError:
            pass

deps = set()
for path in out.glob("ldd-*.txt"):
    for line in path.read_text(errors="replace").splitlines():
        match = re.search(r"=>\s+(/\S+)", line)
        if match:
            deps.add(match.group(1))
            continue
        match = re.match(r"\s*(/\S+)\s+\(", line)
        if match:
            deps.add(match.group(1))

proc = parse_proc_status(out / "process-status.txt")
cgroup = parse_key_value_tsv(out / "cgroup.tsv")
listeners = parse_listeners(out / "proc-net-tcp.txt") + parse_listeners(out / "proc-net-tcp6.txt")

try:
    open_fds = int((out / "process-open-fds.txt").read_text().strip())
except (ValueError, FileNotFoundError):
    open_fds = None

mounts = json.loads((out / "mounts.json").read_text() or "[]")
tmpfs = json.loads((out / "tmpfs.json").read_text() or "{}")
mount_usage = parse_key_value_tsv(out / "mount-usage.tsv")

container = json.loads((out / "container-inspect.json").read_text())[0]
host_config = container.get("HostConfig", {})

summary = {
    "service": sys.argv[4],
    "image_size_bytes": int(sys.argv[2]),
    "image_layer_count": int(sys.argv[3]),
    "rootfs_accessible_file_count": rootfs_count,
    "rootfs_accessible_file_bytes": rootfs_bytes,
    "executable_file_count": exec_count,
    "executable_file_bytes": exec_bytes,
    "dpkg_package_count": len(packages),
    "dpkg_installed_size_kib": package_kib,
    "dynamic_dependency_count": len(deps),
    "dynamic_dependency_paths": sorted(deps),
    "process_vm_rss_kib": proc.get("VmRSS"),
    "process_vm_hwm_kib": proc.get("VmHWM"),
    "process_threads": proc.get("Threads"),
    "process_open_fd_count": open_fds,
    "cgroup_memory_current_bytes": cgroup.get("memory_current_bytes"),
    "cgroup_memory_peak_bytes": cgroup.get("memory_peak_bytes"),
    "cgroup_pids_current": cgroup.get("pids_current"),
    "listening_tcp_socket_count": len(listeners),
    "listening_tcp_ports": sorted(set(listeners)),
    "read_only_rootfs": bool(host_config.get("ReadonlyRootfs")),
    "cap_drop": sorted(host_config.get("CapDrop") or []),
    "security_opt": sorted(host_config.get("SecurityOpt") or []),
    "declared_mount_count": len(mounts),
    "declared_mount_destinations": sorted(m.get("Destination") for m in mounts if m.get("Destination")),
    "tmpfs_mount_count": len(tmpfs or {}),
    "tmpfs_mount_destinations": sorted((tmpfs or {}).keys()),
    "persistent_mount_usage_bytes": mount_usage,
    "persistent_mount_bytes_total": sum(v for v in mount_usage.values() if isinstance(v, int)),
}
(out / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")
print(json.dumps(summary, sort_keys=True))
PY
