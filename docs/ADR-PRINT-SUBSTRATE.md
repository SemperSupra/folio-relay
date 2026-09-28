# ADR: Printing substrate — custom upstream build, no CUPS fork

Status: proposed for qualification.

## Decision direction

FolioRelay should control its own printing runtime artifacts, but should **not**
carry a long-lived source fork of CUPS unless qualification proves an
unavoidable upstream defect.

The preferred order is:

1. determine the minimum printing substrate actually required by FolioRelay;
2. build that substrate ourselves from pinned upstream source;
3. compile/package only required features and runtime artifacts;
4. keep local patches at zero whenever possible;
5. submit required fixes upstream rather than carrying permanent forks.

## Why this matters

FolioRelay now owns:

- durable job/artifact state;
- idempotent orchestration;
- route-leg state;
- retry/ambiguity policy;
- hostile-document policy;
- effect execution.

A general-purpose spooler should therefore not become a competing durable state
authority.

## Candidate A — minimal upstream CUPS scheduler

Build the current supported CUPS source ourselves in a multi-stage build and
install only the runtime artifacts required by FolioRelay.

Candidate configuration/defaults:

- web interface disabled;
- raw printing disabled;
- default sharing disabled unless the FolioRelay profile enables it;
- page logging disabled unless required for audit;
- only required language/locale data;
- D-Bus disabled unless qualification proves it is needed;
- PAM/GSSAPI/LDAP/etc. omitted when not required;
- DNS-SD compiled/installed only for the discovery profile that needs it;
- no LPD/SMB legacy server/backend support;
- no legacy PPD/driver bundles in the default profile;
- no development headers, compiler, shell tooling, examples, documentation, or
  administration CLIs in the final runtime unless a specific runtime function
  requires them.

The build stage may contain the complete toolchain. The runtime stage receives
only the selected scheduler/libraries/backend/filter/data files.

FolioRelay configuration remains explicit at runtime; build-time defaults are
defense-in-depth, not the sole policy enforcement.

### Maintenance rule

Track upstream release tags/digests directly. A new upstream CUPS security
release should normally be adoptable by changing the pinned upstream version,
rebuilding, and rerunning qualification — not by rebasing FolioRelay source
patches.

## Candidate B — IPP Everywhere ingress without full cupsd

OpenPrinting's `ippeveprinter` provides a simple IPP Everywhere server and can
run a command for every submitted job.

This maps unusually well to FolioRelay:

```
IPP client
  -> IPP server
  -> immutable accepted artifact
  -> FolioRelay state authority
  -> renderer/route effects
```

Advantages:

- avoids a second general-purpose spool/job authority;
- much smaller conceptual surface;
- easier mapping between IPP submission and FolioRelay job identity;
- potentially fewer CUPS administration/authentication mechanisms.

Risks:

- upstream describes it as a simple/basic print server and test tool;
- production concurrency, persistence, authentication, discovery, accounting,
  stress, and interoperability behavior must be independently qualified;
- we must not copy/fork the implementation casually and inherit a private IPP
  server maintenance burden.

Treat `ippeveprinter` first as an oracle/prototype substrate, not an automatic
production choice.

## Candidate C — PAPPL-based IPP service

PAPPL is an actively maintained framework for Printer Applications and provides
IPP-service, spooling, printer/application, TLS, and discovery primitives.

Potential fit:

- standards-facing IPP server without the entire CUPS 2.x scheduler surface;
- upstream-maintained IPP implementation;
- aligns with OpenPrinting's modular direction;
- may provide a more production-oriented foundation than `ippeveprinter`.

Risk:

- FolioRelay already owns job/state orchestration, so PAPPL's own job/spool
  model may still duplicate authority;
- C library integration may introduce a native runtime/CGO boundary unless kept
  in a dedicated printing process/container;
- must prove that the framework can be constrained to FolioRelay's state and
  security invariants.

## Physical printer output

Physical printer transport is separable from virtual-printer ingress.

For modern printers, prefer IPP/IPPS/IPP Everywhere.

Do not keep full CUPS merely to obtain legacy output backends.

Optional legacy USB/PPD/driver support may be a separately selected compatibility
profile/image if it earns its maintenance and attack surface.

## Qualification experiment

Build three public-GHA fixtures:

1. minimal custom-built `cupsd`;
2. minimal `ippeveprinter` ingress fixture;
3. PAPPL-based minimal printer application fixture.

Compare:

- IPP Everywhere/PWG conformance;
- Windows/Linux/macOS/iOS/Android interoperability;
- AirPrint/DNS-SD behavior;
- authentication/TLS;
- concurrent submission;
- queue/backpressure behavior;
- million-page/copy/pathological metadata rejection;
- malformed/truncated client/job behavior;
- crash/restart semantics;
- artifact handoff correctness;
- privilege/capability requirements;
- package/executable count;
- image size;
- CVE/dependency surface;
- idle/loaded RSS and startup latency;
- amount of duplicate state relative to FolioRelay;
- maintenance/update complexity.

## Selection rule

Choose the **smallest upstream-maintained substrate that satisfies required
interoperability and robustness without becoming a competing durable state
authority**.

Image size is secondary.

## Fork threshold

A CUPS/PAPPL source patch may be carried temporarily only when:

- an upstream issue/PR is linked;
- the patch is minimal and separately tested;
- removal conditions are documented;
- security updates can still be rebased automatically.

A permanent FolioRelay fork requires explicit architectural approval and must
demonstrate that the required behavior cannot be achieved through supported
configuration, selective build/install, wrapper isolation, or upstream change.
