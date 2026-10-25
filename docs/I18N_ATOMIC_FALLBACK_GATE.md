# FolioRelay atomic locale switching and offline fallback — source-only

**Authority:** [product language DLE #85](https://github.com/SemperSupra/folio-relay/issues/85), [program DLE #79](https://github.com/SemperSupra/folio-relay/issues/79); stacked after [localized core UI draft #91](https://github.com/SemperSupra/folio-relay/pull/91). The accepted physical TrueNAS virtual-printer installation does not include this code.

## RED: reproducible negative conditions in previous source

1. When an initial browser language catalog fetch failed, the state retained `messages={}`. Subsequent dynamic messages rendered *raw keys* such as `overview.ready` while the markup appeared partially English.
2. Concurrent language changes raced: a slow earlier `/locales/th` fetch could commit after a later English or German choice, replacing the user's intended language and `html.lang`.
3. The browser accepted any response containing just `login.title` and `diagnostics.next_action`; omission or interpolation-placeholder drift in other keys could render incomplete instructions or blank values.
4. Language changes unnecessarily re-fetched Inbox jobs, creating additional network traffic and a new nonlocal failure point. The diagnostic POST was not repeated, but the read-model replay was avoidable.
5. There was no explicit polite live announcement for a changed language. Automated axe/Lighthouse passing did not establish a screen-reader announcement.

## BLUE: bounded correction

- Embed the canonical **complete candidate English UI dictionary** in `app.js` as immutable fallback. The Go `internal/i18n/catalog/en.json` remains the source of truth and a hosted browser contract test demands full dictionary equality so copies cannot silently drift. English selection requires **no catalog fetch** and can recover when the locale server is down.
- Validate *every* foreign catalog key against the English dictionary and *every* interpolation placeholder; reject extra/missing keys, missing text, markup delimiters and placeholder substitutions **before committing** the new language. Keep last committed choice on any failure.
- Use a monotonically increasing request generation in browser memory so only the last **selected** language commits. A stale network result or rejected fetch never mutates `locale`, `messages`, `html.lang` or focus.
- Cache the already-authorized Inbox page for immediate presentation-only rendering when a language changes. Switching language does not invoke self-test, send/print/delivery, mutation, credentials, or a second jobs read.
- Add a screen-reader status region for locale selection and generic localized `locale_unavailable` failure. Display meaningful English if the initial fetch fails instead of raw machine keys. Accessibility witness still requires actual assistive-technology review.
- Correct misleading 'accepting and retaining jobs' overview detail while health state is degraded.

## Verify and release bounds

New five-profile synthetic Playwright/axe tests inject initial locale endpoint failure, incomplete catalog, placeholder drift, HTTP 503, out-of-order held Thai response followed by English choice, repeated selection and exact English embedded/server catalog parity. Existing core login, locale, runtime, Go security/fuzz and Lighthouse checks must stay green at the **exact head**.

This is source/CI evidence only: English/Thai/German full-system journeys and human translations still need independent review. Login is still management-credential based; TrueNAS ordinary-user OIDC identity #96 and per-record ACL have not been implemented. No new deployment/materialization, provider credential, SMB permission, mailbox send, cloud upload, Paperless metadata write, protected Inbox bytes or runtime target is authorized.

**Next baton:** independently review the browser bundle/dictionary duplication and generation handling, add keyboard/actual screen-reader and network impairment witness; design complete EN/DE/TH provider flows. Separately finish #96 actor identity/AAA and #87 verified-before-expose downloads prior to enabling scoped document actions.
