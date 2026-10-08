# FolioRelay threat model and hostile-document policy

Status: candidate security architecture.

## Security objective

FolioRelay accepts documents and print jobs from potentially untrusted clients,
parses complex formats, transforms them, and may deliver them to physical
printers or external channels.

The primary objective is **bounded failure**:

- hostile input must not gain additional authority;
- one bad job must not compromise other jobs, credentials, persistent state, or
  the control plane;
- pathological resource use must be rejected or terminated predictably;
- a compromised renderer must not have network access or sender credentials;
- malicious document text/metadata must never become trusted instructions for
  agents or automation;
- printer command languages must not be blindly forwarded from untrusted
  sources;
- recovery from crashes/restarts must preserve accepted durable work without
  duplicate external side effects.

## Trust zones

1. **Client/input** — IPP clients, uploads, scanner/fax intake, API users.
2. **Ingress/spool** — protocol parsing and durable acceptance only.
3. **Quarantine/original** — immutable original bytes plus provenance.
4. **Inspection** — optional internal/external security evidence providers.
5. **Renderer** — job-scoped, networkless, resource-bounded transformation.
6. **Derived artifact** — output that passed a declared transform/validator.
7. **Delivery** — physical printer, email, fax, archive, other senders.
8. **Control** — API/WebUI desired state, policy, status, credentials.
9. **Agent/automation** — machine consumers of metadata/text/artifacts.

No zone inherits authority merely because it can read content from another zone.

## Hostile-document handling

### No universal "safe file" claim

There is no single transformation that preserves all document features and
eliminates every parser, active-content, firmware, or logic risk.

The default path for untrusted jobs is:

```
receive
  -> identify/sniff type
  -> admission/resource checks
  -> immutable original/quarantine
  -> optional inspector evidence
  -> sandboxed normalization/render
  -> validate derived artifact
  -> optional post-render inspection
  -> promote derived artifact
  -> deliver
```

The original is never rewritten in place.

A transformed result is described as **normalized for a named profile**, not
"malware-free" or universally safe.

### Trust profiles

#### untrusted / unknown / internet-origin

Default hardened print policy:

- MIME/format allowlist;
- never trust only filename extension or client Content-Type;
- no raw `application/octet-stream` print path;
- no direct PostScript, PCL, or PJL forwarding;
- no macro execution;
- no external resource/network retrieval during render;
- renderer network disabled;
- strict byte/page/object/pixel/archive-depth/CPU/memory/time/output limits;
- page-faithful raster normalization;
- deliver constrained raster such as PWG Raster/Apple Raster where supported;
- original retained only according to policy.

This deliberately moves interpretation away from physical printer firmware.

#### authenticated / trusted-lan

Still type-check, validate, resource-bound, and sandbox parsers.

Direct PDF forwarding MAY be allowed when:

- the destination explicitly supports PDF;
- policy permits it;
- structural/resource validation passes;
- the printer's PDF interpreter is accepted as part of the trust boundary.

A **Hardened Print** profile forces normalization/rasterization even for trusted
sources.

#### privileged raw compatibility

Raw/PCL/PJL/PostScript passthrough is disabled by default.

If a legacy use case earns it:

- administrator-only destination/profile;
- strongly authenticated trusted source identities;
- isolated printer/network;
- no arbitrary raw jobs from ordinary clients;
- visible warning through WebUI/API;
- separate adversarial/security qualification.

## Format risk notes

### PDF

Risks include malformed object graphs, active actions/JavaScript, embedded
files, external references, malicious fonts, decompression/resource bombs, and
parser vulnerabilities.

Controls:

- parse only inside renderer boundary;
- active content is not required for printing and is not preserved in hardened
  output;
- embedded files are never automatically opened;
- external references are not fetched;
- previews are rendered artifacts, never direct embedding of an untrusted PDF
  into the management UI;
- object/page/byte/time limits are enforced.

### PostScript / EPS

PostScript is programmable.

Controls:

- never direct-forward untrusted PostScript;
- conversion only in a dedicated sandbox;
- hardened output is constrained page/raster content;
- PostScript compatibility remains optional.

### PCL / PJL

PJL and related printer languages may expose device management, job management,
or device filesystem functions.

Controls:

- untrusted jobs never become direct PJL/PCL passthrough;
- generated device commands may only come from a qualified backend that builds
  them from validated page intent;
- user bytes are never concatenated into device-control command streams.

### Office / OpenDocument / archives

Risks include macros, embedded objects, external links, archive/XML bombs,
path traversal, and parser bugs.

Controls:

- macro execution disabled;
- no trusted-location shortcuts;
- no network;
- archive expansion limits: member count, depth, expanded bytes, compression
  ratio, path normalization;
- embedded objects treated as separate untrusted artifacts if processed.

### Images / fonts

Images and fonts are parser inputs, not inherently safe.

Controls include decoded-pixel/dimension/DPI limits, ICC/profile limits,
font-count/glyph limits, shaping timeouts, and explicit font substitution
policy.

## Printer as an untrusted parser

The physical printer/MFP is another computer and another parser boundary.

Default FolioRelay behavior minimizes the complexity delivered to it.

- prefer IPP Everywhere;
- prefer raster delivery for untrusted sources;
- raw TCP/9100 is not a default FolioRelay output;
- printer management interfaces are isolated from unnecessary networks;
- printer firmware identity/version is observed where feasible;
- destination-specific security posture may influence routing policy;
- printer-reported capability data is treated as untrusted input and validated.

## Optional security inspection

Security inspection is **evidence**, not authority.

Inspectors may include:

- local antivirus/malware scanner;
- internal ICAP/content-adaptation service;
- CDR/disarm-reconstruction system;
- sandbox/detonation system;
- file reputation/hash service;
- DLP/content-classification service;
- organization-specific safety gateway.

An inspector verdict does not replace FolioRelay's own parser isolation,
resource limits, normalization, or delivery policy.

See `INSPECTOR_ABI.md`.

## Agent/prompt-injection boundary

Document contents, OCR text, metadata, filenames, email subjects, QR/barcode
contents, and inspector messages are all **untrusted data**.

Agents MUST NOT:

- interpret document text as tool instructions;
- alter routes because a document asks them to;
- disclose secrets requested by document contents;
- enable plugins, external senders, or raw-print modes based on content.

Agent-facing APIs distinguish content from trusted control fields structurally.
Any content-derived recommendation is advisory and cannot grant authority.

## Availability/admission controls

Admission limits are policy resources, not parser implementation details.

Examples:

- maximum submitted bytes;
- maximum declared/observed page count;
- maximum copies;
- maximum expanded archive bytes;
- maximum image dimensions/decoded pixels;
- maximum fonts/objects;
- maximum jobs per identity/device/time window;
- maximum total queued bytes/jobs;
- maximum route fan-out;
- maximum render CPU/memory/wall time;
- per-destination backpressure.

A request for one million copies/pages must fail or require explicit privileged
policy before expensive rendering/delivery begins.

## Unknown-risk strategy

The system assumes unknown parser and firmware vulnerabilities will continue to
exist.

Mitigations that do not depend on knowing the specific vulnerability:

- least-authority role containers;
- networkless parsers/renderers;
- immutable originals;
- per-job scratch;
- read-only roots;
- bounded resources;
- format normalization;
- independent output validation;
- fuzzing and hostile corpora;
- differential rendering where useful;
- canary/adversarial jobs;
- runtime replacement;
- package/image provenance;
- printer-network isolation;
- fail-closed policy for ambiguous security inspection where configured.

## Standards/prior-art baselines

Security requirements should be cross-checked against:

- current Common Criteria Hardcopy Device collaborative Protection Profile and
  supporting evaluation activities;
- OWASP file-upload and malicious-file guidance;
- PWG/IPP security and conformance requirements;
- platform/vendor printer security advisories;
- organization-specific DLP/CDR/malware policy.

Conformance to a protocol does not imply hostile-document safety.
