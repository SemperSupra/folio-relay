# Paperless archived-effect review authorization — source-only policy boundary

Status: **DRAFT / NO PHYSICAL OR PRODUCTION AUTHORITY** (2026-10-24)  
Governed by [product filing DLE #58](https://github.com/SemperSupra/folio-relay/issues/58) and [post-MVP program #79](https://github.com/SemperSupra/folio-relay/issues/79). Stacked on source-only [PR #78](https://github.com/SemperSupra/folio-relay/pull/78), which itself stacks on #77.

## Discover / Observe

The previously source-qualified grant issuer in #77/78 relied on an externally injected `ReviewGrantDecision` and had tests for token, expiry, encryption, revocation and journal rollback. That did **not** show that an actual intended reviewer was entitled to read a particular archived record. Any callback that permitted syntactically valid actor/effect values could issue a bearer, despite the filing effect being unarchived or for the wrong DMS.

The `Outbox` is the product's durable source of `approved → submitting → accepted → archived` for a target/document; it does not contain an independently authenticated identity/ACL directory. A cryptographically valid grant is not evidence that an actor owns or may review the remote document.

## Source-only Apply

`NewArchivedGrantDecision(outbox, targetID, oracle)` provides a **non-networking, fail-closed wrapper** for `ReviewGrantDecision`:

1. Validate action, actor, operator and SHA-shaped effect ID.
2. Require a present durable filing effect with `State=archived`, a positive native `DocumentID`, matching task UUID, validated original intent, exact target binding, and recomputed effect ID.
3. Pass an immutable `ReviewAccessSubject` containing actor, authenticated-operator **claims**, action, effect, target, canonical digest, policy version, approval ID, task UUID and archived document ID to a **mandatory independent provider**.
4. Require that provider to return an exact subject echo, with all of `ActorIdentityVerified`, `OperatorIdentityVerified`, `RecordPermissionVerified`, and `Allowed` true. Errors, cancellations, substitution, missing/unknown approval or unavailable provider **deny**.
5. The resulting decision can be passed directly to the existing anchored encrypted ledger from PR #78. A bearer still cannot be issued unless both this scoped policy and the external monotonic journal checkpoint succeed.

`ApprovalID` is an input to policy cross-check, **not** proof of authorization. The `ReviewAccessVerdict` is only trustworthy if its caller injects a genuinely authenticated, independently administered actor directory and per-document ACL/ownership resolver. **No such provider is implemented here.** An in-process test callback returning true does not qualify real identity nor an owner. Documents' text/OCR/AI can never provision identities, operator roles, targets or ACL.

Both `GrantIssue` and `GrantRevoke` require a positive current policy decision. A missing archived receipt or external authority may block ordinary revocation; production emergency revocation/disable must have a separately qualified fail-closed global path, rather than bypassing this check. This is still a prototype, not the final operator model.

## Red-team / synthetic-only verification

Tests exercise positive subject binding, missing/wrong/unarchived effect and target (zero oracle calls), malformed actor/action/effect, actor swap and issuer swap, missing each positive authorization bit, and substitution of task/document/approval/digest/target/actor/effect/action in a forged verdict. An anchored ledger integration test requires independently checkpointed issuance, rejects cross-actor minting, survives restart, refuses wrong issuer's revoke, and records the valid revoke as a separate checkpoint.

No test uses a real document, production operator, browser login, TrueNAS storage, model endpoint, SMTP/SMB service, or Paperless metadata PATCH.

## Deferred qualification / next executable work

- Qualify a **real distinct identity and document ownership/ACL authority** using authenticated administrative policy, provenance of the archived effect's `ApprovalID`, immutable target-bound archived receipt, and denial for revoked/moved records. Define privacy and audit semantics.
- Qualify a durable external monotonic checkpoint provider with independent credentials, replay/restore resistance, unavailable-provider behavior, key rotation and recovery; the #78 test fake is not one.
- Evaluate whether revocation should be independently accepted even when the archive becomes unavailable, without broadening grant issuance. Preserve explicit emergency quarantine and deny-by-default semantics.
- Actual local HTTPS Chromium/Firefox keyboard/screen-reader EN/DE/TH review UX; language-specific Thai classification accuracy. No automatic filing or FolioRelay-originated metadata PATCH.
- Source/security review before merging this stack; then new immutable builds, virtual exact-version F0–F5 and separately approved physical admission. Historical #35/#291 physical printing PASS is a baseline **only**, never filing deployment approval.

Use `Discover → Observe → Plan → Apply → Verify` and preserve first failures, exact head, CI receipts and handoff in #58 and #79. No paid resources or physical target mutations.
