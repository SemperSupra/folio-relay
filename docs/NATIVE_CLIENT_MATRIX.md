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
| macOS 15 native print subsystem | — | — | capability interview running |
| Windows Server 2025 native print subsystem | — | — | capability interview running |
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
