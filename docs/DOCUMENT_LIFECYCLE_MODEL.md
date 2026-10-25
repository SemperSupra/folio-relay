# Document lifecycle command model

Status: source-only draft for [#94](https://github.com/SemperSupra/folio-relay/issues/94), stacked on canonical integrity drafts #90 and #93. No user-facing mutating API or UI and no live App change.

`internal/documents.Service` accepts only jobs already present in the durable accepted Inbox projection. It maintains a separate `document/<job-id>` aggregate for user annotations and Inbox review state. Source title, intake identity, acceptance time and canonical artifact digest/size remain in the original accepted record. A metadata action replaces only display label, tags and note. Archive and Restore change whether the document needs Inbox attention; neither removes the document, its journal history, its canonical bytes or any delivery copy.

Every read and change requires an injected `Authorizer` and a separately authenticated principal of kind `human`, `automation` or `agent`. The shared management token is not a document-owner identity, so this package is deliberately not wired into the current HTTP server. The authorizer must use actual per-document ownership/access policy before a transport can expose commands. No allow-all production policy is supplied.

Every mutating command supplies a stable action ID and expected revision. The durable state engine serializes by document aggregate, rejects stale revisions and changed idempotency replays, and returns the original receipt on exact replay even after later changes. The journal records actor, action, original artifact digest and before/after annotations. No effect intent is emitted, so annotation changes do not resend mail, cloud, export or filing deliveries. The package projects the latest revision after restart.

Local synthetic tests cover admitted versus forged jobs, owner allow/deny, full metadata edit, archive, restore, multilingual label/tags, audit payload, exact replay after later revisions and restart, altered replay rejection, stale revision, 32 concurrent claimants with one winner, and no journal write for denied/invalid/no-op actions.

Next gates: independent ownership and actor credential model; scoped read list/cursor API and typed mutation transport; accessible EN/DE/TH UI and equivalent automation/agent commands; integrity and delivery observations; target profile qualification. Retention-governed canonical purge is a separate, higher-risk action and is absent from this model.
