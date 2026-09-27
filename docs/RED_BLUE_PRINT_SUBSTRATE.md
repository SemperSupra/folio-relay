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
