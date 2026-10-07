# Physical client HIL acceptance

Status: final post-RDTE acceptance boundary.

This HIL is intentionally **client/LAN confirmation only**. The FolioRelay
TrueNAS implementation has already completed exact-version virtual F0-F5
qualification on 25.04.1, 25.04.2.6, 25.10.7, and 26.0.0-BETA.3. Do not use
the physical TrueNAS system for install/debug iteration.

## Qualified server candidate

The physical HIL target is the already-qualified TrueNAS 25.04.1 realization:

- TrueNAS: `25.04.1`
- Foundry control: `6494dbd336260b549772ffa5c398d7a104fabf15`
- FolioRelay control:
  `ghcr.io/sempersupra/foliorelay-control@sha256:c8d5787162db919f84e9607d13f368995138861355f3fa269cbb10561f24d80d`
- CUPS:
  `ghcr.io/sempersupra/foliorelay-cups@sha256:0997ad2054ca5e57f34291372aed55f549eee9ff201f171b430436b0655c0814`
- Virtual qualification receipt: Agent Dispatch run `37572795630`,
  artifact `11461469097`,
  digest `sha256:5142c17cbe96fd3b80cccb09e15754a16346144e3fdac4ddb20e3050dae35b3e`.

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
- a submitted job does not appear exactly once in the durable Inbox.

A stopped HIL is evidence for a new bounded diagnostic campaign, not permission
to repair production in place.

## H0 — preflight/read-only identity

Record, without modifying the deployment:

1. TrueNAS reports version `25.04.1`.
2. FolioRelay control, CUPS, and discovery services are running.
3. Running control/CUPS image digests match the qualified values above.
4. Record the printer name, printer UUID, public IPP URI, and current durable
   Inbox count.

Acceptance: all four observations match the existing candidate.

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

Acceptance: native Windows IPP submission reaches durable FolioRelay acceptance
exactly once with no server-side reconfiguration.

## H3 — iPhone/iPad AirPrint

Use a physical iPhone or iPad on the same LAN.

1. Open a small PDF or other ordinary printable document in a native Apple app
   such as Files or Safari.
2. Choose Share -> Print.
3. Select FolioRelay from the AirPrint printer picker. Do not enter a URI
   manually.
4. Submit one print.
5. Confirm the client reports submission/completion without an error.
6. Confirm FolioRelay durable Inbox advances by exactly one and the accepted
   artifact corresponds to the Apple device job.

Acceptance: the physical Apple device discovers FolioRelay through AirPrint and
delivers one durable job without duplicates.

## H4 — identity and duplicate check

After H2 and H3:

1. Re-read the FolioRelay printer UUID/public URI.
2. Confirm both remain identical to H0.
3. Confirm the Inbox increased by exactly two total accepted jobs, one Windows
   and one Apple-device job.
4. Confirm no duplicate durable effect was created by client retries.
5. Confirm the production services remain healthy.

## Client-side evidence kit

Use the repository's HIL collector from the real Windows client to minimize
manual evidence handling. The collector never changes TrueNAS or FolioRelay
server configuration. It prompts for the FolioRelay management token using a
secure prompt and keeps the plaintext value in memory only.

The automated H1 observer sends an RFC 6762 legacy-unicast query from an
ephemeral client UDP port. It requires one same-instance
`_universal._sub._ipp._tcp` PTR/TXT/SRV result matching the H0 UUID, host,
port, resource path, and PDF/URF projection. It does not bind UDP/5353 and is
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
  -BaseUrl http://<foliorelay-host>:18080 \
  -SessionDir $session \
  -ObservedTrueNASVersion 25.04.1 \
  -ObservedControlImage 'ghcr.io/sempersupra/foliorelay-control@sha256:c8d5787162db919f84e9607d13f368995138861355f3fa269cbb10561f24d80d' \
  -ObservedCupsImage 'ghcr.io/sempersupra/foliorelay-cups@sha256:0997ad2054ca5e57f34291372aed55f549eee9ff201f171b430436b0655c0814'
```

The preflight fails closed if version/images differ, FolioRelay is not
healthy/ready, printer identity is not the qualified IPP shape, or coherent
AirPrint discovery is absent.

For H2, the collector uses the already-qualified Windows primitive
`Add-Printer -IppURL`, requires an IPP class driver, creates a uniquely named
temporary local queue, submits exactly one bounded Windows spooler job, proves
the durable Inbox advanced by exactly one, and removes only the queue it
created:

```powershell
.\scripts\hil\Invoke-FolioRelayPhysicalHil.ps1 \
  -Phase Windows \
  -BaseUrl http://<foliorelay-host>:18080 \
  -SessionDir $session
```

Then perform H3 manually from the physical iPhone/iPad as specified above.
Do not run any other print jobs against FolioRelay between H0 and H4.

Finally run H4, recording the physical Apple device type and OS version:

```powershell
.\scripts\hil\Invoke-FolioRelayPhysicalHil.ps1 \
  -Phase Final \
  -BaseUrl http://<foliorelay-host>:18080 \
  -SessionDir $session \
  -AppleClientType iPhone \
  -AppleOSVersion '<observed iOS version>'
```

A successful final phase requires the original UUID/URI to remain unchanged,
the durable Inbox to be exactly H0+2, the known Windows durable job to be one
of those two jobs, and health/readiness to remain good. It emits
`receipt.json` plus phase evidence in the session directory. The script scans
its text evidence for accidental management-token disclosure before reporting
PASS.

## Completion rule

Physical HIL is PASS only when H0-H4 all pass without server mutation.

Retain a compact receipt containing:

- timestamp and LAN context (no secrets);
- TrueNAS version;
- immutable control/CUPS digests;
- printer UUID and public URI;
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
