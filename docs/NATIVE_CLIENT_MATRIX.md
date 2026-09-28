# Native client qualification matrix

Status: active Phase-3 qualification.

FolioRelay distinguishes protocol conformance from native-client behavior. A
platform result is only promoted to admission evidence when the operating
system's real print subsystem or native print client reaches an actual
FolioRelay candidate endpoint. A generic HTTP/IPP script running on a platform
does not count as that platform's native-print qualification.

## Current matrix

| Platform/client | CUPS | PAPPL | Status |
|---|---:|---:|---|
| Linux Ubuntu 24.04 native libcups direct Print-Job | PASS | PASS | qualified |
| Ubuntu local CUPS `-m everywhere` + `lp` conversion | host filter crash | host filter crash | negative-space evidence; not a candidate gate |
| macOS 15 native print subsystem | PASS | PASS | qualified through system CUPS `lpadmin -m everywhere` + native `lp` |
| Windows Server 2025 native print subsystem | PASS | PASS | qualified through PrintManagement/Spooler + Microsoft IPP Class Driver |
| Android print service / Mopria-compatible path | — | — | unqualified; require faithful hosted execution |
| iOS/iPadOS AirPrint path | — | — | unqualified; require faithful simulator/device execution |

## Promotion rule

A platform capability interview is read-only. It inventories the print
subsystem, client commands/APIs, discovery primitives, and relevant class
drivers/frameworks.

A platform is promoted from **capability interview** to a real candidate
qualification only when all of these are true:

1. the hosted environment exposes the operating system's actual native print
   path rather than a synthetic protocol-only client;
2. the path can address a real CUPS or PAPPL candidate endpoint;
3. a real document reaches FolioRelay durable acceptance;
4. the same document/job identity cannot create a duplicate durable effect;
5. any required discovery setup is representative of the platform behavior;
6. the rep does not require broad privileges or runner modifications that would
   make it unlike an ordinary client.

If a hosted runner cannot meet those conditions, the platform remains
unqualified rather than being represented by a weaker substitute. HIL/device
qualification can be added later when it earns its keep.

## Linux result

The Ubuntu local scheduler/filter route was deliberately separated from the
candidate result. The host `cups-filters` `universal` process crashed with
signal 11 before transport on both candidates, including with a valid one-page
PDF. Direct system libcups `Print-Job`, which exercises the native CUPS client
library without unrelated local conversion, returned `successful-ok` for both
CUPS and PAPPL and advanced FolioRelay durable acceptance without conflict.

## Capability census

`.github/workflows/native-client-capability-census.yml` runs read-only
interviews on standard public GitHub-hosted macOS 15 and Windows Server 2025
runners. It intentionally does not create printers yet. The evidence determines
whether a faithful next-stage native-print rep can be automated before any such
rep becomes a required gate.


## Capability interview results

### macOS 15

The public hosted runner exposes the real native print stack:

- `cupsd` is running under launchd;
- `lp`, `lpstat`, `lpadmin`, `ippfind`, `ipptool`, and
  `cups-config` are present;
- Bonjour `dns-sd` is present;
- the selected macOS SDK contains `cups/cups.h`;
- a native libcups header/link probe succeeds.

This earns a real next-stage experiment. PAPPL v1.4.12 explicitly supports
macOS, so the first rep builds the exact PAPPL commit and actual FolioRelay
bridge natively on the macOS runner, creates a temporary driverless queue with
the host's system CUPS, submits via native `lp`, and requires FolioRelay
durable acceptance with no conflict.

### Windows Server 2025

The hosted runner also exposes a credible native print stack:

- Print Spooler is present, Automatic, and Running;
- PowerShell PrintManagement 1.1 is present;
- `Add-Printer`, `Get-Printer`, `Get-PrinterDriver`,
  `Get-PrinterPort`, and `Out-Printer` are available;
- the `MSFT_Printer` CIM class is available;
- `Microsoft IPP Class Driver` is installed.

Windows therefore remains eligible for a real native rep. The capability
interview now also inventories WSL, optional virtualization features,
container/runtime tools, relevant services, and the exact `Add-Printer`
parameter surface (including whether `-IppURL` is available). The preferred
strategy is a real local candidate through WSL/container execution if the hosted
runner can support it faithfully; otherwise the next course is a bounded
ephemeral network-reachable real candidate. If neither can be qualified without
distorting the native path, Windows remains deferred to HIL rather than using a
synthetic IPP substitute.


### macOS PAPPL native result

`macOS Native Client Qualification #1` (run ID `36411288637`) passed on
the public macOS 15 arm64 runner. The workflow built the exact PAPPL v1.4.12
source commit and the real FolioRelay bridge natively, then used the runner's
system CUPS scheduler to create a temporary driverless queue with
`lpadmin -m everywhere`. Native `lp` submitted the generated one-page PDF;
the request was accepted by the PAPPL candidate, FolioRelay durable acceptance
advanced, and the idempotency-conflict counter did not advance.

This is promoted admission evidence because it exercises the actual macOS print
subsystem rather than a platform-labeled protocol script.

A symmetric exact-CUPS macOS job is now part of the same workflow. Its first
build attempt exposed upstream CUPS 2.4.19 CDSA/SecureTransport source drift
against the current macOS 15/Xcode SDK: `tls-darwin.c` calls an internal helper
using an older signature. FolioRelay does not patch or fork CUPS for the rep.
The retry builds the same exact CUPS commit with upstream's supported OpenSSL
TLS backend, matching the existing production candidate policy more closely,
then uses the same system `lpadmin -m everywhere` + native `lp` path.


### Windows WSL candidate-host strategy

The expanded Windows Server 2025 census found a promising fully local path:

- WSL is present and both status/version probes succeed;
- Windows Subsystem for Linux and VirtualMachinePlatform are enabled;
- `vmcompute`, HNS, and Docker services are running;
- firmware virtualization reports enabled;
- the native PrintManagement `Add-Printer` command exposes `-IppURL`;
- Microsoft IPP Class Driver remains installed.

The next rep therefore avoids a public tunnel. A Linux job exports the already
pinned Ubuntu 24.04 OCI rootfs with a static real FolioRelay state-authority
probe. The Windows hosted runner imports that tarball as WSL2 and must reach the
running FolioRelay probe through Windows localhost forwarding. Passing that rep
earns WSL2 as the candidate-host substrate for the real Windows native-printer
qualification; it does not by itself count as Windows printing qualification.


### macOS CUPS native result — upstream sandbox preserved

The symmetric exact-CUPS macOS candidate is now qualified. Run
`36427659628` built exact CUPS v2.4.19 commit
`6ba0487abb05afc93d639f37676add8fd65d3756` with the upstream-supported
OpenSSL TLS backend, started the isolated FolioRelay queue, and passed the same
macOS system `lpadmin -m everywhere` plus native `lp` path used for PAPPL.

The qualification exposed two useful upstream macOS sandbox invariants before
passing:

1. CUPS 2.4.19 explicitly denies `process-exec` below `/Users/`, so a
   GitHub-runner temporary backend under `/Users/runner/work/_temp` cannot be
   executed by `cups-exec`.
2. Moving the executable surface under `/private/tmp` permits execution, but
   arbitrary writes there are still denied by the generated job profile.
   CUPS does explicitly permit the system spool hierarchy.

The passing rep therefore keeps sandboxing enabled and makes no CUPS source
patch: the bounded backend/ingest executable surface lives under
`/private/tmp/foliorelay-cups-<run>`, while the qualification artifact store
uses a dedicated runner-owned
`/private/var/spool/foliorelay-ingress-<run>` directory. Durable FolioRelay
acceptance advances and the idempotency-conflict counter remains unchanged.

macOS native-client parity is therefore closed for both active substrate
candidates.


### Windows native parity result

`Windows Native Client Qualification #27` (run ID `36441803876`) closes
Windows parity for both active candidates on the public Windows Server 2025
runner.

The qualified path is the operating system's real print stack:

1. the exact packaged candidate rootfs is exported from the pinned Ubuntu build;
2. Windows imports it into WSL2;
3. Windows reaches the candidate through the qualified NAT direct-IP path;
4. `Add-Printer -IppURL` creates the queue using Microsoft IPP Class Driver;
5. `Out-Printer` submits through PrintManagement/Spooler;
6. FolioRelay durable acceptance must advance without an idempotency conflict.

Both PAPPL v1.4.12 and minimal CUPS v2.4.19 pass.

The CUPS qualification exposed one genuine minimization requirement. Windows
sends the IPP document with gzip transport encoding, and upstream `cupsd`
automatically inserts its `gziptoany` filter before the FolioRelay backend.
The minimized image had intentionally removed all stock filters, so the first
native Windows rep stopped the job with `gziptoany` missing. The repair retains
only upstream CUPS `gziptoany`; it does not add cups-filters or a broader
legacy driver stack.

Engineering CUPS Runtime #69 proves that the retained transport filter is
present while `pstops`, `rastertopwg`, `foomatic-rip`, the generic CUPS
`ipp` and `snmp` backends, the `mailto` notifier, and `sendmail` remain
absent. The only installed backend is the bounded FolioRelay backend.

The passing Windows CUPS evidence records:

- driver: `Microsoft IPP Class Driver`;
- native job state: Complete/Retained, one page;
- durable FolioRelay stats: accepted `0 -> 1`;
- idempotency conflicts: `0`.
