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
