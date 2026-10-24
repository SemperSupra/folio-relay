# Paperless review-grant independent checkpoint — bounded source prototype

Status: **SOURCE-ONLY, DRAFT, NOT ADMITTED FOR DEPLOYMENT** (2026-10-24)  
Authority: [FolioRelay #58](https://github.com/SemperSupra/folio-relay/issues/58).  
Parent source: [PR #77](https://github.com/SemperSupra/folio-relay/pull/77) at `58bd113e8c1567bdbd313e730434ca9fe5c47aa5`.  
This prototype is stacked on that source and must not be merged or deployed separately.

## Objective and observed failure

The existing durable encrypted grant journal survives restarts, authenticates encrypted frames and stores only a digest of the bearer. It cannot by itself detect removal of the **last complete revocation frame**. Earlier source-only tests and review confirmed that a rollback could restore a previously valid, unrevoked read permission. This is a high-impact confidentiality failure if grant access is enabled in a real browser or document system.

The accepted physical FolioRelay Windows/AirPrint **printing** H0–H4 is already COMPLETE (product #35 and Foundry #291 closed). This prototype does **not** reopen, update, or extend the accepted print deployment. Paperless and SMB/email integrations need entirely separate acceptance.

## Source-only implementation

`ReviewGrantCheckpoint` stores an event sequence and a SHA-256 digest chain over the **encrypted** journal frame bytes (domain-separated v1, previous digest, big-endian sequence and frame). It does not contain the plaintext actor, effect, grant hash or credential.

`ReviewGrantAnchor` deliberately has no production implementation. Its contract requires a trust domain and durable storage **independent of the FolioRelay grant journal and the same privileged filesystem owner**:

- `Read(ctx)`: authoritative latest monotonic checkpoint, including provisioned genesis (zero sequence / zero digest);
- `CompareAndAdvance(ctx, before, after)`: durable atomic CAS from the exact previous checkpoint to the next one; no replay or backwards transition; independent authentication.

`OpenAnchoredEncryptedReviewGrantLedger` refuses a missing or mismatched anchor, including orphan historical journal content. On every anchored issuance/revocation, the process checks the authoritative checkpoint, fsyncs a journal event, advances the remote checkpoint and independently reads it back before reporting success. On any ambiguity it **quarantines the running ledger** and denies subsequent lookup, issue or revoke. Every anchored lookup verifies the independent checkpoint again, rather than treating the initial open as perpetual authorization. Future actual actors must not invoke the legacy unanchored constructor.

A deliberately unanchored constructor remains **only** for historical pure-regression and source-only integration fixtures. It is not mounted in `cmd/foliorelayd` and is NOT a supported production permission path.

## Adversarial qualification

Disposable synthetic tests must demonstrate:

1. issue / independent checkpoint / close / restart / revoke / independent checkpoint / close / restart;
2. remove the **whole last revocation frame** (not just corrupt its checksum): reopen is DENIED even though local journal framing would otherwise be well-formed;
3. journal fsync succeeds but the independent CAS outcome is refused/ambiguous: no raw bearer is returned; running instance quarantines; next open refuses unmatched journal;
4. independently held checkpoint missing, unavailable, stale, or inconsistent: initial admission and subsequent read access are denied;
5. never infer an anchor from the document, a model suggestion, browser input, management cookie, local state, or historical one-use physical deployment record.

The in-memory test double proves the source contract only. It is not evidence of any real independent durable monotonic provider, hardened audit trail, or TrueNAS storage isolation.

## Non-goals and unresolved gates

- **No real checkpoint backend, trust provisioning, external persistent CAS durability test, or key rotation exists in this prototype.** Do not deploy a test fake or write a checkpoint into the same ZFS dataset/root as the grant journal.
- Crash between local fsync and remote CAS is a **reconciliation HOLD**, not permission to truncate, adopt a journal tail, mint another grant, or silently retry; independently verified operator intervention is required. A revoke that has not reached the independent anchor must not be represented as completed.
- Identity, archived-effect ownership/visibility and record-level actor policy are not yet proved. Grant possession alone is insufficient to establish identity issuance policy.
- No HTTP issuer, email delivery, external DMS metadata PATCH, real document ingestion, model egress, browser runtime registration, or automatic filing.
- No physical/virtual TrueNAS qualification, Windows ACL qualification, human assistive-technology acceptance, trusted anchor backup/restore, DLE trust root, or production authorization in this slice.
- English/German/Thai and mixed document semantics remain required. Thai native classification remains unqualified; no automatic filing.

## Next gated work / handoff

Discover → Observe → Plan → Apply → Verify:

1. Run and inspect exact-head public hosted Go security, fuzz, integration and disposable Paperless tests for this stacked draft; preserve first failures and fixes.
2. Independent security review: checkpoint MAC/domain evolution, CAS ambiguity, restart/read availability, DoS via external anchor, lock/actor lifecycle and unauthorized unanchored constructor reuse.
3. Qualify a genuinely independent monotonic CAS authority in a disposable environment, with adversarial restart, stale replica, rollback and snapshot restoration tests. Do not infer production protection from a single process mock.
4. Bind operator identity to archived effect and authorized reviewer with a positive, independently testable policy; keep source-only.
5. Perform disposable browser + HTTPS UI accessibility acceptance, EN/DE/TH semantic quality qualification, then exact target F0–F5 before requesting a new owner-approved physical deployment capsule.

All changes in this branch are additive **source and synthetic tests only**. Preserve the proven printing MVP and its consumed CREATE ledgers.
