# Cross-lane FolioRelay composition — disposable source-only qualification

Authority: program DLE #79, Documents #87/#94, internationalization #85, identity #96. This branch deliberately **does not merge** any feature PR, change deployed images, or grant actor/site authority.

## Source identities (unaltered candidate components)

- Base from draft #98 @ `4889449d7cca8799dd163a46ff30adb019743350` (including canonical verifier #90 + read-only status API #93 + verified-before-exposure snapshot handler #98).
- Locale catalog/portal/accessibility from draft #89→#91→#97, final source head `ace6f3103ec790df4ca53946e2e12639fa87b9c5`. The Go embedded locale catalog, JSON dictionaries, browser bundle, Playwright test, control-image Dockerfile and locale handler/route are composed into the base. The handler is inserted at `/locales/{locale}`; the authenticated `/api/v1/jobs/{job_id}/integrity` and guarded `/artifact` routes remain.
- Durable reversible Documents service from draft #95 @ `d362256b48da6fec5bf461381a2641a43c27cee9`, exact service source and tests. **No mutation HTTP route or per-user actor bridge** is added.

## Integration falsification oracles

1. Five-profile Playwright/axe: login, EN/DE/TH catalogue, offline fallback, delayed/out-of-order locale changes and stable printer/diagnostics; no extra self-test or outbound action.
2. Go Security Kernel race/vet/fuzz across Windows/macOS/Linux and `internal/documents` service tests; controls for exact actor issuer+subject, stale revisions, idempotent replay and journal immutability.
3. The same authenticated control server returns locale catalogs and integrity status, and serves an artifact *only* after full private-snapshot hash verification. Corrupt/missing source denies with 409/404 before any unverified body bytes.
4. Immutable builder contract: pinned scratch control Dockerfile explicitly includes the new `internal/i18n` package, compiles all needed code in a sandboxed source stage. Integrated runtime and management TLS remain green at **this exact composite commit**.
5. No phantom mutation: human browser, automation read-only API and future agents are not granted identity/ownership by this source integration. Doc metadata/archive service remains transport-inert pending #96. No email, cloud, SMB, Paperless, provider or physical effects.

## Source custody / qualification limits

This disposable combined branch is **evidence only**, not an approved merged dependency DAG. All individual feature PRs remain DRAFT and owned by their own DLEs. No independent security/linguistic/accessibility reviews are implied. Human-verified German/Thai, screenreader witnesses, identity #96, per-record ownership/ACL, scratch storage quotas/crash scavenging, unsafe-provider no-clobber, site target and exact-image TrueNAS F0–F5 remain hard gates.

**Next actor baton:** On a green exact-head CI, record status `SOURCE_COMPOSITION_TESTED` only. If any first failure, preserve its exact job/log and fix the smallest source discrepancy without bypassing defenses. Later independent review may select minimal commit/stack composition; no automatic merge or source promotion.
