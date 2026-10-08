# Print substrate bake-off results

Status: active evidence ledger.

## Phase 0 — build/surface feasibility

Public GitHub Actions run: `Print Substrate Bakeoff #1`, run ID
`36340940028`.

All three candidate lanes completed successfully without user intervention.

### Results

| Candidate | Functional probe | Executable bytes | Dynamic libraries | Dynamic library file bytes | Measured staged bytes | Notes |
|---|---:|---:|---:|---:|---:|---|
| CUPS 2.4.19 minimal-build baseline | build/startup probe PASS | 1,423,240 | 32 | 16,742,488 | 10,125,461 | staged tree is the upstream install tree and is not yet runtime-pruned; 372 files |
| CUPS 2.4.19 ippeveprinter-static | IPP Get-Printer-Attributes PASS | 1,639,064 | 6 | 9,437,568 | 1,639,064 | despite target name, binary is not fully static; libcups is linked statically but six system/TLS/etc. libraries remain dynamic |
| PAPPL 1.4.11 | upstream API/client/PWG Raster test set PASS | 2,557,616 | 34 | 22,611,120 | 5,370,912 | executable is upstream `testpappl`, not a FolioRelay-minimal PAPPL application; staged bytes are `libpappl.a` proxy |

### Interpretation

This phase answers **feasibility**, not production selection.

The measurements are deliberately not treated as apples-to-apples final image
sizes:

- CUPS measurement includes an unpruned upstream install tree.
- PAPPL uses the upstream test application, which intentionally enables
  functionality FolioRelay would not ship.
- ippeveprinter measurement is closest to a narrow executable, but it is an
  upstream basic/test print server and still needs production robustness
  qualification.

Current evidence supports three statements:

1. all three remain technically viable for deeper testing;
2. ippeveprinter currently has the lowest executable/runtime-library surface;
3. no candidate has yet earned production selection.

## Red-team observations from source review

### ippeveprinter command boundary

The upstream implementation invokes the configured per-job command directly
with `execve`, not via a shell. This is a useful property.

It passes IPP/job attributes through environment variables. Therefore the
FolioRelay adapter must:

- be a fixed absolute executable;
- treat every environment value as untrusted data;
- never interpolate those values into a shell command;
- never allow those values to select an executable;
- validate size/type/character bounds before creating a FolioRelay command;
- submit a structured idempotent command to the state authority;
- not directly mutate durable state.

### PAPPL test defaults are not production defaults

The upstream test application includes broad system options such as WebUI and
raw socket support because it is a framework test fixture.

A production FolioRelay PAPPL fixture must be written independently and enable
only required options. The test fixture cannot be used as the production image.

### CUPS pruning must be measured after packaging

The CUPS upstream build can turn several functions off by configuration, but a
feature being disabled by default does not mean all corresponding code or files
are absent.

The real comparison therefore requires a FolioRelay-owned final runtime image
with an explicit install manifest.

## Next evidence phases

### Phase 1 — FolioRelay ingress adapters

For each survivor:

- build a deliberately minimal server/adapter;
- submit a real print artifact;
- compute the received artifact digest;
- submit exactly one idempotent command to a test state authority;
- verify duplicate/replayed substrate callbacks do not create duplicate state
  transitions or effect intents;
- verify cancellation/status projection;
- restart substrate and adapter;
- verify FolioRelay state remains authoritative.

### Phase 2 — resource and failure pressure

- malformed IPP;
- slow/truncated clients;
- huge attributes;
- declared million-copy/page jobs;
- concurrency;
- server/adapter kill during receive and after acceptance;
- storage full;
- malformed artifact;
- downstream state authority unavailable;
- bounded backpressure.

### Phase 3 — standards/native clients

- PWG engineering conformance;
- Linux native client;
- Windows native client;
- macOS native client;
- Android emulator;
- iOS Simulator;
- DNS-SD/AirPrint/Mopria behavior where hosted networking permits.

## Selection remains gated

A permanent upstream fork is still a veto.

The eventual choice is the lowest-surface **survivor of all required gates**,
not the lowest number in this Phase 0 table.

## Phase 1 — ippeveprinter FolioRelay handoff

Public GitHub Actions run: `IPP Ingress Phase 1 #1`, run ID
`36341337776`.

Result: **PASS**.

Evidence:

- a real PDF was submitted with IPP `Print-Job`;
- upstream IPP response was `successful-ok`;
- exact input bytes were committed to the content-addressed ingress store;
- the stored object's SHA-256 matched the submitted file;
- one real substrate job plus one synthetic job produced exactly two accepted
  transitions;
- replay of the synthetic substrate job produced exactly one replay and did not
  advance state again;
- same substrate identity with different bytes/fingerprint produced exactly one
  idempotency conflict and failed closed;
- the ingest helper was built as a statically linked Go executable;
- dependency inspection found no CUPS/PDF/PostScript/image/renderer libraries in
  the helper.

Observed fixture stats:

```json
{"accepted":2,"conflicts":1,"replayed":1}
```

### Architectural finding

The upstream `ippeveprinter` implementation invokes the fixed job command with
`execve`, passing the spool filename as an argument and IPP/job metadata as
environment variables. This avoids an implicit shell but still requires strict
metadata validation.

Its local completed-job list is short-lived projection state; completed jobs are
cleaned after roughly 60 seconds. FolioRelay therefore must not rely on this job
list for durable history or idempotency.

### Resource-risk finding

The FolioRelay ingest helper can reject an artifact after the substrate hands it
off, but the substrate has already spooled the request by then.

Therefore application-level artifact-size checks are **not sufficient** against
disk exhaustion.

The substrate's untrusted ingress spool must have an independently bounded
filesystem/quota. Filling that bounded scratch area must not consume the
FolioRelay state journal or user artifact store.

This is now a Phase 2 qualification requirement.

## Phase 2 — ippeveprinter concurrency veto

Public GHA stress evidence showed:

```json
{"attempts":100,"successes":8,"failures":92}
```

Representative failed requests returned:

```
status-code = server-error-busy (Currently printing another job.)
```

This matches upstream source behavior: `create_job()` explicitly refuses a new
job while a previous `active_job` remains non-terminal.

### Decision

`ippeveprinter` is **rejected as the FolioRelay production ingress substrate**
for the current requirements.

Reason:

- FolioRelay requires robust concurrent submissions from many devices;
- upstream intentionally supports only one active job at a time;
- adding a durable/concurrent scheduler around or inside it would duplicate the
  very functionality FolioRelay is trying not to fork/maintain;
- relying on every native client to implement sufficient retry/backoff would
  make robustness client-dependent.

It remains useful as:

- an IPP Everywhere behavior oracle;
- a tiny client-interoperability fixture;
- a negative-space reference for the minimum protocol surface.

It is not a production candidate unless upstream semantics materially change.

## Phase 2 — CUPS and PAPPL upstream pressure baseline

### CUPS 2.4.19

The stripped-build CUPS lane completed its upstream scheduler/unit test suite
successfully in public GHA.

Result: **survives to deeper FolioRelay-specific qualification**.

This is not yet a production pass: the next CUPS work must use a
FolioRelay-owned runtime install manifest and real FolioRelay virtual queue
handoff, then repeat concurrency/failure tests against that exact deployment.

### PAPPL 1.4.11

The first upstream high-load run executed 100 clients x 1,000 requests:

- total requests: 100,000;
- measured throughput: approximately 945 requests/second;
- errors: 12;
- elapsed: approximately 105.84 seconds.

Result: **not rejected, but not yet zero-error qualified**.

This workload is an upstream framework stress test dominated by
Get-Printer-Attributes requests, not a FolioRelay print-job acceptance test.
The error rate therefore cannot be directly translated into production print
failure probability.

The autonomous workflow has been changed to measure a stepped concurrency
frontier, including a required 100-client/10,000-request zero-error gate, plus
larger characterization tiers. A minimal FolioRelay PAPPL application is still
required before product selection.

### Current production-candidate set

- `ippeveprinter`: **rejected** for production ingress; retain as oracle.
- minimal custom-built `cupsd`: **active candidate**.
- minimal PAPPL service: **active candidate with concurrency investigation**.


## Active-candidate parity closure and native arm64

Latest software-only evidence closes several previously open parity gates without
selecting a production winner.

### Shared FolioRelay semantics

`IPP Ingress Phase 1 #58` (run ID `36361061756`) passed for both active
candidates:

- 25 simultaneous real Print-Job submissions reached the same durable FolioRelay
  state/idempotency authority without conflicts;
- replay, idempotency-conflict and state-authority restart semantics passed;
- Create-Job/Get-Job-Attributes/Cancel-Job projection behavior passed without
  creating durable FolioRelay acceptance;
- malformed and slow clients left the substrate and state-authority control
  surfaces healthy;
- a one-million-copy request did not advance durable accepted state;
- a 2 MiB isolated substrate-spool exhaustion test did not advance durable
  accepted state and did not take down the FolioRelay state authority.

### Authority-outage behavior

PAPPL passed the real-substrate authority-outage gate directly.

CUPS initially exposed its upstream default behavior: `ErrorPolicy
stop-printer` left the virtual queue stopped after the FolioRelay backend failed
while the authority was unavailable. This was not a journal or idempotency
failure—the journal remained unchanged—but it prevented autonomous recovery.

The qualification fixture now uses upstream CUPS `retry-job` with a bounded
retry budget (1-second qualification interval, 10 retries). `IPP Ingress Phase
1 #57` (run ID `36360803858`) then passed the same fail-closed/recovery gate
as PAPPL without a CUPS fork or a FolioRelay-specific recovery daemon.

### PAPPL independent-client pressure

`Print Substrate Stress #47` (run ID `36360255038`) exercised the actual
minimal FolioRelay PAPPL fixture with independent client processes:

- 100 clients x 100 requests: 0 failed, 0 timed out;
- 100 clients x 1000 requests: 0 failed, 0 timed out.

The older 7/12-error 100k-request observations remain useful historical evidence
from the upstream synthetic harness, but the degradation was not reproduced by
the later upstream run or the actual minimal FolioRelay fixture.

### Native architecture gate

`Print Substrate Bakeoff #64` (run ID `36361318869`) runs the same pinned
Phase-0 build/smoke implementation on GitHub-hosted native architectures.

Results:

- minimal CUPS 2.4.19: amd64 PASS, arm64 PASS;
- PAPPL 1.4.11: amd64 PASS, arm64 PASS;
- ippeveprinter remains amd64 oracle-only because its production concurrency
  veto is already established.

The arm64 evidence comes from native `ubuntu-24.04-arm` runners, not a
cross-compile proxy.

### Candidate status

The production-candidate set remains:

- minimal upstream CUPS scheduler — active;
- minimal PAPPL service — active;
- ippeveprinter — production-vetoed, retained as an interoperability oracle.

Do not narrow CUPS vs PAPPL yet. Remaining required software gates include TLS,
discovery when enabled, final-runtime least privilege/hardening, and deeper
standards/native-client qualification.


## TLS, discovery, conformance, and runnable engineering profile

The previously open hosted software gates for both active production candidates
are now qualified.

Public GitHub Actions evidence:

- `IPP Ingress Phase 1 #84` (run ID `36378285282`): PASS.
  - minimal CUPS: least-privilege envelope, dedicated TLS IPP surface, and the
    pinned IPP Everywhere engineering conformance suite all pass;
  - minimal PAPPL: least-privilege envelope, TLS IPP surface, and the same
    pinned IPP Everywhere engineering conformance suite all pass;
  - replay/conflict/restart, authority-outage, cancellation/status projection,
    malformed/abusive pressure, and bounded spool-exhaustion gates remain green.
- `Print Substrate Bakeoff #88` (run ID `36378285220`): PASS.
  - isolated DNS-SD discovery profiles for both minimal CUPS and PAPPL advertise
    the expected IPP service successfully.
- `Engineering CUPS Runtime #11` (run ID `36378285205`): PASS.
  - the provisional Compose-packaged CUPS profile builds, starts, accepts a real
    print job into FolioRelay, preserves stable substrate identity across a CUPS
    restart, and continues accepting work without idempotency conflicts.
- `Formal State Models #101` and `Go Security Kernel #113`: PASS.

The `ippeveprinter` oracle briefly made the overall Phase-1 workflow fail after
the IPP Everywhere oracle was added ahead of the synthetic replay test. The
conformance suite legitimately advanced the shared accepted counter, while the
older replay assertion still assumed an absolute total of two accepted jobs.
Commit `0b5d17922a2fa3ec5c15ef6c9e8d50084ce6d8a7` changed that oracle-only
assertion to compare replay/conflict counter deltas against a captured baseline.
Run #84 then passed. This was a test-isolation defect, not a FolioRelay state or
substrate correctness failure.

### Selection status

The production selection remains intentionally open:

- minimal upstream CUPS scheduler — active;
- minimal PAPPL service — active;
- ippeveprinter — production-vetoed, retained as protocol/interoperability oracle.

The runnable CUPS engineering profile is an operational bridge, not evidence by
itself that CUPS has won the bake-off.

Highest remaining software-only work before narrowing the active candidates:

1. harden the packaged engineering runtime to read-only roots, explicit writable
   paths, dropped capabilities, and no-new-privileges, then requalify it;
2. produce comparable final-runtime surface/package evidence for PAPPL so the
   selection comparison is symmetric;
3. extend native-client interoperability evidence beyond the common `ipptool`
   oracle where hosted runners can faithfully exercise the platform print path;
4. qualify the upstream security-update/rebuild path and immutable
   supply-chain inputs;
5. only then apply the documented survivor/maintenance/surface selection rule.


## Packaged CUPS authority-envelope qualification

`Engineering CUPS Runtime #14` (run ID `36378584624`) passed at commit
`20472040f27f7aec96efbccd5d187290420b35dd`.

The directly runnable engineering profile now proves, in the same rep:

- state and CUPS services execute as UID 10001;
- both containers have read-only root filesystems;
- all Linux capabilities are dropped;
- `no-new-privileges` is enforced;
- the state journal, CUPS state/spool, immutable accepted-artifact store, and
  bounded cache/log tmpfs surfaces remain writable as explicitly intended;
- an attempted write to the image/root filesystem fails;
- a real print job still reaches FolioRelay;
- restarting CUPS preserves the stable substrate identity and subsequent work is
  accepted without an idempotency conflict.

This closes the packaged CUPS read-only-root/authority-envelope gate. It does
**not** select CUPS over PAPPL: PAPPL still needs a comparable packaged-runtime
surface rep before runtime packaging/surface can be used symmetrically in the
selection decision.


## Packaged PAPPL restart identity and normalized runtime surface

The packaged PAPPL lane exposed and then closed a real restart-identity defect.

The initial restart rep kept the FolioRelay substrate-instance epoch stable but
PAPPL reset its numeric job allocator. A distinct post-restart document then
collided with the pre-restart command identity:

```text
before: accepted=1 conflicts=0
after:  accepted=1 conflicts=1
```

The bridge now uses upstream PAPPL's own projection-state mechanism:
`papplSystemSaveState` persists `NextJobId`, and
`papplSystemLoadState` restores it. If an existing state file cannot be
loaded, the bridge fails closed instead of silently resetting the allocator.

On the current head `11bc4c295577049f11494339229afac544073e29`,
Engineering PAPPL Runtime #10 (run ID `36381873028`) passes:

- real PDF acceptance into FolioRelay;
- read-only root filesystem;
- UID 10001;
- `CAP_DROP=ALL`;
- `no-new-privileges`;
- explicit writable PAPPL projection/artifact/scratch paths;
- stable substrate-instance epoch across a process restart;
- restored PAPPL source-job allocation across that restart;
- a distinct post-restart document advances durable FolioRelay acceptance
  without an idempotency conflict.

PAPPL's saved state remains substrate-local projection/identity-continuity
state; it is not a second durable FolioRelay job authority.

### Apples-to-apples packaged surface

The earlier PAPPL package measurement was distorted by linking the candidate to
Ubuntu's broad distro `libcups`, which pulled Avahi, D-Bus, GSSAPI, GnuTLS,
and related transitive dependencies despite those features being disabled in
the PAPPL build.

The current PAPPL build instead uses the same pinned/minimized upstream
CUPS/libcups policy as the CUPS candidate. Current symmetric public-GHA
measurements are:

| Metric | CUPS | PAPPL |
|---|---:|---:|
| Image size | 87,175,497 B | 87,566,902 B |
| Accessible rootfs files | 2,594 | 2,607 |
| Accessible rootfs bytes | 91,180,391 B | 91,268,910 B |
| Executable files | 498 | 496 |
| Executable bytes | 40,534,904 B | 39,712,648 B |
| dpkg packages | 92 | 94 |
| dpkg installed size | 94,583 KiB | 95,501 KiB |
| Measured dynamic dependency paths | 8 | 9 |

The package difference is now only PAPPL's explicit JPEG/PNG runtime support;
both candidates use the same private minimized `libcups.so.2`. PAPPL's
measured dependency set adds JPEG and PNG while CUPS adds `libcrypt`.

The image-size difference is about 0.45%, far too small to justify a substrate
selection on size alone. These results close the prior packaging asymmetry and
move the critical path to native-client interoperability and immutable
supply-chain/security-update qualification.


## Immutable-input rebuild qualification

The packaged-runtime build chain now uses immutable external inputs:

- Ubuntu base:
  `ubuntu:24.04@sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3`;
- Go builder:
  `golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195`;
- Ubuntu archive snapshot: `20260928T000000Z`;
- CUPS source: commit
  `6ba0487abb05afc93d639f37676add8fd65d3756`;
- PAPPL source: commit
  `ad86a0a8473f0f83233a374226561181570d4f81`.

The release tags remain human-readable metadata, while CI independently verifies
the upstream annotated tag objects and their peeled commits before the rebuild
jobs run.

Ubuntu's snapshot resolver initially exposed a bootstrap edge case: the minimal
pinned Ubuntu base has no CA bundle, but snapshot resolution switches to HTTPS.
The build now bootstraps trust from the already pinned Go builder image, then
installs Ubuntu's own `ca-certificates` from the frozen Ubuntu snapshot. This
keeps HTTPS transport without allowing one mutable "latest" package to bootstrap
the supposedly immutable build.

`Runtime Rebuild Reproducibility #1` proves two independent no-cache rebuilds
per active candidate from the frozen tuple.

CUPS:

- source-authority check: PASS;
- provenance labels identical: PASS;
- installed package manifest identical: PASS;
- key runtime binary hashes identical: PASS;
- image size A/B: 90,321,940 / 90,321,940 bytes.

PAPPL:

- source-authority check: PASS;
- provenance labels identical: PASS;
- installed package manifest identical: PASS;
- key runtime binary hashes identical: PASS;
- image size A/B: 90,328,920 / 90,328,920 bytes.

This closes the current upstream security-update/rebuild *mechanism* gate: an
update is an explicit change to the source/base/snapshot tuple followed by the
same qualification suite. It does not imply that future upstream releases are
automatically safe; each candidate update must earn admission with fresh
evidence.


## PAPPL 1.4.12 admitted update rehearsal

FolioRelay exercised the immutable update mechanism against a real newer
upstream release rather than a synthetic version bump. PAPPL moved from
`v1.4.11` / `ad86a0a8473f0f83233a374226561181570d4f81` to
`v1.4.12` / `6db8e137557ad84662e78d24fdb2a591c621f4ac`.

The v1.4.12 tuple passed:

- IPP Ingress Phase 1 #107, including replay/conflict/restart, TLS, pressure,
  authority-outage, and IPP Everywhere engineering-conformance gates;
- Print Substrate Bakeoff #111, including discovery plus native amd64 and arm64;
- Print Substrate Stress #101, including the PAPPL concurrency frontier and
  independent-client pressure;
- Runtime Rebuild Reproducibility #6, including source-authority verification,
  two no-cache builds, identical package manifests, identical key-binary hashes,
  and identical image sizes;
- Engineering PAPPL Runtime #23 and Engineering CUPS Runtime #38 for the native
  direct-libcups interoperability rep, packaged authority envelope, real print,
  restart/idempotency, and current surface characterization.

This closes the currently defined upstream security-update/rebuild mechanism
gate. Future source/base/snapshot updates still require a fresh qualification
transaction; this pass does not grant automatic trust to later releases.

### Linux native-client oracle correction

The first Linux-native attempt created a local Ubuntu CUPS
`-m everywhere` queue and submitted through `lp`. On both FolioRelay
candidate endpoints, Ubuntu's local `/usr/lib/cups/filter/universal` process
crashed with signal 11 while loading PPD color-profile state. The failure
reproduced with a standards-conventional one-page PDF, and the IPP backend
exited without an error before FolioRelay durable acceptance changed.

Because the same host-side conversion failure occurred before transport to both
otherwise-green candidate endpoints, it is retained as a client-stack
compatibility/negative-space finding, not a CUPS-vs-PAPPL substrate failure.

The admission rep now uses the Ubuntu host's native libcups directly and issues
IPP `Print-Job` to each explicit candidate URI with the same valid PDF. Both
returned:

```text
status=successful-ok job-id=2
```

and each advanced FolioRelay durable acceptance with no idempotency conflict.
This qualifies the Linux native libcups transport path without involving the
unrelated local scheduler/filter conversion pipeline.

### Current immutable packaged surface

After immutable OCI bases, the frozen Ubuntu snapshot, and the pinned minimized
private libcups closure are applied symmetrically:

| Metric | CUPS | PAPPL |
|---|---:|---:|
| Image size | 90,321,940 B | 90,328,920 B |
| Accessible rootfs files | 2,745 | 2,756 |
| Accessible rootfs bytes | 93,324,056 B | 93,324,474 B |
| Executable files | 507 | 505 |
| Executable bytes | 41,583,958 B | 40,761,702 B |
| dpkg packages | 94 | 96 |
| dpkg installed size | 96,801 KiB | 97,719 KiB |
| Measured dynamic dependency paths | 8 | 9 |

The image-size difference is about 0.008%. Surface size is therefore not a
meaningful selector between the two survivors. Broader native-platform
interoperability remains a separate Phase-3 work item.


## Windows parity and current selection characterization

Commit `253edbfc3ae1605a17a767c46c78ae3aff384d26` closes the remaining hosted
desktop-native parity gate.

`Windows Native Client Qualification #27` passes for both active candidates
using the real Windows Server 2025 PrintManagement/Spooler path,
`Add-Printer -IppURL`, Microsoft IPP Class Driver, and candidate processes
running from the exact pinned engineering rootfs under WSL2.

For minimal CUPS, the Windows rep discovered one upstream transport dependency:
`gziptoany`. Windows submits gzip-encoded IPP document data, and upstream
`cupsd` inserts that filter before the FolioRelay backend. Retaining only
`gziptoany` is sufficient. Engineering CUPS Runtime #69 passes with explicit
negative assertions that legacy `pstops`, `rastertopwg`,
`foomatic-rip`, generic `ipp`/SNMP backends, `mailto`, and `sendmail`
are absent.

### Current packaged and operating footprint

| Metric | CUPS | PAPPL |
|---|---:|---:|
| Image size | 90,336,412 B | 90,328,920 B |
| Accessible rootfs files | 2,746 | 2,756 |
| Executable files | 508 | 505 |
| Dynamic dependency paths | 8 | 9 |
| PID 1 RSS | 5,668 KiB | 9,160 KiB |
| Cgroup current memory | 6,094,848 B | 7,217,152 B |
| Cgroup peak memory | 14,684,160 B | 14,630,912 B |
| PID 1 open FDs | 11 | 5 |
| Candidate TCP listener ports | 8634, 8635 | 8633 |
| Persistent projection bytes in this rep | 3,781 B | 2,371 B |
| Restart-to-IPP-ready median | 228 ms | 171 ms |
| FolioRelay executable glue | 34 effective LOC | 258 effective LOC |
| Candidate configuration | 249 effective LOC | 182 effective LOC |
| Carried upstream source patches | 0 | 0 |

These are characterization values, not independent selection verdicts. The image
surfaces and peak memory remain effectively tied. CUPS currently has lower idle
RSS and substantially less executable integration glue; PAPPL uses fewer open
FDs/listeners, less substrate projection state, and a smaller declarative
configuration surface. Restart timing is runner-sensitive and remains
characterization only.

Hosted desktop native-client parity is now symmetric on Linux, macOS, and
Windows. Android and iOS/iPadOS remain deliberately unqualified until a hosted
or HIL path can exercise their actual native print service/AirPrint behavior
without substituting a generic protocol client.
