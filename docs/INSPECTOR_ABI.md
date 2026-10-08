# FolioRelay security inspector ABI

Status: candidate plugin contract.

## Purpose

Optional inspectors let deployments route an artifact to an internal or
external safety/security system **without coupling the FolioRelay core to a
specific antivirus, CDR, sandbox, DLP, or reputation product**.

ABI:

`foliorelay.inspector/v1`

Inspectors return evidence. The FolioRelay policy engine makes the routing
decision.

## Minimal interface

Input:

```json
{
  "abi": "foliorelay.inspector/v1",
  "request_id": "01K...",
  "artifact": {
    "id": "sha256:...",
    "media_type": "application/pdf",
    "bytes": 123456,
    "sha256": "..."
  },
  "stage": "pre-render",
  "trust_profile": "untrusted"
}
```

Result:

```json
{
  "abi": "foliorelay.inspector/v1",
  "request_id": "01K...",
  "verdict": "suspicious",
  "confidence": "medium",
  "findings": [
    {
      "category": "malware",
      "identifier": "example.signature",
      "severity": "high"
    }
  ],
  "engine": {
    "id": "example",
    "version": "1.2.3",
    "definition_version": "..."
  },
  "transformed_artifact": null
}
```

Verdicts:

- `clean`
- `malicious`
- `suspicious`
- `unknown`
- `error`
- `timeout`
- `unsupported`

"clean" means only "the named inspector reported no finding under its current
engine/definitions". It does not mean safe.

## Inspector classes

### local-socket

Example: ClamAV over a Unix socket.

Preferred properties:

- no network listener;
- read-only scan input or descriptor/stream;
- scanner signatures/database stored in its own persistent store;
- scanner cannot modify FolioRelay artifacts;
- scanner process cannot access sender credentials.

### internal-network

Examples:

- internal ICAP adaptation service;
- enterprise CDR;
- malware sandbox;
- DLP service.

Controls:

- explicit hostname/IP/service identity;
- TLS/mTLS where supported;
- egress restricted to that service;
- size/time limits;
- no redirects to arbitrary destinations;
- result provenance preserved.

ICAP is a useful adapter target because it is a documented content-adaptation
protocol, but policy remains FolioRelay's responsibility.

### external-service

Disabled by default.

Sending user content outside the deployment may disclose confidential material.

External inspection therefore requires an explicit policy that specifies:

- which trust classes may leave the system;
- allowed provider/service;
- whether full content, a derived artifact, or only a hash may be sent;
- region/jurisdiction requirements;
- retention/training/privacy terms acknowledged by the administrator;
- maximum size;
- timeout/failure behavior;
- audit/provenance requirements.

No external inspector is enabled merely by installing its plugin.

### hash/reputation-only

Where supported, a deployment may query reputation using only a cryptographic
hash before deciding whether full-content inspection is justified.

Hash disclosure is still recorded as external metadata egress.

## CDR relationship

Content Disarm and Reconstruction is modeled as a **renderer/transformation**,
not merely a verdicting inspector.

An inspector may recommend or require a CDR profile, but any reconstructed file
is a new FolioRelay artifact with its own digest and provenance.

This prevents a security vendor from invisibly mutating the original.

## Stages

Policies may request inspection at:

- `pre-render` — original artifact;
- `post-render` — normalized/derived artifact;
- `pre-delivery` — final output about to cross a trust boundary.

Running all three is not the default. Every stage must earn its latency/cost.

Typical untrusted print profile:

```
original
 -> local malware scan (optional)
 -> hardened renderer
 -> output validator
 -> printer
```

Typical external email/archive profile may additionally inspect the final
attachment before delivery.

## Policy examples

### fail closed

For a high-security route:

- malicious/suspicious/unknown/error/timeout => quarantine;
- clean => continue to sandboxed render.

### fail bounded

For ordinary local printing:

- malicious => quarantine;
- suspicious/unknown/error/timeout => hardened raster render;
- clean => normal configured render path.

### advisory

Inspector findings are logged and surfaced but do not block. Suitable only when
an administrator deliberately chooses that posture.

## Privacy and security requirements

- inspector findings are untrusted input and must be schema/size validated;
- plugins cannot grant their own network/mount/credential authority;
- no document text is copied into logs by default;
- credentials are plugin-scoped;
- result caches are keyed by artifact digest + inspector identity/version +
  definition version + policy;
- cached clean verdicts expire when definitions/policy require re-evaluation;
- a malicious inspector or compromised external service cannot directly alter
  destination configuration or sender authority.

## Baseline adapters worth qualifying

These are optional adapters, not mandatory runtime components:

- ClamAV local Unix-socket scanner;
- generic ICAP adapter;
- generic HTTPS inspector adapter with strict destination allowlisting;
- null inspector for contract testing.

A third-party CDR/sandbox/DLP system should normally fit behind one of these
without adding product-specific logic to FolioRelay core.
