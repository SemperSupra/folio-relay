# Canonical verified-snapshot download — source-only qualification

**Authority:** [FolioRelay Documents #87](https://github.com/SemperSupra/folio-relay/issues/87); stacked source branch on [integrity observation draft #93](https://github.com/SemperSupra/folio-relay/pull/93) → #90. Historical native Windows IPP + AirPrint H0–H4 #35 / Foundry #291 remain accepted only on pinned old installed images. No new physical grant.

## Red-team defect and narrow correction

The existing authenticated `GET /api/v1/jobs/{job_id}/artifact` opened the hash-named file, checked only its file mode, immediately emitted HTTP 200 and streamed bytes. Deleting, truncating or changing an externally mutable file could release unverified bytes while retaining a valid durable Inbox record. Calling `GET .../integrity` first is insufficient: the recorded observation can become stale between two requests.

The candidate replaces the streaming path with `inbox.SnapshotVerifiedArtifact(ctx, root, durableRecord, time)`:

1. Preserve the accepted-job record and validate its lowercase 64-byte SHA-256, expected size (0–100 MiB) and accepted state. Reject nonregular and unsafe canonical references. Use Go `os.OpenRoot` + `Lstat` and compare opened file identity.
2. Copy **at most expected size + 1 byte** from the protected canonical source into a fresh 0700 private temporary directory, with exclusive 0600 snapshot, **under the protected artifact root**, because the pinned control image is `scratch` and does not necessarily include `/tmp`. The directory is not under the SMB/group-writable export root. Hash the exact bytes written to scratch in a single bounded copy, compare count and digest with durable identity; cancellation/errors delete the scratch snapshot without returning a reader.
3. Return the private snapshot for HTTP response only after the entire digest, length, and scratch write/rewind checks pass. The response has `Cache-Control: no-store`, original sanitized attachment filename and content type, explicit `Accept-Ranges: none`. A missing original returns stable 404; corrupted source returns stable 409; verification/storage failure 503, and shared one-at-a-time checksum slot returns bounded 429/Retry-After. Never place source file paths or content in error receipts.
4. The snapshot `Close` deletes its ephemeral scratch directory. The accepted job and canonical source are never modified, removed or readopted. An orphan or a remote delivery copy is not a substitute source. A mutating concurrent writer cannot cause **unchecked** snapshot bytes to be returned; any full-snapshot contents must match the original SHA-256 to succeed.

## Limits / non-effects

- This is source-only and still uses the legacy *shared management session* for HTTP authentication; it does **not** satisfy scoped owner/ACL, TrueNAS ordinary-user OIDC, account offboarding or agent actuator admission in #96. Do not expose as an ordinary user delivery API until #96 is separately qualified.
- Private scratch uses the canonical protected dataset's capacity and disk IO; it must not be on a shared SMB writable path. The existing per-server integrity slot caps in-process concurrent checksum/snapshot work to one, but it is not a cluster-wide quota or disk free-space reservation. Shared-account/UID hostile processes and ZFS quotas need independent exact-target testing. The file is cleaned by `Close`, not securely erased at the block layer. A process crash can leave a private temporary directory: implement and test bounded restart janitor only with exact namespace ownership.
- On Windows, 0700/0600 POSIX checks are not equivalent to an ACL proof; separate ACL qualifier and target support gate. No physical proof follows from portable CI.
- HTTP transport can fail or a client can disconnect **after** it begins receiving verified bytes; a 200 is not guaranteed remote delivery, and the journal must not record a new durable delivery effect based on the response. For real external sends, preserve destination-specific at-most-once/uncertain semantics.
- The prior `GET .../integrity` remains a point-in-time read-only observation and should never be advertised as authorizing an individual read.
- Browser user journeys need EN/DE/TH accessible corrupt/missing/pending/download-retry messaging, progress and provenance; automation and agents use stable codes and must not silently auto-repair, reimport or resend. The installed App remains unchanged.

## Qualification / next baton

Synthetic source tests must prove valid byte equality even if the canonical source is modified/deleted *after* verification; missing source, equal-sized malicious modification, truncation, symlink/traversal attempt, oversized declared source, disabled root, cancelled context, no staging survivors after `Close`, HTTP 401, 409/404/429/503 without leaking any unverified body bytes. The existing integrated Go/browser/management TLS/Control runtime and fuzz suites must qualify the **exact PR head**.

Next independent red-team review: adversarial concurrent rename/rewrite/truncate during snapshot, scratch namespace/permissions/capacity and crash recovery, per-actor authorization and frontend disabled/error states. Then compose this exact source with #89→#91→#97 (WebUI) and #95 (Documents lifecycle) on a disposable integration branch; no blind merge, no site action, no reused consumed physical CREATE. New immutable images, virtual TrueNAS F0–F5, and new owner-reviewed physical admission are independent later gates.
