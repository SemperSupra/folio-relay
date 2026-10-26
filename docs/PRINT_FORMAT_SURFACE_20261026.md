# FolioRelay printing format inventory — source versus installed image (2026-10-26)

**Governing authority:** physical HIL #35, capability honesty #49, download filename #47, post-MVP program #79. Read-only source evidence plus negative Go source-contract tests; this document **does not** claim a new live printer observation, release approval, or new capability.

## Physical accepted image baseline

Product source for installed immutable control publication: `1c50579209d513e7bf9fe405062ed9b641a048b5`; installed accepted control digest `sha256:d0ba6d1efbed0d9f84b20d374eeb44ee28ab0874a683396e628850f159193cf5`, CUPS digest `sha256:0997ad2054ca5e57f34291372aed55f549eee9ff201f171b430436b0655c0814`. #35 recorded real Windows native IPP and iPhone AirPrint submissions with one durable job each, zero duplicates; those two clients/formats are not evidence of arbitrary application/document or printer output support. Source-only PRs #98–#102 are NOT deployed.

## Format and transport boundaries

| Layer | Qualified/represented | Behavior |
| --- | --- | --- |
| User application/document | E.g. webpage, Word document, photo, PDF | **Client-side conversion** by Windows native IPP/AirPrint may yield one of the printer's accepted print formats. No FolioRelay Office/image-import or rendering converter is implemented; arbitrary source formats are not independently validated. |
| IPP/IPP Everywhere and Apple AirPrint print data | `application/pdf` or `image/urf` | Installed PPD has two `cupsFilter2` identity paths `PDF → PDF` and `URF → URF`. `internal/airprint/profile.go` advertises `application/pdf,image/urf`. |
| CUPS fixed backend handoff | `application/pdf` or `image/urf` only | `cups-backend.sh` explicitly rejects all other final MIME types; forwards accepted artifact unchanged to `foliorelay-ipp-ingest`. No JPEG/PNG/Office/PostScript-to-PDF converter in the deployed backend path. |
| Durable canonical artifact | Original accepted PDF or URF bytes | Preserved under content-addressed SHA-256. Intake cap is 100 MiB in packaged adapter; the immutable journal records MIME, digest and accepted identity. |
| Human/API download | Same original PDF/URF, not a new render | Legacy installed `Content-Type` matches retained MIME, but accepted source `handleArtifact` names the attachment using an opaque `JobID` **without extension**. Later unmerged/source integration has better `.pdf`/`.urf` download names; do not misreport these as installed. |
| Output to physical printer | **None** | Device URI `foliorelay:/` captures into the FolioRelay Inbox; no connected physical printer or IPP/PCL/PostScript output driver route is accepted/deployed. Physical IPP/IPPS output remains a future adapter. |
| Output copies to SMB/email/cloud/Paperless | Source-only drafts | No installed delivery/filer route is qualified for real external effect. |

The installed PPD advertises **A4 and US Letter**. The recorded live read-only printer attributes in #49 said **one-sided and monochrome**; no support claim for duplex, color, finishing, scan or fax should be inferred from PPD `*DefaultColorSpace: RGB` (this setting alone is not printer-output acceptance). `image/urf` is **Apple Universal Raster**, not a TIFF/JPEG/PNG file. PDF is an accepted print data MIME format, not evidence of a native PDF file upload endpoint.

## Regression gates and next implementation steps

- `internal/airprint/profile_test.go` checks exact Go AirPrint PDL vs two PPD zero-conversion MIME paths and actual CUPS backend whitelist, and verifies printer remains capture-only, B&W/small-size, A4/Letter. A new format should fail this conservative contract test until the full pipeline is explicitly revised and qualified.
- To **add input formats**, perform a client-format/provider matrix by operating system/application, identify PDF/URF conversion capabilities and reject unsupported representations with stable error codes; add converters only as separately justified, isolated and tested plugins with resource bounds, without silently changing immutable source artifacts.
- To **add output formats/routing**, qualify immutable original PDF/URF downloads separately from derived PDF, PNG/JPEG, OCR/PDFA, image thumbnails, and optional routed physical IPP/IPPS. Derivatives must have their own provenance, MIME, digest, safety policy and explicit authorization. Do not call an unverified export an accepted print.
- Establish human EN/DE/TH labeling and retry/error paths, machine-readable automation/agent format negotiation and independent per-record access control #96 before exposing other APIs.
- Preserve draft-only, no physical app changes, $0 public CI and exact-head review. A new immutable image, virtual TrueNAS F0-F5 and owner-authorized new site admission are prerequisites for deployment.

Source anchors: `experiments/print-substrate/cups-foliorelay.ppd`, `experiments/print-substrate/cups-backend.sh`, `internal/airprint/profile.go`, `deploy/engineering-cups/printers.conf`, `cmd/foliorelay-ipp-ingest/main.go`, installed `internal/control/server.go@1c505792...`, issues #35/#47/#49.
