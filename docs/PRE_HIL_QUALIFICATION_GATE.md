# FolioRelay pre-HIL release qualification gate

Status: **NOT QUALIFIED — source/fixture qualification in progress**.
Authority: product [#35](https://github.com/SemperSupra/folio-relay/issues/35), managed exports [#50](https://github.com/SemperSupra/folio-relay/issues/50), email [#51](https://github.com/SemperSupra/folio-relay/issues/51), and generic physical lifecycle [private Foundry #291](https://github.com/SemperSupra/truenas-app-foundry-private/issues/291).

## Non-negotiable boundary

The current installed App on TrueNAS 25.04.2.6 is RUNNING but generic H0 is **HOLD / STOP_AND_RECONCILE**; its historical one-use resource and App CREATE attempts are consumed. The protected launcher is pinned to older code and is historical evidence only. No CI result, source merge, site-bound contract edit, or this document authorizes a target repair/redeployment or physical H1-H4.

The pinned CUPS source at `6ba0487abb05afc93d639f37676add8fd65d3756` explicitly enforces `RequestRoot` mode `0710`, whereas the admitted site artifact required `0755`. Proposed single-field site correction is draft [infrastructure PR #280](https://github.com/mark-e-deyoung/infrastructure-as-code-private/pull/280); the original ADMITTED capsule remains untouched.

## Gate matrix

| Gate | Required evidence | State |
|---|---|---|
| S0 source custody | Exact integrated product head, dependency lock, source diff review, no credentials or artifact/target mutation | Partial: draft [#56](https://github.com/SemperSupra/folio-relay/pull/56), unmerged |
| S1 Go and regression | All cross-OS Go test/race, fuzz, lint/static checks and integrated hosted regressions PASS on **the identical combined head** | Pending exact-head hosted evidence |
| S2 managed export | Deterministic validated naming; confined, size/digest-verified atomic exclusive export; collisions; symlinks/races; parallel writers; durable attempt/explicit re-export; restart/rename/delete reconciliation; canonical unchanged | Package tests present; runtime integration pending |
| S3 SMB authorization | Disposable genuine SMB tests with two synthetic named RW accounts plus unrelated denied account, write/read/delete/rename and canonical isolation; evaluate named group/ACL inheritance | New public-hosted Samba workflow in [#56](https://github.com/SemperSupra/folio-relay/pull/56); no real TrueNAS ACL acceptance |
| S4 email | Credential-free post-install profile validation; durable effect/ambiguous delivery; independent SMTP sink with TLS, MIME attachments, XOAUTH2 and rejects; restart/duplicate/unknown behavior | Durable primitives in draft [#55](https://github.com/SemperSupra/folio-relay/pull/55); actual sender/sink/provider onboarding NOT implemented |
| S5 management UI/API | Authenticated printer-wide export defaults, optional per-job setting and re-export; post-install named SMB accounts through approved TrueNAS control; provider profile/test connection with masked credentials; sender/recipient authority | NOT implemented |
| S6 runtime packaging | Build new immutable control/CUPS images, exact source/OCI digests, retained offline artifact, no ad hoc Compose/legacy actuator | NOT DONE; current installed images are older |
| S7 TrueNAS virtual matrix | Fresh independent Foundry F0-F5 plus lifecycle/NOOP and zero-residue on 25.04.1, 25.04.2.6, 25.10.7, and 26.0.0-BETA.3 **for new exact image pair**, including spool mode 0710 and export/email bindings | NOT DONE for this candidate; prior matrix qualifies older images only |
| S8 independent review | Separate architecture/security reviewer signs off exact code/config, no unreviewed check substitution or waiver, source diff and provenance reconciled | NOT DONE |
| P0 new physical H0 | New source/site/capsule authority; read-only independent target observation; separately admitted narrowly scoped reconciliation; correct root/ACL mode and replan NOOP | HOLD; no mutation under consumed grant |
| P1 H1-H4 | Physical native Windows IPP + Apple AirPrint + HTTPS client acceptance | BLOCKED until P0 and all S gates |

A fully tested configuration requires **all S gates** to pass on a single exact immutable candidate before approaching the separately controlled physical gate P0. Hosted synthetic Samba tests and test SMTP sinks reduce risk but do not establish live site ACLs or provider entitlements.

## Execution partition

Parallel source work while H0 is HOLD: export worker/effects [#54](https://github.com/SemperSupra/folio-relay/pull/54); email profiles/outbox [#55](https://github.com/SemperSupra/folio-relay/pull/55); combined integration and hosted named-user SMB [#56](https://github.com/SemperSupra/folio-relay/pull/56); site-contract review [#280](https://github.com/mark-e-deyoung/infrastructure-as-code-private/pull/280). The source PRs are drafts and are not deployed.

Sequential gates: independent source and permission-policy review → merge accepted source to canonical production-candidate branch → finish actual runtime management/config/sender implementations → publish immutable images → fresh hosted target-version virtual matrix → exact site source/artifact/config intake and fresh read-only planning → newly admitted physical H0 reconciliation → physical H1-H4.

## Security limits

No direct SMB access to the protected canonical artifact root, historical attempt ledger, or secrets. Named users are selected **after install** by the operator, with separate filesystem/share ACL proof and denied unrelated account. Secrets must not appear in Git, evidence, log output or argv. Provider onboarding is not a reason to send a live email from CI. Ambiguous external delivery never automatically retries. All noncanonical or runtime-modified resources fail closed.

## Evidence taxonomy

A hosted workflow in progress is **not PASS**. A Go test is evidence for source behavior, not real SMTP provider delivery, site ACL policy, target runtime mode, or physical client UI. A virtual TrueNAS matrix from an older image pair is not transitive to a new OCI image digest. Every accepted verdict must include exact source/OCI/test/artifact hashes, target profile, full pass/fail counts and the smallest outstanding blocker.
