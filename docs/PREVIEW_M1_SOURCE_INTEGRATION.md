# M1 Inbox preview integration boundaries (source-only)

Status: DRAFT, unmerged; not a deployable preview feature. See product #112, Documents #87, AAA #96, existing source snapshot draft #102 and PDF.js synthetic draft #114.

## Client/server contract

The Inbox currently displays an accepted job entry and **Download**, with no preview link. This source-only UI candidate adds a document-title link only if **all** are true:
- The job's media type is exactly `application/pdf` (not raw URF or arbitrary HTML).
- `job_id` matches `job-[0-9a-f]{64}`.
- A **trusted server** explicitly returned `preview: {state:"available",kind:"pdf",contract:"scoped-v1"}` for that exact record/actor.

It deliberately **ignores** `preview.url` and every arbitrary API-provided external location. The browser derives only `/viewer?job_id=<canonical ID>` from the validated ID. The `/viewer` runtime route is NOT mounted today; therefore no existing backend advertises `scoped-v1`, and no new link appears in the currently installed application. Back-end authorization, rather than this cosmetic capability flag, MUST decide before returning document bytes.

A later exact vetted, locally hosted PDF.js viewer must read from an independently authorized and verified `/api/v1/jobs/{job_id}/preview` endpoint. The viewer must never take an external PDF URL parameter, arbitrary filesystem path, token in query, or unbounded/fake status. The accepted original download path remains unchanged.

## Red/blue negative proof

The separate Playwright source test `tests/webui/inbox_preview_contract.spec.mjs` intercepts only synthetic local API responses and verifies:
1. No supported preview capability -> title remains plain text, only Download.
2. Correct scoped PDF capability -> same-origin viewer href.
3. An untrusted external `preview.url` is ignored.
4. HTML injection in the title is displayed only as text.
5. URF, HTML, malformed job IDs, stale or pending contracts cannot surface an inline viewer.

The existing five-browser WebUI/a11y and synthetic PDF.js #114 qualification suites still apply. This branch does NOT add a PDF.js bundle, a mounted Go route, any identity provider, document storage, API write or file conversion.

## Integration holds

P0: qualified per-document issuer/subject AAA #96; #87 SHA/size-verified snapshot before any HTTP preview byte, emergency revocation and resource budgets; correct CSP and MIME; typed unsupported URF. The isolated consumption proof must be independent and test-only until current scopes and site admission are qualified. No earlier physical/CREATE grant is reusable. The next user-visible goal is View/Download, not PDF content editing, metadata sync, full copy-on-write editor or stamps (#113 FUTURE).
