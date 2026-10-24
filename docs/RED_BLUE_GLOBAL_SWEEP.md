# Red/blue team and global concept sweep

Status: living architecture review.

## Method

Review FolioRelay against:

- known-known failure/security modes;
- known unknowns requiring qualification;
- latent prior art/requirements we may otherwise overlook;
- unknown-unknown mitigation patterns;
- minimum-system test: every component must justify its authority, dependency,
  latency, state, and operational cost.

The sweep used English and non-English printer/security terminology and public
guidance from international/national hardcopy-security communities.

## Findings that changed the architecture

### Printer languages are a security boundary

Japanese hardcopy-security guidance explicitly treats PJL/PCL/PostScript page
description languages as capable of job-management, configuration, and in some
implementations filesystem operations. Japanese evaluation reports have tested
malicious PostScript/PJL/TIFF/PDF print inputs.

Recent Spanish/CERT reporting documents printer RCE/elevation vulnerabilities
triggered by PostScript print jobs.

**Blue-team action:** untrusted jobs never get raw PostScript/PCL/PJL
passthrough. Prefer constrained raster output to physical printers.

### The printer itself must be isolated

French CERT guidance has long treated reachable printers and raw printing
interfaces as security-relevant, including cross-site printing and exposed
device files/configuration.

**Blue-team action:** FolioRelay does not require raw port 9100 as a default
backend, keeps printer management networks narrow, and treats discovered printer
metadata/capabilities as untrusted.

### Confidentiality continues after "successful print"

German BSI hardcopy guidance highlights unattended printed pages and fax output
as confidentiality risks.

**Opportunity:** support held/secure-release printing as a first-class route
profile where IPP/device capability permits it. Avoid claiming job success as
equivalent to confidential delivery.

### Client-side helpers create their own attack surface

Current French CERT advisories include arbitrary-code-execution vulnerabilities
in commercial print-deployment clients.

**Blue-team action:** optional FolioRelay clients remain unprivileged,
role-minimal, signed/updateable, and are not required for the standards-native
path. No legacy kernel printer driver.

### Malicious documents require layered handling

OWASP file-upload guidance independently converges on:

- allowlist expected formats;
- sniff/validate type rather than trusting headers/extensions;
- generated storage identities;
- byte/expansion limits;
- antivirus/sandbox where available;
- CDR where appropriate;
- awareness that external scanning services create data-leakage risk.

**Blue-team action:** immutable quarantine + optional inspector + sandboxed
renderer + derived-artifact validation.

### Security scanning is not binary truth

ClamAV documentation explicitly treats its daemon as a scanning toolkit, warns
against unauthenticated exposure of its TCP socket, and warns that false
positives make blind deletion dangerous.

**Blue-team action:** local scanners use Unix sockets where possible; verdicts
are evidence; quarantine instead of destructive deletion; scanner result never
bypasses renderer isolation.

### Content-adaptation is established prior art

ICAP provides a generic content-adaptation interface and explicitly includes
virus scanning as a use case, while leaving policy decisions outside the
protocol.

**Opportunity:** a generic inspector ABI with an ICAP adapter matches existing
enterprise security architecture without coupling FolioRelay to a vendor.

### Hardcopy security has a formal evaluation vocabulary

The Common Criteria Hardcopy Device international community publishes a current
collaborative Protection Profile and supporting evaluation activities.

**Opportunity:** map FolioRelay's security claims/tests against relevant HCD
cPP threat/objective categories even though FolioRelay is not itself claiming
Common Criteria certification.

## Known-knowns

- malicious/malformed PDF/PostScript/PCL/PJL/image/font input;
- decompression/archive/XML bombs;
- enormous page/copy/image/object counts;
- queue and storage exhaustion;
- printer offline/full/stopped/held states;
- slow or malicious clients;
- duplicate/replayed jobs;
- route fan-out amplification;
- parser crashes;
- renderer hangs;
- external-resource fetch/SSRF;
- credentials in logs/jobs;
- discovery spoofing;
- malicious printer capability metadata;
- printer firmware vulnerabilities;
- raw TCP printing abuse;
- unattended confidential paper output;
- sender retry duplication;
- compromised plugin/supply-chain dependency;
- client helper compromise;
- external inspection data leakage;
- document-based prompt injection into agents.

## Known unknowns

Require measurement/qualification rather than architectural guessing:

- exact Windows/macOS/Linux/iOS/Android discovery behavior on every current
  client build;
- CUPS versus strict IPP Everywhere DNS-SD behavior;
- printer firmware-specific PDF/raster bugs;
- practical raster resource limits for pathological documents;
- optimal admission defaults without harming legitimate engineering/large-format
  documents;
- exact syscall envelopes for Go roles;
- whether CUPS can be run without root after bootstrap in the selected base;
- which printer capabilities reliably implement secure/held release;
- performance/cost of optional malware/CDR stages;
- third-party inspector privacy/retention semantics;
- multilingual shaping/font corner cases.

## Latent requirements / prior art worth mining

- Common Criteria HCD cPP: authentication, access control, stored-data
  protection, trusted communications, audit, software update verification,
  self-protection/separation concepts;
- secure/pull printing and job release;
- ICAP/content-adaptation;
- AV + sandbox + CDR pipelines;
- DLP classification as advisory policy input;
- malware hash reputation before content upload;
- quarantine lifecycle and analyst/manual release;
- printer fleet firmware posture as destination metadata;
- rate/quota/accounting controls.

Each is a candidate capability, not an automatic component.

## Unknown-unknown mitigation

We cannot enumerate future parser or firmware vulnerabilities. Therefore use
controls that remain useful against unanticipated faults:

- job-scoped parser isolation;
- networkless renderer;
- independent artifact validation;
- content-addressed immutable originals;
- resource quotas;
- fail-safe state machines;
- fuzzing/property tests;
- mutation corpora;
- differential parser/render testing;
- chaos/fault injection;
- canary documents;
- runtime replacement;
- narrow mounts/egress;
- independent supply-chain evidence;
- explicit trust boundaries;
- security events that preserve enough provenance for postmortem without logging
  document bodies.

## Minimal-system test

A proposed component is accepted only if at least one required capability cannot
be provided comparably well by an existing boundary.

### Required core components

- standards-facing IPP/print ingress;
- durable artifact/job state;
- control API;
- normalizer/renderer boundary;
- destination/sender boundary;
- persistent external stores.

### Optional components that must earn deployment

- Avahi/mDNS: only when LAN discovery is enabled;
- Ghostscript: only if a qualified conversion requires it;
- legacy printer drivers: only for explicit legacy profile;
- antivirus scanner: optional inspector;
- CDR: optional renderer;
- external sandbox/DLP: optional inspector;
- Windows/Android/iOS helper: only where native path or richer route UX benefits;
- OCR/EPUB/Markdown renderer: only when requested.

There is no always-on "security appliance" container in the base product.

## Efficiency principles

- reject pathological jobs before expensive parsing when metadata is sufficient;
- do not scan/render the same content repeatedly: cache by content digest +
  exact engine/plugin/policy identity;
- use backpressure rather than unbounded worker creation;
- separate admission from rendering and delivery;
- bounded worker pools;
- weighted scheduling so one enormous job cannot starve ordinary jobs;
- independent per-destination queues/circuit breakers;
- keep control/status responsive when data plane is saturated;
- batch/stream where doing so does not weaken atomicity or validation.

## Safety state model

Security/format processing should expose states such as:

- accepted
- quarantined
- inspecting
- inspection_unknown
- rendering
- policy_rejected
- malformed
- resource_rejected
- ready
- delivery_blocked
- delivered

Do not collapse "scanner error", "unknown", and "clean" into one outcome.

## Next qualification additions

- EICAR-safe malware detection integration test;
- malformed polyglot and spoofed MIME corpus;
- PDF/PostScript/image/font fuzz corpus;
- archive/zip/XML bombs with safe generated test fixtures;
- page/copy-count admission tests including million-page/copy requests;
- 100/1,000 concurrent submitters;
- queue/full-disk/stopped-printer recovery;
- renderer OOM/timeout/kill;
- inspector timeout/error/malicious/suspicious/unknown state tests;
- external-inspector egress allowlist tests;
- prompt-injection content tests proving agents do not execute document
  instructions;
- raw PCL/PJL/PostScript rejection for untrusted profile;
- held-job survival across complete runtime replacement.
