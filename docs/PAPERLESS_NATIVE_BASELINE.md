# Paperless-ngx native filing baseline — source-only qualification

**Status: PROTOTYPE / NOT DEPLOYED / NOT APPROVED FOR HIL.** Product ticket: [#58](https://github.com/SemperSupra/folio-relay/issues/58). Existing physical H0 \`STOP_AND_RECONCILE\` and one-use CREATE holds in Foundry #291 are unchanged.

## The bet

Use the **official TrueNAS Community Paperless-ngx App**, currently catalog 3.1.3, and **its native OCR, deterministic matching, trained classifier and LLM suggestion workflow**. Optional inference uses locally operated Ollama via Paperless configuration. FolioRelay does **not** run its own LLM, choose filesystem paths, create uncontrolled metadata, or replace the records archive. The immutable FolioRelay canonical artifact and Inbox remain independent of the DMS.

Paperless implementation/release evidence at tag v3.1.3:
- TrueNAS catalog: https://apps.truenas.com/catalog/paperless-ngx/ (3.1.3 Community; NOT installed by this branch)
- Paperless release: https://github.com/paperless-ngx/paperless-ngx/releases/tag/v3.1.3 (AI workflow action present from v3.1.0)
- Paperless REST API: https://docs.paperless-ngx.com/api/
- Exact 3.1.3 REST task tests: https://github.com/paperless-ngx/paperless-ngx/blob/v3.1.3/src/documents/tests/test_api_tasks.py (API version 10, paginated \`results\`, status/related ID)
- A catalog app may lag upstream; do not assume latest upstream fixes/configuration exist in 3.1.3.

## Implemented connector seam

\`internal/filing/paperless\` is Go stdlib + existing FolioRelay state journal. No production router hook, UI, credentials, mounts, new App config, network job or background execution.

- An **explicitly configured, single-origin HTTPS client**, token only in memory; no redirects, no remote URLs from documents, and HTTP only for disposable loopback fixture tests.
- Verify a bounded immutable PDF/image digest. Native Paperless receives only \`document\` via \`POST /api/documents/post_document/\`, not a predecided title, tag or destination. Paperless itself classifies.
- Parse the returned asynchronous task UUID, \`GET /api/tasks/?task_id=<uuid>\` with \`Accept: application/json; version=10\`; distinguish pending, archived, duplicate, rejected and unknown.
- \`GET /api/documents/<id>/\` read-only retrieves Paperless' own title, type, correspondent, storage path, tags and date as evidence of suggestion/filing quality.
- Mandatory explicit \`ApprovalID\`, \`TargetID\` and \`PolicyVersion\`, unique canonical document digest. The separate fsynced \`Outbox\` journal reserves the action **before** any external POST. Replayed action IDs cannot submit twice. Crash after reservation but before a task UUID => \`destination_unknown\`; it is **never automatically retried**. Once task UUID is durably recorded, read-only observation can resume and a terminal result can be recorded without resending.
- No UI capability exists yet to authorize filing, expose connectors or reconcile an unknown outcome. Any application binding must be separately implemented/reviewed.

## Earn-its-keep experiment and thresholds

Run synthetic documents only, with explicit frozen ground-truth:
- 20+ PDFs/images spanning plain text, two languages, invoices, contracts, correspondence, tables, page scans, OCR defects, and repeated documents;
- metadata ground truth: title/subject, correspondent, document type, date, tags, storage path; score field exactness separately;
- control A: native deterministic rules; B: trained Paperless classifier; C: optional native Paperless LLM suggestions with local Ollama; **no custom model pipeline** unless C demonstrably beats A/B;
- decision quality: micro/macro precision, recall, abstention rate, false-route rate, human corrections per document, evidence coverage, classification latency and GPU/CPU memory/time;
- minimum MVP acceptance target (subject to review): 0 wrong high-confidence automatic destinations on the holdout; at least 90% accuracy for proposed document type on a representative held-out set; human review for all ambiguous/high-impact records;
- inspect model instructions injection resilience and privacy (document text is untrusted, model cannot write tags/files/ACLs autonomously).

The installed catalog configuration is **not validated** merely by an HTTP mock. To qualify native AI and earn its keep, use a fresh **disposable public free-tier runner** with pinned Paperless 3.1.3 + synthetic Redis/storage and a locally substituted/dummy LLM inference endpoint (or Ollama CPU fixture). Capture image digest and full test metrics. No actual TrueNAS access, provider accounts, user documents or real model egress.

## Gates to any opt-in physical deployment

1. All exact-head cross-platform Go tests, fuzz, security/integration workflows and failure-path tests PASS.
2. Disposable actual Paperless 3.1.3 end-to-end upload → async task → native metadata readback, duplicate → distinct terminal result and restart/unknown safety PASS; native classifier/LLM comparison documented.
3. Independent review of all classifier control effects, privacy boundaries, source/data retention and network pinning.
4. Build a distinct **opt-in** native App/connector integration with operator configuration, durable credentials, approved target TLS, health and review UX; no default-enabled network or LLM.
5. Separate artifact/OCI digest qualification and renewed Foundry F0-F5 version matrix; no reinterpretation of generic FolioRelay physical H0.
6. Only then: separately authorized physical lifecycle. Existing H0 HOLD remains live throughout.

**Negative spaces**: native Paperless AI is not a guarantee of accurate metadata. API v10/3.1.3 compatibility must be proven, not extrapolated from current docs. Document submissions after network interruption must not be replayed based solely on missing task readback. Do not assign arbitrary tags/storage paths from LLM strings or share canonical artifacts directly as a Paperless consume directory.


## Qualification finding — repeated identical upload

The initial real Paperless-ngx 3.1.3 CI run accepted, processed and archived the first synthetic PDF, but a second explicit raw POST of identical bytes produced a **different archived document ID**, not a `duplicate` verdict. This is an observed contract result; the exact product-side cause (version handling versus dedup configuration) remains to be investigated independently. Do not rely on a DMS duplicate flag to enforce at-most-once filing.

FolioRelay now keys its durable filing effect by immutable canonical content SHA-256, approved Paperless destination and policy version, **excluding the ApprovalID** from the effect identity. A repeated approval with the same content/destination/policy resolves idempotently to the **original** effect and retains its first authorized ApprovalID; it cannot create or send a second effect. An archived effect cannot be re-sent after restart. The original approval ID is preserved in durable metadata for evidence. A genuinely new destination or policy revision creates another effect only through a separate admitted action; its remote document correlation requires independent qualification.

The first real post-upload test is no longer a destructive/raw duplicate probe; it now verifies pre-network duplicate suppression through the journal across restart. The original observed second document on the disposable runner is recorded as an unresolved DMS dedup limitation, not silently reinterpreted as a PASS.
