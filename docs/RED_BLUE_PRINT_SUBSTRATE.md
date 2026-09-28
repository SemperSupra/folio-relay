# Red/blue review: FolioRelay print substrate

Status: active autonomous bake-off.

## Question

What is the smallest upstream-maintained standards-facing print substrate that
gives FolioRelay the required native-client interoperability and robustness
without becoming a second durable state authority?

Candidates:

1. selectively built upstream CUPS scheduler (`cupsd`);
2. upstream CUPS `ippeveprinter`;
3. PAPPL-based IPP service.

No candidate is selected by reputation or image size alone.

## Red team

### Full cupsd

Risks:

- duplicates queue/job/spool concepts that FolioRelay already owns;
- broad scheduler/admin/auth/config surface;
- accidental reintroduction of WebUI, raw print, legacy backends, PPD/driver
  support, SNMP, PAM/GSSAPI, or other generic printing features;
- scheduler state can become a competing source of truth;
- more persistent files and recovery semantics to reconcile;
- larger parser/protocol/admin surface.

Failure mode to prevent:

> "CUPS accepted/changed something, FolioRelay disagrees, and both believe they
> are authoritative."

Red-team verdict: viable only if its state and administration surface can be
subordinated to FolioRelay and runtime persistence is narrow and explicit.

### ippeveprinter

Strengths are also risks:

- very small and directly implements IPP Everywhere;
- upstream documentation explicitly calls it a simple/basic print server and a
  client-testing tool;
- invokes a command for submitted jobs and exports IPP attributes through the
  environment, so the handoff command must treat all values as untrusted data;
- owns a temporary/spool directory and job model that must not silently become
  durable authority;
- production behavior under large concurrency, malformed requests, auth/TLS,
  restart/recovery, cancellation, discovery conflicts, and resource pressure is
  not established merely by protocol conformance.

Failure mode to prevent:

> Choosing the smallest binary and discovering later that we have become the
> maintainers of a production IPP server around a test utility.

Red-team verdict: strongest minimality hypothesis, but production robustness is
a veto gate.

### PAPPL

Risks:

- more framework than a minimal ingress listener;
- maintains its own system/printer/job/spool state;
- can expose web administration and raw socket support;
- uses native libraries (CUPS/libcups, TLS, DNS-SD, optional image/USB/PAM
  libraries), increasing dependency surface;
- easy to accidentally enable functionality useful to a full Printer
  Application but unnecessary for FolioRelay;
- integrating it in-process with Go would invite CGO/native-library coupling.

The upstream test fixture itself enables multi-queue, WebUI/security/network
pages, TLS UI, and raw socket functionality. Those are **test fixture defaults,
not FolioRelay production defaults**.

Red-team verdict: promising production-quality IPP framework, but a FolioRelay
adapter must be a dedicated process/container with a deliberately tiny PAPPL
option set.

## Blue team

### Cross-candidate ingress findings

Two controls are substrate-independent and are now mandatory:

1. **Ingress spool is bounded hostile scratch.** An IPP server necessarily
   receives bytes before FolioRelay's artifact admission code can inspect the
   complete object. Therefore the substrate spool must live on an independently
   bounded filesystem/quota and must not share capacity with the FolioRelay
   state journal or user artifact store. Spool exhaustion may reject/abort data
   plane jobs but must not make the control/state plane unavailable.
2. **Substrate job state is projection state.** A print substrate may keep its
   own short-lived job objects for IPP status/cancellation, but that state is not
   FolioRelay's durable authority. Restarting the substrate may lose projection
   state without losing the durable FolioRelay job/artifact/effect history.

Admission limits such as byte count and copies are enforced again by the
FolioRelay state authority even if the IPP substrate advertises its own limits.

Common controls for every candidate:

- exact upstream tag/commit pinned;
- no permanent source fork;
- local patches require an upstream issue/PR and removal condition;
- multi-stage build;
- final runtime contains no compiler, headers, source checkout, package manager,
  development tools, examples, or test programs;
- read-only runtime root;
- external durable state only where FolioRelay contract says it belongs;
- print substrate does not directly mutate FolioRelay durable state;
- handoff to FolioRelay is an idempotent command/effect protocol;
- all job metadata supplied by the substrate is untrusted input;
- no raw PostScript/PCL/PJL path for untrusted profiles;
- native discovery is optional by deployment profile rather than mandatory
  runtime surface;
- physical-printer transport is separable from virtual-printer ingress.


## Operating envelope, throttling, backpressure, and warnings

The stress results are sufficient to define a **qualified envelope**, but not a
production RPS ceiling. GitHub-hosted runner throughput is environment-specific,
so FolioRelay must not hard-code a requests-per-second throttle from CI
measurements.

The control rule is resource-driven:

- preserve protocol/control-plane responsiveness while bounded ingress scratch
  absorbs normal bursts;
- reduce worker concurrency when a deployment-defined pressure watermark is
  crossed;
- before a hard resource budget is exhausted, stop admitting new durable work
  and return a retryable/backpressure disposition;
- a retryable pressure response occurs before durable acceptance, consumes no
  idempotency key, advances no generation, and creates no effect intent;
- permanent policy/resource violations remain distinguishable from transient
  pressure;
- every automatic retry loop has an explicit finite attempt/time budget and
  preserves stable job/command identity;
- recovery from saturation uses hysteresis so the system does not oscillate at
  a threshold.

The initial proven envelope is evidence, not a capacity claim:

- CUPS and PAPPL each passed 25 simultaneous real Print-Job submissions into
  the same FolioRelay state/idempotency semantics;
- the minimal PAPPL fixture passed 100 independent clients x 100 requests and
  100 x 1000 requests with zero failed or timed-out clients;
- both active candidates pass authority-outage fail-closed/recovery semantics.

### Warning/eventing policy

Warnings should be emitted as structured, rate-limited operational events and
metrics. They are advisory: an unavailable log collector or event sink must
never change state-machine correctness.

Useful signals include substrate-spool utilization, durable-store free
capacity, pending/active work, oldest pending age, authority unavailability,
retry-budget consumption, resource rejections, idempotency conflicts, and
journal fsync latency.

Do **not** introduce a mandatory event broker for this. Structured local events
and metrics are the minimal first implementation; external routing can remain
a replaceable observer.

Hard safety belongs in invariants and admission control. Warning thresholds,
worker concurrency, and pressure watermarks are deployment-tunable policy and
must remain below the hard safety boundary.

## Veto gates

A candidate is rejected regardless of size if any required gate cannot be met
without a long-lived fork.

Required:

- native IPP client interoperability;
- IPP Everywhere engineering conformance target;
- canonical artifact handoff;
- stable job/effect correlation;
- bounded concurrent clients and requests;
- bounded request/document size;
- clean shutdown and runtime replacement;
- no duplicate external effects after crash/retry;
- TLS;
- required discovery profile;
- least privilege;
- hostile/malformed request resilience;
- no hidden durable authority outside FolioRelay;
- amd64 and arm64 build;
- upstream security-update path.

Later platform gates:

- Windows;
- Linux;
- macOS;
- Android emulator;
- iOS Simulator.

Hosted-runner limitations remain BLOCKED/HIL_REQUIRED, never converted to PASS.

## Selection metrics

Hard gates are pass/fail. Metrics only rank candidates that pass.

Measure:

- executable and runtime dependency bytes;
- executable count;
- dynamic library closure;
- package count in final OCI image;
- CVE/SBOM surface;
- startup latency;
- idle RSS;
- loaded RSS;
- request throughput;
- concurrent-client ceiling;
- malformed-request behavior;
- crash/recovery behavior;
- privileges/capabilities;
- writable paths;
- listening sockets;
- duplicated scheduler/spool state;
- custom FolioRelay glue LOC;
- number/size of carried patches;
- upstream release/security cadence.

## Autonomy

All software-only qualification is public-GHA driven.

The user is not asked to:

- build CUPS/PAPPL;
- run print commands;
- manually install clients;
- gather logs;
- compare binaries;
- execute stress tests.

Physical HIL is deferred until public runners have exhausted all faithful
software paths. A HIL request must name the exact property that cannot be
qualified in hosted environments.

## Current experiment phases

### Phase 0 — build/surface feasibility

Automatically build pinned upstream:

- CUPS 2.4.19;
- CUPS 2.4.19 `ippeveprinter-static`;
- PAPPL 1.4.11.

Run upstream/self-contained smoke tests and collect binary/runtime-closure
metrics.

### Phase 1 — FolioRelay ingress fixture

Create the smallest FolioRelay adapter for each surviving candidate and prove:

- Print-Job/Create-Job+Send-Document;
- artifact digest preserved;
- accepted command reaches FolioRelay state authority exactly once;
- cancellation/status projection;
- restart behavior.

### Phase 2 — adversarial protocol/load

Malformed IPP, slow clients, concurrency, huge attributes, page/copy abuse,
queue saturation, worker death, storage pressure, runtime replacement.

### Phase 3 — standards/client matrix

PWG plus native Windows/Linux/macOS and Android/iOS hosted clients.

### Phase 4 — decision

Adopt the lowest-surface survivor. Keep the others as qualification oracles
where useful; do not ship multiple production substrates without a distinct
required capability.


## Evidence update — concurrent acceptance

The first autonomous stress run changed candidate status.

### ippeveprinter

Observed 100 concurrent real `Print-Job` submissions:

- accepted: 6;
- rejected/busy: 94;
- FolioRelay state-authority conflicts: 0.

The rejected requests returned IPP `server-error-busy` with
"Currently printing another job."

Upstream CUPS 2.4.19 source confirms this is intentional in
`tools/ippeveprinter.c:create_job`:

- if an active job exists and has not reached a terminal state, creation
  returns `NULL`;
- the source comment states that the implementation accepts a single job at a
  time.

This is therefore not an adapter/state-authority defect.

**Disposition:** production substrate vetoed under the no-fork/minimal-system
rules. Retain `ippeveprinter` as a useful IPP Everywhere/client interoperability
oracle and lightweight test fixture.

### PAPPL

Upstream concurrency characterization:

- 25 clients × 100 requests: PASS, 0 errors;
- 100 × 100: PASS, 0 errors;
- 250 × 100: PASS, 0 errors;
- 500 × 20: PASS, 0 errors;
- 100 × 1000: completed with 7 errors.

The 100 × 100 operational gate is green. The longer 100k-request run remains a
characterization finding requiring root-cause analysis before production
selection.

### CUPS

The selectively configured CUPS 2.4.19 build completed the upstream scheduler
test suite successfully in the public runner.

This does not yet prove FolioRelay handoff/state-authority integration, but it
keeps CUPS in the active candidate set.

## Active production candidate set

After the first stress round:

1. minimal upstream CUPS scheduler — active;
2. PAPPL — active;
3. ippeveprinter — oracle only, production-vetoed.

Do not spend engineering effort adding queueing to `ippeveprinter`; that would
create exactly the maintenance fork/duplicate queue authority this bake-off is
intended to avoid.
