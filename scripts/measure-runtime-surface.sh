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
docker image history --no-trunc --format '{{json .}}' "$image"   >"$output_dir/image-history.jsonl"

image_size_bytes="$(docker image inspect -f '{{.Size}}' "$image")"
image_layer_count="$(docker image inspect -f '{{len .RootFS.Layers}}' "$image")"

docker compose -f "$compose_file" exec -T "$service"   sh -c 'find / -xdev -type f -printf "%p\t%s\n" 2>/dev/null | sort'   >"$output_dir/rootfs-files.tsv"

docker compose -f "$compose_file" exec -T "$service"   sh -c 'find / -xdev -type f -perm /111 -printf "%p\t%s\n" 2>/dev/null | sort'   >"$output_dir/executables.tsv"

if docker compose -f "$compose_file" exec -T "$service"     sh -c 'command -v dpkg-query >/dev/null 2>&1'; then
  docker compose -f "$compose_file" exec -T "$service"     dpkg-query -W '-f=${binary:Package}\t${Version}\t${Installed-Size}\n'     | sort >"$output_dir/packages.tsv"
else
  : >"$output_dir/packages.tsv"
fi

for binary in "$@"; do
  safe="$(printf '%s' "$binary" | sed 's#[^A-Za-z0-9_.-]#_#g')"
  docker compose -f "$compose_file" exec -T "$service"     stat -c '%n\t%s' "$binary" >"$output_dir/binary-$safe.tsv"
  docker compose -f "$compose_file" exec -T "$service"     ldd "$binary" >"$output_dir/ldd-$safe.txt" 2>&1 || true
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
}
(out / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")
print(json.dumps(summary, sort_keys=True))
PY
