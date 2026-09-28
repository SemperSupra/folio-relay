# Runtime image and hardening policy

Status: bootstrap contract for the FolioRelay extraction.

## Current prototype assessment

The current TrueNAS Foundry prototype deliberately optimized for discovery of
working behavior rather than final image quality.  It reuses
`chuckcharlie/cups-avahi-airprint:2.1.3` for CUPS **and** for the control,
normalizer, and null-sender processes.

That image is not an acceptable FolioRelay production runtime baseline.

Observed excess/risks in the upstream image include:

- Alpine 3.20 plus manually appended `edge/testing`, `edge/community`, and
  `edge/main` repositories;
- build-only packages retained in the runtime image (`cups-dev`,
  `build-base`, git, cmake, wget, etc.);
- multiple legacy printer stacks bundled whether needed or not (HPLIP, SpliX,
  brlaser, Gutenprint);
- source builds in the final image rather than a multi-stage build;
- fetched source archives/repositories without a FolioRelay-owned immutable
  provenance/checksum contract;
- Python present in the CUPS image because the prototype embeds gateway code
  there;
- all gateway worker services currently inherit the CUPS image and run as root;
- broad upstream CUPS defaults such as `ServerAlias *` and
  `DefaultEncryption Never`;
- the upstream entrypoint enables shell command tracing while handling the CUPS
  password;
- native CUPS job request data defaults to `/var/spool/cups`, which is
  container-local in the prototype and therefore violates the disposable
  runtime invariant.

These are migration inputs, not accepted product behavior.

## Target image graph

Keep the build graph small, but optimize **deployed executable surface** rather
than image-count aesthetics.

A shared source tree and shared distroless base produce role-specific final
images/binaries. OCI layer deduplication means this does not imply large storage
duplication.

### Core role images

The control API/WebUI backend, normalizer, null senders, and other small workers
are built from the same Go source revision but receive separate static binaries.
No role image contains dormant implementations for another authority class.

Target properties:

- static Go role binary only;
- distroless-static/non-root runtime is the preferred candidate;
- no shell;
- no package manager;
- no compiler/build toolchain;
- no CUPS packages;
- no Avahi/D-Bus;
- no printer drivers;
- no network utilities unless the process actually needs network access;
- default non-root UID/GID;
- read-only root filesystem;
- writable paths limited to explicit external persistent mounts and bounded
  tmpfs scratch;
- `CAP_DROP=ALL`;
- `no-new-privileges`;
- no device access;
- renderer/normalizer workers use `network_mode=none` when network is not part
  of their contract.

Role images reuse identical base layers and build provenance, but each final
image contains only its role binary. Runtime authority is also restricted
independently at deployment time.

### `ghcr.io/sempersupra/folio-relay-cups`

Contains only the printing/discovery functionality that belongs in the CUPS
boundary.

Default runtime package intent:

- CUPS scheduler/libraries/client utilities required by reconciliation;
- CUPS-PDF only while the prototype still uses it;
- Ghostscript only if required by the qualified PDF path;
- Avahi/D-Bus only if the supported discovery mode requires them;
- timezone/CA material as required.

Explicitly absent from the default image unless qualification demonstrates a
real requirement:

- HPLIP;
- SpliX;
- brlaser;
- Gutenprint;
- CUPS development headers;
- compiler/linker/build toolchains;
- git/cmake;
- generic download/debug tools;
- Python after CUPS/gateway separation is complete;
- control/normalizer/sender code not required by the CUPS agent.

Modern physical printers use IPP Everywhere. Legacy printer support, if it
earns its keep, becomes an explicit optional image/profile rather than silently
expanding every installation.

CUPS is the only currently anticipated service with a documented root/startup
exception.  Its capabilities are experimentally minimized and the service is
isolated from renderer/sender credentials and unrelated user data.

### Renderer images

A renderer gets a separate image only when its dependency or parser isolation
justifies one, for example OCR or a complex document conversion stack.

Renderer defaults:

- network disabled;
- read-only input;
- bounded writable output/scratch;
- non-root;
- no sender credentials;
- CPU/memory/time/output-size limits;
- exact dependency/image identity recorded in provenance.

### Sender images

Network-capable sender plugins are isolated from renderers and receive only the
credential/egress authority necessary for their channel.

The `null` senders remain in the core image because they perform no network
side effects.

## Role authority matrix

The binary is not the security boundary by itself. Each deployed role has an
independent authority envelope.

| Role | User | Network | Persistent mounts | Devices | Linux capabilities | Credentials |
|---|---|---|---|---|---|---|
| control | non-root | management/API only | desired-state store; minimal document metadata access | none | none | scoped control-plane auth material |
| normalizer | non-root | **none** | spool + document/artifact store | none | none | none |
| fax-null | non-root | **none** | fax outbox/status only | none | none | none |
| email-null | non-root | **none** | email outbox/status only | none | none | none |
| cups | minimized startup exception | IPP + explicitly selected discovery/printer networks | CUPS config + accepted-job spool only | USB only when explicitly enabled | experimentally minimized CUPS set | CUPS-local bootstrap secret only if still required |
| cups-agent | preferably non-root peer identity | none or local-only | CUPS socket/runtime state + desired queue subset | none | none | none |
| renderer | non-root | **none by default** | read-only input + bounded output/scratch | none | none | none |
| inspector-local | non-root | none/local Unix socket only | read-only artifact input + scanner state as required | none | none | scanner-specific only |
| inspector-external | non-root | explicit inspector endpoint only | read-only artifact stream + result cache | none | none | only that inspector's scoped secret |
| real sender | non-root | channel-specific egress only | channel outbox + result state | none | none | only that sender's scoped secret |

The CUPS container should not mount the general user document store when a
narrow spool handoff is sufficient.

No FolioRelay Go binary carries Linux **file capabilities**. Capabilities are
assigned only in the deployment manifest so the authority is visible,
materializer-specific, testable, and removable without rebuilding the binary.

Use `no-new-privileges` everywhere unless a documented startup path
demonstrates that it cannot function with it.

After the basic role boundaries pass, public CI should derive syscall evidence
for each role and evaluate tighter seccomp/AppArmor/SELinux profiles. Do not
blindly copy one syscall allowlist between roles; Go runtime syscalls plus each
role's I/O pattern must be measured.

## Build policy

### Multi-stage builds

Build/compile/download stages are separate from runtime stages. Runtime images
receive only the final application/runtime files they need.

No compiler, package headers, git checkout, source archive, package cache, or
build tool survives into a production runtime image unless it is itself a
runtime requirement.

### Stable bases only

Do not mix a stable distribution release with rolling/edge repositories.

Base image distribution/version is explicit and digest pinned after update
automation has resolved the current approved digest.

At bootstrap time:

- Alpine 3.24 stable is a candidate for the CUPS image;
- Debian 13 distroless static `nonroot` is the preferred candidate for role-specific static Go binaries.

Base selection is evidence-driven rather than ideological.  Size, CVE burden,
architecture support, CUPS/Python compatibility, debugging/operations, and
reproducibility are measured.

### Immutable inputs

Production builds pin/verify:

- base-image digest;
- package/repository release;
- source commit/tag plus checksum when source compilation is unavoidable;
- Go module/toolchain lock and checksums for application dependencies;
- build actions by immutable commit SHA.

No `curl | sh`, unverified tarball download, floating Git branch, or
stable+edge package mixture is permitted in a release build.

### Build secrets

No secret is passed via Dockerfile `ARG`, persisted `ENV`, copied file, or
layer.  If a build genuinely requires private material, BuildKit secret/SSH
mounts are used and the final layer is checked for residue.

Normal public FolioRelay image builds should require no secret at all.

### Reproducibility

Images should be reproducible enough that:

- the build inputs are completely enumerated;
- source date/version metadata is explicit;
- generated content avoids uncontrolled timestamps/randomness;
- rebuilding the same release can be compared structurally and, where
  achievable, byte-for-byte.

## Runtime policy

### Stateless/disposable

Runtime writable layers are never an authority.

Durable external state includes:

- desired configuration;
- printer/destination definitions;
- route/output profiles;
- accepted print-job request/spool data;
- user documents/artifacts;
- fax/email outboxes;
- retry/idempotency state;
- durable provenance/events according to retention;
- stable TLS/device identity;
- plugin selection/configuration;
- import/fingerprint ledgers.

The CUPS `RequestRoot` must therefore be placed on the external FolioRelay
spool store rather than container-local `/var/spool/cups`.

Qualification destroys containers while jobs are queued/held and verifies that
fresh instances recover them from persistent storage.

### Read-only root filesystems

Every service uses `read_only: true` unless an evidence-backed incompatibility
is documented.

Writable locations are explicit:

- external persistent volume/bind;
- bounded `tmpfs` for `/tmp`, `/run`, or equivalent ephemeral needs.

A process that unexpectedly attempts to mutate its image/root filesystem should
fail qualification rather than causing the deployment to become writable by
default.

### Least privilege

Default:

- non-root;
- all capabilities dropped;
- no-new-privileges;
- no host PID/IPC namespace;
- no Docker/Podman socket;
- no devices;
- no host network;
- no network at all for workers that do not need it.

Each exception is service-specific, documented, and covered by a test proving
that removing the privilege breaks an intended function.

### CUPS administration

FolioRelay owns desired printer state. Reconciliation uses the local CUPS Unix
socket/peer boundary, not a password-bearing network admin request.

Remote CUPS administration is not a normal product surface. The FolioRelay
control plane is the normal management interface.

Avoid `ServerAlias *`; CUPS documentation explicitly warns that it can expose
DNS-rebinding risk. Advertised names/aliases are explicit.

### Secrets

Runtime secrets come from deployment secret providers/files with restrictive
permissions and are never:

- baked into an image;
- stored in a public Compose file;
- put in DNS-SD/IPP metadata;
- put in command-line arguments;
- printed to logs;
- copied into job metadata.

Shell tracing is prohibited in credential-bearing startup paths.

## Supply-chain policy

Every release image produces and publishes:

- exact OCI digest;
- multi-architecture manifest for supported architectures;
- SBOM attestation;
- build provenance attestation;
- source revision and license metadata;
- vulnerability scan result;
- package/file inventory diff against the previous release.

Release qualification fails on unexpected package additions.

Signing/verification policy will be selected before external release; the
architecture assumes signatures/attestations are verifiable independently of a
mutable tag.

## Surface-area budgets

Image size is an observation, not the primary security metric.  We track both
bytes and **what executable/package surface exists**.

For each image record:

- compressed/uncompressed size;
- OS/package count;
- executable count;
- setuid/setgid files;
- Linux file capabilities;
- open/listening ports;
- effective runtime capabilities;
- writable paths;
- network reachability;
- known vulnerabilities by severity;
- SBOM package delta from the previous release.

Budgets are initially baselined from the first purpose-built images and may only
grow with an explicit reason.

## Qualification

The public free-tier GHA pipeline will include:

1. build each production image using BuildKit/buildx;
2. inspect history/layers for accidental secrets and build residue;
3. generate/attach SBOM and provenance;
4. scan vulnerabilities;
5. run container structure/policy tests;
6. verify declared user, capabilities, read-only root filesystem, mounts,
   ports, and network modes;
7. run product unit/integration/protocol/client tests;
8. destroy/recreate runtimes while durable state and queued jobs exist;
9. chaos/stress/failure tests;
10. compare package/executable/image-size deltas against the previous qualified
    baseline.

A smaller image that fails functionality is not an optimization.  A larger
dependency that is actually required and isolated may earn its keep.


## Immutable engineering-build input tuple

The packaged CUPS and PAPPL qualification images now use an explicit immutable
input tuple rather than resolving floating source tags and current package
archives during each build.

Pinned inputs:

- Ubuntu base: `ubuntu:24.04@sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3`;
- Go builder: `golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195`;
- Ubuntu archive snapshot: `20260928T000000Z`;
- CUPS 2.4.19 source commit: `6ba0487abb05afc93d639f37676add8fd65d3756`;
- PAPPL 1.4.12 source commit: `6db8e137557ad84662e78d24fdb2a591c621f4ac`.

The human-readable release tags remain metadata, but builds fetch the exact
commit SHA. Runtime image labels record the source revision, base identities,
and Ubuntu snapshot used.

Ubuntu 24.04's supported APT snapshot mechanism freezes package resolution to
the selected archive time instead of silently consuming whatever packages are
current on a future rebuild. An intentional security/update transaction changes
the source/base/snapshot tuple and reruns the complete qualification suite before
the new tuple is admitted.

This does not claim byte-for-byte reproducibility yet. The next qualification
gate compares repeated no-cache rebuilds for package inventory, declared input
labels, and key runtime artifacts and records any remaining nondeterministic
build products rather than masking them.


### PAPPL 1.4.12 update rehearsal

The first real immutable-input update transaction uses PAPPL 1.4.12, published
2026-08-20. Upstream describes it as a bug-fix release and lists overflow
protection in dithering and ready-media handling plus HTTP error-path, printer
deletion locking, USB, and polling fixes.

The admitted candidate tuple moves from:

- `v1.4.11` / `ad86a0a8473f0f83233a374226561181570d4f81`

to:

- annotated tag `4f6a8d87dde5ab99d33abe172f767e6db1beb20d`;
- source commit `6db8e137557ad84662e78d24fdb2a591c621f4ac`.

All qualification workflows fetch that exact commit rather than the mutable tag
name. The update is admitted only after the normal protocol, stress,
architecture, packaged-runtime, restart/idempotency, conformance, and repeated
no-cache rebuild gates pass on the new tuple.
