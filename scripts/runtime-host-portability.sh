#!/usr/bin/env bash
set -euo pipefail

candidate="${1:?candidate cups|pappl}"
evidence="${2:?evidence directory}"

case "$candidate" in
  cups)
    compose="deploy/engineering-cups/compose.yaml"
    service="cups"
    state_port="18080"
    ipp_url="ipp://127.0.0.1:8634/printers/FolioRelay"
    ;;
  pappl)
    compose="deploy/engineering-pappl/compose.yaml"
    service="pappl"
    state_port="18081"
    ipp_url="ipp://127.0.0.1:8633/ipp/print"
    ;;
  *)
    echo "unsupported candidate: $candidate" >&2
    exit 64
    ;;
esac

mkdir -p "$evidence"

case "$(uname -m)" in
  x86_64) expected_arch="amd64" ;;
  aarch64|arm64) expected_arch="arm64" ;;
  *) echo "unsupported host architecture: $(uname -m)" >&2; exit 1 ;;
esac

cleanup() {
  docker compose -f "$compose" down -v >"$evidence/compose-down.txt" 2>&1 || true
}
trap cleanup EXIT

{
  echo "candidate=$candidate"
  echo "runner_label=${RUNNER_LABEL:-unknown}"
  echo "expected_arch=$expected_arch"
  echo "uname_m=$(uname -m)"
  echo "uname_r=$(uname -r)"
  echo "os_release=$(tr '\n' ' ' </etc/os-release)"
  echo "cgroup_v2=$([ -e /sys/fs/cgroup/cgroup.controllers ] && echo true || echo false)"
} >"$evidence/host.txt"
docker version >"$evidence/docker-version.txt"
docker info >"$evidence/docker-info.txt"

docker compose -f "$compose" up -d --build
docker compose -f "$compose" ps -a >"$evidence/compose-ps.txt"

for container_service in state "$service"; do
  cid="$(docker compose -f "$compose" ps -q "$container_service")"
  test -n "$cid"
  docker inspect "$cid" >"$evidence/${container_service}-inspect.json"

  image_id="$(docker inspect -f '{{.Image}}' "$cid")"
  arch="$(docker image inspect -f '{{.Architecture}}' "$image_id")"
  printf '%s\n' "$arch" >"$evidence/${container_service}-image-arch.txt"
  test "$arch" = "$expected_arch"

  test "$(docker inspect -f '{{.HostConfig.ReadonlyRootfs}}' "$cid")" = "true"
  docker inspect -f '{{json .HostConfig.CapDrop}}' "$cid" | grep -q '"ALL"'
  docker inspect -f '{{json .HostConfig.SecurityOpt}}' "$cid" | grep -q 'no-new-privileges'

  uid="$(docker compose -f "$compose" exec -T "$container_service" id -u)"
  test "$uid" = "10001"
done

for _ in $(seq 1 60); do
  curl -fsS -o /dev/null "http://127.0.0.1:${state_port}/health" && break
  sleep 1
done
curl -fsS -o /dev/null "http://127.0.0.1:${state_port}/health"

gettest="$(find /usr/share/cups -type f -name get-printer-attributes.test -print -quit)"
printtest="$(find /usr/share/cups -type f -name print-job.test -print -quit)"
test -n "$gettest"
test -n "$printtest"

for _ in $(seq 1 60); do
  ipptool -q "$ipp_url" "$gettest" && break
  sleep 1
done
ipptool -q "$ipp_url" "$gettest"

python3 scripts/make-probe-pdf.py "$evidence/portability-probe.pdf"
before="$(curl -fsS "http://127.0.0.1:${state_port}/v1/stats")"
ipptool -q -f "$evidence/portability-probe.pdf" "$ipp_url" "$printtest"

for _ in $(seq 1 30); do
  after="$(curl -fsS "http://127.0.0.1:${state_port}/v1/stats")"
  b="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["accepted"])' <<<"$before")"
  a="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["accepted"])' <<<"$after")"
  [ "$a" -gt "$b" ] && break
  sleep 1
done
after="$(curl -fsS "http://127.0.0.1:${state_port}/v1/stats")"

python3 - "$before" "$after" "$candidate" "$expected_arch" >"$evidence/result.json" <<'PY'
import json, sys
before=json.loads(sys.argv[1])
after=json.loads(sys.argv[2])
candidate=sys.argv[3]
arch=sys.argv[4]
assert after["accepted"] > before["accepted"], (before, after)
assert after["conflicts"] == before["conflicts"], (before, after)
print(json.dumps({
    "candidate": candidate,
    "native_architecture": arch,
    "durable_acceptance_advanced": True,
    "conflicts_unchanged": True,
    "before": before,
    "after": after,
}, indent=2, sort_keys=True))
PY

printf '%s\n' "$before" >"$evidence/stats-before.json"
printf '%s\n' "$after" >"$evidence/stats-after.json"
docker compose -f "$compose" logs --no-color >"$evidence/compose.log" 2>&1
