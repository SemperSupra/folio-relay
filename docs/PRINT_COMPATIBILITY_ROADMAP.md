# Modern print compatibility findings and execution roadmap

Status: 2026-09-29  
Authority: FolioRelay runtime/product work, stacked on PR #1 without changing substrate-selection authority.

## 1. Executive finding

The modern client ecosystem is converging rather than fragmenting. Windows Ready Print / Windows Protected Print Mode, Apple AirPrint, Android/Mopria, ChromeOS, and current Unix clients increasingly meet FolioRelay at a driverless IPP boundary.

The efficient strategy is therefore to qualify a small set of protocol/profile generations rather than build per-vendor drivers or test every OS point release.

The current highest-value sequence is:

1. close canonical public printer identity and discovery coherence;
2. qualify libcups 3 clients;
3. qualify Windows Protected Print Mode;
4. maintain an IPP Everywhere 2.0 draft-forward conformance lane;
5. add truthful PWG Raster + JPEG admission;
6. add truthful URF/AirPrint admission and discovery;
7. broaden representative native-client and deployment profiles.

CUPS-versus-PAPPL selection remains a separate human authority decision. Every work package below is defined so that product semantics and acceptance gates remain symmetric across both active candidates.

## 2. Current evidence and newly discovered invariants

### 2.1 Integrated baseline is green

PR #1 head `2d305d2632ca6af51cf2add4c5a9117464b58413` completed its integrated requalification cycle green across formal models, both engineering runtimes, ingress, bakeoff/stress, security, reproducibility, Windows, macOS, Android/PAPPL, and the amd64/arm64 host-portability matrix.

### 2.2 DNS-SD and IPP UUID coherence is mandatory

PR #9 proved the earlier Android/CUPS discovery negative was a harness/projection defect. Stock Android BIPS admits the CUPS queue when DNS-SD advertises the live queue's authoritative IPP `printer-uuid`.

PR #11 strengthens the generic discovery gate so DNS-SD UUID must equal live IPP `printer-uuid`. Run `36595868557` passes.

Invariant:

`DNS-SD UUID == IPP printer-uuid`.

### 2.3 Canonical public printer URI is the next blocker

PR #10 run `36595404720` still fails after successful Android discovery and selection.

The exact CUPS queue reports:

`printer-uri-supported = ipp://localhost:631/printers/FolioRelay`

while DNS-SD reaches the candidate through the hosted runner's externally reachable address.

AOSP BIPS source parses `printer-uri-supported` and adopts a valid returned URI for subsequent printer interaction. Android therefore discovers the external service, then follows the advertised `localhost` identity back to Android's own loopback.

This is a deployment/product identity defect, not an emulator-performance failure.

Required invariant:

- one canonical externally reachable printer URI;
- IPP `printer-uri-supported` uses it;
- DNS-SD SRV/TXT resource path agrees with it;
- DNS-SD UUID agrees with IPP UUID;
- advertised hostname/address is reachable by the client;
- when IPPS is enabled, TLS certificate identity agrees with the advertised host.

Do not repair this with an Android-specific redirect or a hard-coded GitHub-runner address.

### 2.4 Current format surface is PDF-shaped

Current qualification fixtures are intentionally narrow:

- CUPS PPD advertises a PDF-only filter path;
- CUPS backend currently exports `application/pdf`;
- PAPPL bridge sets `data->format = "application/pdf"` and deliberately rejects raster callbacks;
- FolioRelay's durable ingest authority is already media-type agnostic and stores the exact submitted artifact content-addressably.

This makes format expansion primarily a substrate/admission concern rather than a durable-authority redesign.

## 3. External ecosystem findings

### 3.1 Windows

Windows Ready Print is Microsoft's modern driverless path based on IPP/Mopria. Windows Protected Print Mode restricts printing to this modern stack and removes dependence on legacy third-party print drivers.

Implication: WPP is a high-ROI native qualification lane, not a reason to add a Windows-specific server protocol.

### 3.2 IPP Everywhere

Current IPP Everywhere requires a broader document profile than PDF-only interoperability. PWG Raster is a required core format and JPEG is required for color printers; PDF is an important/recommended format but should not be treated as the only generic driverless PDL.

IPP Everywhere 2.0 is still draft-forward work. Treat its changing requirements as sensors until finalized, while promoting requirements that already overlap FolioRelay security/correctness contracts.

### 3.3 Apple / AirPrint

AirPrint discovery is a profile over IPP/DNS-SD, not arbitrary browsing of every IPP service. Apple deployment guidance distinguishes the URF-capable universal-print subset and the `_universal._sub` DNS-SD subtype.

Do not claim AirPrint compatibility until FolioRelay truthfully accepts URF and advertises corresponding URF/DNS-SD attributes.

### 3.4 libcups 3 / CUPS 3

libcups 3 is stable and source/binary incompatible with libcups 2.x, so it deserves a client-generation qualification lane now.

The newer CUPS server projects remain a future substrate research line; do not migrate production architecture merely because libcups 3 clients exist.

### 3.5 ChromeOS and Linux/Unix

ChromeOS and current Unix printing remain aligned with driverless IPP. Broad support should be earned through standards profiles plus representative native clients, not a permanent matrix of every distribution and point release.

## 4. Support model: profiles, not point releases

Define support in layers.

### Profile A - canonical driverless IPP baseline

- IPP 2.x semantics used by current clients;
- canonical externally reachable URI;
- coherent DNS-SD UUID/resource path/host identity;
- TLS/IPPS identity when enabled;
- exact durable acceptance oracle;
- PDF path retained.

### Profile B - IPP Everywhere current generation

Profile A plus truthful current-standard document/capability support, including PWG Raster and JPEG where applicable.

### Profile C - Apple AirPrint

Profile B foundations plus:

- URF admission;
- truthful URF capability tokens;
- `_universal._sub`;
- AirPrint-correct DNS-SD/TXT identity;
- real Apple-device release qualification.

### Profile D - IPP Everywhere next generation

Draft-forward sensors for IPP Everywhere 2.0, explicitly separated from current production vetoes until the specification/certification basis is stable.

### Native client generations

Sample representative generations:

- Windows 11 current / Protected Print Mode;
- Windows Server 2025 where relevant;
- libcups 2.x and libcups 3.x clients;
- macOS current and current-1 where hosted runners exist;
- Android current-2 / current / current+preview when furnished emulators are faithful;
- real iOS/iPadOS HIL for AirPrint;
- ChromeOS/Flex/ChromiumOS only when a faithful free substrate exists.

## 5. Prioritized work packages

## WP0 - Canonical public printer identity and discovery closure

Priority: immediate  
Effort: S  
ROI: very high

### Implementation

Introduce one canonical deployment input such as `FOLIORELAY_PUBLIC_PRINTER_URI`, parsed and validated centrally.

Derived values should include:

- scheme;
- public hostname/address;
- public port;
- IPP resource path;
- DNS-SD `rp`;
- certificate hostname expectation.

CUPS and PAPPL may implement the mapping differently internally, but must produce the same externally observable semantics.

### Qualification

For each candidate:

1. start with an externally reachable public identity;
2. query `Get-Printer-Attributes`;
3. resolve DNS-SD;
4. assert URI/host/port/path/UUID coherence;
5. validate IPPS certificate identity where applicable;
6. run real native client E2E;
7. require durable acceptance to advance and conflicts not to advance.

Android/CUPS native parity is the first regression oracle.

### Deployment

The deployment/materialization layer chooses the public identity from the real environment. Runtime images must not guess Docker hostnames, loopback, GitHub-runner addresses, or LAN topology.

DONE when external clients never receive an unreachable self-reference.

## WP1A - libcups 3 native-client qualification

Priority: immediately after/parallel with WP0 contract  
Effort: XS-S  
ROI: high

### Implementation

No product feature should be added unless evidence requires it.

Build a pinned libcups 3 native client and submit a real Print-Job directly to both candidates.

### Matrix

- amd64;
- arm64 where free runner exists;
- CUPS candidate;
- PAPPL candidate.

### Oracle

- native client returns IPP success;
- exact artifact reaches FolioRelay durable authority;
- accepted increments;
- conflicts unchanged;
- no local CUPS 2 scheduler required.

## WP1B - Windows Protected Print Mode

Priority: early  
Effort: S when a faithful Windows 11 substrate exists; otherwise HIL-limited  
ROI: very high

### Qualification requirements

- prove WPP is actually enabled;
- prove the printer is installed/used through Windows Ready Print;
- prove no vendor/V3/V4 driver participates;
- submit a real native Windows job;
- require durable FolioRelay acceptance.

Do not weaken the gate to ordinary Server 2025 IPP qualification; that evidence already exists.

## WP2 - IPP Everywhere 2.0 draft-forward conformance lane

Priority: early, parallel sensor  
Effort: S-M  
ROI: high

Create a versioned machine-readable requirement ledger with states such as:

- current-required;
- current-recommended;
- draft-forward-required;
- draft-forward-recommended;
- HIL-only;
- not-applicable.

Automate exact attributes/security semantics that can be tested in GHA.

Draft-only failures are informational unless the same behavior violates an already-current FolioRelay correctness/security contract.

Do not claim IPP Everywhere 2.0 certification before the final standard and applicable certification process exist.

## WP3 - PWG Raster + JPEG capability

Priority: first major capability expansion  
Effort: M  
ROI: very high

### Design rule

Separate format admission from format transformation.

The durable authority should preserve the submitted source artifact and media type. Add conversion only where product semantics require it.

### Source reconnaissance first

Before coding:

- inspect CUPS scheduler/filter/backend behavior to find the narrowest source-artifact-preserving handoff;
- inspect PAPPL printfile/raster callbacks to determine whether original raster/JPEG can be handed off without unnecessary rendering;
- use OpenPrinting generators and transforms as pinned fixtures/oracles before adopting them as runtime dependencies.

### CUPS path

- remove the PDF-only assumption from the qualification profile;
- propagate the real final document MIME type;
- advertise only formats the backend can actually hand off;
- retain minimal upstream filter surface.

### PAPPL path

- replace deliberate raster rejection with bounded PWG Raster support;
- add JPEG only when faithfully accepted;
- keep document-format, raster-type, resolution, color, and media attributes truthful.

### Adversarial fixtures

- truncated headers;
- extreme dimensions/resolutions;
- oversized page declarations;
- many-page streams;
- malformed JPEG;
- decompression/resource pressure;
- high copy count combined with large artifacts.

All must stay inside existing hard budgets and must not advance durable accepted state when rejected.

## WP4 - URF / AirPrint profile

Priority: after the raster harness exists  
Effort: M  
ROI: very high

### Implementation

- bounded `image/urf` admission;
- truthful URF capability tokens;
- `_universal._sub` DNS-SD subtype;
- `image/urf` in PDL advertisement only after the admission path passes;
- preserve URI/UUID/resource-path/TLS identity coherence.

### Qualification ladder

1. protocol-level generated URF;
2. hosted macOS discovery/submission where faithful;
3. real iPhone/iPad HIL as release admission.

The iOS simulator remains API/capability evidence, not network-delivery proof.

## WP5 - Native breadth expansion

Use generation sampling, not every point release.

### Windows

- current supported Windows 11 with WPP;
- one preview/current+1 sensor when a qualified/free substrate exists;
- x64 first; ARM64 when a faithful runtime/hardware substrate is available.

### Android

- current-2/current/current+preview APIs when furnished emulators are faithful;
- retain PrintManager -> PrintSpooler -> stock BIPS;
- UI automation is an actuator, not the acceptance oracle.

### Apple

- hosted macOS current/current-1 where runners exist;
- real iOS/iPadOS HIL for AirPrint release qualification;
- simulator retained for API regression.

### ChromeOS

- standards-level expectation after current IPP Everywhere profile passes;
- native HIL only when a faithful free substrate exists;
- no ChromeOS-specific server protocol.

## WP6 - Deployment breadth: IPv4/IPv6 and routed discovery

Priority: after canonical identity  
Effort: S-M  
ROI: high for real deployments

Define explicit deployment profiles:

1. Local LAN - mDNS/DNS-SD on directly attached subnet.
2. Dual stack - coherent IPv4/IPv6 identity and reachability.
3. Managed routed network - explicit authoritative/wide-area DNS-SD or a known reflector integration.

Never silently enable broad multicast reflection across security boundaries.

## 6. Execution DAG

```text
WP0 canonical public identity
 |\
 | +--> WP1A libcups 3 client -----------+
 | +--> WP1B Windows WPP ----------------+----> WP5 native breadth
 |                                       |
 +--> WP2 IPP Everywhere 2.0 scaffold ---+
 |                  |                    |
 +--> WP3 PWG Raster/JPEG ---------------+
          |                              |
          +--> WP4 URF/AirPrint ---------+
 |
 +--> WP6 deployment discovery profiles
```

Rules:

- WP1A, WP1B, and the WP2 scaffold may proceed in parallel once WP0's identity contract is defined.
- WP3 does not wait for the entire 2.0 draft lane.
- WP4 reuses the raster/security harness from WP3.
- HIL-limited work never blocks CI-qualifiable work.

## 7. Per-work-package execution TTP

1. Reconcile live repository/workflow authority.
2. Read upstream source where behavior is subtle.
3. State one bounded hypothesis.
4. Use a stacked draft PR or experiment branch.
5. Prefer generator -> candidate -> independent validator.
6. Require durable acceptance, not UI success alone.
7. Classify red correctly: candidate, harness, environment, or unfaithful substrate.
8. Promote only the minimal proven delta.
9. Pay for one integrated requalification cycle after promotion.
10. Leave a durable evidence baton with exact run/job/artifact references.

CI economy:

- public/free GitHub Actions through existing Agent Dispatch constraints;
- no broad environment-census reruns;
- permanent matrices only for meaningful independent variables;
- use one-shot experiments and path filtering;
- preserve HIL as an explicit gate instead of faking it in CI.

## 8. Red-team findings and blue-team controls

| Red-team failure mode | Consequence | Blue-team control |
| --- | --- | --- |
| Advertise format/profile before accepting it | False compatibility claim | Capability-honesty manifest; every advertised value needs a passing receipt |
| Split DNS-SD/IPP/TLS identity | Discovery succeeds then status/print fails | Canonical public URI plus machine coherence gate |
| Count emulator/simulator UI success as product success | False positive | Durable acceptance + candidate logs + native framework state |
| Treat hosted-environment failure as candidate failure | Wrong architecture/substrate decision | Explicit candidate/harness/environment/unfaithful-substrate taxonomy |
| Add transforms unnecessarily | Larger attack/resource surface | Preserve source artifact; transform only when semantics require |
| Raster/JPEG bombs | CPU/memory/disk exhaustion | Existing hard budgets plus dimension/resolution/page-count guards |
| Draft 2.0 churn blocks current product | Roadmap thrash | Separate current vetoes from draft-forward sensors |
| Multiply OS matrices until CI is exhausted | Slow feedback/cost creep | Protocol generations + representative native clients |
| Let WPP or Apple HIL block everything | Dead critical path | Parallelize HIL-limited nodes |
| Implement PCLm/eSCL/USB/cloud because they are adjacent | Scope creep | Watchlist until a concrete client/product requirement earns them |
| Add a feature to only CUPS or PAPPL and then use it as selection evidence | Invalid bakeoff | Symmetric acceptance semantics; substrate-specific implementation allowed |
| Implicit mDNS reflection | Security boundary violation | Explicit deployment profile; no automatic cross-VLAN reflection |
| Normalize all input to PDF | Data loss/extra renderer surface | Source artifact + original media type remain authoritative |

Red-team conclusion: keep the semantic center small. FolioRelay is an IPP-native ingress authority, not a universal vendor-driver emulator.

## 9. Multilingual / global concept sweep

The sweep covered English, German, Spanish, Chinese, Japanese, Korean, and Thai material, plus localized Apple deployment guidance.

### Semantic convergence

The dominant human concept is driverless / OS-native printing:

- German: `treiberloses Drucken`;
- Spanish: `impresión sin controladores`;
- Chinese: `无需驱动` / OS-native wireless-printing concepts;
- Japanese: `OS標準のプリント機能`;
- Korean material presents AirPrint, Mopria, and IPP Everywhere as native/mobile printing paths;
- Thai material presents IPP Everywhere / ChromeOS printing without dedicated driver/application installation.

This supports a profile architecture: the human intent is "it appears in the operating system and prints without vendor software"; AirPrint, Mopria, and IPP Everywhere are implementation/certification profiles underneath.

### Regional negative-space targets

Chinese vendor material surfaces Kylin, UnionTech/UOS, and HarmonyOS compatibility. These are useful future client-validation targets after the standards core passes. They do not currently justify a new core protocol.

### Deployment concepts

German/enterprise and Thai materials reinforce:

- same-subnet mDNS expectations;
- IPv4/IPv6 discovery concerns;
- explicit reflector/routed discovery configurations;
- IPPS/security identity.

Discovery topology belongs to deployment policy, not hidden product behavior.

### No competing core protocol found

The sweep did not identify a major current OS family moving away from the IPP/driverless model.

Adjacent watchlist only:

- PCLm;
- eSCL scanning;
- IPP-USB / Wi-Fi Direct;
- cloud/managed-print connectors and IPP Shared Infrastructure.

## 10. Definition of done

### Canonical identity

- external client receives a reachable `printer-uri-supported`;
- DNS-SD UUID == IPP UUID;
- DNS-SD `rp` == public URI path;
- IPPS certificate identity matches advertised host;
- Android/CUPS native E2E advances durable acceptance.

### libcups 3

- pinned libcups 3 client prints to both candidates on representative architectures;
- exact source artifact reaches durable authority;
- no libcups 2 local scheduler dependency.

### WPP

- WPP proven enabled on supported Windows 11;
- Windows Ready Print is the actual path;
- no vendor driver;
- durable acceptance passes.

### IPP Everywhere 2.0 forward lane

- stable-draft requirements live in versioned machine-readable ledger;
- CI covers machine-testable requirements;
- HIL requirements explicit;
- draft failures remain separate from current release gates.

### PWG Raster/JPEG

- formats truthfully advertised and accepted;
- media type preserved;
- malformed/resource-abusive input fails closed;
- a client forced away from PDF still succeeds.

### AirPrint

- URF accepted;
- `_universal._sub` and URF TXT are correct;
- hosted macOS profile passes where faithful;
- real Apple mobile device completes native AirPrint E2E.

## 11. Deferred/watchlist

Not on the current critical path:

- Windows V3/V4 vendor drivers;
- custom Windows Print Support App without a real requirement;
- CUPS 3 cups-local/cups-sharing production migration while upstream remains immature;
- PCLm without a target client requirement;
- eSCL scanning unless product mission expands;
- IPP-USB/USB gadget emulation;
- Wi-Fi Direct;
- Microsoft Universal Print cloud integration;
- vendor mobile apps;
- per-distribution/per-point-release CI explosion.

## 12. Primary external references

- Microsoft Windows Ready Print: https://learn.microsoft.com/en-us/windows/modern-print/windows-ready-print
- Microsoft Windows Protected Print Mode: https://learn.microsoft.com/en-us/windows/modern-print/windows-protected-print-mode/windows-protected-print-mode
- Microsoft Printers Policy CSP: https://learn.microsoft.com/windows/client-management/mdm/policy-csp-printers
- PWG IPP Everywhere: https://www.pwg.org/ipp/everywhere.html
- PWG IPP workgroup/drafts: https://www.pwg.org/ipp/
- OpenPrinting libcups: https://github.com/OpenPrinting/libcups
- OpenPrinting ipptransform: https://openprinting.github.io/cups/libcups/ipptransform.html
- OpenPrinting CUPS 3 architecture: https://github.com/OpenPrinting/cups/wiki/CUPS-3.0
- Apple AirPrint deployment guidance: https://support.apple.com/en-gb/guide/deployment/dep3b4cf515/web
- Google Chromebook printer setup: https://support.google.com/chromebook/answer/7225252

## 13. Durable repository evidence

- PR #1 - integrated FolioRelay runtime/product authority;
- PR #9 / run `36593621802` - corrected Android/CUPS BIPS discovery PASS;
- PR #10 / run `36595404720` - canonical public-URI defect evidence;
- PR #11 / run `36595868557` - DNS-SD/IPP UUID coherence gate PASS;
- `contracts/print-substrate-evaluation.yaml` - current substrate/client evidence ledger;
- `experiments/print-substrate/cups-foliorelay.ppd` - current PDF-shaped CUPS profile;
- `experiments/print-substrate/cups-backend.sh` - current CUPS media-type assumption;
- `experiments/print-substrate/pappl-ingress.c` - current PAPPL PDF format and deliberate raster rejection;
- `cmd/foliorelay-ipp-ingest/main.go` - format-agnostic durable artifact/media-type intake.
