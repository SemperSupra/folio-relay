# Physical client HIL acceptance

Status: client/LAN acceptance procedure prepared; **execution HOLD** until the
current WebUI/HTTPS control image independently re-earns the required
exact-version virtual matrix.

This HIL remains intentionally **client/LAN confirmation only**. Do not use the
physical TrueNAS system for install/debug iteration. A virtual PASS authorizes
only the bounded client checks below.

## Candidate bound to this procedure

The intended physical HIL target remains TrueNAS `25.04.1`, but the following
WebUI/HTTPS candidate must first re-earn virtual support:

- TrueNAS physical target: `25.04.1`
- FolioRelay product/source: `1c50579209d513e7bf9fe405062ed9b641a048b5`
- Foundry authority: `81deb97185760975fd8d3162df42056c77b3c0fd`
- FolioRelay control:
  `ghcr.io/sempersupra/foliorelay-control@sha256:d0ba6d1efbed0d9f84b20d374eeb44ee28ab0874a683396e628850f159193cf5`
- CUPS:
  `ghcr.io/sempersupra/foliorelay-cups@sha256:0997ad2054ca5e57f34291372aed55f549eee9ff201f171b430436b0655c0814`
- supported management surface: HTTPS Web UI/API on port `18443`; HTTP
  `18080` is app-private and is not a supported LAN endpoint.

Do not execute H0-H4 until the fresh virtual matrix is complete and issue #35
is explicitly released from HOLD.

Expected public printer identity:

- service: `_ipp._tcp`
- AirPrint subtype: `_universal._sub._ipp._tcp`
- resource path: `/printers/FolioRelay`
- port: `8634`

Use the actual printer name, UUID, and `.local` host advertised by the running
deployment. Do not rewrite them for HIL.

## Stop conditions

Stop and retain evidence without changing the server if any of these occur:

- FolioRelay services are not already healthy;
- the server version or immutable image digests differ from the qualified
  candidate;
- mDNS discovery is absent from a client on the same LAN;
- a native client cannot submit without changing server/container privileges,
  mounts, networking, CUPS configuration, or Avahi configuration;
- the HTTPS management portal is not reachable from a real client, presents a
  certificate for the wrong host, or changes certificate identity during the
  HIL;
- the Web UI cannot be rendered and used from a real Windows browser and real
  iPhone/iPad Safari;
- a submitted job does not appear exactly once in the durable Inbox.

A stopped HIL is evidence for a new bounded diagnostic campaign, not permission
to repair production in place.

## H0 — preflight/read-only identity

Record, without modifying the deployment:

1. TrueNAS reports version `25.04.1`.
2. FolioRelay control, CUPS, and discovery services are running.
3. Running control/CUPS image digests match the qualified values above.
4. Use the supported HTTPS management portal at
   `https://<foliorelay-host>:18443/`; confirm the root Web UI returns HTTP
   200, `/healthz` and `/readyz` are healthy, and record the presented
   certificate SHA-256 fingerprint.
5. Record the printer name, printer UUID, public IPP URI, and current durable
   Inbox count.

The client collector permits an untrusted/self-signed chain for the
product-generated certificate, but it **rejects certificate hostname mismatch**
and pins the observed certificate fingerprint for all later HIL phases.

Acceptance: the observations match the candidate and the HTTPS management
identity is stable and bound to the FolioRelay host.

## H1 — real-LAN DNS-SD / AirPrint discovery

From a client on the same ordinary LAN/VLAN as the printer:

1. Confirm a single FolioRelay `_ipp._tcp` service is visible.
2. Confirm the same service instance exposes the AirPrint `_universal`
   subtype.
3. Confirm the discovered SRV target/port and TXT resource path resolve to the
   running FolioRelay endpoint.
4. Record the observed service instance, host, port, resource path, and printer
   UUID if surfaced by the client.

Acceptance: one coherent service instance describes the same printer identity
as H0. No synthetic mDNS publisher/proxy is permitted for this HIL.

## H2 — Windows native IPP

Use a real Windows client and the operating system print stack.

Before printing, open the exact HTTPS portal recorded in H0 in a real Windows
browser (Edge or another installed browser), authenticate, and confirm the
Overview and Inbox views render and are usable. If the default self-signed
certificate produces a trust warning, compare its SHA-256 fingerprint with
`preflight.json`; do not proceed through a hostname mismatch or a different
certificate.

1. Add FolioRelay using Windows' native IPP printer flow. Prefer discovery;
   direct entry of the qualified IPP URI is acceptable only after H1 has
   independently proved discovery.
2. Confirm Windows uses **Microsoft IPP Class Driver** (or the current native
   Microsoft IPP class-driver equivalent), not a vendor/legacy driver.
3. Submit one bounded test document through the Windows spooler.
4. Wait for the Windows job to leave the active queue normally.
5. Confirm FolioRelay durable Inbox advances by exactly one and that the
   accepted artifact corresponds to this Windows job.
6. Remove the temporary client queue after evidence capture if it was created
   only for HIL.

Useful read-only PowerShell evidence where available:

```powershell
Get-Printer | Where-Object Name -Like '*FolioRelay*' |
  Select-Object Name, DriverName, PortName, PrinterStatus
```

Acceptance: the HTTPS Web UI is usable from the real Windows browser and native
Windows IPP submission reaches durable FolioRelay acceptance exactly once with
no server-side reconfiguration.

## H3 — iPhone/iPad AirPrint

Use a physical iPhone or iPad on the same LAN.

Before printing, open the same HTTPS FolioRelay portal in real Safari,
authenticate, and confirm the Overview and Inbox views render and are usable.
For a product-generated self-signed certificate, do not accept a hostname
mismatch or a certificate whose fingerprint differs from H0.

1. Open a small PDF or other ordinary printable document in a native Apple app
   such as Files or Safari.
2. Choose Share -> Print.
3. Select FolioRelay from the AirPrint printer picker. Do not enter a URI
   manually.
4. Submit one print.
5. Confirm the client reports submission/completion without an error.
6. Confirm FolioRelay durable Inbox advances by exactly one and the accepted
   artifact corresponds to the Apple device job.

Acceptance: the HTTPS Web UI is usable from real iPhone/iPad Safari, and the
physical Apple device discovers FolioRelay through AirPrint and delivers one
durable job without duplicates.

## H4 — identity and duplicate check

After H2 and H3:

1. Re-read the FolioRelay printer UUID/public URI.
2. Confirm both remain identical to H0.
3. Confirm the Inbox increased by exactly two total accepted jobs, one Windows
   and one Apple-device job.
4. Confirm no duplicate durable effect was created by client retries.
5. Confirm the production services remain healthy.
6. Confirm the HTTPS management certificate fingerprint remains identical to
   H0 and the Web UI remains reachable.

## Client-side evidence kit

Use the repository's HIL collector from the real Windows client to minimize
manual evidence handling. The collector never changes TrueNAS or FolioRelay
server configuration. It prompts for the FolioRelay management token using a
secure prompt and keeps the plaintext value in memory only.

The automated H1 observer sends an RFC 6762 legacy-unicast query from an
ephemeral client UDP port. It requires the same service instance to appear in both the base
`_ipp._tcp` PTR set and the `_universal._sub._ipp._tcp` subtype PTR set,
with matching TXT/SRV records for the H0 UUID, host, port, resource path, and
PDF/URF projection. It does not bind UDP/5353 and is
not a synthetic publisher or proxy.

Create one evidence directory and reuse it for all phases:

```powershell
$session = Join-Path $PWD ("foliorelay-physical-hil-" + (Get-Date -Format yyyyMMdd-HHmmss))
```

For H0/H1, first read the TrueNAS version and running control/CUPS image
identities from the existing deployment without modifying it. Then run:

```powershell
.\scripts\hil\Invoke-FolioRelayPhysicalHil.ps1 \
  -Phase Preflight \
  -BaseUrl https://<foliorelay-host>:18443 \
  -SessionDir $session \
  -ObservedTrueNASVersion 25.04.1 \
  -ObservedControlImage 'ghcr.io/sempersupra/foliorelay-control@sha256:d0ba6d1efbed0d9f84b20d374eeb44ee28ab0874a683396e628850f159193cf5' \
  -ObservedCupsImage 'ghcr.io/sempersupra/foliorelay-cups@sha256:0997ad2054ca5e57f34291372aed55f549eee9ff201f171b430436b0655c0814'
```

The preflight fails closed if version/images differ, the BaseUrl is not the
HTTPS portal on 18443, certificate hostname validation fails, FolioRelay is not
healthy/ready, printer identity is not the qualified IPP shape, or coherent
AirPrint discovery is absent. The observed management certificate fingerprint
is recorded in `preflight.json` and must remain stable.

For H2, the collector uses the already-qualified Windows primitive
`Add-Printer -IppURL`, requires an IPP class driver, creates a uniquely named
temporary local queue, submits exactly one bounded Windows spooler job, proves
the durable Inbox advanced by exactly one, and removes only the queue it
created:

```powershell
.\scripts\hil\Invoke-FolioRelayPhysicalHil.ps1 \
  -Phase Windows \
  -BaseUrl https://<foliorelay-host>:18443 \
  -SessionDir $session \
  -WindowsWebUiConfirmed
```

Then perform the Safari Web UI smoke and H3 manually from the physical
iPhone/iPad as specified above. Do not run any other print jobs against
FolioRelay between H0 and H4.

Finally run H4, recording the physical Apple device type and OS version:

```powershell
.\scripts\hil\Invoke-FolioRelayPhysicalHil.ps1 \
  -Phase Final \
  -BaseUrl https://<foliorelay-host>:18443 \
  -SessionDir $session \
  -AppleClientType iPhone \
  -AppleOSVersion '<observed iOS version>' \
  -AirPrintConfirmed \
  -AppleWebUiConfirmed
```

`-WindowsWebUiConfirmed` and `-AppleWebUiConfirmed` are explicit operator
assertions that the real Windows browser and real iPhone/iPad Safari each
rendered the HTTPS FolioRelay Web UI and that authenticated Overview/Inbox
views were usable. `-AirPrintConfirmed` separately asserts that the physical
Apple device selected FolioRelay from the native AirPrint picker and submitted
exactly one job. A successful final phase also requires the original UUID/URI
and management TLS fingerprint to remain unchanged,
the durable Inbox to be exactly H0+2, the known Windows durable job to be one
of those two jobs, and health/readiness to remain good. It emits
`receipt.json` plus phase evidence in the session directory. The script scans
its text evidence for accidental management-token disclosure before reporting
PASS.

## Completion rule

Physical HIL is PASS only when H0-H4 plus the real Windows and iPhone/iPad
HTTPS Web UI smoke checks all pass without server mutation.

Retain a compact receipt containing:

- timestamp and LAN context (no secrets);
- TrueNAS version;
- immutable control/CUPS digests;
- printer UUID and public URI;
- HTTPS management portal, certificate SHA-256 fingerprint, and Windows/Safari
  Web UI confirmations;
- H1 DNS-SD/AirPrint discovery observations;
- Windows client version, driver, queue/port identity, and durable job result;
- Apple device OS version and durable job result;
- Inbox before/after counts;
- any screenshots or client-side logs needed to substantiate discovery and
  submission.

The receipt must not include authentication tokens, private keys, or unrelated
LAN inventory.

## Non-claims

This HIL does not qualify:

- arbitrary VLAN/mDNS reflector configurations;
- WAN printing;
- vendor-specific Windows drivers;
- every iOS/iPadOS release or every Windows release;
- physical printer passthrough;
- TrueNAS hardware, HA, GPU, SMART, or storage-controller behavior.

It closes only the real-client/native-discovery boundary left intentionally
outside the virtual exact-version matrix.
