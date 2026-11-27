# Canonical verified byte snapshots — in-memory hardening (source-only)

**Authority:** FolioRelay Documents #87, AAA #96, and program #79. Source predecessor: draft #98, then disposable composed source #99. This corrective branch remains a draft without merge or runtime admission.

## RED / finding

The previously qualified draft #98 copied the canonical artifact into a private filesystem scratch directory, hashed that file's incoming bytes, and then streamed the scratch file. Filesystem permissions alone do not protect against an unrelated process running as the **same UID**: such a peer may potentially find and modify the scratch file between hash acceptance and the later HTTP read. The installed TrueNAS target's user/ACL separation has not been independently proven. A 100 MiB disk scratch also introduced capacity exhaustion, crash orphan cleanup, and dependence on a writable scratch dataset.

A point-in-time hash of a *mutable path* is not an immutable byte-delivery guarantee, even if the hash itself was valid.

## BLUE / corrective implementation

`inbox.SnapshotVerifiedArtifact` now creates a bounded **in-process memory snapshot**, with no scratch pathname or directory. It opens only the canonical source within Go `os.OpenRoot`, rejects nonregular files and inode substitutions, and copies exactly the durable accepted length into a privately owned byte buffer capped at **100 MiB**. It reads one additional source byte to detect growth, verifies SHA-256 of the **same bytes held for eventual delivery**, checks cancellation and exposes `io.Reader` only after the full digest passes. Failures return no snapshot and stable reason codes; the original durable job/source are unchanged. `Close` clears the data buffer and makes further reads fail closed.

The authenticated HTTP handler continues to use the shared one-at-a-time checksum slot for both integrity observations and downloads, preventing unbounded concurrent large allocations **per server instance**, with a separate process-wide one-active snapshot memory reservation held until Close. Process-wide contention returns snapshot_memory_busy / HTTP 429. It issues HTTP 200 only after verification; missing/corrupt/unavailable/busy paths remain 404/409/503/429, with JSON errors and no unverified artifact body. No user/agent actor authority, delivery effect, canonical mutation, or provider egress was added.

## Qualification matrix and bounded limitations

Run pinned Go test/race/vet/fuzz on Ubuntu/macOS/Windows, control amd64/arm64 scratch-image build, HTTPS/TLS, DNS-SD, integrated runtime and five-profile browser+Lighthouse against a single exact source commit. Restore the missing Go i18n parity/fallback unit tests in the composite. Synthetic tests require preserved verified bytes if the canonical pathname is later modified/deleted, no scratch entries, unavailable/corrupt/too-large/missing/zero-length fixtures and closed-reader denials. Browser failure/recovery still needs fully localized EN/DE/TH acceptance.

**Residual gates:**
- At most **100 MiB per Go process** of buffered document content plus baseline memory/GC overhead. Neither the HTTP semaphore nor the memory reservation bounds concurrent separate processes. Qualify cgroup/container/TrueNAS memory reservation and deliberate 100 MiB faults; a Go process OOM is not handled as an ordinary HTTP 503. Ensure no uncontrolled multi-instance fanout.
- A malicious actor with arbitrary process-memory read/write or control execution still defeats in-process separation; this change only closes *filesystem same-UID scratch mutation*, not process compromise. Qualify runtime isolation and file ownership.
- Source file may change during read. No source byte is served unless the complete held copy matches the accepted hash; stale integrity status is never authority to download.
- The temporary document bytes exist in application RAM until `Close` and GC reuse. Clearing the buffer is best-effort application hygiene, not a formal memory-forensics guarantee.
- Both private HTTP and management HTTPS control listeners now have a 5-minute absolute write timeout (from request headers). This bounds how long a slow/stalled response can retain a verified memory snapshot at the transport layer. It can terminate a slow legitimate large download; clients must treat interrupted transfers as incomplete and retry by policy, not as a newly delivered document. Test exact target/proxy and HTTP/2 timeout semantics before deployment.
- Client cancellation or network failure can still leave a partially received *verified* response; HTTP 200 is not proof of successful remote delivery.
- Shared management token is NOT an ordinary user's record authorization. TrueNAS local/SMB user/OIDC path, object ACL/revocation, native screen-reader/linguistic review, Provider adapters and export effects remain gated on #96 and their owners.
- Original physical TrueNAS H0–H4 and closed Foundry #291 apply only to historic pinned installed digests; never reuse consumed physical CREATE. A new reviewed immutable image, virtual F0–F5 and fresh site owner physical admission are required before any deployment.

## Next independent reviewer baton

Review this source-only alternative against its predecessor #98 for resource safety, same-UID filesystem threats, zero/100 MiB sizes, cancellation races, process memory limits and per-server vs multi-instance concurrency. Preserve exact first failures/CI receipts. Continue Go-first, $0 hosted source qualification, no cross-stack merge until owner-review and separate deployment/identity gates.

## M1 read-scope parity source experiment (draft #123, not runtime admission)

This document previously described the original in-memory-custody implementation.
It did **not** mean that a shared management session authenticated the individual
owner of a document. As of this unmerged source experiment, the **registered**
`Server.Handler` still sends `GET /api/v1/jobs/{job_id}/artifact` through
`s.requireAuth` (management Bearer token or management cookie). It is **not**
equivalent to a human user's independently authenticated issuer+subject,
per-record read authorization or globally authoritative revocation/STOP.

The unregistered `internal/control/preview_gate.go` now provides two *separate*
proposed constructors, `NewScopedPDFPreviewHandler` and
`NewScopedOriginalDownloadHandler`, sharing the same verified snapshot,
five-minute request context, current-record and emergency-stop checks, and
16 KiB per-send recheck. Their caller-supplied policy receives a separate
`ScopedArtifactPreview` or `ScopedArtifactOriginal` purpose: granting
inline preview cannot automatically grant an original attachment download.
Neither handler is installed into `Server.Handler`; the older
`NewPDFPreviewHandler` remains a preview-only compatibility test wrapper and
cannot authorize originals. No identity provider or STOP service is shipped.

The original-download source experiment also rejects noncanonical media
metadata and dangerous filenames (path separators, invalid UTF-8, control
characters and excessive length). `mime.FormatMediaType` produces a standards-
encoded `Content-Disposition: attachment` for quotes and multilingual names.
It sends no canonical bytes before a completed digest/length snapshot and
purpose/STOP rechecks. A failed midstream check or short writer results in
`http.ErrAbortHandler`, **not** an assertion that an already-issued HTTP 200
was fully delivered. See `internal/control/scoped_original_read_contract_test.go`
for synthetic actor, deny/outage, cross-purpose, quoted/Thai filename,
content-type and partial-transfer counterexamples.

**Nonnegotiable remaining acceptance:**
- A separately verified user/workload principal with issuer, subject, audience,
  token expiry and binding to the intended document, not the shared management
  session or a caller's asserted fields.
- A single independently authoritative and current per-record policy/STOP
  source for **both** installed preview and installed original-download routes,
  with fail-closed authority outage, bounded distributed revocation latency,
  cheap locally enforceable lease and fresh decision at each delivery chunk.
- Honest typed transfer outcome/bytes sent at server, proxy and consumer
  boundary, and client-verified complete length and digest. An abort cannot
  recall previously sent bytes; a successful write does not prove receipt.
- A reconciled client/server artifact limit (current synthetic browser 10 MiB
  versus Go custody 100 MiB), proven memory/cgroup ceilings, backpressure and
  resource release under blocked network writes.
- Self-hosted pinned PDF.js worker/assets, SBOM/LICENSE/CSP, actual EN/DE/TH
  assistive testing, adversarial PDF/URF input, virtual TrueNAS F0–F5, and a
  **new physical owner grant** before replacing accepted H0–H4 printing images.

This is a scoped-source CI milestone, **not** a reason to merge the draft stack,
close #87/#96/#112, enable Paperless remote PATCH, or mutate the live site.
