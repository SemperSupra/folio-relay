# FolioRelay core portal EN/DE/TH presentation — draft only

Date: 2026-10-25. Product locale authority [#85](https://github.com/SemperSupra/folio-relay/issues/85); parallel integration [#79](https://github.com/SemperSupra/folio-relay/issues/79). Stacked on [source-only catalog PR #89](https://github.com/SemperSupra/folio-relay/pull/89).

## Applied design

- The existing FolioRelay web UI now has one visible, labeled 44-pixel locale selector (English, Deutsch, ไทย) in the management shell, including the unauthenticated login view. The initial preference follows `navigator.languages`; explicit selection changes only the in-memory view. **No local storage** is used and no claim of cross-device/per-actor persistence is made while FolioRelay has only a shared management credential.
- Server exposes only `GET /locales/en|de|th` for versioned, embedded text-only catalogs. All other locale paths fail 404. No document data, user access or provider secrets are sent by this endpoint.
- Static labels and dynamic overview, Inbox, printer details, authenticated login error, and diagnostic messages use language-independent message keys. The browser changes `<html lang>`, renders translations using `textContent`, formats dates/numbers with built-in `Intl`, and never treats catalog/interpolated data as markup.
- Server diagnostic `code` and `status` values remain unmodified; the client translates the known codes and their recovery actions, showing a generic code-only fallback for unknown checks. Technical detail from the preexisting server is labeled, not used for access decisions. Error APIs remain machine-code-driven and do not expose raw server error prose as localization.
- Switching languages rerenders cached status and diagnostics **without** invoking `POST /api/v1/self-test` a second time or performing any document/print/send mutation. Changing language does not change authentication or canonical paths, job IDs, MIME or exported filenames.
- Accessibility: named locale selector, translated management labels, `html.lang`, a responsive header, minimum 44-pixel target and Thai/German wrapping. Existing CLI/API/agent DX remains one stable code authority.

## Test and admission gates

Five-profile Playwright fixture (Chromium/Firefox/WebKit/Android/iPhone) now exercises en/de/th catalogs, bad login in German and Thai, Thai overview/inbox/printer/diagnostics, German switch without a second diagnostics POST, authentication/session preservation, axe-core accessibility, locale selector size and narrow-screen overflow.

The test environment uses disposable synthetic data and a public free-tier runner; no live TrueNAS site, real document, real email, provider identity or Paperless integration.

**This source-only PR does not yet qualify the full system as internationally supported.** Full human review of Thai/German copy, all error/recovery states (including rare failure injections), detailed screen-reader and locale/typography QA, localization of filing, email, SMB, cloud and Documents/Deliveries journeys, and new immutable runtime/physical-site qualification remain outstanding. No current installed image or previous HIL grant may be reused as acceptance for this UI.

Keep the stacked PR **DRAFT** pending exact-head browser Playwright and security/integrated tests. Record first failures, receipts and next handoff in #85 and #79.
