# Documents / Inbox: canonical artifact integrity verifier — source-only

Status: DRAFT, NOT DEPLOYED (2026-10-25). Authority: [FolioRelay #87](https://github.com/SemperSupra/folio-relay/issues/87), source design constrained by accepted [MVP #35](https://github.com/SemperSupra/folio-relay/issues/35).

## Existing state and newly bounded behavior

FolioRelay's current Inbox is a durable projection of accepted jobs and content hashes, **not a directory scan**. External filesystem changes can leave an accepted record whose file is missing, altered or non-regular. The current artifact-download endpoint does not validate the bytes against the canonical SHA-256. Do not silently treat a 200 response as evidence of original content integrity.

This source-only change adds `inbox.VerifyCanonicalArtifact(rootDir, durableRecord, checkedAt)`, producing machine-readable `IntegrityReceipt`: stable job ID, `available`, `missing`, `corrupt` or `verification_pending`, checked-at timestamp and stable reason code. No file paths, user content, English error details or credentials enter the receipt.

- Verify lowercase 64-digit SHA identity, committed `accepted` status, nonnegative bounded size (maximum 100 MiB).
- Use Go `os.OpenRoot` to confine path traversal, `Lstat` for regular-file admission, `os.SameFile` after opening to detect pathname swaps, streaming SHA-256 limited to expected length+1, exact size/digest equality.
- Fail closed on nonregular sources and hash/length divergence; report `verification_pending` rather than falsely claiming corruption after unavailable root/I/O/invalid check context.
- Preserve the historical record on deletion or damage. Do not adopt orphan files. Do not patch original content, fix permissions, delete SMB/cloud copies, or silently recreate a remote destination.
- A receipt is a **point-in-time observation** and can become stale; any outward delivery must reverify immediately against the currently pinned artifact handle and record. Remote SMB/cloud/email status is independently reconciled.

## Synthetic first qualification

Tests cover intact synthetic PDF; missing file, truncated file and same-size modified bytes; symlink/unsafe source, invalid SHA, nonaccepted record, orphan file and oversized declared source; root outage and invalid verification context. These unit tests intentionally do **not** mutate the current public API, the downloaded bytes, installed datasets or any real records.

## Next READY work

1. Exact-head hosted Ubuntu/macOS/Windows Go, race and fuzz, integrated runtime (first failure/receipt recorded in #87).
2. Independent security review: TOCTOU under concurrent writer, root/path identity, I/O/permission distinctions, bounded traversal; preserve artifact store confinement and no logger leak.
3. A stacked source-only slice adds authenticated `GET /api/v1/jobs/{id}/integrity` with the same stable receipt and `Cache-Control: no-store`, leaving the existing job response unchanged. Only one verification runs per server instance at a time; excess concurrent requests receive `429` and `Retry-After: 2`. It is an on-demand observation, not an automatic sweep or permission to download. Gate downloads and all outward sends **only after** atomic stream/verify/TOCTOU design and throughput tested; do not convert hash checking on a private dataset into a persistent mutator.
4. Plan bounded background sweep with resume/cursor, sampling/time budgets and no automatic document deletion; separate SMB/cloud effect reconciliation with freshness labels. Translate human copy through #85 rather than return localized machine codes.
5. Requalify new immutable images, actual target F0–F5, browser and assistive UX; new owner-reviewed site admission required before any physical update. Accepted HIL #35 and Foundry #291 remain exact-image proofs, and prior CREATE authority remains consumed.

No egress, model inference, Paperless PATCH, provider credentials, SMB grant, physical TrueNAS/App mutation or background automation is enabled by this code.
