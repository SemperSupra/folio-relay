# PAPPL qualification archive

Status: archived reference as of 2026-09-30.

FolioRelay selected the minimized upstream CUPS runtime as its production print
substrate. PAPPL qualification remains preserved, but normal FolioRelay CI must
not continue paying to requalify it.

## Why PAPPL was not selected for FolioRelay

PAPPL passed substantial qualification across native clients, security,
restart/idempotency, stress, discovery/TLS, and amd64/arm64 portability.

The decisive product-fit difference is the artifact boundary. PAPPL v1.4.12
processes Apple Raster/URF and PWG Raster as streaming raster before ordinary
job-file spooling. Its public driver callbacks therefore do not expose the
exact submitted raster artifact to the FolioRelay bridge.

FolioRelay intentionally preserves the submitted artifact as an immutable
source object for Inbox, routing, later transformation, and provenance.
Preserving that invariant with PAPPL would require an extra raw-capture layer,
an upstream API/behavior change or maintained fork, or relaxing the invariant.
The qualified CUPS backend path does not require those changes.

## Why the work remains valuable

PAPPL is a strong fit for a product shaped more like:

`driverless IPP -> raster processing -> printer driver -> physical printer`

Retain this work for a future Printer Application or physical-printer-side
product and as an interoperability oracle.

## Pinned reference and evidence

- PAPPL v1.4.12
- source commit: `6db8e137557ad84662e78d24fdb2a591c621f4ac`
- Android native PAPPL E2E: run `36591642298`
- selection characterization: PR #1 comment `5893706199`
- URF/PWG streaming source finding: PR #1 comment `5901940748`
- final MVP selection packet: PR #1 comment `5907983800`

Retained implementation surfaces include `deploy/engineering-pappl/` and
`experiments/print-substrate/pappl-ingress.c`. Reactivate qualification only
under a new explicit requirement/product authority.
