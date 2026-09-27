# ADR: Go for the FolioRelay core

Status: proposed and favored; to be confirmed by public-GHA A/B qualification.

## Decision

Rewrite the FolioRelay **core runtime** from the current prototype Python into Go
before the public API/ABI reaches 1.0 stability.

The preferred production shape is **one Go codebase compiled into role-specific
static binaries**, not one all-capability multicall executable.

Candidate commands:

```
cmd/foliorelay-control
cmd/foliorelay-normalizer
cmd/foliorelay-fax-null
cmd/foliorelay-email-null
cmd/foliorelay-cups-agent
cmd/foliorelay-doctor
```

Shared implementation lives under internal packages, but each `cmd/*` imports
only the packages required by that role. Go's linker can then eliminate
unreferenced code, and each final runtime image receives only the binary it
actually executes.

Deployment still runs separate containers/processes for authority and
failure-domain isolation. Shared source and shared build tooling do not imply a
shared deployed executable.

## Why Go fits the current product

The existing Python prototype mostly uses standard-library capabilities:

- HTTP/JSON API;
- filesystem watching/polling and atomic rename;
- hashing;
- JSON/JSONL state and events;
- subprocess execution for the CUPS boundary;
- reconciliation loops;
- durable queue inspection;
- health/status endpoints.

All are straightforward in Go without requiring a large third-party runtime.

A static Go build can run in a distroless-static or scratch-style runtime with
no interpreter, shell, or package manager.  The application binary becomes the
dominant executable surface.

## What this changes

### Core image

Target:

- exactly the role-specific static FolioRelay binary required by that container;
- CA certificates only where network TLS is actually needed;
- timezone data only where runtime-local zone rules are needed;
- non-root user;
- read-only root;
- no shell;
- no package manager;
- no Python;
- no CUPS/Avahi/Ghostscript.

### CUPS image

The CUPS image still exists because the CUPS/IPP/Avahi/Ghostscript dependency
boundary is fundamentally different from the core.

However, the Go shift removes the reason for Python to exist in the CUPS image.
The CUPS image receives only the minimal `foliorelay-cups-agent` binary needed
for FolioRelay bootstrap/reconciliation, not the control API, normalizer, or
sender implementations.

The size reduction will therefore be large for the core image and modest for
the CUPS image, where CUPS and document-processing dependencies dominate.

## Why not one giant Go+CUPS image

Changing language does not change the authority boundaries:

- control/API needs network but not CUPS privilege;
- normalizer should normally have no network;
- CUPS needs a printer/network/discovery-specific privilege envelope;
- renderers parse hostile documents and should be sandboxable;
- real senders may hold external-delivery credentials.

A single all-purpose image would carry unnecessary CUPS/parser/network
dependencies into every role even if only one binary is used.

The packaging split and the executable split are independent.

The preferred runtime images are small role images/layers produced from the same
build graph:

```
folio-relay-control     = distroless-static + control binary
folio-relay-normalizer  = distroless-static + normalizer binary
folio-relay-fax-null    = distroless-static + null-fax binary
folio-relay-email-null  = distroless-static + null-email binary
folio-relay-cups        = CUPS stack + cups-agent binary
```

These share the same tiny base layers and source revision, so storage/build
deduplication remains high while executable attack surface is minimized.
Additional renderer/sender images are created only when a dependency/trust
boundary earns one.

## Renderer/plugin implication

The renderer ABI remains language-neutral and process/container oriented.

Go does **not** require OCR, office-document, PDF, or ML tooling to be rewritten
in Go.  A renderer may remain Python/Rust/C++/Java/etc. inside its own qualified
image when that ecosystem is the best tool for the job.

This is a key reason to migrate the orchestration/control core to Go while
keeping plugin ABIs external.

## CGO policy

Prefer `CGO_ENABLED=0` for the core binary.

CGO may be enabled only for a component whose required native integration
cannot reasonably be isolated behind an external process/plugin boundary.

A CGO dependency changes the runtime-base and reproducibility assumptions and
must therefore be explicit in qualification evidence.

## Persistence

The language change does not change the durable-state model.

The Go implementation must preserve:

- external `/config`, `/spool`, and `/data` authority;
- atomic writes/renames;
- idempotent client-selected job identifiers;
- desired/observed generations;
- append-only event/provenance semantics;
- runtime replacement recovery.

No embedded local database is introduced merely because Go makes one easy to
bundle.  A database has to earn its keep separately.

## Qualification before finalizing the decision

Public GHA should compare Python prototype vs Go candidate on representative
core functions:

- compressed/unpacked image size;
- package count;
- executable count;
- known CVEs;
- cold start;
- idle RSS;
- concurrent job throughput;
- control-plane latency;
- file normalization throughput;
- 100/1,000-client concurrency;
- malformed/adversarial inputs;
- runtime replacement recovery;
- amd64/arm64 builds;
- reproducibility/provenance/SBOM;
- race detector and fuzz tests.

Go-specific gates:

- `go test -race ./...`;
- native fuzz targets for parsers/validators/state transitions;
- `go vet`;
- static analysis;
- no unexpected CGO linkage;
- binary dependency/build-info inspection.

## Migration strategy

Do not translate line-by-line.

1. freeze existing public API/ABI behavior in conformance tests;
2. implement the control/state types and atomic persistence primitives in Go;
3. port the control API;
4. port normalizer;
5. port null senders;
6. port CUPS reconciler/bootstrap helper;
7. run Python and Go implementations against the same black-box test suite;
8. switch the reference runtime only when behavior and stress/failure gates pass;
9. remove Python from production images.

The existing Python prototype remains a behavioral oracle during the migration
and can then be retired from production.

## Reversal rule

If the A/B evidence shows that the Go rewrite materially harms correctness,
platform compatibility, or development velocity without a compensating runtime
benefit, revisit the decision before 1.0.

The choice is an engineering decision, not a language preference.
