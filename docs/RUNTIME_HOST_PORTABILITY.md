# Runtime host portability qualification

This workflow consumes the broader Agent Dispatch GitHub-hosted runner census
rather than duplicating it. The census established that Ubuntu 22.04, 24.04 and
26.04 public runners exist in both x64 and arm64 forms with live Docker daemons.

FolioRelay therefore exercises the real engineering CUPS and PAPPL containers on
all six host classes. A matrix row passes only when:

- the candidate and state images build for the runner's native architecture;
- the running image architecture matches the runner architecture (no hidden
  QEMU cross-architecture success);
- read-only rootfs, drop-all-capabilities, no-new-privileges and UID 10001
  invariants remain intact;
- the candidate reaches real IPP readiness;
- a generated PDF is submitted through `ipptool Print-Job`;
- FolioRelay durable acceptance advances;
- the conflict counter does not advance.

Host-version success is a Docker/kernel/Compose portability result, not a claim
that Ubuntu 22/24/26 are separately supported product distributions. FolioRelay
continues to ship its pinned runtime userland inside the OCI image.

BSD/illumos/Haiku QEMU guests from the Agent Dispatch census are intentionally
not included here because the current product deployment contract is OCI/Linux.
They remain useful portability environments for future native components if a
real requirement appears.
