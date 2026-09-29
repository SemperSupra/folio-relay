# Print-substrate security surface characterization

Status: active, time-indexed characterization.

Last reviewed: 2026-09-28.

This document does **not** treat an upstream advisory count as a product score.
It maps published upstream advisories to the exact FolioRelay runtime profile
and distinguishes four things:

1. vulnerable component/path physically absent;
2. vulnerable precondition prohibited by the static runtime profile;
3. affected code present but not exercised by the current profile;
4. residual exposure requiring a targeted regression or upstream update.

A result of "precondition blocked" is not a claim that the upstream bug does
not exist. It means the documented exploit path is not admitted by the current
FolioRelay profile. Any later profile expansion must re-run this analysis.

## Runtime authorities

Current upstream source authority:

- CUPS: v2.4.19, commit
  `6ba0487abb05afc93d639f37676add8fd65d3756`;
- PAPPL: v1.4.12, commit
  `6db8e137557ad84662e78d24fdb2a591c621f4ac`;
- both candidates use the same minimized private CUPS/libcups build policy;
- no candidate carries an upstream source patch.

The CUPS image is not a general-purpose CUPS installation. It contains one
preconfigured PDF-ingress queue and one FolioRelay backend. Windows native IPP
qualification proved one additional upstream CUPS transport primitive is
required: `gziptoany`.

Engineering CUPS CI proves the following are absent:

- `pstops`;
- `rastertopwg`;
- `foomatic-rip`;
- generic CUPS `ipp` backend;
- CUPS SNMP backend;
- `mailto` notifier;
- banner data tree;
- `sendmail`.

The only installed CUPS backend is `foliorelay`.

The CUPS policy exposes the one FolioRelay printer resource and permits only the
bounded job/query operation set required for printing. Printer-management and
subscription operations are not in the admitted policy.

## CUPS advisory applicability snapshot

Source index:
https://github.com/OpenPrinting/cups/security/advisories

The following mappings are based on the published advisories visible on
2026-09-28 and the exact runtime profile above.

| Advisory | Current profile assessment | Evidence / reason |
|---|---|---|
| GHSA-fw7q-ww8w-phx8 | precondition blocked | Exploit requires driverless queue creation/modify against attacker-influenced printer attributes. FolioRelay ships a static queue and does not admit CUPS add/modify/local-printer operations. |
| GHSA-w9hj-hq9p-m7f6 | structurally blocked | Published end-to-end chain requires a shared legacy PostScript/PPD queue and a path through `pstops`. FolioRelay is PDF-to-PDF and `pstops` is absent. |
| GHSA-r4wf-366f-f6g3 | component and operation absent | `mailto`, `sendmail`, and printer-subscription operations are not present/admitted. |
| GHSA-559w-7676-3xrq | component absent | Published reachability is through `backend/snmp-supplies.c`; the SNMP backend is not shipped. |
| GHSA-gq9p-4w7m-2f5g | precondition blocked | Published banner disclosure requires a registered banner and job-sheet path. The banner tree is not shipped and the static queue defaults to `JobSheets none none`. |
| GHSA-7hqf-mfhx-7r3v | components absent | Published chain requires the generic CUPS `ipp` backend and conditional `foomatic-rip` execution; neither is shipped. |
| GHSA-69qc-prxg-h2c7 | profile does not instantiate affected queue class | FolioRelay exposes one PDF-ingress printer, not a CUPS fax queue. |
| GHSA-jj94-x3qh-ffp9 | management precondition blocked | Published path is printer add/modify with model PPD generation. Runtime printer-management operations are not admitted. |
| GHSA-r8jp-q6fh-g5r2 | residual scheduler code; current auth profile does not depend on affected identity matching | The current printer path is not guarded by local user/group identity matching; all unlisted/admin operations are denied by policy. Re-evaluate before enabling authenticated/admin surfaces. |

The scheduler remains materially larger than the PAPPL service and some
scheduler advisory code is therefore still present in the CUPS executable even
when the documented exploit preconditions are absent. That residual code is a
real maintenance/security cost and should remain visible in final selection.

## PAPPL / shared libcups exposure

The PAPPL runtime does not run `cupsd` and does not ship the CUPS notifier,
SNMP backend, generic IPP backend, PPD legacy filters, or banner subsystem
listed above. Scheduler/backend-specific CUPS advisories therefore do not map
directly onto the PAPPL runtime.

PAPPL still links the same minimized upstream `libcups.so.2`. A vulnerability
in shared libcups code can affect PAPPL when PAPPL exercises the vulnerable
call path. Do not infer safety merely because a CUPS advisory names a scheduler
or backend; each shared-library advisory must be mapped to the actual PAPPL call
graph/profile.

The PAPPL repository did not surface a comparable published GitHub advisory
list in this review. That is **not** evidence that PAPPL is vulnerability-free.
Release notes, issues, source updates, shared-library advisories, and periodic
scanner results remain required inputs.

## Selection implications

Security exposure is not reducible to one count.

Current evidence supports these bounded observations:

- CUPS carries more scheduler code and therefore more residual upstream
  scheduler attack surface.
- The FolioRelay CUPS profile physically removes several currently published
  vulnerable components and blocks multiple published exploit preconditions.
- PAPPL avoids the CUPS scheduler/backend classes entirely but requires more
  FolioRelay-specific executable integration glue.
- Both candidates use the same immutable build/update process and carry zero
  upstream source patches.
- A future security release or changed profile can invalidate this snapshot.

The final selection should therefore consider reachable code and maintenance
burden, not raw advisory totals.
