# FolioRelay Virtual Printer MVP implementation and deployment plan

Status: 2026-09-29  
Purpose: shortest evidence-backed path from the qualified FolioRelay runtime to an installable, useful TrueNAS MVP.

## 1. MVP mission

The MVP is complete when a user can install FolioRelay on TrueNAS, open a small management portal, and use the virtual printer from both:

- a normal Windows client through the inbox Microsoft IPP print path; and
- a real iPhone/iPad through native AirPrint.

A successful print is not merely a client-side success message. The submitted artifact must reach FolioRelay durable acceptance exactly once and become visible in the FolioRelay Inbox.

The MVP must be useful immediately after installation. The default route is therefore:

`native print client -> virtual printer -> durable FolioRelay acceptance -> Inbox`

The Inbox preserves the submitted source artifact and makes it available to an authenticated user/API client. Rich routing, transformations, external senders, broad standards matrices, and enterprise topology are post-MVP unless they fall out naturally from the MVP implementation.

## 2. MVP definition of done

### Installation

- FolioRelay is installable through a TrueNAS App candidate using the current catalog/app format.
- Normal installation requires no shell commands or manual edits inside containers.
- Persistent state lives in an explicitly selected TrueNAS dataset.
- Images are pinned by immutable digest for the qualified MVP.
- Restart/recreate does not lose accepted jobs, configuration, printer identity, or artifacts.

### Windows

- A real Windows system can add FolioRelay using the inbox Microsoft IPP class path.
- No vendor printer driver is required.
- A real document reaches FolioRelay durable acceptance.
- The accepted artifact appears in the Inbox.
- Idempotency conflicts do not advance.
- The final installed TrueNAS instance passes this flow.

### AirPrint

- FolioRelay is discoverable from a real iPhone/iPad through native AirPrint on the supported local-LAN deployment profile.
- Advertising is truthful: any advertised PDL/capability is actually accepted.
- At minimum, PDF/document printing and the Apple-required URF path are accepted.
- The AirPrint service advertises the correct universal subtype and coherent DNS-SD/IPP identity.
- A real Apple mobile print reaches durable FolioRelay acceptance.
- The accepted artifact appears in the Inbox.
- The final installed TrueNAS instance passes this flow.

### Product usefulness

- Human user can see Ready/Degraded state.
- Human user can see the printer name and Windows IPP endpoint.
- Human user can see whether AirPrint discovery is healthy.
- Human user can see recent accepted/rejected jobs.
- Human user can download an accepted source artifact.
- Automation and agents can obtain the same state without scraping the GUI.
- A self-test explains the first actionable failure rather than only reporting "unhealthy".

## 3. Product architecture

Keep the deployed semantic center small.

### 3.1 `foliorelayd` - product authority and control plane

Promote the proven state engine out of the qualification-only `foliorelay-state-fixture` into a product command, tentatively `cmd/foliorelayd`.

Responsibilities:

- durable state authority;
- ingest API consumed by the print substrate adapter;
- persistent configuration;
- job/inbox index;
- authenticated management API;
- embedded static web UI;
- health/readiness;
- capability document and OpenAPI document;
- self-test/diagnostics orchestration.

It should remain a static Go service with no shell, package manager, CUPS, Avahi, renderer stack, or printer drivers.

The existing internal state engine and durable journal remain the correctness core. The fixture is not copied wholesale; its proven ingest semantics are promoted into a production server with explicit versioned API contracts.

### 3.2 Selected print substrate

One production print substrate is selected after the Windows + AirPrint MVP gates are exercised.

Candidates remain:

- minimal pinned upstream CUPS; and
- pinned PAPPL.

The selection criterion is not image size alone. Prefer the candidate that satisfies both MVP native-client paths with the least FolioRelay-owned glue, least duplicate authority, least privileged/runtime surface, and simplest reliable TrueNAS deployment.

The non-selected candidate is retained as an interoperability/protocol oracle, not a second production implementation.

### 3.3 Discovery

Prefer upstream discovery machinery from the selected substrate if it can:

- advertise the canonical FolioRelay printer identity;
- coexist reliably with TrueNAS networking;
- advertise the AirPrint universal subtype and required TXT records;
- avoid a competing state/configuration authority.

A separate FolioRelay discovery sidecar is a fallback only if upstream discovery cannot meet those requirements cleanly. Do not add a custom mDNS implementation merely for architectural symmetry.

### 3.4 Default Inbox

Accepted artifacts already enter a content-addressed store. Add a durable human/machine-readable projection:

- stable job/aggregate ID;
- accepted timestamp;
- source substrate job ID;
- media type;
- byte size;
- SHA-256;
- copies;
- acceptance/replay/conflict/rejection state;
- source artifact location/reference.

For MVP, do not parse document contents merely to populate metadata.

Authenticated download streams the source artifact by job ID without exposing internal filesystem paths.

## 4. One contract, three audience views

The GUI, automation interface, and agent interface must not become independent products.

### 4.1 Human GUI/UX

The web portal is intentionally small.

#### Overview

Show:

- large `Ready`, `Degraded`, or `Action required` state;
- printer display name;
- AirPrint: Discoverable / Not discoverable / Unknown;
- Windows IPP endpoint with copy button;
- last successful print;
- recent job count;
- one primary action: `Run self-test`.

Avoid a dashboard full of implementation metrics.

#### Inbox

Show recent jobs in reverse chronological order:

- time;
- status;
- media type;
- size;
- source/client class when known;
- Download action;
- stable job ID in details.

No document preview is required for MVP. That avoids adding a PDF/image renderer to the management plane.

#### Printer

Show product-level settings:

- display name;
- location/note;
- canonical public printer URI;
- enabled native profiles: Windows IPP, AirPrint;
- advertised formats/capabilities;
- discovery identity/UUID.

Advanced protocol values are visible but not required for normal setup.

#### Diagnostics

Show:

- state authority health;
- print substrate health;
- IPP self-query;
- canonical identity coherence;
- DNS-SD advertisement health;
- durable store free space;
- last error with an actionable explanation;
- `Run full self-test`;
- `Download support bundle`.

The support bundle must exclude document contents and secrets by default.

### 4.2 Automation DX

Versioned JSON API is authoritative; the GUI consumes it.

Minimum API:

- `GET /healthz` - process liveness, unauthenticated, no sensitive detail;
- `GET /readyz` - admission readiness, unauthenticated, no sensitive detail;
- `POST /api/v1/ingest` - internal print-adapter contract;
- `GET /api/v1/status`;
- `GET /api/v1/printer`;
- `GET /api/v1/jobs`;
- `GET /api/v1/jobs/{id}`;
- `GET /api/v1/jobs/{id}/artifact`;
- `GET /api/v1/diagnostics`;
- `POST /api/v1/self-test`;
- `GET /api/v1/config`;
- `PATCH /api/v1/config` for the small product-setting set;
- `GET /openapi.json`;
- `GET /.well-known/foliorelay`.

Rules:

- stable JSON error envelope;
- pagination for job lists;
- request IDs in responses/logs;
- idempotency keys on state-changing operations;
- explicit API version;
- no HTML-only operations;
- no requirement to parse logs for normal automation.

A small `foliorelayctl` client is optional for MVP only if it remains a thin API client and materially improves operator/test ergonomics. The API is the required DX; a CLI must earn its keep.

### 4.3 Agent DX

Agents use the same API as automation.

`/.well-known/foliorelay` and/or `/api/v1/capabilities` should expose machine-readable:

- product/API version;
- printer identity;
- enabled profiles;
- supported safe operations;
- current health state;
- OpenAPI URL;
- whether mutation is allowed by the presented credential.

Agent rules:

- never scrape the web UI;
- never infer success from HTTP 200 when durable job state is available;
- mutations use the same validation/idempotency path as humans/automation;
- diagnostics include bounded evidence and stable error codes;
- no hidden agent-only authority.

## 5. Configuration authority

Avoid two configuration systems fighting each other.

### TrueNAS owns deployment settings

Examples:

- persistent dataset/mount;
- host/app IP binding;
- management port;
- resource limits;
- image digests;
- deployment secret/bootstrap credential;
- optional advanced network/discovery mode.

### FolioRelay owns product settings

Examples:

- printer display name;
- printer location/note;
- canonical printer URI/identity;
- profile enablement;
- Inbox retention policy.

TrueNAS can provide bootstrap defaults on first start. Once product state exists, `foliorelayd` is authoritative for product settings.

The portal and API mutate the same durable product state.

## 6. Security/authentication MVP

Do not expose document/job management unauthenticated.

MVP auth:

- bootstrap one management secret during install;
- use it to establish an authenticated browser session;
- API clients use a bearer token;
- secret is stored outside images and logs;
- health/readiness remain non-sensitive and unauthenticated;
- artifact download and job metadata require authentication;
- management API binds only to the selected LAN/interface by default.

Fine-grained multiple users, OIDC, SSO, and complex RBAC are post-MVP.

If adding separate read-only and operator tokens is low-cost, retain the distinction; otherwise one MVP management credential is acceptable as long as it is not embedded in the image or discovery metadata.

## 7. Canonical printer identity

This is a hard MVP gate, not an Android-specific fix.

Define one canonical public identity object:

- scheme: `ipp` or `ipps`;
- hostname/address;
- port;
- resource path;
- stable printer UUID;
- display name;
- location.

Derive from it:

- IPP `printer-uri-supported`;
- DNS-SD SRV target/port;
- DNS-SD `rp`;
- DNS-SD UUID;
- AirPrint TXT identity;
- IPPS certificate name expectation.

Qualification asserts coherence every time.

The runtime must never advertise an externally unreachable `localhost` URI.

## 8. AirPrint implementation

MVP AirPrint should be a truthful, bounded profile, not full release-product standards expansion.

### Minimum user story

From a real iPhone/iPad on the same supported LAN:

1. Share/Print a PDF/document.
2. FolioRelay appears in the native Printer selector without entering a URL.
3. Select FolioRelay.
4. Print.
5. FolioRelay accepts exactly one artifact.
6. The job appears in the Inbox and the source artifact can be downloaded.

### Format path

Retain PDF support.

Add Apple Raster/URF (`image/urf`) as an actual accepted format before advertising it.

JPEG may be included in MVP if the selected substrate already supports a bounded source-preserving path with negligible extra code; otherwise photo-specific printing is post-MVP.

Do not add a renderer merely to claim AirPrint. FolioRelay is accepting a document artifact, not rendering it to a physical engine.

### PAPPL experiment

PAPPL upstream already contains Apple/PWG raster and image handling plus DNS-SD machinery. The current FolioRelay PAPPL fixture deliberately rejects raster callbacks.

Bounded experiment:

- accept URF through the upstream raster path;
- at raster-job start, obtain the original spooled source artifact and media type;
- hand the original artifact to FolioRelay ingest once;
- consume/validate the raster stream without creating a second durable effect;
- ensure malformed URF fails closed;
- preserve restart/idempotency invariants.

### CUPS experiment

Bounded experiment:

- add `image/urf` MIME recognition;
- route URF unchanged to the FolioRelay backend when possible;
- propagate the real MIME type rather than hard-coding `application/pdf`;
- advertise URF only after the passthrough path passes;
- do not import a broad cups-filters/legacy stack merely for AirPrint unless source evidence proves it necessary.

### AirPrint advertisement

The qualified service record must include the universal AirPrint discovery subtype and truthful PDL/URF TXT values.

Identity coherence remains mandatory:

`DNS-SD UUID == IPP printer-uuid`

and the advertised resource path/host/port must resolve to the same endpoint reported by IPP.

## 9. Windows implementation

Windows MVP capability is preservation, not a new subsystem.

Existing evidence already proves:

`PrintManagement/Add-Printer -IppURL -> Microsoft IPP Class Driver -> Spooler -> candidate -> FolioRelay acceptance`

for both active candidates.

MVP work:

- ensure canonical identity changes do not regress Windows;
- provide human-readable Windows setup guidance in the portal;
- optionally expose a copyable `ipp://...` or `ipps://...` endpoint;
- repeat a real Windows native print after TrueNAS installation;
- require Inbox visibility and exact durable acceptance.

Windows Protected Print Mode is post-MVP.

## 10. TrueNAS application architecture

TrueNAS 25.04+ uses Docker-backed Apps and supports custom/catalog application metadata, portal configuration, persistent host-path storage, explicit port binding, and host networking when required for discovery.

Create a Foundry candidate:

`candidates/folio-relay-app/ix-dev/community/foliorelay/`

Minimum catalog files follow the existing Foundry pattern:

- `app.yaml`;
- `item.yaml`;
- `questions.yaml`;
- `ix_values.yaml`;
- `README.md`;
- `app-readme.md`;
- `changelog.md`;
- `templates/docker-compose.yaml`;
- representative `templates/test_values/*`.

### Services

Target final deployment:

1. `control` - `foliorelayd`;
2. `print` - selected CUPS or PAPPL runtime;
3. discovery only if it cannot remain cleanly inside the selected upstream print runtime.

Do not deploy both production candidates.

### Storage

Expose one normal user choice: **FolioRelay Data Dataset**.

Inside the mount use separate directories:

- `config/`;
- `state/`;
- `artifacts/`;
- `spool/`;
- `support/` ephemeral/rotatable evidence as appropriate.

Permissions are explicit. The print runtime gets only the spool/handoff paths it needs, not broad write access to all product state.

### Network

Normal user goal: no protocol expertise required.

The app should determine/use a canonical LAN identity and expose:

- IPP/IPPS service;
- management portal;
- AirPrint discovery.

The highest-risk TrueNAS-specific item is multicast DNS coexistence because the TrueNAS host itself can use mDNS.

Qualification order:

1. test upstream selected-substrate DNS-SD in the target TrueNAS app network model;
2. prefer a dedicated app/host IP binding where supported so IPP identity is unambiguous;
3. verify whether discovery can coexist with TrueNAS's mDNS service without host modifications;
4. if upstream discovery cannot coexist, test a bounded discovery sidecar;
5. only if required, qualify a dedicated L2 identity/network mode; do not make users manually reconfigure switches/VLANs for the default MVP.

The MVP supported topology is one local broadcast domain/subnet. Routed mDNS/VLAN reflection is post-MVP.

### Portal

TrueNAS App portal points to the FolioRelay web UI.

The user should not need to find a container IP.

## 11. TrueNAS install wizard UX

Keep normal installation to a small first screen; hide expert options.

### Basic

- FolioRelay Data Dataset;
- Printer Name (default `FolioRelay`);
- Location (optional);
- Management credential;
- LAN/interface/IP selection only when TrueNAS cannot choose safely.

AirPrint and Windows IPP are enabled by default because both are MVP capabilities.

### Advanced

- fixed public printer URI override;
- management port;
- discovery interface;
- IPP/IPPS selection;
- certificate/TLS options;
- resource limits;
- diagnostic logging level.

Do not expose CUPS-vs-PAPPL. Substrate selection is an engineering decision, not an end-user configuration knob.

## 12. First-run UX

First portal load should answer three questions immediately:

1. Is it ready?
2. How do I print to it?
3. Did my print arrive?

Recommended first-run card:

- **Printer:** FolioRelay
- **AirPrint:** Ready
- **Windows:** Ready
- **Inbox:** 0 jobs
- buttons/links: `Run self-test`, `Windows setup`, `View Inbox`.

If AirPrint is not discoverable, say why:

- discovery socket unavailable;
- interface not multicast-capable;
- advertised identity mismatch;
- IPP endpoint unreachable;
- URF capability missing;
- certificate mismatch;
- unknown client-side condition.

Do not tell the user merely to "check network settings".

## 13. Self-test contract

The same self-test drives GUI, automation, agents, CI, and HIL.

Return structured checks:

- `state.durable_store`;
- `state.journal`;
- `storage.free_space`;
- `print.process`;
- `ipp.get_printer_attributes`;
- `identity.public_uri_reachable`;
- `identity.uuid_coherent`;
- `identity.resource_path_coherent`;
- `tls.identity` when enabled;
- `airprint.dnssd_advertised`;
- `airprint.urf_advertised_truthfully`;
- `inbox.write_read`.

Each check returns:

- status;
- stable code;
- short human message;
- bounded technical detail;
- suggested remediation when known.

CI/HIL validators consume the JSON result, not the formatted GUI text.

## 14. Implementation work breakdown

### M0 - Freeze MVP contracts

Deliverables:

- this plan;
- explicit MVP admission contract;
- OpenAPI skeleton;
- capability document schema;
- canonical printer identity schema;
- Inbox record schema;
- TrueNAS deployment contract.

No runtime fanout is required merely to document these contracts.

### M1 - Productize state authority and Inbox

Implement:

- `foliorelayd`;
- durable ingest endpoint preserving current semantics;
- durable job/inbox projection;
- artifact download;
- health/readiness;
- status/jobs/printer API;
- OpenAPI/capability document.

Qualification:

- replay/conflict semantics unchanged;
- restart recovery;
- bounded corrupt-journal failure;
- concurrent reads while ingesting;
- artifact references survive container recreation.

### M2 - Canonical printer identity

Implement a single identity configuration consumed by the print runtime and discovery.

Qualification:

- Get-Printer-Attributes;
- DNS-SD identity;
- external reachability;
- TLS identity if enabled;
- Android/CUPS regression rep as an existing oracle;
- Windows native regression.

### M3 - AirPrint capability spike

Run CUPS and PAPPL bounded reps against the same acceptance contract:

- truthful URF acceptance;
- PDF retained;
- AirPrint DNS-SD universal advertisement;
- source artifact preserved;
- malformed URF bounded;
- no duplicate durable effect.

Use source evidence to fix only proven candidate/harness defects.

### M4 - Substrate selection checkpoint

Present the user with the material evidence packet.

Selection inputs:

- Windows MVP pass;
- AirPrint protocol/profile pass;
- FolioRelay-owned glue;
- runtime surface;
- discovery complexity on TrueNAS;
- restart/state behavior;
- resource footprint;
- maintenance/update path.

After explicit selection:

- freeze selected substrate in production config/docs;
- retain the other as oracle;
- remove qualification-only end-user substrate switches.

### M5 - Human UI and authentication

Build the embedded GUI against the same API.

Pages:

- Overview;
- Inbox;
- Printer;
- Diagnostics.

Add bootstrap management auth and authenticated artifact download.

UI acceptance is browser-level behavior plus API oracle; frontend screenshots alone never prove product correctness.

### M6 - TrueNAS candidate

In Foundry:

- create FolioRelay candidate metadata;
- catalog questions;
- Compose template;
- storage mounts;
- portal;
- health checks;
- selected runtime image digests;
- app schema/lint/render tests.

Qualification uses public/free GitHub Actions wherever possible.

Private Actions minutes are not required for normal candidate qualification.

### M7 - TrueNAS networking/discovery HIL

On a real or faithful TrueNAS target:

- install from clean state;
- reboot/restart;
- inspect canonical identity;
- browse AirPrint from LAN;
- validate no mDNS/port collision;
- verify management portal;
- verify persistence.

If discovery fails, fix the smallest platform-specific adapter. Do not modify the TrueNAS host manually as the product solution.

### M8 - Native MVP acceptance

Against the installed TrueNAS app:

Windows:
- discover/add with inbox IPP path;
- print fixture;
- verify one durable Inbox job;
- download and hash source artifact.

Apple:
- real iPhone/iPad AirPrint discovery;
- print PDF/document;
- verify one durable Inbox job;
- download and hash/validate source artifact.

Negative:
- restart app between acceptance and Inbox read;
- retry same source identity where applicable;
- fill bounded spool to threshold;
- temporarily stop state authority;
- malformed URF;
- invalid auth to management API.

### M9 - MVP cut

- immutable image digests;
- SBOM/provenance;
- clean install;
- upgrade/reinstall preservation rehearsal;
- backup/restore instructions for data dataset;
- concise Windows and AirPrint onboarding;
- release notes with supported topology;
- durable MVP evidence ledger.

MVP is declared only from this installed-system evidence.

## 15. Execution DAG

```text
                         +--> M1 foliorelayd + Inbox ---------+
M0 contracts ------------+                                    |
                         +--> M2 canonical identity ----------+------+
                         |                                    |      |
                         +--> M3 AirPrint CUPS/PAPPL reps ----+      |
                                                                  M4 select
                                                                     |
                                                                     v
                                                           M5 GUI/auth/API polish
                                                                     |
                                                                     v
                                                           M6 TrueNAS candidate
                                                                     |
                                                                     v
                                                    M7 TrueNAS discovery/network HIL
                                                                     |
                                                                     v
                                                        M8 Windows + AirPrint E2E
                                                                     |
                                                                     v
                                                                  M9 MVP
```

Parallelism:

- M1, M2, and M3 begin in parallel after contracts.
- GUI can begin against M1 API fixtures before substrate selection, but must not dictate API semantics.
- TrueNAS catalog scaffolding can begin before selection with a qualification-only substrate input, then be frozen after M4.
- HIL-only work does not idle CI-capable work.

## 16. CI and deployment efficiency

Use public/free GHA through the existing Agent Dispatch constraints.

Permanent CI should cover:

- Go unit/state tests;
- API contract tests;
- OpenAPI/schema validation;
- GUI build + small browser smoke;
- selected print-runtime build;
- IPP protocol tests;
- AirPrint advertisement/URF protocol tests;
- Windows hosted native qualification where faithful;
- TrueNAS catalog schema/render/lint;
- amd64/arm64 image build qualification;
- restart/idempotency/security regressions.

Do not run real Apple HIL on every commit. Run it:

- before substrate selection;
- before MVP cut;
- after material AirPrint/discovery/network changes.

Use one integrated fanout only after a minimal proven delta is promoted.

## 17. Deployment path to the user's TrueNAS

### Predeployment

Automation produces:

- selected multi-arch image digests;
- TrueNAS catalog candidate;
- default values;
- self-test;
- immutable release manifest.

### Install

Preferred user interaction:

1. Open TrueNAS Apps.
2. Install FolioRelay candidate.
3. Select FolioRelay data dataset.
4. Confirm printer name.
5. Set management credential.
6. Install.

No SSH/shell.

### Automatic startup

The app:

- opens durable state/config;
- creates/loads stable printer UUID;
- derives canonical identity;
- starts print service;
- starts/validates AirPrint advertisement;
- runs local self-test;
- becomes Ready only when durable ingest and the public IPP endpoint are coherent.

### User acceptance

Portal shows:

- Ready;
- AirPrint Ready;
- Windows Ready;
- empty Inbox.

Then perform one Windows print and one AirPrint print.

After both appear in Inbox, the MVP is operational.

## 18. Red team

### R1 - "MVP" works only in CI

Risk: protocol tests pass but TrueNAS multicast/ports differ.

Attack: require final admission from the installed TrueNAS system.

### R2 - AirPrint appears but jobs fail

Risk: false discovery success.

Attack: real Apple mobile job must reach durable acceptance; discovery alone is insufficient.

### R3 - Windows works only with manual/legacy driver tricks

Risk: hidden support burden.

Attack: record actual Microsoft IPP class driver/native path and prohibit vendor-driver dependency.

### R4 - GUI becomes a second state authority

Risk: browser settings disagree with API/TrueNAS values.

Attack: GUI calls the same versioned API; one durable product configuration authority.

### R5 - TrueNAS values and product settings drift

Risk: edits in two places produce surprising behavior.

Attack: explicitly separate deployment settings from product settings and make bootstrap semantics one-way.

### R6 - Printed documents disappear into an object store

Risk: technically correct but useless MVP.

Attack: default Inbox projection and authenticated source-artifact download are MVP requirements.

### R7 - Management surface leaks documents

Risk: LAN user can enumerate/download prints.

Attack: management auth required for job metadata/artifacts; only non-sensitive health endpoints are anonymous.

### R8 - Supporting agents creates a privileged backdoor

Risk: agent-only endpoints bypass normal validation.

Attack: agents use the same API/auth/idempotency contracts as automation and humans.

### R9 - mDNS conflict with TrueNAS host

Risk: AirPrint intermittently invisible or host services disturbed.

Attack: HIL qualification of discovery coexistence; prefer supported network configuration; no host file hacks.

### R10 - Full IPP Everywhere/AirPrint certification scope delays use

Risk: gold-plating.

Attack: implement the smallest truthful AirPrint profile that passes the concrete Apple MVP user story. Keep broader PWG/JPEG/2.0 work post-MVP.

### R11 - Two print substrates become permanent

Risk: doubled engineering/operations burden.

Attack: explicit M4 human selection; only one ships.

### R12 - UI work delays functional MVP

Risk: frontend polish dominates critical path.

Attack: API-first, four-page embedded UI, no SPA architecture requirement unless it earns its keep.

### R13 - HIL becomes a manual ritual

Risk: every release requires ad-hoc human debugging.

Attack: self-test and acceptance scripts produce machine-readable receipts; manual action is limited to the unavoidable physical-client print initiation until a qualified actuator exists.

## 19. Blue team

Controls that make the plan resilient without inflating scope:

- durable acceptance is the universal success oracle;
- one canonical printer identity feeds IPP/DNS-SD/TLS;
- one API feeds GUI, automation, agents, tests;
- one data dataset simplifies backup and recovery;
- one production print substrate after selection;
- upstream print/discovery capabilities preferred over custom implementations;
- source artifacts preserved rather than normalized by default;
- explicit bounded-input/resource policy retained;
- structured self-test replaces log archaeology;
- install wizard defaults hide protocol complexity;
- supported MVP topology is intentionally local-LAN first;
- post-MVP features remain in a separate roadmap.

## 20. Audience acceptance

### Human

A non-developer with TrueNAS access can:

- install without shell;
- know whether the printer is ready;
- find it on iPhone/iPad;
- add/use it on Windows;
- see the resulting job;
- download the captured artifact;
- understand the first failure from the portal.

### Automation

A script can:

- query health/readiness;
- retrieve the canonical printer endpoint;
- list jobs;
- retrieve job metadata/artifact;
- run self-test;
- change the small supported config set idempotently;
- distinguish stable error codes.

### Agent

An authorized agent can:

- discover capabilities programmatically;
- read current state without UI scraping;
- perform the same bounded management actions;
- receive structured diagnostics;
- cite stable job/action IDs;
- never receive more authority merely because it is an agent.

## 21. Explicit post-MVP work

Do not put these back on the MVP critical path unless new evidence demands them:

- Windows Protected Print Mode qualification;
- full current IPP Everywhere PDL breadth beyond MVP AirPrint needs;
- IPP Everywhere 2.0;
- libcups 3 client-generation qualification;
- ChromeOS;
- broad Android generation matrix;
- Kylin/UOS/HarmonyOS;
- routed/wide-area mDNS;
- Wi-Fi Direct/IPP-USB;
- cloud print connectors;
- vendor drivers/apps;
- full workflow/routing designer;
- SSO/OIDC/RBAC;
- inline document previews;
- external email/fax/physical-printer sender productionization.

## 22. Immediate READY work

Without waiting for TrueNAS HIL:

1. freeze the MVP/API/identity/Inbox contracts;
2. create `foliorelayd` from the proven state engine;
3. add Inbox read/download projection;
4. implement canonical public printer identity;
5. run bounded CUPS and PAPPL URF/AirPrint reps;
6. scaffold the four-page GUI against mock/API fixtures;
7. scaffold the TrueNAS candidate using current Foundry conventions;
8. add catalog render/schema tests;
9. prepare machine-readable HIL self-test and acceptance receipts.

Only M7/M8 require the faithful LAN/Apple/TrueNAS environment.
